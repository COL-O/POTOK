package bus

import (
	"errors"
	"math"
	"testing"
	"unsafe"
)

func newTestBus(t *testing.T) *Bus {
	t.Helper()
	cfg := BusConfig{
		PulseRingSize:   64,
		HDCRingSize:     64,
		GraphArenaSize:  64 * 1024,
		TensorArenaSize: 128 * 1024,
		MetaRingSize:    64,
		ErrorRingSize:   64,
		VectorArenaSize: 32 * 1024,
		AudioArenaSize:  32 * 1024,
		PinFrontInRAM:   false,
	}
	b, err := NewBus(cfg)
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func TestMemoryLayoutAlignment(t *testing.T) {
	if got := unsafe.Sizeof(EventHeader{}); got != 64 {
		t.Fatalf("EventHeader size = %d, want 64 bytes", got)
	}
	if got := unsafe.Sizeof(HDCSlot{}); got != 1280 {
		t.Fatalf("HDCSlot size = %d, want 1280 bytes", got)
	}
	if got := unsafe.Sizeof(MetadataSlot{}); got != 64 {
		t.Fatalf("MetadataSlot size = %d, want 64 bytes", got)
	}
	if got := unsafe.Sizeof(ErrorSlot{}); got != 128 {
		t.Fatalf("ErrorSlot size = %d, want 128 bytes", got)
	}
	b := newTestBus(t)
	base := uintptr(b.pulseReg.BasePtr())
	if base%CacheLineSize != 0 {
		t.Fatalf("Pulse region base address %#x is not aligned to %d bytes", base, CacheLineSize)
	}
}

func TestArenasZeroCopyAndCRC32(t *testing.T) {
	b := newTestBus(t)
	rowOffsets := []uint32{0, 2, 3, 3}
	colIndices := []uint32{1, 2, 0}
	weights := []float32{1.5, 2.0, 0.75}
	seq := uint32(1)
	gOff, err := b.Graphs.Write(seq, rowOffsets, colIndices, weights)
	if err != nil {
		t.Fatalf("Graphs.Write failed: %v", err)
	}
	dims := [4]uint16{1, 2, 2, 2}
	tensorData := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	tOff, err := b.Tensors.Write4D(seq, dims, tensorData)
	if err != nil {
		t.Fatalf("Tensors.Write4D failed: %v", err)
	}
	gView, err := b.Graphs.Read(gOff, seq)
	if err != nil {
		t.Fatalf("Graphs.Read failed: %v", err)
	}
	if gView.NumNodes != 3 || gView.NumEdges != 3 || gView.EdgeWeights[1] != 2.0 {
		t.Fatalf("unexpected CSRView: %+v", gView)
	}
	tView, err := b.Tensors.Read4D(tOff, seq)
	if err != nil {
		t.Fatalf("Tensors.Read4D failed: %v", err)
	}
	dataAddr := uintptr(unsafe.Pointer(&tView.Data[0]))
	if dataAddr%CacheLineSize != 0 {
		t.Fatalf("Tensor data pointer %#x is not 64-byte aligned", dataAddr)
	}
	if tView.Data[7] != 0.8 {
		t.Fatalf("unexpected tensor value: got %v, want 0.8", tView.Data[7])
	}
	raw, _ := b.graphReg.Bytes(int(gOff)+csrHeaderSize, 4)
	raw[0] ^= 0xFF
	if _, err := b.Graphs.Read(gOff, seq); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch on corrupted graph, got %v", err)
	}
	if _, err := b.Graphs.Read(uint32(b.graphReg.Size()+100), seq); !errors.Is(err, ErrBoundsExceeded) {
		t.Fatalf("expected ErrBoundsExceeded, got %v", err)
	}
}

func TestEpochFencingOverrun(t *testing.T) {
	b := newTestBus(t)
	var hdc HDCSlot
	hdc.Words[0] = 0xDEADBEEFCAFEBABE
	firstID := b.Publish(&hdc, NullOffset, NullOffset, 100, 10)
	view, err := b.ViewFor(firstID, LaneMask(0).With(LaneHDC), nil)
	if err != nil {
		t.Fatalf("ViewFor(firstID) failed: %v", err)
	}
	if view.HDC.Words[0] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("HDC mismatch: got %#x", view.HDC.Words[0])
	}
	for i := 0; i < b.cfg.PulseRingSize; i++ {
		b.Publish(&hdc, NullOffset, NullOffset, 100, 10)
	}
	_, err = b.ViewFor(firstID, LaneMask(0).With(LaneHDC), nil)
	if !errors.Is(err, ErrEpochMismatch) {
		t.Fatalf("expected ErrEpochMismatch after ring wrap, got %v", err)
	}
	if b.Metrics.BackpressureCount.Load() == 0 {
		t.Fatalf("expected BackpressureCount > 0 after epoch mismatch")
	}
}

func TestZeroAllocationsFullCycle(t *testing.T) {
	b := newTestBus(t)
	pool, err := NewScratchPool(16, 4096)
	if err != nil {
		t.Fatalf("NewScratchPool failed: %v", err)
	}
	defer pool.Close()
	var hdc HDCSlot
	hdc.Words[0] = 42
	lanes := LaneMask(0).With(LaneHDC)
	allocs := testing.AllocsPerRun(1000, func() {
		scratch, _ := pool.Acquire()
		id := b.Publish(&hdc, NullOffset, NullOffset, 50, 1)
		view, err := b.ViewFor(id, lanes, scratch)
		if err != nil {
			t.Fatalf("ViewFor failed: %v", err)
		}
		_ = view.HDC.Words[0]
		err = b.CommitVerdict(id, Verdict{OrganID: 1, Status: StatusOK, Confidence: 0.95, ActionCode: 7})
		if err != nil {
			t.Fatalf("CommitVerdict failed: %v", err)
		}
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated: expected 0 allocs/op, got %f", allocs)
	}
	_ = math.Float32bits(0.95)
}
