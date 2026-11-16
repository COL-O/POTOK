#!/usr/bin/env bash
set -euo pipefail

echo "=== [ЧАСТЬ 3/4] СОЗДАНИЕ ЭТАЖЕЙ 2, 3 И 4 (polygon/, growth/, stratos/) ==="

# ==================== ЭТАЖ 2: POLYGON ====================
cat << 'EOF' > polygon/models.go
package polygon

import (
	"sync/atomic"
	"time"

	"organism/bus"
)

const (
	OrganIDMatrixProfile uint8 = 7
	OrganIDKAN           uint8 = 8
	OrganIDCfC           uint8 = 9
	OrganIDCBS           uint8 = 10
	OrganIDESN           uint8 = 11
	OrganIDPolyTsetlin   uint8 = 12
)

type KANRuleTable struct {
	Version   uint64
	MinVal    float32
	MaxVal    float32
	Knots     []float32
	CauseCode uint32
}

type KANOrgan struct {
	table atomic.Pointer[KANRuleTable]
}

func NewKANOrgan(t *KANRuleTable) *KANOrgan {
	o := &KANOrgan{}
	o.table.Store(t)
	return o
}

func (o *KANOrgan) UpdateTableRCU(next *KANRuleTable) {
	if next != nil {
		o.table.Store(next)
	}
}

func (o *KANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDKAN, Name: "KAN", Layer: bus.LayerPolygon, ReadLanes: bus.LaneMask(0).With(bus.LaneTensor), WriteLane: bus.LanePulse}
}
func (o *KANOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *KANOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *KANOrgan) CoolDown()                    {}

func (o *KANOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(v) {
		return bus.Verdict{OrganID: OrganIDKAN, Status: bus.StatusAbstain}, bus.OrganMetrics{Status: bus.StatusAbstain}
	}
	tbl := o.table.Load()
	last := v.Tensor.Data[len(v.Tensor.Data)-1]
	// Domain Guard (КОН 10): честный отказ при выходе за паспорт применимости
	if last < tbl.MinVal || last > tbl.MaxVal {
		return bus.Verdict{
			OrganID:    OrganIDKAN,
			Status:     bus.StatusOutOfBounds,
			Confidence: 0.2,
			ActionCode: tbl.CauseCode,
		}, bus.OrganMetrics{Status: bus.StatusOutOfBounds}
	}
	norm := (last - tbl.MinVal) / (tbl.MaxVal - tbl.MinVal + 1e-6)
	return bus.Verdict{
		OrganID:    OrganIDKAN,
		Status:     bus.StatusOK,
		Confidence: 0.90,
		ActionCode: tbl.CauseCode,
	}, bus.OrganMetrics{Confidence: norm, Status: bus.StatusOK}
}

type MatrixProfileOrgan struct {
	WindowSize    int
	DiscordThresh float32
}

func (o *MatrixProfileOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDMatrixProfile, Name: "MatrixProfile", Layer: bus.LayerPolygon}
}
func (o *MatrixProfileOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) >= 4 }
func (o *MatrixProfileOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *MatrixProfileOrgan) CoolDown()                    {}
func (o *MatrixProfileOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusOK, Confidence: 0.92}, bus.OrganMetrics{Status: bus.StatusOK}
}

type CfCOrgan struct {
	DecayTau    float32
	HorizonStep float32
}

func (o *CfCOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDCfC, Name: "CfC", Layer: bus.LayerPolygon}
}
func (o *CfCOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *CfCOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CfCOrgan) CoolDown()                    {}
func (o *CfCOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDCfC, Status: bus.StatusOK, Confidence: 0.89}, bus.OrganMetrics{Status: bus.StatusOK}
}

type CBSOrgan struct{ MaxNodeCapacity float32 }

func (o *CBSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDCBS, Name: "CBS", Layer: bus.LayerPolygon}
}
func (o *CBSOrgan) InBounds(v bus.TickView) bool { return true }
func (o *CBSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CBSOrgan) CoolDown()                    {}
func (o *CBSOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDCBS, Status: bus.StatusOK, Confidence: 0.91}, bus.OrganMetrics{Status: bus.StatusOK}
}

type ESNOrgan struct{ LeakRate float32 }

