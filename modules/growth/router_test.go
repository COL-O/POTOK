package growth

import (
	"testing"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/polygon"
)

type mockStratosArchive struct {
	swans        []BlackSwanCase
	recordedLaws []uint64
	toxicLaws    map[uint64]uint32
}

func newMockStratosArchive(swans []BlackSwanCase) *mockStratosArchive {
	return &mockStratosArchive{swans: swans, toxicLaws: make(map[uint64]uint32)}
}

func (m *mockStratosArchive) FetchBlackSwans(limit int) []BlackSwanCase {
	if limit > 0 && limit < len(m.swans) {
		return m.swans[:limit]
	}
	return m.swans
}

func (m *mockStratosArchive) RecordNewLaw(eq SINDyEquation, lut CompiledLUT, genealogyID uint64) {
	m.recordedLaws = append(m.recordedLaws, genealogyID)
}

func (m *mockStratosArchive) MarkLawToxic(genealogyID uint64, reason uint32) {
	m.toxicLaws[genealogyID] = reason
}

type mockLowerUpdater struct {
	deployedKAN   *polygon.KANRuleTable
	deployedAlpha float32
	frontReflCnt  int
	rehabilitated uint32
	currentSnap   GenomeSnapshot
	restoredSnap  *GenomeSnapshot
}

func (m *mockLowerUpdater) DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32) {
	m.deployedKAN = table
	m.deployedAlpha = alpha
}
func (m *mockLowerUpdater) ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	m.frontReflCnt++
}
func (m *mockLowerUpdater) RehabilitateVeto(mask uint32) {
	m.rehabilitated |= mask
}
func (m *mockLowerUpdater) SnapshotCurrentGenome() GenomeSnapshot {
	return m.currentSnap
}
func (m *mockLowerUpdater) RestoreGenomeSnapshot(snap GenomeSnapshot) {
	m.restoredSnap = &snap
}

func setupGrowthTest(t *testing.T, swans []BlackSwanCase) (*bus.Bus, *Router, *mockStratosArchive, *mockLowerUpdater) {
	t.Helper()
	b, err := bus.NewBus(bus.BusConfig{
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
	t.Cleanup(func() { b.Close() })
	pool, err := bus.NewScratchPool(16, 4096)
	if err != nil {
		t.Fatalf("NewScratchPool failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	archive := newMockStratosArchive(swans)
	updater := &mockLowerUpdater{currentSnap: GenomeSnapshot{GenerationID: 1, KANVersion: 1, FrontVersion: 1}}
	sage := &GraphSAGEOrgan{MaxDegreeChange: 2.0}
	actInf := &ActiveInferenceOrgan{ExpectedBaseline: 1.0, SurpriseThresh: 0.5}
	sindy := &SINDyOrgan{MaxFitError: 1.0}
	lutkan := &LUTKANOrgan{Resolution: 8, SafetyPad: 0.2}
	mcts := &GrowthMCTSOrgan{Simulations: 16, Horizon: 3}
	tdmpc2 := &TDMPC2Organ{BlendSteps: 4}
	cfg := RouterConfig{BlackSwanBatchSize: 5, InitialAlpha: 0.1, AlphaStep: 0.3, PanicSurpriseRate: 10}
	router := NewRouter(cfg, b, pool, archive, updater, sage, actInf, sindy, lutkan, mcts, tdmpc2)
	return b, router, archive, updater
}

func TestGrowthEvolveSuccessAndRehabilitation(t *testing.T) {
	swans := []BlackSwanCase{
		{CaseID: 1, TestInput: 3.0, MinSafeOut: 0.0, MaxSafeOut: 1.0, CriticalTTS: 50},
		{CaseID: 2, TestInput: 99.0, MinSafeOut: 0.0, MaxSafeOut: 1.0, CriticalTTS: 20},
	}
	b, router, archive, updater := setupGrowthTest(t, swans)
	series := []float32{1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 8}, series)
	gOff, _ := b.Graphs.Write(0, []uint32{0, 1, 1}, []uint32{1}, []float32{1.0})
	var hdc bus.HDCSlot
	hdc.Words[0] = 0xCAFEBABE
	id := b.Publish(&hdc, gOff, tOff, 100, 10)
	audit, err := router.Evolve(id, 77, front.BitDStarLite)
	if err != nil {
		t.Fatalf("Evolve failed: %v", err)
	}
	if audit.Rejected {
		t.Fatalf("expected law to pass, got rejected with reason %d", audit.RejectionReason)
	}
	if audit.PassedSwans != 2 || audit.TotalSwans != 2 {
		t.Fatalf("expected 2/2 black swans passed, got %d/%d", audit.PassedSwans, audit.TotalSwans)
	}
	if updater.deployedKAN == nil || updater.deployedAlpha != 0.1 {
		t.Fatalf("expected KANRuleTable deployed with alpha 0.1, got alpha=%v", updater.deployedAlpha)
	}
	if updater.rehabilitated != front.BitDStarLite {
		t.Fatalf("expected BitDStarLite to be rehabilitated, got %#x", updater.rehabilitated)
	}
	if len(archive.recordedLaws) != 1 || router.EvolvedLawsCount.Load() != 1 {
		t.Fatalf("expected law recorded in stratos")
	}
}

func TestGrowthAdversarialReplayRejectsBadLaw(t *testing.T) {
	swans := []BlackSwanCase{
		{CaseID: 42, TestInput: 3.0, MinSafeOut: 0.0, MaxSafeOut: 0.05, CriticalTTS: 10},
	}
	b, router, archive, updater := setupGrowthTest(t, swans)
	series := []float32{1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 8}, series)
	var hdc bus.HDCSlot
	id := b.Publish(&hdc, bus.NullOffset, tOff, 100, 10)
	audit, err := router.Evolve(id, 77, front.BitDStarLite)
	if err != nil {
		t.Fatalf("Evolve failed: %v", err)
	}
	if !audit.Rejected || audit.RejectionReason != 6005 {
		t.Fatalf("expected law rejected with 6005 on black swan failure, got %+v", audit)
	}
	if updater.deployedKAN != nil || updater.rehabilitated != 0 {
		t.Fatalf("rejected law leaked to lower layers!")
	}
	if archive.toxicLaws[audit.GenerationID] != 6005 {
		t.Fatalf("expected toxic mark 6005 in stratos")
	}
}

func TestGrowthCircuitBreakerRollback(t *testing.T) {
	b, router, archive, updater := setupGrowthTest(t, nil)
	series := []float32{1.0, 1.2, 1.4, 1.6, 1.8, 2.0}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 6}, series)
	var hdc bus.HDCSlot
	id := b.Publish(&hdc, bus.NullOffset, tOff, 100, 10)
	audit, err := router.Evolve(id, 88, 0)
	if err != nil || audit.Rejected {
		t.Fatalf("Evolve failed: err=%v, rejected=%v", err, audit.Rejected)
	}
	rolledBack := router.CircuitBreakerCheck(15)
	if !rolledBack {
		t.Fatalf("expected CircuitBreakerCheck to trigger rollback")
	}
	if updater.restoredSnap == nil || updater.restoredSnap.GenerationID != 1 {
		t.Fatalf("expected rollback to healthy GenerationID 1, got %+v", updater.restoredSnap)
	}
	if router.RollbackCount.Load() != 1 || archive.toxicLaws[audit.GenerationID] != 6999 {
		t.Fatalf("expected RollbackCount=1 and toxic mark 6999 in stratos")
	}
}
