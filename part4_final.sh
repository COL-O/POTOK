#!/usr/bin/env bash
set -euo pipefail

echo "=== [ЧАСТЬ 4/4] СОЗДАНИЕ СЕНСОРНОГО ШЛЮЗА, ЯДРА СБОРКИ, ПРИМЕРОВ И ЗАПУСК ПРОВЕРКИ ==="

# ==================== СЕНСОРНЫЙ ШЛЮЗ (network/) ====================
cat << 'EOF' > network/gateway.go
package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"organism/bus"
	"organism/front"
)

const (
	FrameMagicTensor uint32 = 0x4F524731 // "ORG1"
	FrameMagicGraph  uint32 = 0x4F524732 // "ORG2"
	FrameHeaderSize         = 24
	MaxFeatures             = 64
	MaxGraphNodes           = 128
	MaxGraphEdges           = 512
	MaxPacketSize           = FrameHeaderSize + 4 + (MaxGraphNodes+1)*4 + MaxGraphEdges*8
)

type FeatureStats struct {
	Mean [MaxFeatures]float32
	Std  [MaxFeatures]float32
}

type SensoryGateway struct {
	coreBus     *bus.Bus
	frontRouter *front.Router
	udpConn     *net.UDPConn

	bufPool      sync.Pool
	featureBasis [MaxFeatures]bus.HDCSlot
	graphBasis   [MaxGraphNodes]bus.HDCSlot
	stats        atomic.Pointer[FeatureStats]

	stopOnce sync.Once
	stopping atomic.Bool
	wg       sync.WaitGroup

	PacketsReceived atomic.Uint64
	PacketsDropped  atomic.Uint64
	ImpulsesPushed  atomic.Uint64
	GraphsIngested  atomic.Uint64
}

func NewSensoryGateway(b *bus.Bus, fr *front.Router) *SensoryGateway {
	gw := &SensoryGateway{
		coreBus:     b,
		frontRouter: fr,
		bufPool: sync.Pool{
			New: func() any {
				buf := make([]byte, MaxPacketSize)
				return &buf
			},
		},
	}

	var def FeatureStats
	for i := 0; i < MaxFeatures; i++ {
		def.Std[i] = 1.0
	}
	gw.stats.Store(&def)
	gw.initBasisVectors()
	return gw
}

func (g *SensoryGateway) UpdateStatsRCU(next *FeatureStats) {
	if next != nil {
		g.stats.Store(next)
	}
}

func (g *SensoryGateway) initBasisVectors() {
	for f := 0; f < MaxFeatures; f++ {
		seed := uint64(f+1) * 0x9E3779B97F4A7C15
		for w := 0; w < bus.HDCWords; w++ {
			seed += 0x9E3779B97F4A7C15
			z := seed
			z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
			z = (z ^ (z >> 27)) * 0x94D049BB133111EB
			g.featureBasis[f].Words[w] = z ^ (z >> 31)
		}
	}

	for n := 0; n < MaxGraphNodes; n++ {
		seed := uint64(n+10007) * 0xBF58476D1CE4E5B9
		for w := 0; w < bus.HDCWords; w++ {
			seed += 0x9E3779B97F4A7C15
			z := seed
			z = (z ^ (z >> 30)) * 0x94D049BB133111EB
			z = (z ^ (z >> 27)) * 0xBF58476D1CE4E5B9
			g.graphBasis[n].Words[w] = z ^ (z >> 31)
		}
	}
}

func (g *SensoryGateway) StartUDP(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	g.udpConn = conn

	g.wg.Add(1)
	go g.readLoop()
	return nil
}

func (g *SensoryGateway) LocalAddr() net.Addr {
	if g.udpConn == nil {
		return nil
	}
	return g.udpConn.LocalAddr()
}

func (g *SensoryGateway) Stop() error {
	var closeErr error
	g.stopOnce.Do(func() {
		g.stopping.Store(true)
		if g.udpConn != nil {
			_ = g.udpConn.SetReadDeadline(time.Now())
			closeErr = g.udpConn.Close()
		}
	})

	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return closeErr
	case <-time.After(5 * time.Second):
		timeoutErr := fmt.Errorf("sensory gateway stop timeout after 5s")
		if closeErr != nil {
			return errors.Join(timeoutErr, closeErr)
		}
		return timeoutErr
	}
}

func (g *SensoryGateway) isStopping() bool { return g.stopping.Load() }

func (g *SensoryGateway) readLoop() {
	defer g.wg.Done()
	for {
		if g.isStopping() {
			return
		}

		bufPtr := g.bufPool.Get().(*[]byte)
		n, _, err := g.udpConn.ReadFromUDP(*bufPtr)
		if err != nil {
			g.bufPool.Put(bufPtr)
			if g.isStopping() {
				return
			}
			g.PacketsDropped.Add(1)
			continue
		}

		g.PacketsReceived.Add(1)
		g.IngestRawFrame((*bufPtr)[:n])
		g.bufPool.Put(bufPtr)
	}
}

