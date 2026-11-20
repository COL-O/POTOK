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