func (o *ESNOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDESN, Name: "ESN", Layer: bus.LayerPolygon}
}
func (o *ESNOrgan) InBounds(v bus.TickView) bool { return true }
func (o *ESNOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *ESNOrgan) CoolDown()                    {}
func (o *ESNOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDESN, Status: bus.StatusOK, Confidence: 0.88}, bus.OrganMetrics{Status: bus.StatusOK}
}

type AntibodyRule struct {
	CauseCode      uint32
	AntibodyAction uint32
	ProposeMask    uint32
	Confidence     float32
}

type PolygonTsetlinOrgan struct {
	rules []AntibodyRule
}

func NewPolygonTsetlinOrgan(r []AntibodyRule) *PolygonTsetlinOrgan {
	return &PolygonTsetlinOrgan{rules: r}
}

func (o *PolygonTsetlinOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDPolyTsetlin, Name: "PolyTsetlin", Layer: bus.LayerPolygon}
}
func (o *PolygonTsetlinOrgan) InBounds(v bus.TickView) bool { return true }
func (o *PolygonTsetlinOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *PolygonTsetlinOrgan) CoolDown()                    {}
func (o *PolygonTsetlinOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if len(o.rules) > 0 {
		r := o.rules[0]
		return bus.Verdict{
			OrganID:       OrganIDPolyTsetlin,
			Status:        bus.StatusOK,
			Confidence:    r.Confidence,
			ActionCode:    r.AntibodyAction,
			SuggestedMask: r.ProposeMask,
		}, bus.OrganMetrics{Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOK, Confidence: 0.85, ActionCode: 800}, bus.OrganMetrics{Status: bus.StatusOK}
}

var _ = time.Second
EOF

cat << 'EOF' > polygon/router.go
package polygon

import (
	"sync/atomic"
	"time"

	"organism/bus"
	"organism/front"
)

type PrecedentStore interface {
	RecallByHDC(slot *bus.HDCSlot, minSim float32) (actionCode uint32, mask uint32, found bool)
}

type FrontBridge interface {
	ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause)
	ApplyVetoRule(rule front.VetoRule)
}

type DiagnosisAudit struct {
	EventID   uint64
	CauseCode uint32
	Escalated bool
}

type RouterConfig struct {
	MinPrecedentSim  float32
	RecurrenceLimit  int
	RecurrenceWindow time.Duration
	DefaultVetoTTL   time.Duration
}

type Router struct {
	cfg      RouterConfig
	coreBus  *bus.Bus
	registry *bus.OrganRegistry
	pool     *bus.ScratchPool
	store    PrecedentStore
	bridge   FrontBridge

	mp          *MatrixProfileOrgan
	kan         *KANOrgan
	cfc         *CfCOrgan
	cbs         *CBSOrgan
	esn         *ESNOrgan
	polyTsetlin *PolygonTsetlinOrgan

	Heatmap         [bus.MaxActiveOrgans]atomic.Uint64
	DiagnosedCount  atomic.Uint64
	PrecedentHits   atomic.Uint64
	AntibodiesSent  atomic.Uint64
	VetoIssuedCount atomic.Uint64
	EscalatedGrowth atomic.Uint64
}

func NewRouter(
	cfg RouterConfig,
	b *bus.Bus,
	reg *bus.OrganRegistry,
	pool *bus.ScratchPool,
	st PrecedentStore,
	br FrontBridge,
	mp *MatrixProfileOrgan,
	kan *KANOrgan,
	cfc *CfCOrgan,
	cbs *CBSOrgan,
	esn *ESNOrgan,
	pt *PolygonTsetlinOrgan,
) *Router {
	return &Router{
		cfg:         cfg,
		coreBus:     b,
		registry:    reg,
		pool:        pool,
		store:       st,
		bridge:      br,
		mp:          mp,
		kan:         kan,
		cfc:         cfc,
		cbs:         cbs,
		esn:         esn,
		polyTsetlin: pt,
	}
}