func (g *SensoryGateway) IngestRawFrame(raw []byte) (uint64, bool) {
	if len(raw) < FrameHeaderSize {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	magic := binary.LittleEndian.Uint32(raw[0:4])
	switch magic {
	case FrameMagicTensor:
		return g.ingestTensorFrame(raw)
	case FrameMagicGraph:
		return g.ingestGraphFrame(raw)
	default:
		g.PacketsDropped.Add(1)
		return 0, false
	}
}

func (g *SensoryGateway) ingestTensorFrame(raw []byte) (uint64, bool) {
	tts := binary.LittleEndian.Uint32(raw[8:12])
	priority := raw[12]
	featCount := int(raw[13])

	if featCount <= 0 || featCount > MaxFeatures || featCount > len(g.featureBasis) || len(raw) < FrameHeaderSize+featCount*4 {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	var normFeatures [MaxFeatures]float32
	st := g.stats.Load()

	for i := 0; i < featCount; i++ {
		off := FrameHeaderSize + i*4
		bits := binary.LittleEndian.Uint32(raw[off : off+4])
		val := math.Float32frombits(bits)
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			g.PacketsDropped.Add(1)
			return 0, false
		}

		std := st.Std[i]
		if std <= 1e-6 {
			std = 1.0
		}
		normFeatures[i] = (val - st.Mean[i]) / std
	}

	tOff, err := g.coreBus.Tensors.Write4D(0, [4]uint16{1, 1, 1, uint16(featCount)}, normFeatures[:featCount])
	if err != nil || tOff == bus.NullOffset {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	var hdc bus.HDCSlot
	if !g.encodeFeaturesToHDC(normFeatures[:featCount], &hdc) {
		return 0, false
	}

	eventID := g.coreBus.Publish(&hdc, bus.NullOffset, tOff, tts, priority)
	g.ImpulsesPushed.Add(1)

	if g.frontRouter != nil {
		_, _ = g.frontRouter.Step(eventID)
	}
	return eventID, true
}

func (g *SensoryGateway) ingestGraphFrame(raw []byte) (uint64, bool) {
	tts := binary.LittleEndian.Uint32(raw[8:12])
	priority := raw[12]

	if len(raw) < FrameHeaderSize+4 {
		g.PacketsDropped.Add(1)
		return 0, false
	}
	numNodes := int(binary.LittleEndian.Uint16(raw[FrameHeaderSize : FrameHeaderSize+2]))
	numEdges := int(binary.LittleEndian.Uint16(raw[FrameHeaderSize+2 : FrameHeaderSize+4]))

	if numNodes < 2 || numNodes > MaxGraphNodes || numNodes > len(g.graphBasis) || numEdges <= 0 || numEdges > MaxGraphEdges {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	needBytes := FrameHeaderSize + 4 + (numNodes+1)*4 + numEdges*8
	if len(raw) < needBytes {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	var rowOff [MaxGraphNodes + 1]uint32
	var colIdx [MaxGraphEdges]uint32
	var weights [MaxGraphEdges]float32

	cursor := FrameHeaderSize + 4
	for i := 0; i <= numNodes; i++ {
		rowOff[i] = binary.LittleEndian.Uint32(raw[cursor : cursor+4])
		cursor += 4
	}
	for i := 0; i < numEdges; i++ {
		dst := binary.LittleEndian.Uint32(raw[cursor : cursor+4])
		if int(dst) >= numNodes {
			g.PacketsDropped.Add(1)
			return 0, false
		}
		colIdx[i] = dst
		cursor += 4
	}
	for i := 0; i < numEdges; i++ {
		w := math.Float32frombits(binary.LittleEndian.Uint32(raw[cursor : cursor+4]))
		if math.IsNaN(float64(w)) || math.IsInf(float64(w), 0) {
			g.PacketsDropped.Add(1)
			return 0, false
		}
		weights[i] = w
		cursor += 4
	}

	gOff, err := g.coreBus.Graphs.Write(0, rowOff[:numNodes+1], colIdx[:numEdges], weights[:numEdges])
	if err != nil || gOff == bus.NullOffset {
		g.PacketsDropped.Add(1)
		return 0, false
	}

	var hdc bus.HDCSlot
	if !g.encodeGraphToHDC(rowOff[:numNodes+1], colIdx[:numEdges], weights[:numEdges], &hdc) {
		return 0, false
	}

	eventID := g.coreBus.Publish(&hdc, gOff, bus.NullOffset, tts, priority)
	g.ImpulsesPushed.Add(1)
	g.GraphsIngested.Add(1)

	if g.frontRouter != nil {
		_, _ = g.frontRouter.Step(eventID)
	}
	return eventID, true
}

func (g *SensoryGateway) encodeFeaturesToHDC(feats []float32, out *bus.HDCSlot) bool {
	for w := 0; w < bus.HDCWords; w++ {
		if (w&31) == 0 && g.isStopping() {
			return false
		}
		var word uint64
		for bit := 0; bit < 64; bit++ {
			var acc float32
			mask := uint64(1) << bit
			for f, val := range feats {
				if f >= len(g.featureBasis) {
					break
				}
				if (g.featureBasis[f].Words[w] & mask) != 0 {
					acc += val
				} else {
					acc -= val
				}
			}
			if acc >= 0 {
				word |= mask
			}
		}
		out.Words[w] = word
	}
	return true
}

func (g *SensoryGateway) encodeGraphToHDC(rowOff []uint32, colIdx []uint32, weights []float32, out *bus.HDCSlot) bool {
	numNodes := len(rowOff) - 1
	if numNodes > len(g.graphBasis) {
		numNodes = len(g.graphBasis)
	}
	for w := 0; w < bus.HDCWords; w++ {
		if (w&31) == 0 && g.isStopping() {
			return false
		}
		var word uint64
		for bit := 0; bit < 64; bit++ {
			var acc float32
			mask := uint64(1) << bit
			for src := 0; src < numNodes; src++ {
				start, end := rowOff[src], rowOff[src+1]
				for e := start; e < end && int(e) < len(colIdx); e++ {
					dst := int(colIdx[e])
					if dst < 0 || dst >= len(g.graphBasis) {
						continue
					}
					val := weights[e]
					srcBasis := g.graphBasis[src].Words[w]
					dstBasis := (g.graphBasis[dst].Words[w] << 1) | (g.graphBasis[dst].Words[w] >> 63)
					if ((srcBasis ^ dstBasis) & mask) != 0 {
						acc += val
					} else {
						acc -= val
					}
				}
			}
			if acc >= 0 {
				word |= mask
			}
		}
		out.Words[w] = word
	}
	return true
}
EOF

cat << 'EOF' > network/gateway_test.go
package network

import (
	"encoding/binary"
	"math"
	"net"
	"slices"
	"testing"
	"time"

	"organism/bus"
	"organism/front"
)

func buildTestTensorFrame(tts uint32, priority uint8, features []float32) []byte {
	buf := make([]byte, FrameHeaderSize+len(features)*4)
	binary.LittleEndian.PutUint32(buf[0:4], FrameMagicTensor)
	binary.LittleEndian.PutUint32(buf[4:8], 1)
	binary.LittleEndian.PutUint32(buf[8:12], tts)
	buf[12] = priority
	buf[13] = uint8(len(features))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(time.Now().UnixNano()))

	for i, f := range features {
		off := FrameHeaderSize + i*4
		binary.LittleEndian.PutUint32(buf[off:off+4], math.Float32bits(f))
	}
	return buf
}

func buildTestGraphFrame(tts uint32, priority uint8, rows, cols []uint32, weights []float32) []byte {
	numNodes := len(rows) - 1
	numEdges := len(cols)
	buf := make([]byte, FrameHeaderSize+4+(numNodes+1)*4+numEdges*8)
	binary.LittleEndian.PutUint32(buf[0:4], FrameMagicGraph)
	binary.LittleEndian.PutUint32(buf[8:12], tts)
	buf[12] = priority
	cursor := FrameHeaderSize
	binary.LittleEndian.PutUint16(buf[cursor:cursor+2], uint16(numNodes))
	binary.LittleEndian.PutUint16(buf[cursor+2:cursor+4], uint16(numEdges))
	cursor += 4
	for _, r := range rows {
		binary.LittleEndian.PutUint32(buf[cursor:cursor+4], r)
		cursor += 4
	}
	for _, c := range cols {
		binary.LittleEndian.PutUint32(buf[cursor:cursor+4], c)
		cursor += 4
	}
	for _, w := range weights {
		binary.LittleEndian.PutUint32(buf[cursor:cursor+4], math.Float32bits(w))
		cursor += 4
	}
	return buf
}

func TestSensoryGatewayZScoreAndGraphIngest(t *testing.T) {
	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   256,
		HDCRingSize:     256,
		GraphArenaSize:  32 * 1024,
		TensorArenaSize: 32 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()
	pool, _ := bus.NewScratchPool(8, 1024)
	defer pool.Close()
	hdc := front.NewHDCOrgan(nil)
	tsetlin := front.NewTsetlinOrgan(nil)
	reg := bus.NewOrganRegistry()
	frontR := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 10 * time.Millisecond,
		MaxFlapsInWindow:  5,
		FlapWindowNs:      int64(time.Second),
	}, coreBus, reg, pool, hdc, tsetlin, nil)
	gw := NewSensoryGateway(coreBus, frontR)
	if err := gw.StartUDP("127.0.0.1:0"); err != nil {
		t.Fatalf("StartUDP failed: %v", err)
	}
	defer func() {
		if err := gw.Stop(); err != nil {
			t.Errorf("gw.Stop failed: %v", err)
		}
	}()
	var stats FeatureStats
	stats.Mean[0], stats.Std[0] = 60.0, 10.0
	stats.Mean[1], stats.Std[1] = 2.0, 0.5
	gw.UpdateStatsRCU(&stats)

	badFrame := buildTestTensorFrame(50, 10, []float32{float32(math.NaN()), 2.1})
	if _, ok := gw.IngestRawFrame(badFrame); ok {
		t.Fatalf("expected frame with NaN to be dropped")
	}
	if gw.PacketsDropped.Load() != 1 {
		t.Fatalf("expected PacketsDropped = 1, got %d", gw.PacketsDropped.Load())
	}

	oversizedHeader := make([]byte, FrameHeaderSize+4)
	binary.LittleEndian.PutUint32(oversizedHeader[0:4], FrameMagicGraph)
	binary.LittleEndian.PutUint16(oversizedHeader[FrameHeaderSize:FrameHeaderSize+2], MaxGraphNodes+1)
	binary.LittleEndian.PutUint16(oversizedHeader[FrameHeaderSize+2:FrameHeaderSize+4], 1)
	if _, ok := gw.IngestRawFrame(oversizedHeader); ok {
		t.Fatalf("expected oversized graph (129 nodes) to be dropped")
	}
	if gw.PacketsDropped.Load() != 2 {
		t.Fatalf("expected PacketsDropped = 2, got %d", gw.PacketsDropped.Load())
	}

	outOfBoundsNodeFrame := buildTestGraphFrame(100, 30, []uint32{0, 1, 1}, []uint32{5}, []float32{1.0})
	if _, ok := gw.IngestRawFrame(outOfBoundsNodeFrame); ok {
		t.Fatalf("expected graph with dst >= numNodes to be dropped")
	}
	if gw.PacketsDropped.Load() != 3 {
		t.Fatalf("expected PacketsDropped = 3, got %d", gw.PacketsDropped.Load())
	}

	conn, err := net.Dial("udp", gw.LocalAddr().String())
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()
	goodFrame := buildTestTensorFrame(100, 20, []float32{65.0, 2.2})
	_, _ = conn.Write(goodFrame)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if gw.ImpulsesPushed.Load() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if gw.PacketsReceived.Load() != 1 || gw.PacketsDropped.Load() != 3 {
		t.Fatalf("expected PacketsReceived=1 and PacketsDropped=3, got recv=%d drop=%d",
			gw.PacketsReceived.Load(), gw.PacketsDropped.Load())
	}
	view, err := coreBus.ViewFor(1, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor), nil)
	if err != nil || len(view.Tensor.Data) != 2 {
		t.Fatalf("ViewFor tensor failed: err=%v, len=%d", err, len(view.Tensor.Data))
	}
	if math.Abs(float64(view.Tensor.Data[0])-0.5) > 1e-5 || math.Abs(float64(view.Tensor.Data[1])-0.4) > 1e-5 {
		t.Fatalf("expected normalized [0.5, 0.4], got %v", view.Tensor.Data)
	}

	graphFrame1 := buildTestGraphFrame(100, 30, []uint32{0, 1, 1}, []uint32{1}, []float32{3.5})
	eventID2, ok := gw.IngestRawFrame(graphFrame1)
	if !ok || gw.GraphsIngested.Load() != 1 {
		t.Fatalf("expected graph frame ingested")
	}
	gView1, err := coreBus.ViewFor(eventID2, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC).With(bus.LaneGraph), nil)
	if err != nil || gView1.Graph.NumNodes != 2 || gView1.Graph.NumEdges != 1 {
		t.Fatalf("unexpected CSRView counts: err=%v, graph=%+v", err, gView1.Graph)
	}
	if len(gView1.Graph.RowOffsets) != 3 || gView1.Graph.RowOffsets[0] != 0 || gView1.Graph.RowOffsets[1] != 1 || gView1.Graph.RowOffsets[2] != 1 {
		t.Fatalf("expected RowOffsets [0 1 1], got %v", gView1.Graph.RowOffsets)
	}
	if len(gView1.Graph.ColIndices) != 1 || gView1.Graph.ColIndices[0] != 1 {
		t.Fatalf("expected ColIndices, got %v", gView1.Graph.ColIndices)
	}
	if len(gView1.Graph.EdgeWeights) != 1 || gView1.Graph.EdgeWeights[0] != 3.5 {
		t.Fatalf("expected EdgeWeights [3.5], got %v", gView1.Graph.EdgeWeights)
	}

	graphFrame2 := buildTestGraphFrame(100, 30, []uint32{0, 0, 1}, []uint32{0}, []float32{8.2})
	eventID3, _ := gw.IngestRawFrame(graphFrame2)
	gView2, _ := coreBus.ViewFor(eventID3, bus.LaneMask(0).With(bus.LaneHDC), nil)
	if slices.Equal(gView1.HDC.Words[:], gView2.HDC.Words[:]) {
		t.Fatalf("expected distinct HDC words for topologically different graphs")
	}
}
EOF

