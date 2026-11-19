package front

import (
	"testing"
	"time"

	"organism/bus"
)

func TestFrontRouterZeroGCAndVetoMask(t *testing.T) {
	b, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   128,
		HDCRingSize:     128,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer b.Close()
	var pat bus.HDCSlot
	pat.Words[0] = 0x12345678
	hdc := NewHDCOrgan([]HDCPrototype{
		{Pattern: pat, ClassID: 7, BaseMask: BaseRoutineMask | BitDStarLite, MinSim: 0.9},
	})
	ts := NewTsetlinOrgan([]TsetlinClause{
		{IncludeFlags: bus.FlagReady, ActionCode: 55, Weight: 0.95},
	})
	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(ts)
	_ = reg.HotSwap(&DStarLiteOrgan{})
	_ = reg.HotSwap(&PasportOrgan{})
	r := NewRouter(RouterConfig{BaseLeaseDuration: 10 * time.Millisecond}, b, reg, nil, hdc, ts, nil)
	// Проверка Zero-GC на полном такте front.Router.Step
	allocs := testing.AllocsPerRun(500, func() {
		id := b.Publish(&pat, bus.NullOffset, bus.NullOffset, 100, 10)
		v, err := r.Step(id)
		if err != nil || v.Status != bus.StatusOK || v.ActionCode != 301 {
			t.Fatalf("unexpected Step result: err=%v, v=%+v", err, v)
		}
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated in front.Router.Step: got %f allocs/op", allocs)
	}
	// Проверка VetoMask (блокировка D* Lite -> остаётся действие Tsetlin = 55)
	r.UpdatePolicyRCU(&RouterPolicy{
		Version:   2,
		VetoRules: []VetoRule{{Mask: BitDStarLite, Level: VetoHard, ReasonCode: 99}},
	})
	id2 := b.Publish(&pat, bus.NullOffset, bus.NullOffset, 100, 10)
	v2, _ := r.Step(id2)
	if v2.ActionCode != 55 || r.VetoBlocked.Load() != 1 {
		t.Fatalf("expected VetoMask to block DStarLite, got action=%d, blocked=%d", v2.ActionCode, r.VetoBlocked.Load())
	}
}
