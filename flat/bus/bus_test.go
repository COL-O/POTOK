package bus

import (
	"testing"
	"unsafe"
)

func TestBusAlignmentZeroGCAndEpochFencing(t *testing.T) {
	if got := unsafe.Sizeof(EventHeader{}); got != 64 {
		t.Fatalf("EventHeader size = %d, want 64", got)
	}
	if got := unsafe.Sizeof(HDCSlot{}); got != 1280 {
		t.Fatalf("HDCSlot size = %d, want 1280", got)
	}
	b, err := NewBus(BusConfig{
		PulseRingSize:   64,
		HDCRingSize:     64,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer b.Close()
	var slot HDCSlot
	slot.Words[0] = 0xDEADBEEF
	// Проверка Zero-GC (0.0 аллокаций на горячем пути Publish -> ViewFor -> CommitVerdict)
	allocs := testing.AllocsPerRun(500, func() {
		id := b.Publish(&slot, NullOffset, NullOffset, 100, 10)
		v, err := b.ViewFor(id, LaneMask(0).With(LanePulse).With(LaneHDC), nil)
		if err != nil || v.HDC.Words[0] != 0xDEADBEEF {
			t.Fatalf("ViewFor failed: %v", err)
		}
		_ = b.CommitVerdict(id, Verdict{Status: StatusOK, Confidence: 0.99, ActionCode: 1}, 0)
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated on Bus hot path: got %f allocs/op", allocs)
	}
	// Проверка Epoch Fencing при обороте кольца (64 слота)
	firstID := b.Publish(&slot, NullOffset, NullOffset, 100, 10)
	for i := 0; i < 65; i++ {
		b.Publish(&slot, NullOffset, NullOffset, 100, 10)
	}
	if _, err := b.ViewFor(firstID, LaneMask(0).With(LanePulse), nil); err != ErrEpochMismatch {
		t.Fatalf("expected ErrEpochMismatch after ring wrap, got %v", err)
	}
}