# ==================== ДАШБОРД И ЯДРО СБОРКИ (cmd/organism/) ====================
cat << 'EOF' > cmd/organism/dashboard.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"organism/bus"
	"organism/front"
	"organism/growth"
	"organism/polygon"
	"organism/stratos"
)

type OrganHeatEntry struct {
	ID    uint8  `json:"id"`
	Name  string `json:"name"`
	Layer string `json:"layer"`
	Hits  uint64 `json:"hits"`
}

type OrganismTelemetry struct {
	TimestampNs int64                `json:"timestamp_ns"`
	Bus         bus.BusStatsSnapshot `json:"bus"`
	Front       FrontStats           `json:"front"`
	Polygon     PolygonStats         `json:"polygon"`
	Growth      GrowthStats          `json:"growth"`
	Stratos     StratosStats         `json:"stratos"`
	Heatmap     []OrganHeatEntry     `json:"heatmap"`
}

type FrontStats struct {
	SurpriseCount   uint64 `json:"surprise_count"`
	VetoBlocked     uint64 `json:"veto_blocked"`
	EscalatedGrowth uint64 `json:"escalated_growth"`
	DemotedReflexes uint64 `json:"demoted_reflexes"`
}

type PolygonStats struct {
	DiagnosedCount  uint64 `json:"diagnosed_count"`
	PrecedentHits   uint64 `json:"precedent_hits"`
	AntibodiesSent  uint64 `json:"antibodies_sent"`
	VetoIssuedCount uint64 `json:"veto_issued_count"`
	EscalatedGrowth uint64 `json:"escalated_growth"`
}

