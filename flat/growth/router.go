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
