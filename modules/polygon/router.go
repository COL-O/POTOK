package polygon

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/COL-O/POTOK/modules/bus"
	"github.com/COL-O/POTOK/modules/front"
)

// Precedent — исторический прецедент из stratos.
type Precedent struct {
	PrecedentID     uint64
	CauseCode       uint32
	AntibodyAction  uint32
	RecommendedMask uint32
	Confidence      float32
}

// StratosRecall — интерфейс чтения долгой памяти stratos.
type StratosRecall interface {
	RecallByHDC(hdc *bus.HDCSlot, minSim float32) (Precedent, bool)
}

// FrontPolicyUpdater — интерфейс атомарного обновления Карантина и VetoMask в front.
type FrontPolicyUpdater interface {
	ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause)
	ApplyVetoRule(rule front.VetoRule)
	RehabilitateVeto(mask uint32)
}

// DiagnosisAudit — запись прозрачного аудита диагноза.
type DiagnosisAudit struct {
	EventID        uint64
	Timestamp      int64
	CauseCode      uint32
	TrendCode      uint32
	AntibodyAction uint32
	UsedPrecedent  uint64
	Escalated      bool
	Confidence     float32
}

// RouterConfig — настройки гомеостаза Иммунитета.
type RouterConfig struct {
	MinPrecedentSim  float32
	RecurrenceLimit  uint32
	RecurrenceWindow time.Duration
	DefaultVetoTTL   time.Duration
}

// Router — Совет управляющих / Иммунитет организма.
type Router struct {
	cfg      RouterConfig
	bus      *bus.Bus
	registry *bus.OrganRegistry
	pool     *bus.ScratchPool
	stratos  StratosRecall
	frontUpd FrontPolicyUpdater

	mp      *MatrixProfileOrgan
	kan     *KANOrgan
	cfc     *CfCOrgan
	cbs     *CBSOrgan
	esn     *ESNOrgan
	tsetlin *PolygonTsetlinOrgan

	lastCauseCode   atomic.Uint32
	recurrenceCount atomic.Uint32
	recurrenceStart atomic.Int64

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
	stratos StratosRecall,
	frontUpd FrontPolicyUpdater,
	mp *MatrixProfileOrgan,
	kan *KANOrgan,
	cfc *CfCOrgan,
	cbs *CBSOrgan,
	esn *ESNOrgan,
	tsetlin *PolygonTsetlinOrgan,
) *Router {
	return &Router{
		cfg: cfg, bus: b, registry: reg, pool: pool,
		stratos: stratos, frontUpd: frontUpd,
		mp: mp, kan: kan, cfc: cfc, cbs: cbs, esn: esn, tsetlin: tsetlin,
	}
}