type GrowthStats struct {
	EvolvedLawsCount   uint64 `json:"evolved_laws_count"`
	RejectedLawsCount  uint64 `json:"rejected_laws_count"`
	RollbackCount      uint64 `json:"rollback_count"`
	RehabilitatedCount uint64 `json:"rehabilitated_count"`
}

type StratosStats struct {
	RecallQueries   uint64 `json:"recall_queries"`
	RecallHits      uint64 `json:"recall_hits"`
	LawsRecorded    uint64 `json:"laws_recorded"`
	ToxicMarked     uint64 `json:"toxic_marked"`
	SwansRegistered uint64 `json:"swans_registered"`
}

type DashboardServer struct {
	srv      *http.Server
	coreBus  *bus.Bus
	registry *bus.OrganRegistry
	frontR   *front.Router
	polyR    *polygon.Router
	growthR  *growth.Router
	store    *stratos.Store
}

func NewDashboardServer(
	addr string,
	b *bus.Bus,
	reg *bus.OrganRegistry,
	fr *front.Router,
	pr *polygon.Router,
	gr *growth.Router,
	st *stratos.Store,
) *DashboardServer {
	d := &DashboardServer{
		coreBus:  b,
		registry: reg,
		frontR:   fr,
		polyR:    pr,
		growthR:  gr,
		store:    st,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/metrics", d.handleMetrics)
	mux.HandleFunc("/prometheus", d.handlePrometheus)
	d.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	return d
}

func (d *DashboardServer) Start() error {
	ln, err := net.Listen("tcp", d.srv.Addr)
	if err != nil {
		return err
	}
	go func() {
		defer ln.Close()
		_ = d.srv.Serve(ln)
	}()
	return nil
}

func (d *DashboardServer) Shutdown(ctx context.Context) error {
	return d.srv.Shutdown(ctx)
}

func (d *DashboardServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := d.coreBus.Metrics.Snapshot()
	status := "healthy"
	code := http.StatusOK
	if stats.ChecksumErrors > 0 || stats.BoundsErrors > 0 {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          status,
		"published_ticks": stats.Published,
		"checksum_errors": stats.ChecksumErrors,
		"bounds_errors":   stats.BoundsErrors,
	})
}

func (d *DashboardServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	telemetry := d.CollectTelemetry()
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(telemetry)
}

