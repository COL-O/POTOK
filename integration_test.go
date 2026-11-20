package main

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/growth"
	"organism/modules/polygon"
	"organism/modules/stratos"
)

type fullBridge struct {
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

	snapshots map[uint64]fullGenomeState
}

type fullGenomeState struct {
	protos    []front.HDCPrototype
	vetoRules []front.VetoRule
	kanTable  *polygon.KANRuleTable
}

func newFullBridge(fr *front.Router, hdc *front.HDCOrgan, ts *front.TsetlinOrgan, kan *polygon.KANOrgan, initProtos []front.HDCPrototype, initKAN *polygon.KANRuleTable) *fullBridge {
	b := &fullBridge{
		frontRouter:  fr,
		hdcOrgan:     hdc,
		tsetlinOrgan: ts,
		kanOrgan:     kan,
		activeProtos: append([]front.HDCPrototype(nil), initProtos...),
		currentKAN:   initKAN,
		snapshots:    make(map[uint64]fullGenomeState),
	}
	b.genCounter.Store(1)
	b.kanVersion.Store(initKAN.Version)
	b.frontVersion.Store(1)
	return b
}

func (b *fullBridge) ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
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

func (b *fullBridge) ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeProtos = append(b.activeProtos, proto)
	v := b.frontVersion.Add(1)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{
		Version:    v,
		Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...),
	})
}