func (r *Router) Diagnose(eventID uint64) (bus.Verdict, DiagnosisAudit, error) {
	view, err := r.coreBus.ViewFor(eventID, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC).With(bus.LaneGraph).With(bus.LaneTensor), nil)
	if err != nil {
		return bus.Verdict{Status: bus.StatusError}, DiagnosisAudit{EventID: eventID}, err
	}

	r.DiagnosedCount.Add(1)
	r.Heatmap[OrganIDMatrixProfile].Add(1)
	_, _ = r.mp.Process(view)

	r.Heatmap[OrganIDKAN].Add(1)
	kanV, _ := r.kan.Process(view)

	// Выход за границы паспорта KAN -> немедленная эскалация в growth (код 4001)
	if kanV.Status == bus.StatusOutOfBounds {
		r.EscalatedGrowth.Add(1)
		v := bus.Verdict{
			OrganID:    OrganIDKAN,
			Status:     bus.StatusOutOfBounds,
			Confidence: kanV.Confidence,
			ActionCode: 4001,
		}
		_ = r.coreBus.CommitVerdict(eventID, v, bus.FlagEscalateToGrowth)
		return v, DiagnosisAudit{EventID: eventID, CauseCode: kanV.ActionCode, Escalated: true}, nil
	}

	r.Heatmap[OrganIDCfC].Add(1)
	_, _ = r.cfc.Process(view)
	r.Heatmap[OrganIDCBS].Add(1)
	_, _ = r.cbs.Process(view)
	r.Heatmap[OrganIDESN].Add(1)
	_, _ = r.esn.Process(view)

	r.Heatmap[OrganIDPolyTsetlin].Add(1)
	ptV, _ := r.polyTsetlin.Process(view)

	if r.store != nil && view.HDC != nil {
		if act, msk, ok := r.store.RecallByHDC(view.HDC, r.cfg.MinPrecedentSim); ok {
			r.PrecedentHits.Add(1)
			ptV.ActionCode = act
			ptV.SuggestedMask = msk
		}
	}

	if r.bridge != nil && view.HDC != nil {
		r.bridge.ProposeQuarantineReflex(front.HDCPrototype{
			Pattern:  *view.HDC,
			ClassID:  ptV.ActionCode,
			BaseMask: ptV.SuggestedMask | front.BaseRoutineMask,
			MinSim:   0.85,
		}, front.TsetlinClause{
			IncludeFlags: bus.FlagReady,
			ActionCode:   ptV.ActionCode,
			Weight:       ptV.Confidence,
		})
		r.AntibodiesSent.Add(1)
	}

	_ = r.coreBus.CommitVerdict(eventID, ptV, 0)
	return ptV, DiagnosisAudit{EventID: eventID, CauseCode: kanV.ActionCode}, nil
}
EOF

# ==================== ЭТАЖ 3: GROWTH ====================
cat << 'EOF' > growth/models.go
package growth

import "organism/bus"

const (
	OrganIDGraphSAGE       uint8 = 13
	OrganIDActiveInference uint8 = 14
	OrganIDSINDy           uint8 = 15
	OrganIDLUTKAN          uint8 = 16
	OrganIDGrowthMCTS      uint8 = 17
	OrganIDTDMPC2          uint8 = 18
)

type GraphSAGEOrgan struct{ MaxDegreeChange float32 }

func (o *GraphSAGEOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDGraphSAGE, Name: "GraphSAGE", Layer: bus.LayerGrowth}
}
func (o *GraphSAGEOrgan) InBounds(v bus.TickView) bool { return true }
func (o *GraphSAGEOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GraphSAGEOrgan) CoolDown()                    {}
func (o *GraphSAGEOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDGraphSAGE, Status: bus.StatusOK, Confidence: 0.90}, bus.OrganMetrics{Status: bus.StatusOK}
}

type ActiveInferenceOrgan struct {
	ExpectedBaseline float32
	SurpriseThresh   float32
}

func (o *ActiveInferenceOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDActiveInference, Name: "ActiveInference", Layer: bus.LayerGrowth}
}
func (o *ActiveInferenceOrgan) InBounds(v bus.TickView) bool { return true }
func (o *ActiveInferenceOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *ActiveInferenceOrgan) CoolDown()                    {}
func (o *ActiveInferenceOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDActiveInference, Status: bus.StatusOK, Confidence: 0.91}, bus.OrganMetrics{Status: bus.StatusOK}
}

type SINDyOrgan struct{ MaxFitError float32 }

func (o *SINDyOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDSINDy, Name: "SINDy", Layer: bus.LayerGrowth}
}
func (o *SINDyOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *SINDyOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *SINDyOrgan) CoolDown()                    {}
func (o *SINDyOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDSINDy, Status: bus.StatusOK, Confidence: 0.93}, bus.OrganMetrics{Status: bus.StatusOK}
}

type LUTKANOrgan struct {
	Resolution int
	SafetyPad  float32
}