func (d *DashboardServer) handlePrometheus(w http.ResponseWriter, r *http.Request) {
	t := d.CollectTelemetry()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# TYPE organism_bus_published_total counter\norganism_bus_published_total %d\n", t.Bus.Published)
	fmt.Fprintf(w, "# TYPE organism_front_surprise_total counter\norganism_front_surprise_total %d\n", t.Front.SurpriseCount)
	fmt.Fprintf(w, "# TYPE organism_front_veto_blocked_total counter\norganism_front_veto_blocked_total %d\n", t.Front.VetoBlocked)
	fmt.Fprintf(w, "# TYPE organism_polygon_diagnosed_total counter\norganism_polygon_diagnosed_total %d\n", t.Polygon.DiagnosedCount)
	fmt.Fprintf(w, "# TYPE organism_growth_evolved_laws_total counter\norganism_growth_evolved_laws_total %d\n", t.Growth.EvolvedLawsCount)
	fmt.Fprintf(w, "# TYPE organism_stratos_laws_recorded_total counter\norganism_stratos_laws_recorded_total %d\n", t.Stratos.LawsRecorded)
	fmt.Fprintf(w, "# TYPE organism_organ_hits_total counter\n")
	for _, h := range t.Heatmap {
		fmt.Fprintf(w, "organism_organ_hits_total{id=\"%d\",layer=\"%s\",organ=\"%s\"} %d\n", h.ID, h.Layer, h.Name, h.Hits)
	}
}

func (d *DashboardServer) CollectTelemetry() OrganismTelemetry {
	var heatmap []OrganHeatEntry
	for id := uint8(0); id < bus.MaxActiveOrgans; id++ {
		org, ok := d.registry.Get(id)
		if !ok {
			continue
		}
		spec := org.Spec()
		var hits uint64
		switch spec.Layer {
		case bus.LayerFront:
			hits = d.frontR.Heatmap[id].Load()
		case bus.LayerPolygon:
			hits = d.polyR.Heatmap[id].Load()
		case bus.LayerGrowth:
			hits = d.growthR.Heatmap[id].Load()
		}
		heatmap = append(heatmap, OrganHeatEntry{
			ID:    id,
			Name:  spec.Name,
			Layer: layerName(spec.Layer),
			Hits:  hits,
		})
	}
	sort.Slice(heatmap, func(i, j int) bool {
		if heatmap[i].Hits == heatmap[j].Hits {
			return heatmap[i].ID < heatmap[j].ID
		}
		return heatmap[i].Hits > heatmap[j].Hits
	})
	return OrganismTelemetry{
		TimestampNs: time.Now().UnixNano(),
		Bus:         d.coreBus.Metrics.Snapshot(),
		Front: FrontStats{
			SurpriseCount:   d.frontR.SurpriseCount.Load(),
			VetoBlocked:     d.frontR.VetoBlocked.Load(),
			EscalatedGrowth: d.frontR.EscalatedGrowth.Load(),
			DemotedReflexes: d.frontR.DemotedReflexes.Load(),
		},
		Polygon: PolygonStats{
			DiagnosedCount:  d.polyR.DiagnosedCount.Load(),
			PrecedentHits:   d.polyR.PrecedentHits.Load(),
			AntibodiesSent:  d.polyR.AntibodiesSent.Load(),
			VetoIssuedCount: d.polyR.VetoIssuedCount.Load(),
			EscalatedGrowth: d.polyR.EscalatedGrowth.Load(),
		},
		Growth: GrowthStats{
			EvolvedLawsCount:   d.growthR.EvolvedLawsCount.Load(),
			RejectedLawsCount:  d.growthR.RejectedLawsCount.Load(),
			RollbackCount:      d.growthR.RollbackCount.Load(),
			RehabilitatedCount: d.growthR.RehabilitatedCount.Load(),
		},
		Stratos: StratosStats{
			RecallQueries:   d.store.RecallQueries.Load(),
			RecallHits:      d.store.RecallHits.Load(),
			LawsRecorded:    d.store.LawsRecorded.Load(),
			ToxicMarked:     d.store.ToxicMarked.Load(),
			SwansRegistered: d.store.SwansRegistered.Load(),
		},
		Heatmap: heatmap,
	}
}

func layerName(l bus.LayerID) string {
	switch l {
	case bus.LayerFront:
		return "front"
	case bus.LayerPolygon:
		return "polygon"
	case bus.LayerGrowth:
		return "growth"
	case bus.LayerStratos:
		return "stratos"
	default:
		return "unknown"
	}
}
EOF

cat << 'EOF' > cmd/organism/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"organism/bus"
	"organism/front"
	"organism/growth"
	"organism/network"
	"organism/polygon"
	"organism/stratos"
)

type OrganismBridge struct {
	mu           sync.Mutex
	frontRouter  *front.Router
	hdcOrgan     *front.HDCOrgan
	tsetlinOrgan *front.TsetlinOrgan
	kanOrgan     *polygon.KANOrgan

	genCounter   atomic.Uint64
	kanVersion   atomic.Uint64
	frontVersion atomic.Uint64

	activeProtos    []front.HDCPrototype
	activeVetoRules []front.VetoRule
	quarantinePool  []front.HDCPrototype
	currentKAN      *polygon.KANRuleTable
	snapshots       map[uint64]organismGenomeState
}

type organismGenomeState struct {
	protos    []front.HDCPrototype
	vetoRules []front.VetoRule
	kanTable  *polygon.KANRuleTable
}

func NewOrganismBridge(
	fr *front.Router,
	hdc *front.HDCOrgan,
	ts *front.TsetlinOrgan,
	kan *polygon.KANOrgan,
	initProtos []front.HDCPrototype,
	initKAN *polygon.KANRuleTable,
) *OrganismBridge {
	b := &OrganismBridge{
		frontRouter:  fr,
		hdcOrgan:     hdc,
		tsetlinOrgan: ts,
		kanOrgan:     kan,
		activeProtos: append([]front.HDCPrototype(nil), initProtos...),
		currentKAN:   initKAN,
		snapshots:    make(map[uint64]organismGenomeState),
	}
	b.genCounter.Store(1)
	b.kanVersion.Store(initKAN.Version)
	b.frontVersion.Store(1)
	return b
}

