package growth

import (
	"sync/atomic"
	"unsafe"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/polygon"
)

// BlackSwanCase — историческая авария из stratos.
type BlackSwanCase struct {
	CaseID      uint64
	TestInput   float32
	MinSafeOut  float32
	MaxSafeOut  float32
	CriticalTTS uint32
}

// StratosArchive — интерфейс взаимодействия growth с долгой памятью.
type StratosArchive interface {
	FetchBlackSwans(limit int) []BlackSwanCase
	RecordNewLaw(eq SINDyEquation, lut CompiledLUT, genealogyID uint64)
	MarkLawToxic(genealogyID uint64, reason uint32)
}

// LowerLayerUpdater — интерфейс спуска новых законов в polygon и front.
type LowerLayerUpdater interface {
	DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32)
	ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause)
	RehabilitateVeto(mask uint32)
	SnapshotCurrentGenome() GenomeSnapshot
	RestoreGenomeSnapshot(snap GenomeSnapshot)
}

// GenomeSnapshot — неизменяемый снимок здорового поколения.
type GenomeSnapshot struct {
	GenerationID uint64
	Timestamp    int64
	KANVersion   uint64
	FrontVersion uint64
}

// EvolutionAudit — полная родословная выведенного закона.
type EvolutionAudit struct {
	GenerationID    uint64
	EventID         uint64
	Equation        SINDyEquation
	GuardMin        float32
	GuardMax        float32
	PassedSwans     int
	TotalSwans      int
	AlphaBlend      float32
	Rehabilitated   uint32
	Rejected        bool
	RejectionReason uint32
}

// RouterConfig — настройки эволюционного контура growth.
type RouterConfig struct {
	BlackSwanBatchSize int
	InitialAlpha       float32
	AlphaStep          float32
	PanicSurpriseRate  uint64
}

// Router — Стратег / Архитектор организма.
type Router struct {
	cfg     RouterConfig
	bus     *bus.Bus
	pool    *bus.ScratchPool
	stratos StratosArchive
	updater LowerLayerUpdater

	sage   *GraphSAGEOrgan
	actInf *ActiveInferenceOrgan
	sindy  *SINDyOrgan
	lutkan *LUTKANOrgan
	mcts   *GrowthMCTSOrgan
	tdmpc2 *TDMPC2Organ

	generationSeq   atomic.Uint64
	lastHealthySnap atomic.Pointer[GenomeSnapshot]
	currentAlpha    atomic.Uint32

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
	stratos StratosArchive,
	updater LowerLayerUpdater,
	sage *GraphSAGEOrgan,
	actInf *ActiveInferenceOrgan,
	sindy *SINDyOrgan,
	lutkan *LUTKANOrgan,
	mcts *GrowthMCTSOrgan,
	tdmpc2 *TDMPC2Organ,
) *Router {
	if cfg.InitialAlpha <= 0 || cfg.InitialAlpha > 1.0 {
		cfg.InitialAlpha = 0.1
	}
	if cfg.AlphaStep <= 0 {
		cfg.AlphaStep = 0.3
	}
	r := &Router{
		cfg: cfg, bus: b, pool: pool, stratos: stratos, updater: updater,
		sage: sage, actInf: actInf, sindy: sindy, lutkan: lutkan, mcts: mcts, tdmpc2: tdmpc2,
	}
	r.generationSeq.Store(1)
	return r
}