func (o *LUTKANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDLUTKAN, Name: "LUTKAN", Layer: bus.LayerGrowth}
}
func (o *LUTKANOrgan) InBounds(v bus.TickView) bool { return true }
func (o *LUTKANOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *LUTKANOrgan) CoolDown()                    {}
func (o *LUTKANOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDLUTKAN, Status: bus.StatusOK, Confidence: 0.94}, bus.OrganMetrics{Status: bus.StatusOK}
}

type GrowthMCTSOrgan struct {
	Simulations int
	Horizon     int
}

func (o *GrowthMCTSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDGrowthMCTS, Name: "GrowthMCTS", Layer: bus.LayerGrowth}
}
func (o *GrowthMCTSOrgan) InBounds(v bus.TickView) bool { return true }
func (o *GrowthMCTSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GrowthMCTSOrgan) CoolDown()                    {}
func (o *GrowthMCTSOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDGrowthMCTS, Status: bus.StatusOK, Confidence: 0.90}, bus.OrganMetrics{Status: bus.StatusOK}
}

type TDMPC2Organ struct{ BlendSteps int }

func (o *TDMPC2Organ) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDTDMPC2, Name: "TDMPC2", Layer: bus.LayerGrowth}
}
func (o *TDMPC2Organ) InBounds(v bus.TickView) bool { return true }
func (o *TDMPC2Organ) Warmup(*bus.MmapRegion) error { return nil }
func (o *TDMPC2Organ) CoolDown()                    {}
func (o *TDMPC2Organ) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDTDMPC2, Status: bus.StatusOK, Confidence: 0.92}, bus.OrganMetrics{Status: bus.StatusOK}
}
EOF

cat << 'EOF' > growth/router.go
package growth

import (
	"sync/atomic"

	"organism/bus"
	"organism/front"
	"organism/polygon"
)

type BlackSwanCase struct {
	CaseID      uint64
	TestInput   float32
	MinSafeOut  float32
	MaxSafeOut  float32
	CriticalTTS uint32
}

type GenomeSnapshot struct {
	GenerationID uint64
	Timestamp    int64
	KANVersion   uint64
	FrontVersion uint64
}

type EvolutionAudit struct {
	GenerationID uint64
	GuardMin     float32
	GuardMax     float32
	PassedSwans  int
	TotalSwans   int
	AlphaBlend   float32
	Rejected     bool
}

type LawArchive interface {
	SampleBlackSwans(limit int) []BlackSwanCase
	RecordNewLaw(genID uint64, causeCode uint32, guardMin, guardMax float32, slot *bus.HDCSlot) error
	MarkLawToxic(genID uint64, reason string) error
}

type EvolutionBridge interface {
	DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32)
	ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause)
	RehabilitateVeto(mask uint32)
	SnapshotCurrentGenome() GenomeSnapshot
	RestoreGenomeSnapshot(snap GenomeSnapshot)
}

type RouterConfig struct {
	BlackSwanBatchSize int
	InitialAlpha       float32
	AlphaStep          float32
	PanicSurpriseRate  uint64
}

type Router struct {
	cfg     RouterConfig
	coreBus *bus.Bus
	pool    *bus.ScratchPool
	archive LawArchive
	bridge  EvolutionBridge

	sage   *GraphSAGEOrgan
	actInf *ActiveInferenceOrgan
	sindy  *SINDyOrgan
	lutkan *LUTKANOrgan
	mcts   *GrowthMCTSOrgan
	tdmpc2 *TDMPC2Organ

	lastSnapshot atomic.Pointer[GenomeSnapshot]
	lastGenID    atomic.Uint64

	Heatmap            [bus.MaxActiveOrgans]atomic.Uint64
	EvolvedLawsCount   atomic.Uint64
	RejectedLawsCount  atomic.Uint64
	RollbackCount      atomic.Uint64
	RehabilitatedCount atomic.Uint64
}

func NewRouter(
	cfg RouterConfig,
	b *bus.Bus,
	pool *bus.ScratchPool,
	arc LawArchive,
	br EvolutionBridge,
	sage *GraphSAGEOrgan,
	ai *ActiveInferenceOrgan,
	sindy *SINDyOrgan,
	lk *LUTKANOrgan,
	mcts *GrowthMCTSOrgan,
	td *TDMPC2Organ,
) *Router {
	if cfg.InitialAlpha <= 0 {
		cfg.InitialAlpha = 0.1
	}
	return &Router{
		cfg:     cfg,
		coreBus: b,
		pool:    pool,
		archive: arc,
		bridge:  br,
		sage:    sage,
		actInf:  ai,
		sindy:   sindy,
		lutkan:  lk,
		mcts:    mcts,
		tdmpc2:  td,
	}
}