func (b *OrganismBridge) ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.quarantinePool = append(b.quarantinePool, proto)
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeProtos = append(b.activeProtos, proto)
	v := b.frontVersion.Add(1)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{
		Version:    v,
		Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...),
	})
}

func (b *OrganismBridge) ApplyVetoRule(rule front.VetoRule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeVetoRules = append(b.activeVetoRules, rule)
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) RehabilitateVeto(mask uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var kept []front.VetoRule
	for _, r := range b.activeVetoRules {
		r.Mask &^= mask
		if r.Mask != 0 {
			kept = append(kept, r)
		}
	}
	b.activeVetoRules = kept
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentKAN = table
	b.kanVersion.Store(table.Version)
	b.kanOrgan.UpdateTableRCU(table)
}

func (b *OrganismBridge) SnapshotCurrentGenome() growth.GenomeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	gen := b.genCounter.Load()
	b.snapshots[gen] = organismGenomeState{
		protos:    append([]front.HDCPrototype(nil), b.activeProtos...),
		vetoRules: append([]front.VetoRule(nil), b.activeVetoRules...),
		kanTable:  b.currentKAN,
	}
	return growth.GenomeSnapshot{
		GenerationID: gen,
		Timestamp:    time.Now().UnixNano(),
		KANVersion:   b.kanVersion.Load(),
		FrontVersion: b.frontVersion.Load(),
	}
}

func (b *OrganismBridge) RestoreGenomeSnapshot(snap growth.GenomeSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.snapshots[snap.GenerationID]
	if !ok {
		return
	}
	b.activeProtos = append([]front.HDCPrototype(nil), st.protos...)
	b.activeVetoRules = append([]front.VetoRule(nil), st.vetoRules...)
	b.currentKAN = st.kanTable
	vFront := b.frontVersion.Add(1)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{
		Version:    vFront,
		Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...),
	})
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    vFront,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
	if st.kanTable != nil {
		b.kanOrgan.UpdateTableRCU(st.kanTable)
	}
}

