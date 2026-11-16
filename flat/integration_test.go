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