func (b *fullBridge) ApplyVetoRule(rule front.VetoRule) {
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

func (b *fullBridge) RehabilitateVeto(mask uint32) {
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

func (b *fullBridge) DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentKAN = table
	b.kanVersion.Store(table.Version)
	b.kanOrgan.UpdateTableRCU(table)
}

func (b *fullBridge) SnapshotCurrentGenome() growth.GenomeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	gen := b.genCounter.Load()
	b.snapshots[gen] = fullGenomeState{
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

func (b *fullBridge) RestoreGenomeSnapshot(snap growth.GenomeSnapshot) {
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

func TestFullOrganismEndToEndLifecycle(t *testing.T) {
	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   1024,
		HDCRingSize:     1024,
		GraphArenaSize:  64 * 1024,
		TensorArenaSize: 128 * 1024,
		MetaRingSize:    1024,
		ErrorRingSize:   1024,
		VectorArenaSize: 32 * 1024,
		AudioArenaSize:  32 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	pool, err := bus.NewScratchPool(32, 4096)
	if err != nil {
		t.Fatalf("NewScratchPool failed: %v", err)
	}
	defer pool.Close()

	walPath := filepath.Join(t.TempDir(), "e2e_stratos.wal")
	store, err := stratos.NewStore(walPath, []growth.BlackSwanCase{
		{CaseID: 1, TestInput: 5.0, MinSafeOut: 0.0, MaxSafeOut: 1.0, CriticalTTS: 30},
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var routinePattern bus.HDCSlot
	initProtos := []front.HDCPrototype{
		{Pattern: routinePattern, ClassID: 1, BaseMask: front.BaseRoutineMask, MinSim: 0.85},
	}
	hdcOrgan := front.NewHDCOrgan(initProtos)
	tsetlinOrgan := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 100, Weight: 0.95},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdcOrgan)
	_ = reg.HotSwap(tsetlinOrgan)
	_ = reg.HotSwap(&front.DStarLiteOrgan{})
	_ = reg.HotSwap(&front.PasportOrgan{})

	frontRouter := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 5 * time.Millisecond,
		MaxFlapsInWindow:  5,
		FlapWindowNs:      int64(time.Second),
		QuarantineTrials:  1,
	}, coreBus, reg, pool, hdcOrgan, tsetlinOrgan, nil)

	initKAN := &polygon.KANRuleTable{
		Version:   1,
		MinVal:    0.0,
		MaxVal:    10.0,
		Knots:     []float32{0.1, 0.5, 0.9},
		CauseCode: 77,
	}
	kanOrgan := polygon.NewKANOrgan(initKAN)
	polyTsetlin := polygon.NewPolygonTsetlinOrgan([]polygon.AntibodyRule{
		{CauseCode: 77, AntibodyAction: 888, ProposeMask: front.BaseRoutineMask | front.BitDStarLite, Confidence: 0.92},
	})

	bridge := newFullBridge(frontRouter, hdcOrgan, tsetlinOrgan, kanOrgan, initProtos, initKAN)
	polygonRouter := polygon.NewRouter(
		polygon.RouterConfig{MinPrecedentSim: 0.85, RecurrenceLimit: 3, RecurrenceWindow: time.Minute, DefaultVetoTTL: time.Minute},
		coreBus, reg, pool, store, bridge,
		&polygon.MatrixProfileOrgan{WindowSize: 4, DiscordThresh: 2.0},
		kanOrgan,
		&polygon.CfCOrgan{DecayTau: 2.0, HorizonStep: 1.0},
		&polygon.CBSOrgan{MaxNodeCapacity: 10.0},
		&polygon.ESNOrgan{LeakRate: 0.3},
		polyTsetlin,
	)

	growthRouter := growth.NewRouter(
		growth.RouterConfig{BlackSwanBatchSize: 5, InitialAlpha: 0.1, AlphaStep: 0.3, PanicSurpriseRate: 10},
		coreBus, pool, store, bridge,
		&growth.GraphSAGEOrgan{MaxDegreeChange: 2.0},
		&growth.ActiveInferenceOrgan{ExpectedBaseline: 1.0, SurpriseThresh: 0.5},
		&growth.SINDyOrgan{MaxFitError: 1.5},
		&growth.LUTKANOrgan{Resolution: 8, SafetyPad: 0.25},
		&growth.GrowthMCTSOrgan{Simulations: 16, Horizon: 3},
		&growth.TDMPC2Organ{BlendSteps: 4},
	)

	id1 := coreBus.Publish(&routinePattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, err := frontRouter.Step(id1)
	if err != nil || v1.Status != bus.StatusOK {
		t.Fatalf("Step 1 failed: err=%v, v=%+v", err, v1)
	}

	var alienHDC bus.HDCSlot
	for i := range alienHDC.Words {
		alienHDC.Words[i] = 0x5555555555555555
	}
	// Аномальная серия: HDC её «не узнаёт» (alien), но динамика законна —
	// dx=const=1.5 (арифметическая прогрессия). SINDy обязан найти закон.
	// (Прежняя серия со скачком 1..25 не была функцией dx=f(x) вообще:
	// x=1 встречалась и с dx=0, и с dx=17 — ни одна регрессия не сходилась.)
	series := []float32{1.0, 2.5, 4.0, 5.5, 7.0, 8.5, 10.0, 11.5, 13.0, 14.5, 16.0, 17.5}
	tOff, _ := coreBus.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series)
	gOff, _ := coreBus.Graphs.Write(0, []uint32{0, 1, 1}, []uint32{1}, []float32{1.5})

	id2 := coreBus.Publish(&alienHDC, gOff, tOff, 50, 200)
	v2, _ := frontRouter.Step(id2)
	if v2.Status != bus.StatusOutOfBounds {
		t.Fatalf("expected Surprise on alien HDC, got %v", v2.Status)
	}

	bridge.ApplyVetoRule(front.VetoRule{Mask: front.BitDStarLite, Level: front.VetoHard, ReasonCode: 5001})

	vPoly, _, _ := polygonRouter.Diagnose(id2)
	if vPoly.ActionCode != 4001 {
		t.Fatalf("expected polygon escalation 4001 on KAN out-of-bounds, got %d", vPoly.ActionCode)
	}

	evoAudit, err := growthRouter.Evolve(id2, 77, front.BitDStarLite)
	if err != nil || evoAudit.Rejected {
		t.Fatalf("growth Evolve failed: err=%v, audit=%+v", err, evoAudit)
	}
	if store.LawsRecorded.Load() != 1 || store.ToxicMarked.Load() != 0 {
		t.Fatalf("expected 1 healthy law recorded in stratos, got recorded=%d toxic=%d", store.LawsRecorded.Load(), store.ToxicMarked.Load())
	}

	frontRouter.UpdatePolicyRCU(&front.RouterPolicy{Version: 99})

	id3 := coreBus.Publish(&routinePattern, bus.NullOffset, bus.NullOffset, 100, 10)
	if v3, _ := frontRouter.Step(id3); v3.Status != bus.StatusOK {
		t.Fatalf("old routine reflex lost after RCU update!")
	}

	id4 := coreBus.Publish(&alienHDC, gOff, tOff, 100, 10)
	v4, _ := frontRouter.Step(id4)
	if v4.Status != bus.StatusOK {
		t.Fatalf("expected evolved alienHDC reflex to succeed in front, got %+v", v4)
	}

	if !growthRouter.CircuitBreakerCheck(20) {
		t.Fatalf("expected CircuitBreakerCheck to rollback")
	}

	bridge.mu.Lock()
	protosAfterRollback := len(bridge.activeProtos)
	vetoAfterRollback := len(bridge.activeVetoRules)
	bridge.mu.Unlock()
	if protosAfterRollback != 1 {
		t.Fatalf("expected 1 initial prototype after rollback, got %d", protosAfterRollback)
	}
	if vetoAfterRollback != 0 {
		t.Fatalf("expected empty activeVetoRules after rollback, got %d", vetoAfterRollback)
	}
}
