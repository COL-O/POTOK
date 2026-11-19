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
