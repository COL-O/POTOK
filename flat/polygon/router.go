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
