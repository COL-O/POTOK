package front

import (
	"sync/atomic"
	"testing"
	"time"

	"organism/modules/bus"
)

func setupTestRouter(t *testing.T) (*bus.Bus, *Router, *HDCOrgan, *TsetlinOrgan) {
	t.Helper()
	b, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   1024,
		HDCRingSize:     1024,
		GraphArenaSize:  64 * 1024,
		TensorArenaSize: 64 * 1024,
		MetaRingSize:    1024,
		ErrorRingSize:   1024,
		VectorArenaSize: 32 * 1024,
		AudioArenaSize:  32 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	pool, err := bus.NewScratchPool(32, 4096)
	if err != nil {
		t.Fatalf("NewScratchPool failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var routinePattern bus.HDCSlot
	hdc := NewHDCOrgan([]HDCPrototype{
		{Pattern: routinePattern, ClassID: 1, BaseMask: BaseRoutineMask | BitTTL | BitDStarLite, MinSim: 0.85},
	})
	tsetlin := NewTsetlinOrgan([]TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: BitPasport, Weight: 0.92, ActionCode: 10},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(tsetlin)
	_ = reg.HotSwap(&TTLOrgan{MinSafeTTS: 20})
	_ = reg.HotSwap(&DStarLiteOrgan{})
	_ = reg.HotSwap(&PasportOrgan{})

	cfg := RouterConfig{
		BaseLeaseDuration: 30 * time.Millisecond,
		MaxFlapsInWindow:  3,
		FlapWindowNs:      int64(500 * time.Millisecond),
		QuarantineTrials:  2,
	}
	router := NewRouter(cfg, b, reg, pool, hdc, tsetlin, nil)
	return b, router, hdc, tsetlin
}

func TestRouterRoutineVetoAndQuarantine(t *testing.T) {
	b, router, _, _ := setupTestRouter(t)
	gOff, _ := b.Graphs.Write(0, []uint32{0, 1, 1}, []uint32{1}, []float32{1.2})
	var routineHDC bus.HDCSlot
	id1 := b.Publish(&routineHDC, gOff, bus.NullOffset, 100, 10)
	v, err := router.Step(id1)
	if err != nil {
		t.Fatalf("Step(id1) failed: %v", err)
	}
	if v.Status != bus.StatusOK || v.Confidence < 0.90 {
		t.Fatalf("expected StatusOK with high confidence, got %+v", v)
	}
	if router.Heatmap[OrganIDDStarLite].Load() != 1 {
		t.Fatalf("expected DStarLite to execute once")
	}

	var shadowCand bus.HDCSlot
	router.UpdatePolicyRCU(&RouterPolicy{
		Version: 2,
		VetoRules: []VetoRule{
			{Mask: BitDStarLite, Level: VetoSoftTTS, MinSafeTTS: 50, ReasonCode: 404},
		},
		Quarantine: []HDCPrototype{
			{Pattern: shadowCand, ClassID: 99, BaseMask: BaseRoutineMask, MinSim: 0.95},
		},
	})
	id2 := b.Publish(&routineHDC, gOff, bus.NullOffset, 10, 10)
	if _, err := router.Step(id2); err != nil {
		t.Fatalf("Step(id2) failed: %v", err)
	}
	if router.VetoBlocked.Load() != 1 {
		t.Fatalf("expected VetoBlocked = 1, got %d", router.VetoBlocked.Load())
	}
	if router.Heatmap[OrganIDDStarLite].Load() != 1 {
		t.Fatalf("DStarLite executed despite VetoMask!")
	}
	view2, _ := b.ViewFor(id2, bus.LaneMask(0).With(bus.LanePulse), nil)
	flags := atomic.LoadUint32(&view2.Header.StateFlags)
	if (flags & bus.FlagVetoTriggered) == 0 {
		t.Fatalf("expected FlagVetoTriggered in EventHeader")
	}
	if (flags & bus.FlagInQuarantine) == 0 {
		t.Fatalf("expected FlagInQuarantine from shadow candidate")
	}
}

func TestRouterSurpriseLeaseAndResonanceEscalation(t *testing.T) {
	b, router, _, _ := setupTestRouter(t)
	var alienHDC bus.HDCSlot
	for i := range alienHDC.Words {
		alienHDC.Words[i] = ^uint64(0)
	}
	id1 := b.Publish(&alienHDC, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, _ := router.Step(id1)
	if v1.Status != bus.StatusOutOfBounds {
		t.Fatalf("expected StatusOutOfBounds on unknown HDC, got %v", v1.Status)
	}
	if !router.inSurpriseLease.Load() {
		t.Fatalf("expected router to enter Surprise lease")
	}
	for i := 0; i < 3; i++ {
		router.inSurpriseLease.Store(false)
		id := b.Publish(&alienHDC, bus.NullOffset, bus.NullOffset, 100, 10)
		_, _ = router.Step(id)
	}
	if router.EscalatedGrowth.Load() == 0 {
		t.Fatalf("expected resonance detector to escalate directly to growth")
	}
}

func TestRouterZeroAllocations(t *testing.T) {
	b, router, _, _ := setupTestRouter(t)
	var routineHDC bus.HDCSlot
	allocs := testing.AllocsPerRun(500, func() {
		id := b.Publish(&routineHDC, bus.NullOffset, bus.NullOffset, 100, 10)
		v, err := router.Step(id)
		if err != nil || v.Status != bus.StatusOK {
			t.Fatalf("unexpected step failure: err=%v, v=%+v", err, v)
		}
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated in Router.Step: got %f allocs/op, want 0", allocs)
	}
}