func main() {
	fmt.Println("=== ИНИЦИАЛИЗАЦИЯ ЦИФРОВОГО ОРГАНИЗМА (4 КОНТУРА ВРЕМЕНИ) ===")
	walPath := filepath.Join(os.TempDir(), "organism_stratos.wal")
	memoryStore, err := stratos.NewStore(walPath, []growth.BlackSwanCase{
		{CaseID: 1, TestInput: 5.0, MinSafeOut: 0.0, MaxSafeOut: 1.0, CriticalTTS: 30},
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации stratos: %v", err)
	}
	defer memoryStore.Close()

	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   4096,
		HDCRingSize:     4096,
		GraphArenaSize:  256 * 1024,
		TensorArenaSize: 512 * 1024,
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации Шины: %v", err)
	}
	defer coreBus.Close()

	scratchPool, _ := bus.NewScratchPool(64, 4096)
	defer scratchPool.Close()

	var routinePattern bus.HDCSlot
	initProtos := []front.HDCPrototype{
		{Pattern: routinePattern, ClassID: 1, BaseMask: front.BaseRoutineMask | front.BitTTL, MinSim: 0.85},
	}
	hdcOrgan := front.NewHDCOrgan(initProtos)
	tsetlinOrgan := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 100, Weight: 0.95},
	})

	registry := bus.NewOrganRegistry()
	_ = registry.HotSwap(hdcOrgan)
	_ = registry.HotSwap(tsetlinOrgan)
	_ = registry.HotSwap(&front.TTLOrgan{MinSafeTTS: 20})
	_ = registry.HotSwap(&front.CapacityOrgan{MaxEdgeLoad: 10.0})
	_ = registry.HotSwap(&front.DStarLiteOrgan{})
	_ = registry.HotSwap(&front.MCTSOrgan{MaxRollouts: 16})
	_ = registry.HotSwap(&front.PasportOrgan{})

	prefetcher := front.NewGraphPrefetcher(front.PrefetchConfig{
		DiskBandwidthMBps:  3000,
		WindowDuration:     10 * time.Millisecond,
		MaxTokensPerWindow: 4,
		CriticalTTSThresh:  25,
	}, nil)
	defer prefetcher.Close()

	frontRouter := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 20 * time.Millisecond,
		MaxFlapsInWindow:  4,
		FlapWindowNs:      int64(500 * time.Millisecond),
		QuarantineTrials:  2,
	}, coreBus, registry, scratchPool, hdcOrgan, tsetlinOrgan, prefetcher)

	initKAN := &polygon.KANRuleTable{Version: 1, MinVal: 0.0, MaxVal: 10.0, Knots: []float32{0.1, 0.5, 0.9}, CauseCode: 77}
	kanOrgan := polygon.NewKANOrgan(initKAN)
	polyTsetlin := polygon.NewPolygonTsetlinOrgan([]polygon.AntibodyRule{
		{CauseCode: 77, AntibodyAction: 888, ProposeMask: front.BaseRoutineMask | front.BitDStarLite, Confidence: 0.91},
	})

	bridge := NewOrganismBridge(frontRouter, hdcOrgan, tsetlinOrgan, kanOrgan, initProtos, initKAN)

	polygonRouter := polygon.NewRouter(
		polygon.RouterConfig{MinPrecedentSim: 0.85, RecurrenceLimit: 3, RecurrenceWindow: time.Minute, DefaultVetoTTL: time.Minute},
		coreBus, registry, scratchPool, memoryStore, bridge,
		&polygon.MatrixProfileOrgan{WindowSize: 4, DiscordThresh: 2.0},
		kanOrgan,
		&polygon.CfCOrgan{DecayTau: 2.0, HorizonStep: 1.0},
		&polygon.CBSOrgan{MaxNodeCapacity: 10.0},
		&polygon.ESNOrgan{LeakRate: 0.3},
		polyTsetlin,
	)

	growthRouter := growth.NewRouter(
		growth.RouterConfig{BlackSwanBatchSize: 5, InitialAlpha: 0.1, AlphaStep: 0.3, PanicSurpriseRate: 10},
		coreBus, scratchPool, memoryStore, bridge,
		&growth.GraphSAGEOrgan{MaxDegreeChange: 2.0},
		&growth.ActiveInferenceOrgan{ExpectedBaseline: 1.0, SurpriseThresh: 0.5},
		&growth.SINDyOrgan{MaxFitError: 1.5},
		&growth.LUTKANOrgan{Resolution: 8, SafetyPad: 0.25},
		&growth.GrowthMCTSOrgan{Simulations: 16, Horizon: 3},
		&growth.TDMPC2Organ{BlendSteps: 4},
	)

	dashboard := NewDashboardServer(":8080", coreBus, registry, frontRouter, polygonRouter, growthRouter, memoryStore)
	if err := dashboard.Start(); err != nil {
		log.Fatalf("Ошибка запуска HTTP-дашборда :8080: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dashboard.Shutdown(ctx)
	}()
	fmt.Println("[Dashboard] HTTP-сервер прозрачности готов на :8080 (/health, /metrics, /prometheus)")

	gateway := network.NewSensoryGateway(coreBus, frontRouter)
	if err := gateway.StartUDP(":9000"); err != nil {
		log.Fatalf("Ошибка запуска UDP-шлюза :9000: %v", err)
	}
	defer func() {
		if err := gateway.Stop(); err != nil {
			log.Printf("[Shutdown] Ошибка остановки UDP-шлюза: %v", err)
		}
	}()
	fmt.Println("[Gateway]   UDP-рецептор телеметрии и графов запущен на :9000")

	id1 := coreBus.Publish(&routinePattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, _ := frontRouter.Step(id1)
	fmt.Printf("[Такт 1 | front]   Штатный рейс: Status=%d, Confidence=%.2f | UDP-пакетов=%d\n",
		v1.Status, v1.Confidence, gateway.PacketsReceived.Load())

	if os.Getenv("ORGANISM_DAEMON") == "1" {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\n[Shutdown] Получен сигнал остановки, завершаем контуры...")
	}
}
EOF

# ==================== ПРИМЕРЫ И СКВОЗНОЙ ТЕСТ ====================
cat << 'EOF' > examples/simple/main.go
package main

import (
	"fmt"
	"log"
	"time"

	"organism/bus"
	"organism/front"
)

func main() {
	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   256,
		HDCRingSize:     256,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		log.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	var knownPattern bus.HDCSlot
	knownPattern.Words[0] = 0x1111222233334444

	hdc := front.NewHDCOrgan([]front.HDCPrototype{
		{Pattern: knownPattern, ClassID: 1, BaseMask: front.BaseRoutineMask, MinSim: 0.90},
	})
	tsetlin := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 42, Weight: 0.99},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(tsetlin)
	_ = reg.HotSwap(&front.PasportOrgan{})

	router := front.NewRouter(front.RouterConfig{BaseLeaseDuration: 10 * time.Millisecond}, coreBus, reg, nil, hdc, tsetlin, nil)

	id1 := coreBus.Publish(&knownPattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, _ := router.Step(id1)
	fmt.Printf("Знакомый импульс: Status=%d, Action=%d, Confidence=%.2f\n", v1.Status, v1.ActionCode, v1.Confidence)

	var unknownPattern bus.HDCSlot
	for i := range unknownPattern.Words {
		unknownPattern.Words[i] = ^uint64(0)
	}
	id2 := coreBus.Publish(&unknownPattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v2, _ := router.Step(id2)
	fmt.Printf("Неизвестный импульс: Status=%d, Surprises=%d\n", v2.Status, router.SurpriseCount.Load())
}
EOF

cat << 'EOF' > examples/drone/main.go
package main

import (
	"fmt"
	"log"
	"time"

	"organism/bus"
	"organism/front"
)

const (
	OrganIDDroneAcoustic uint8  = 24
	BitDroneAcoustic     uint32 = 1 << OrganIDDroneAcoustic
)

type DroneAcousticOrgan struct{ ShockThreshold int16 }

func (o *DroneAcousticOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDDroneAcoustic, Name: "DroneAcoustic", Layer: bus.LayerFront, Residency: bus.ResidencyPinned}
}
func (o *DroneAcousticOrgan) InBounds(view bus.TickView) bool { return len(view.Audio) > 0 }
func (o *DroneAcousticOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *DroneAcousticOrgan) CoolDown()                        {}
func (o *DroneAcousticOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusAbstain}, bus.OrganMetrics{Status: bus.StatusAbstain}
	}
	var peak int16
	for _, s := range view.Audio {
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
	}
	if peak >= o.ShockThreshold {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.96, ActionCode: 707}, bus.OrganMetrics{Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.92, ActionCode: 100}, bus.OrganMetrics{Status: bus.StatusOK}
}

func main() {
	coreBus, err := bus.NewBus(bus.BusConfig{PulseRingSize: 256, HDCRingSize: 256, GraphArenaSize: 16 * 1024, TensorArenaSize: 16 * 1024})
	if err != nil {
		log.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(&DroneAcousticOrgan{ShockThreshold: 15000})
	_ = reg.HotSwap(&front.DStarLiteOrgan{})

	droneOrg, _ := reg.Get(OrganIDDroneAcoustic)
	vAudio, _ := droneOrg.Process(bus.TickView{EventID: 1, Audio: []int16{1400, 24000, -19500, 4100}})
	fmt.Printf("[Полёт 2] Акустический удар: ActionCode=%d (Уклонение), Confidence=%.2f, Время=%s\n",
		vAudio.ActionCode, vAudio.Confidence, time.Now().Format("15:04:05"))
}
EOF

cat << 'EOF' > integration_test.go
package main

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"organism/bus"
	"organism/front"
	"organism/growth"
	"organism/polygon"
	"organism/stratos"
)