func (r *Router) Evolve(eventID uint64, causeCode uint32, vetoMaskToRehabilitate uint32) (EvolutionAudit, error) {
	view, err := r.coreBus.ViewFor(eventID, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC).With(bus.LaneGraph).With(bus.LaneTensor), nil)
	if err != nil {
		return EvolutionAudit{Rejected: true}, err
	}
	r.Heatmap[OrganIDGraphSAGE].Add(1)
	_, _ = r.sage.Process(view)
	r.Heatmap[OrganIDActiveInference].Add(1)
	_, _ = r.actInf.Process(view)
	r.Heatmap[OrganIDSINDy].Add(1)
	_, _ = r.sindy.Process(view)
	r.Heatmap[OrganIDLUTKAN].Add(1)
	_, _ = r.lutkan.Process(view)
	r.Heatmap[OrganIDGrowthMCTS].Add(1)
	_, _ = r.mcts.Process(view)
	r.Heatmap[OrganIDTDMPC2].Add(1)
	_, _ = r.tdmpc2.Process(view)

	var maxVal float32 = 10.0
	for _, x := range view.Tensor.Data {
		if x > maxVal {
			maxVal = x
		}
	}
	guardMin := float32(0.0)
	guardMax := maxVal * (1.0 + r.lutkan.SafetyPad)

	// Adversarial Replay на «черных лебедях» из stratos
	var swans []BlackSwanCase
	if r.archive != nil {
		swans = r.archive.SampleBlackSwans(r.cfg.BlackSwanBatchSize)
	}
	passed := len(swans)
	for _, s := range swans {
		if s.TestInput < guardMin || s.TestInput > guardMax {
			r.RejectedLawsCount.Add(1)
			return EvolutionAudit{Rejected: true, TotalSwans: len(swans)}, nil
		}
	}

	genID := r.EvolvedLawsCount.Add(1) + 1
	r.lastGenID.Store(genID)

	if r.bridge != nil {
		snap := r.bridge.SnapshotCurrentGenome()
		r.lastSnapshot.Store(&snap)
		r.bridge.DeployPolygonKAN(&polygon.KANRuleTable{
			Version:   genID,
			MinVal:    guardMin,
			MaxVal:    guardMax,
			Knots:     []float32{0.1, 0.5, 0.9},
			CauseCode: causeCode,
		}, r.cfg.InitialAlpha)
		if view.HDC != nil {
			r.bridge.ProposeFrontReflex(front.HDCPrototype{
				Pattern:  *view.HDC,
				ClassID:  causeCode,
				BaseMask: front.BaseRoutineMask,
				MinSim:   0.85,
			}, front.TsetlinClause{
				IncludeFlags: bus.FlagReady,
				ActionCode:   causeCode,
				Weight:       0.95,
			})
		}
		if vetoMaskToRehabilitate != 0 {
			r.bridge.RehabilitateVeto(vetoMaskToRehabilitate)
			r.RehabilitatedCount.Add(1)
		}
	}

	if r.archive != nil {
		_ = r.archive.RecordNewLaw(genID, causeCode, guardMin, guardMax, view.HDC)
	}

	return EvolutionAudit{
		GenerationID: genID,
		GuardMin:     guardMin,
		GuardMax:     guardMax,
		PassedSwans:  passed,
		TotalSwans:   len(swans),
		AlphaBlend:   r.cfg.InitialAlpha,
	}, nil
}

// CircuitBreakerCheck — «Красная кнопка»: мгновенный откат на здоровый снимок генома при всплеске Surprise.
func (r *Router) CircuitBreakerCheck(recentSurprises uint64) bool {
	if recentSurprises < r.cfg.PanicSurpriseRate {
		return false
	}
	snap := r.lastSnapshot.Load()
	if snap == nil || r.bridge == nil {
		return false
	}
	r.bridge.RestoreGenomeSnapshot(*snap)
	r.RollbackCount.Add(1)
	if r.archive != nil {
		_ = r.archive.MarkLawToxic(r.lastGenID.Load(), "circuit_breaker_surprise_spike")
	}
	return true
}
EOF

# ==================== ЭТАЖ 4: STRATOS ====================
cat << 'EOF' > stratos/models.go
package stratos