func (r *Router) Diagnose(eventID uint64) (bus.Verdict, DiagnosisAudit, error) {
	now := time.Now().UnixNano()
	scratch, _ := r.pool.Acquire()
	lanes := bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC).With(bus.LaneGraph).With(bus.LaneTensor)
	view, err := r.bus.ViewFor(eventID, lanes, scratch)
	if err != nil {
		return bus.Verdict{}, DiagnosisAudit{}, err
	}
	audit := DiagnosisAudit{EventID: eventID, Timestamp: now}

	mpVerdict, _ := r.mp.Process(view)
	r.Heatmap[OrganIDMatrixProfile].Add(1)
	if mpVerdict.Status == bus.StatusAbstain {
		r.DiagnosedCount.Add(1)
		audit.Confidence = 0.90
		return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusOK, Confidence: 0.90}, audit, nil
	}

	kanVerdict, _ := r.kan.Process(view)
	r.Heatmap[OrganIDKAN].Add(1)
	if kanVerdict.Status == bus.StatusOutOfBounds {
		return r.escalate(view.Header, audit, 4001), audit, nil
	}
	causeCode := kanVerdict.ActionCode
	audit.CauseCode = causeCode

	cfcVerdict, _ := r.cfc.Process(view)
	r.Heatmap[OrganIDCfC].Add(1)
	trendCode := cfcVerdict.ActionCode
	audit.TrendCode = trendCode

	if kanVerdict.Status == bus.StatusOK && cfcVerdict.Status == bus.StatusOK &&
		(kanVerdict.Confidence-cfcVerdict.Confidence > 0.35 || cfcVerdict.Confidence-kanVerdict.Confidence > 0.35) {
		if r.stratos != nil && view.HDC != nil {
			if prec, found := r.stratos.RecallByHDC(view.HDC, r.cfg.MinPrecedentSim); found {
				r.PrecedentHits.Add(1)
				audit.UsedPrecedent = prec.PrecedentID
				audit.AntibodyAction = prec.AntibodyAction
				audit.Confidence = prec.Confidence
				r.deployAntibodyAndQuarantine(view, prec.CauseCode, prec.AntibodyAction, prec.RecommendedMask, prec.Confidence, prec.PrecedentID)
				v := bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOK, Confidence: prec.Confidence, ActionCode: prec.AntibodyAction, SuggestedMask: prec.RecommendedMask}
				_ = r.bus.CommitVerdict(eventID, v)
				return v, audit, nil
			}
			return r.escalate(view.Header, audit, 4002), audit, nil
		}
	}

	var routeMask uint32 = front.BaseRoutineMask
	if (cfcVerdict.SuggestedMask & BitCBS) != 0 {
		cbsVerdict, _ := r.cbs.Process(view)
		r.Heatmap[OrganIDCBS].Add(1)
		if cbsVerdict.Status == bus.StatusOutOfBounds {
			r.issueVeto(front.BitDStarLite, front.VetoHard, 0, 5001)
			return r.escalate(view.Header, audit, 4003), audit, nil
		}
		routeMask |= front.BitDStarLite | front.BitCapacity
	}

	esnVerdict, _ := r.esn.Process(view)
	r.Heatmap[OrganIDESN].Add(1)
	if esnVerdict.ActionCode == 2 {
		routeMask |= front.BitTTL
	}

	if len(view.Scratch) >= 8 {
		ctx := unsafe.Slice((*uint32)(unsafe.Pointer(&view.Scratch[0])), 2)
		ctx[0] = causeCode
		ctx[1] = trendCode
	}
	polyVerdict, _ := r.tsetlin.Process(view)
	r.Heatmap[OrganIDPolyTsetlin].Add(1)
	if polyVerdict.Status != bus.StatusOK {
		return r.escalate(view.Header, audit, 4004), audit, nil
	}

	if r.checkChronicRecurrence(now, causeCode) {
		audit.Escalated = true
		r.escalate(view.Header, audit, 4005)
	}

	finalMask := routeMask | polyVerdict.SuggestedMask
	audit.AntibodyAction = polyVerdict.ActionCode
	audit.Confidence = polyVerdict.Confidence
	r.DiagnosedCount.Add(1)
	r.deployAntibodyAndQuarantine(view, causeCode, polyVerdict.ActionCode, finalMask, polyVerdict.Confidence, 0)

	if view.Header.TTS == 0 {
		r.issueVeto(front.BitMCTS, front.VetoTemporary, 0, 5002)
	}

	out := bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOK, Confidence: polyVerdict.Confidence, ActionCode: polyVerdict.ActionCode, SuggestedMask: finalMask}
	_ = r.bus.CommitVerdict(eventID, out)
	return out, audit, nil
}

func (r *Router) deployAntibodyAndQuarantine(view bus.TickView, causeCode uint32, actionCode uint32, mask uint32, conf float32, genealogyID uint64) {
	if r.frontUpd == nil || view.HDC == nil {
		return
	}
	proto := front.HDCPrototype{Pattern: *view.HDC, ClassID: causeCode, BaseMask: mask, MinSim: 0.85, GenealogyID: genealogyID}
	clause := front.TsetlinClause{IncludeFlags: bus.FlagReady, AddMask: mask, ActionCode: actionCode, Weight: conf}
	r.frontUpd.ProposeQuarantineReflex(proto, clause)
	r.AntibodiesSent.Add(1)
}

func (r *Router) issueVeto(mask uint32, level front.VetoLevel, minTTS uint32, reason uint32) {
	if r.frontUpd == nil {
		return
	}
	ttl := r.cfg.DefaultVetoTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	r.frontUpd.ApplyVetoRule(front.VetoRule{Mask: mask, Level: level, MinSafeTTS: minTTS, ExpiresAt: time.Now().Add(ttl).UnixNano(), ReasonCode: reason})
	r.VetoIssuedCount.Add(1)
}

func (r *Router) checkChronicRecurrence(nowNs int64, causeCode uint32) bool {
	win := r.cfg.RecurrenceWindow.Nanoseconds()
	if win <= 0 {
		win = int64(time.Minute)
	}
	if r.lastCauseCode.Load() != causeCode || nowNs-r.recurrenceStart.Load() > win {
		r.lastCauseCode.Store(causeCode)
		r.recurrenceStart.Store(nowNs)
		r.recurrenceCount.Store(1)
		return false
	}
	cnt := r.recurrenceCount.Add(1)
	return cnt >= r.cfg.RecurrenceLimit
}

func (r *Router) escalate(hdr *bus.EventHeader, audit DiagnosisAudit, reasonCode uint32) bus.Verdict {
	r.EscalatedGrowth.Add(1)
	for {
		old := atomic.LoadUint32(&hdr.StateFlags)
		if atomic.CompareAndSwapUint32(&hdr.StateFlags, old, old|bus.FlagEscalateToGrowth) {
			break
		}
	}
	return bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOutOfBounds, Confidence: 0.0, ActionCode: reasonCode}
}