type e2eBridge struct {
	mu           sync.Mutex
	frontRouter  *front.Router
	hdcOrgan     *front.HDCOrgan
	kanOrgan     *polygon.KANOrgan
	activeProtos []front.HDCPrototype
	activeVeto   []front.VetoRule
	currentKAN   *polygon.KANRuleTable
	snapshots    map[uint64][]front.HDCPrototype
	ver          atomic.Uint64
}

func (b *e2eBridge) ProposeQuarantineReflex(p front.HDCPrototype, _ front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{Version: b.ver.Add(1), Quarantine: []front.HDCPrototype{p}})
}

func (b *e2eBridge) ProposeFrontReflex(p front.HDCPrototype, _ front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeProtos = append(b.activeProtos, p)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{Version: b.ver.Add(1), Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...)})
}

func (b *e2eBridge) ApplyVetoRule(r front.VetoRule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeVeto = append(b.activeVeto, r)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{Version: b.ver.Add(1), VetoRules: append([]front.VetoRule(nil), b.activeVeto...)})
}

func (b *e2eBridge) RehabilitateVeto(mask uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeVeto = nil
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{Version: b.ver.Add(1)})
}

func (b *e2eBridge) DeployPolygonKAN(t *polygon.KANRuleTable, _ float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentKAN = t
	b.kanOrgan.UpdateTableRCU(t)
}

func (b *e2eBridge) SnapshotCurrentGenome() growth.GenomeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snapshots[1] = append([]front.HDCPrototype(nil), b.activeProtos...)
	return growth.GenomeSnapshot{GenerationID: 1}
}

func (b *e2eBridge) RestoreGenomeSnapshot(s growth.GenomeSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeProtos = append([]front.HDCPrototype(nil), b.snapshots[s.GenerationID]...)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{Version: b.ver.Add(1), Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...)})
}

func TestFullOrganismEndToEndLifecycle(t *testing.T) {
	b, _ := bus.NewBus(bus.BusConfig{PulseRingSize: 256, HDCRingSize: 256, GraphArenaSize: 32 * 1024, TensorArenaSize: 32 * 1024})
	defer b.Close()
	store, _ := stratos.NewStore(filepath.Join(t.TempDir(), "e2e.wal"), []growth.BlackSwanCase{
		{CaseID: 1, TestInput: 5.0, MinSafeOut: 0.0, MaxSafeOut: 1.0},
	})
	defer store.Close()

	var routine bus.HDCSlot
	initP := []front.HDCPrototype{{Pattern: routine, ClassID: 1, BaseMask: front.BaseRoutineMask, MinSim: 0.85}}
	hdc := front.NewHDCOrgan(initP)
	ts := front.NewTsetlinOrgan([]front.TsetlinClause{{IncludeFlags: bus.FlagReady, ActionCode: 100, Weight: 0.95}})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(ts)
	_ = reg.HotSwap(&front.PasportOrgan{})

	fr := front.NewRouter(front.RouterConfig{BaseLeaseDuration: 10 * time.Millisecond}, b, reg, nil, hdc, ts, nil)
	kan := polygon.NewKANOrgan(&polygon.KANRuleTable{Version: 1, MinVal: 0, MaxVal: 10, CauseCode: 77})
	br := &e2eBridge{frontRouter: fr, hdcOrgan: hdc, kanOrgan: kan, activeProtos: initP, snapshots: make(map[uint64][]front.HDCPrototype)}

	pr := polygon.NewRouter(polygon.RouterConfig{MinPrecedentSim: 0.85}, b, reg, nil, store, br,
		&polygon.MatrixProfileOrgan{}, kan, &polygon.CfCOrgan{}, &polygon.CBSOrgan{}, &polygon.ESNOrgan{}, polygon.NewPolygonTsetlinOrgan(nil))

	gr := growth.NewRouter(growth.RouterConfig{BlackSwanBatchSize: 5, PanicSurpriseRate: 10}, b, nil, store, br,
		&growth.GraphSAGEOrgan{}, &growth.ActiveInferenceOrgan{}, &growth.SINDyOrgan{}, &growth.LUTKANOrgan{SafetyPad: 0.25}, &growth.GrowthMCTSOrgan{}, &growth.TDMPC2Organ{})

	var alien bus.HDCSlot
	for i := range alien.Words {
		alien.Words[i] = 0x5555555555555555
	}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 4}, []float32{1, 2, 18, 25})
	id := b.Publish(&alien, bus.NullOffset, tOff, 50, 200)
	if v, _ := fr.Step(id); v.Status != bus.StatusOutOfBounds {
		t.Fatalf("expected Surprise")
	}
	if vPoly, _, _ := pr.Diagnose(id); vPoly.ActionCode != 4001 {
		t.Fatalf("expected polygon escalation 4001, got %d", vPoly.ActionCode)
	}
	if audit, err := gr.Evolve(id, 77, front.BitDStarLite); err != nil || audit.Rejected {
		t.Fatalf("Evolve failed: %v", err)
	}
	if !gr.CircuitBreakerCheck(20) || len(br.activeProtos) != 1 || store.ToxicMarked.Load() != 1 {
		t.Fatalf("rollback verification failed")
	}
}
EOF

cat << 'EOF' > Makefile
.PHONY: check run simple drone clean

check:
	./bootstrap.sh --check

run:
	go run ./cmd/organism/...

simple:
	go run ./examples/simple/main.go

drone:
	go run ./examples/drone/main.go

clean:
	rm -rf bin
EOF

echo "=== ВСЕ 4 ЧАСТИ РАЗВЁРНУТЫ! ЗАПУСКАЕМ ПЕРВУЮ ПРОВЕРКУ (./bootstrap.sh --check) ==="
./bootstrap.sh --check