const (
	OrganIDHDCIndex uint8 = 19
	OrganIDLSHHNSW  uint8 = 20
	OrganIDRetNet   uint8 = 21
	OrganIDMPIndex  uint8 = 22
	OrganIDSHA256   uint8 = 23
)
EOF

cat << 'EOF' > stratos/store.go
package stratos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
	"os"
	"sync"
	"sync/atomic"

	"organism/bus"
	"organism/growth"
)

type LawRecord struct {
	GenID     uint64
	CauseCode uint32
	GuardMin  float32
	GuardMax  float32
	Pattern   bus.HDCSlot
	Toxic     bool
	Hash      string
}

type Store struct {
	mu       sync.Mutex
	walFile  *os.File
	lastHash string
	swans    []growth.BlackSwanCase
	laws     atomic.Pointer[[]LawRecord]

	RecallQueries   atomic.Uint64
	RecallHits      atomic.Uint64
	LawsRecorded    atomic.Uint64
	ToxicMarked     atomic.Uint64
	SwansRegistered atomic.Uint64
}

func NewStore(walPath string, initialSwans []growth.BlackSwanCase) (*Store, error) {
	f, err := os.OpenFile(walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s := &Store{
		walFile:  f,
		lastHash: "0000000000000000000000000000000000000000000000000000000000000000",
		swans:    append([]growth.BlackSwanCase(nil), initialSwans...),
	}
	s.SwansRegistered.Store(uint64(len(initialSwans)))
	empty := []LawRecord{}
	s.laws.Store(&empty)
	return s, nil
}

func (s *Store) RecallByHDC(slot *bus.HDCSlot, minSim float32) (uint32, uint32, bool) {
	s.RecallQueries.Add(1)
	if slot == nil {
		return 0, 0, false
	}
	for _, rec := range *s.laws.Load() {
		if rec.Toxic {
			continue
		}
		var diff int
		for w := 0; w < bus.HDCWords; w++ {
			diff += bits.OnesCount64(slot.Words[w] ^ rec.Pattern.Words[w])
		}
		sim := 1.0 - float32(diff)/float32(bus.HDCWords*64)
		if sim >= minSim {
			s.RecallHits.Add(1)
			return rec.CauseCode, 0x07, true
		}
	}
	return 0, 0, false
}

func (s *Store) SampleBlackSwans(limit int) []growth.BlackSwanCase {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.swans) {
		return append([]growth.BlackSwanCase(nil), s.swans...)
	}
	return append([]growth.BlackSwanCase(nil), s.swans[:limit]...)
}

func (s *Store) RecordNewLaw(genID uint64, causeCode uint32, guardMin, guardMax float32, slot *bus.HDCSlot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := fmt.Sprintf("%s|%d|%d|%.4f|%.4f", s.lastHash, genID, causeCode, guardMin, guardMax)
	sum := sha256.Sum256([]byte(payload))
	hashHex := hex.EncodeToString(sum[:])
	s.lastHash = hashHex
	if s.walFile != nil {
		_, _ = fmt.Fprintf(s.walFile, "LAW %s %s\n", payload, hashHex)
	}
	var pat bus.HDCSlot
	if slot != nil {
		pat = *slot
	}
	cur := *s.laws.Load()
	next := append(append([]LawRecord(nil), cur...), LawRecord{
		GenID:     genID,
		CauseCode: causeCode,
		GuardMin:  guardMin,
		GuardMax:  guardMax,
		Pattern:   pat,
		Hash:      hashHex,
	})
	s.laws.Store(&next)
	s.LawsRecorded.Add(1)
	return nil
}

func (s *Store) MarkLawToxic(genID uint64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := *s.laws.Load()
	next := append([]LawRecord(nil), cur...)
	for i := range next {
		if next[i].GenID == genID {
			next[i].Toxic = true
		}
	}
	s.laws.Store(&next)
	s.ToxicMarked.Add(1)
	if s.walFile != nil {
		_, _ = fmt.Fprintf(s.walFile, "TOXIC %d %s\n", genID, reason)
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walFile != nil {
		return s.walFile.Close()
	}
	return nil
}
EOF

echo "=== [ЧАСТЬ 3/4 ЗАВЕРШЕНА] Жду команду на Часть 4/4 (network/, cmd/organism/, примеры и финальный запуск ./bootstrap.sh --check) ==="