func (r *Router) Evolve(eventID uint64, targetCauseCode uint32, vetoToRehabilitate uint32) (EvolutionAudit, error) {
	scratch, _ := r.pool.Acquire()
	lanes := bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC).With(bus.LaneGraph).With(bus.LaneTensor)
	view, err := r.bus.ViewFor(eventID, lanes, scratch)
	if err != nil {
		return EvolutionAudit{}, err
	}
	genID := r.generationSeq.Add(1)
	audit := EvolutionAudit{GenerationID: genID, EventID: eventID}

	if r.sage.InBounds(view) {
		_, _ = r.sage.Process(view)
		r.Heatmap[OrganIDGraphSAGE].Add(1)
	}
	aiVerdict, _ := r.actInf.Process(view)
	r.Heatmap[OrganIDActiveInference].Add(1)
	if aiVerdict.Status != bus.StatusOK {
		audit.Rejected = true
		audit.RejectionReason = 6001
		r.RejectedLawsCount.Add(1)
		return audit, nil
	}

	var eq SINDyEquation
	if len(scratch) >= 64 {
		sindyView := view
		sindyView.Scratch = scratch[:64]
		sindyVerdict, _ := r.sindy.Process(sindyView)
		r.Heatmap[OrganIDSINDy].Add(1)
		if sindyVerdict.Status != bus.StatusOK {
			audit.Rejected = true
			audit.RejectionReason = 6002
			r.RejectedLawsCount.Add(1)
			return audit, nil
		}
		eq = *(*SINDyEquation)(unsafe.Pointer(&scratch[0]))
		eq.EquationID = genID
		audit.Equation = eq
	}

	res := r.lutkan.Resolution
	if res <= 1 {
		res = 8
	}
	var compiled CompiledLUT
	if len(scratch) >= 64+(res+2)*4 {
		lutView := view
		lutView.Scratch = scratch
		lutVerdict, _ := r.lutkan.Process(lutView)
		r.Heatmap[OrganIDLUTKAN].Add(1)
		if lutVerdict.Status != bus.StatusOK {
			audit.Rejected = true
			audit.RejectionReason = 6003
			r.RejectedLawsCount.Add(1)
			return audit, nil
		}
		rawOut := unsafe.Slice((*float32)(unsafe.Pointer(&scratch[64])), res+2)
		knots := make([]float32, res)
		copy(knots, rawOut[2:2+res])
		compiled = CompiledLUT{MinVal: rawOut[0], MaxVal: rawOut[1], Knots: knots, EquationID: genID, SafetyScore: lutVerdict.Confidence}
		audit.GuardMin = compiled.MinVal
		audit.GuardMax = compiled.MaxVal
	}

	var bestBranchMask uint32 = front.BaseRoutineMask | front.BitDStarLite
	if r.mcts.InBounds(view) {
		mctsVerdict, _ := r.mcts.Process(view)
		r.Heatmap[OrganIDGrowthMCTS].Add(1)
		if mctsVerdict.Status != bus.StatusOK {
			audit.Rejected = true
			audit.RejectionReason = 6004
			r.RejectedLawsCount.Add(1)
			return audit, nil
		}
		bestBranchMask |= front.BitCapacity
	}
	_, _ = r.tdmpc2.Process(view)
	r.Heatmap[OrganIDTDMPC2].Add(1)

	if r.stratos != nil {
		swans := r.stratos.FetchBlackSwans(r.cfg.BlackSwanBatchSize)
		audit.TotalSwans = len(swans)
		for i := range swans {
			if !r.verifyOnBlackSwan(compiled, swans[i]) {
				audit.Rejected = true
				audit.RejectionReason = 6005
				r.RejectedLawsCount.Add(1)
				r.stratos.MarkLawToxic(genID, 6005)
				return audit, nil
			}
			audit.PassedSwans++
		}
	}

	if r.updater != nil {
		alpha := r.cfg.InitialAlpha
		audit.AlphaBlend = alpha
		// Сначала — доказанное лечение вето (7.4): оно становится частью
		// «предыдущего здорового поколения» (docs/05-growth.md §7.5).
		// Закон (7.5): снимок v.N-1 — ДО развёртывания нового закона,
		// единая связка {HDC-таблица + KAN + VetoMask}, но без уже снятых
		// реабилитацией запретов — иначе откат воскресил бы то, что
		// growth уже признал безопасным.
		if vetoToRehabilitate != 0 {
			r.updater.RehabilitateVeto(vetoToRehabilitate)
			audit.Rehabilitated = vetoToRehabilitate
			r.RehabilitatedCount.Add(1)
		}
		snap := r.updater.SnapshotCurrentGenome()
		r.lastHealthySnap.Store(&snap)
		// Теперь — развёртывание v.N: KAN и рефлекс принадлежат новому поколению.
		newKAN := &polygon.KANRuleTable{
			Version:     genID,
			GenealogyID: genID,
			MinVal:      compiled.MinVal,
			MaxVal:      compiled.MaxVal,
			Knots:       compiled.Knots,
			CauseCode:   targetCauseCode,
		}
		r.updater.DeployPolygonKAN(newKAN, alpha)
		if view.HDC != nil {
			r.updater.ProposeFrontReflex(
				front.HDCPrototype{Pattern: *view.HDC, ClassID: targetCauseCode, BaseMask: bestBranchMask, MinSim: 0.85, GenealogyID: genID},
				front.TsetlinClause{IncludeFlags: bus.FlagReady, AddMask: bestBranchMask, ActionCode: targetCauseCode, Weight: compiled.SafetyScore},
			)
		}
	}

	if r.stratos != nil {
		r.stratos.RecordNewLaw(eq, compiled, genID)
	}
	r.EvolvedLawsCount.Add(1)
	return audit, nil
}

func (r *Router) verifyOnBlackSwan(lut CompiledLUT, swan BlackSwanCase) bool {
	if len(lut.Knots) < 2 {
		return false
	}
	if swan.TestInput < lut.MinVal || swan.TestInput > lut.MaxVal {
		return true
	}
	norm := (swan.TestInput - lut.MinVal) / (lut.MaxVal - lut.MinVal)
	idx := int(norm * float32(len(lut.Knots)-1))
	if idx >= len(lut.Knots) {
		idx = len(lut.Knots) - 1
	}
	out := lut.Knots[idx]
	return out >= swan.MinSafeOut && out <= swan.MaxSafeOut
}

func (r *Router) CircuitBreakerCheck(recentSurprises uint64) bool {
	if r.cfg.PanicSurpriseRate == 0 || recentSurprises < r.cfg.PanicSurpriseRate {
		return false
	}
	snap := r.lastHealthySnap.Load()
	if snap == nil || r.updater == nil {
		return false
	}
	r.updater.RestoreGenomeSnapshot(*snap)
	r.RollbackCount.Add(1)
	if r.stratos != nil {
		r.stratos.MarkLawToxic(r.generationSeq.Load(), 6999)
	}
	return true
}
