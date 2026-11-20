package polygon

import (
	"sync/atomic"
	"testing"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
)

type mockStratos struct {
	hasPrecedent bool
	prec         Precedent
}

func (m *mockStratos) RecallByHDC(hdc *bus.HDCSlot, minSim float32) (Precedent, bool) {
	return m.prec, m.hasPrecedent
}

type mockFrontUpdater struct {
	quarantineProposals []front.HDCPrototype
	vetoRules           []front.VetoRule
}

func (m *mockFrontUpdater) ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	m.quarantineProposals = append(m.quarantineProposals, proto)
}
func (m *mockFrontUpdater) ApplyVetoRule(rule front.VetoRule) {
	m.vetoRules = append(m.vetoRules, rule)
}
func (m *mockFrontUpdater) RehabilitateVeto(mask uint32) {}

func setupPolygonTest(t *testing.T) (*bus.Bus, *Router, *mockStratos, *mockFrontUpdater) {
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
	pool, err := bus.NewScratchPool(32, 4096)
	if err != nil {
		t.Fatalf("NewScratchPool failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	stratos := &mockStratos{}
	frontUpd := &mockFrontUpdater{}
	mp := &MatrixProfileOrgan{WindowSize: 4, DiscordThresh: 2.0}
	kan := NewKANOrgan(&KANRuleTable{Version: 1, MinVal: 0.0, MaxVal: 10.0, Knots: []float32{0.1, 0.5, 0.95}, CauseCode: 77})
	cfc := &CfCOrgan{DecayTau: 2.0, HorizonStep: 1.0}
	cbs := &CBSOrgan{MaxNodeCapacity: 10.0}
	esn := &ESNOrgan{LeakRate: 0.4}
	polyTsetlin := NewPolygonTsetlinOrgan([]AntibodyRule{
		{CauseCode: 77, AntibodyAction: 888, ProposeMask: front.BaseRoutineMask | front.BitDStarLite, Confidence: 0.92},
	})
	reg := bus.NewOrganRegistry()
	cfg := RouterConfig{MinPrecedentSim: 0.85, RecurrenceLimit: 3, RecurrenceWindow: time.Minute, DefaultVetoTTL: time.Minute}
	router := NewRouter(cfg, b, reg, pool, stratos, frontUpd, mp, kan, cfc, cbs, esn, polyTsetlin)
	return b, router, stratos, frontUpd
}

func TestPolygonFullCascadeAndQuarantine(t *testing.T) {
	b, router, _, frontUpd := setupPolygonTest(t)
	series := []float32{1.0, 1.1, 1.0, 0.9, 1.0, 1.1, 1.0, 0.9, 8.0, 8.2, 8.4, 8.5}
	tOff, err := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series)
	if err != nil {
		t.Fatalf("Write4D failed: %v", err)
	}
	gOff, _ := b.Graphs.Write(0, []uint32{0, 1, 1}, []uint32{1}, []float32{2.5})
	var hdc bus.HDCSlot
	hdc.Words[0] = 0x12345678
	id := b.Publish(&hdc, gOff, tOff, 50, 10)
	verdict, audit, err := router.Diagnose(id)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if verdict.Status != bus.StatusOK || verdict.ActionCode != 888 {
		t.Fatalf("expected AntibodyAction 888 with StatusOK, got %+v", verdict)
	}
	if audit.CauseCode != 77 || router.AntibodiesSent.Load() != 1 {
		t.Fatalf("expected CauseCode 77 and 1 antibody sent, got audit=%+v", audit)
	}
	if len(frontUpd.quarantineProposals) != 1 {
		t.Fatalf("expected 1 quarantine proposal to front, got %d", len(frontUpd.quarantineProposals))
	}
}

func TestPolygonKANDomainGuardEscalation(t *testing.T) {
	b, router, _, _ := setupPolygonTest(t)
	series := []float32{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 20.0, 22.0, 24.0, 25.0}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series)
	var hdc bus.HDCSlot
	id := b.Publish(&hdc, bus.NullOffset, tOff, 50, 10)
	verdict, _, err := router.Diagnose(id)
	if err != nil {
		t.Fatalf("Diagnose failed: %v", err)
	}
	if verdict.Status != bus.StatusOutOfBounds || verdict.ActionCode != 4001 {
		t.Fatalf("expected escalation code 4001 on KAN out-of-bounds, got %+v", verdict)
	}
	if router.EscalatedGrowth.Load() != 1 {
		t.Fatalf("expected EscalatedGrowth = 1")
	}
	view, _ := b.ViewFor(id, bus.LaneMask(0).With(bus.LanePulse), nil)
	if (atomic.LoadUint32(&view.Header.StateFlags) & bus.FlagEscalateToGrowth) == 0 {
		t.Fatalf("expected FlagEscalateToGrowth set in EventHeader")
	}
}

func TestPolygonStratosArbitrationAndVeto(t *testing.T) {
	b, router, stratos, frontUpd := setupPolygonTest(t)
	stratos.hasPrecedent = true
	stratos.prec = Precedent{PrecedentID: 9001, CauseCode: 77, AntibodyAction: 999, RecommendedMask: front.BaseRoutineMask, Confidence: 0.96}
	series := []float32{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 5.0, 5.0, 5.0, 0.5}
	tOff, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series)
	var hdc bus.HDCSlot
	id1 := b.Publish(&hdc, bus.NullOffset, tOff, 50, 10)
	v1, audit1, err := router.Diagnose(id1)
	if err != nil {
		t.Fatalf("Diagnose(id1) failed: %v", err)
	}
	if audit1.UsedPrecedent != 9001 || v1.ActionCode != 999 || router.PrecedentHits.Load() != 1 {
		t.Fatalf("expected stratos precedent 9001 to arbitrate, got v=%+v, audit=%+v", v1, audit1)
	}
	stratos.hasPrecedent = false
	seriesAlarm := []float32{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 8.0, 8.0, 8.0, 8.5}
	tOff2, _ := b.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, seriesAlarm)
	id2 := b.Publish(&hdc, bus.NullOffset, tOff2, 0, 10)
	_, _, err = router.Diagnose(id2)
	if err != nil {
		t.Fatalf("Diagnose(id2) failed: %v", err)
	}
	if router.VetoIssuedCount.Load() != 1 || len(frontUpd.vetoRules) != 1 {
		t.Fatalf("expected 1 VetoRule issued on TTS == 0, got %d", len(frontUpd.vetoRules))
	}
	if frontUpd.vetoRules[0].ReasonCode != 5002 {
		t.Fatalf("expected VetoReasonCode 5002, got %d", frontUpd.vetoRules[0].ReasonCode)
	}
}
