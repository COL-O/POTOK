package front

import (
	"math"
	"math/bits"
	"sync/atomic"
	"time"

	"organism/modules/bus"
)

const (
	MaxReflexesPerTick  = 7
	SurpriseEnterThresh = 0.75
	SurpriseExitThresh  = 0.85
)

type VetoLevel uint8

const (
	VetoNone      VetoLevel = 0
	VetoHard      VetoLevel = 1
	VetoSoftTTS   VetoLevel = 2
	VetoTemporary VetoLevel = 3
)

type VetoRule struct {
	Mask       uint32
	Level      VetoLevel
	MinSafeTTS uint32
	ExpiresAt  int64
	ReasonCode uint32
}

type ReflexAudit struct {
	ReflexID     uint64
	OriginLayer  bus.LayerID
	GenealogyID  uint64
	CreatedAt    int64
	DecayScore   int32
	InQuarantine bool
	TrialWins    uint32
}

type RouterPolicy struct {
	Version    uint64
	VetoRules  []VetoRule
	Quarantine []HDCPrototype
}

type Prefetcher interface {
	HintDrift(classID uint32, similarity float32, tts uint32, priority uint8)
	Cancel(classID uint32)
}

type RouterConfig struct {
	BaseLeaseDuration time.Duration
	MaxFlapsInWindow  uint32
	FlapWindowNs      int64
	QuarantineTrials  uint32
}

type Router struct {
	cfg        RouterConfig
	bus        *bus.Bus
	registry   *bus.OrganRegistry
	pool       *bus.ScratchPool
	prefetcher Prefetcher

	hdc     *HDCOrgan
	tsetlin *TsetlinOrgan

	policy atomic.Pointer[RouterPolicy]

	inSurpriseLease atomic.Bool
	leaseUntilNs    atomic.Int64
	activeClassID   atomic.Uint32

	flapCount       atomic.Uint32
	flapWindowStart atomic.Int64

	Heatmap         [bus.MaxActiveOrgans]atomic.Uint64
	SurpriseCount   atomic.Uint64
	EscalatedGrowth atomic.Uint64
	VetoBlocked     atomic.Uint64
	DemotedReflexes atomic.Uint64
	// OrgansOutOfBounds — сколько раз орган из маски не смог быть выполнен,
	// потому что вышел за свою применимость (КОН 10: граница фиксируется,
	// а не скрывается). Такт не обрушается: остальные органы маски отрабатывают.
	OrgansOutOfBounds atomic.Uint64
}

func NewRouter(
	cfg RouterConfig,
	b *bus.Bus,
	reg *bus.OrganRegistry,
	pool *bus.ScratchPool,
	hdc *HDCOrgan,
	tsetlin *TsetlinOrgan,
	pref Prefetcher,
) *Router {
	r := &Router{
		cfg:        cfg,
		bus:        b,
		registry:   reg,
		pool:       pool,
		prefetcher: pref,
		hdc:        hdc,
		tsetlin:    tsetlin,
	}
	r.policy.Store(&RouterPolicy{Version: 1})
	return r
}

func (r *Router) UpdatePolicyRCU(next *RouterPolicy) {
	r.policy.Store(next)
}

func (r *Router) Step(eventID uint64) (bus.Verdict, error) {
	now := time.Now().UnixNano()

	baseView, err := r.bus.ViewFor(eventID, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC), nil)
	if err != nil {
		return bus.Verdict{}, err
	}

	hdcVerdict, hdcMetrics := r.hdc.Process(baseView)
	r.Heatmap[OrganIDHDC].Add(1)

	if r.prefetcher != nil {
		if hdcVerdict.Status == bus.StatusOutOfBounds || hdcVerdict.Confidence < SurpriseExitThresh {
			r.prefetcher.HintDrift(hdcVerdict.ActionCode, hdcVerdict.Confidence, baseView.Header.TTS, baseView.Header.Priority)
		} else {
			r.prefetcher.Cancel(hdcVerdict.ActionCode)
		}
	}

	if r.checkLeaseAndHysteresis(now, hdcVerdict) {
		r.escalateToPolygon(baseView.Header, false)
		return bus.Verdict{OrganID: OrganIDHDC, Status: bus.StatusAbstain, Confidence: hdcVerdict.Confidence, ActionCode: hdcVerdict.ActionCode}, nil
	}

	if hdcVerdict.Status != bus.StatusOK {
		r.triggerSurpriseWithResonanceCheck(now, baseView.Header, hdcVerdict.ActionCode)
		return hdcVerdict, nil
	}

	tsetlinVerdict, _ := r.tsetlin.Process(baseView)
	r.Heatmap[OrganIDTsetlin].Add(1)

	if tsetlinVerdict.Status == bus.StatusAbstain {
		r.triggerSurpriseWithResonanceCheck(now, baseView.Header, hdcVerdict.ActionCode)
		return tsetlinVerdict, nil
	}

	rawMask := hdcVerdict.SuggestedMask | tsetlinVerdict.SuggestedMask

	pol := r.policy.Load()
	vetoMask, vetoTriggered := r.computeVetoMask(now, baseView.Header.TTS, pol)
	finalMask := rawMask &^ vetoMask

	if vetoTriggered {
		r.VetoBlocked.Add(1)
		atomic.StoreUint32(&baseView.Header.VetoMask, vetoMask)
		r.setFlag(baseView.Header, bus.FlagVetoTriggered)
	}

	if bits.OnesCount32(finalMask) > MaxReflexesPerTick || finalMask == 0 {
		r.triggerSurpriseWithResonanceCheck(now, baseView.Header, hdcVerdict.ActionCode)
		return bus.Verdict{OrganID: OrganIDTsetlin, Status: bus.StatusOutOfBounds}, nil
	}

	atomic.StoreUint32(&baseView.Header.ActiveMask, finalMask)

	execMask := finalMask &^ (BitHDC | BitTsetlin)
	bestVerdict := hdcVerdict
	if tsetlinVerdict.Confidence > bestVerdict.Confidence {
		bestVerdict = tsetlinVerdict
	}

	for execMask != 0 {
		organID := uint8(bits.TrailingZeros32(execMask))
		execMask &^= 1 << organID
		org, ok := r.registry.Get(organID)
		if !ok {
			continue
		}
		spec := org.Spec()
		scratch, _ := r.pool.Acquire()
		view, err := r.bus.ViewFor(eventID, spec.ReadLanes, scratch)
		if err != nil {
			continue
		}
		if !org.InBounds(view) {
			// КОН 10: граница органа — факт, фиксируем в телеметрии,
			// но не обрушаем такт: остальные органы маски отрабатывают.
			// Вырожденный случай «никто не отработал» разруливает Confidence Gate ниже.
			r.OrgansOutOfBounds.Add(1)
			r.Heatmap[organID].Add(1)
			continue
		}
		v, m := org.Process(view)
		r.Heatmap[organID].Add(1)
		if spec.HardTimeout > 0 && time.Duration(m.LatencyNs) > spec.HardTimeout {
			v.Status = bus.StatusTimeout
			v.Confidence = 0.0
		}
		if v.Status == bus.StatusOK && v.Confidence > bestVerdict.Confidence {
			bestVerdict = v
		}
		if v.Status == bus.StatusOutOfBounds || v.Status == bus.StatusTimeout || v.Status == bus.StatusError {
			r.triggerSurpriseWithResonanceCheck(now, baseView.Header, hdcVerdict.ActionCode)
			_ = r.bus.CommitVerdict(eventID, v)
			return v, nil
		}
	}

	r.evaluateQuarantineShadow(baseView, bestVerdict, pol)

	if bestVerdict.Confidence < SurpriseEnterThresh {
		r.DemotedReflexes.Add(1)
		r.triggerSurpriseWithResonanceCheck(now, baseView.Header, hdcVerdict.ActionCode)
		_ = r.bus.CommitVerdict(eventID, bestVerdict)
		return bestVerdict, nil
	}

	_ = hdcMetrics
	_ = r.bus.CommitVerdict(eventID, bestVerdict)
	return bestVerdict, nil
}

func (r *Router) computeVetoMask(nowNs int64, tts uint32, pol *RouterPolicy) (uint32, bool) {
	var mask uint32
	for i := range pol.VetoRules {
		rule := &pol.VetoRules[i]
		switch rule.Level {
		case VetoHard:
			mask |= rule.Mask
		case VetoSoftTTS:
			if tts < rule.MinSafeTTS {
				mask |= rule.Mask
			}
		case VetoTemporary:
			if nowNs < rule.ExpiresAt {
				mask |= rule.Mask
			}
		}
	}
	return mask, mask != 0
}

func (r *Router) checkLeaseAndHysteresis(nowNs int64, hdcVerdict bus.Verdict) bool {
	if !r.inSurpriseLease.Load() {
		return false
	}
	if hdcVerdict.Status == bus.StatusOK && hdcVerdict.ActionCode != r.activeClassID.Load() {
		r.inSurpriseLease.Store(false)
		return false
	}
	if hdcVerdict.Status == bus.StatusOK && hdcVerdict.Confidence >= SurpriseExitThresh && nowNs >= r.leaseUntilNs.Load() {
		r.inSurpriseLease.Store(false)
		return false
	}
	r.leaseUntilNs.Store(nowNs + r.cfg.BaseLeaseDuration.Nanoseconds())
	return true
}

func (r *Router) triggerSurpriseWithResonanceCheck(nowNs int64, hdr *bus.EventHeader, classID uint32) {
	r.SurpriseCount.Add(1)
	r.activeClassID.Store(classID)
	r.inSurpriseLease.Store(true)
	r.leaseUntilNs.Store(nowNs + r.cfg.BaseLeaseDuration.Nanoseconds())

	winStart := r.flapWindowStart.Load()
	if nowNs-winStart > r.cfg.FlapWindowNs {
		r.flapWindowStart.Store(nowNs)
		r.flapCount.Store(1)
		r.escalateToPolygon(hdr, false)
		return
	}
	flaps := r.flapCount.Add(1)
	if flaps >= r.cfg.MaxFlapsInWindow {
		r.EscalatedGrowth.Add(1)
		r.escalateToPolygon(hdr, true)
		return
	}
	r.escalateToPolygon(hdr, false)
}

func (r *Router) escalateToPolygon(hdr *bus.EventHeader, directGrowth bool) {
	flags := bus.FlagSurprise
	if directGrowth {
		flags |= bus.FlagEscalateToGrowth
	}
	r.setFlag(hdr, flags)
}

func (r *Router) setFlag(hdr *bus.EventHeader, flag uint32) {
	for {
		old := atomic.LoadUint32(&hdr.StateFlags)
		if atomic.CompareAndSwapUint32(&hdr.StateFlags, old, old|flag) {
			break
		}
	}
}

func (r *Router) evaluateQuarantineShadow(view bus.TickView, activeVerdict bus.Verdict, pol *RouterPolicy) {
	if len(pol.Quarantine) == 0 || view.HDC == nil {
		return
	}
	const totalBits = float32(bus.HDCWords * 64)
	for i := range pol.Quarantine {
		cand := &pol.Quarantine[i]
		var diff int
		for w := 0; w < bus.HDCWords; w++ {
			diff += bits.OnesCount64(view.HDC.Words[w] ^ cand.Pattern.Words[w])
		}
		sim := 1.0 - (float32(diff) / totalBits)
		// Кварантин — это «похоже на опасный класс, даже если активный ответ уверен».
		// Строгое `>` отказывало в кварантине при РАВНОЙ уверенности: нулевой кадр
		// совпадает с нулевым шаблоном на 1.0, активный вердикт тоже 1.0 — флаг не
		// ставился (падение TestRouterRoutineVetoAndQuarantine, router_test.go:101).
		if sim >= cand.MinSim && sim >= activeVerdict.Confidence {
			r.setFlag(view.Header, bus.FlagInQuarantine)
			break
		}
	}
}

func BitsToConfidence(hdr *bus.EventHeader) float32 {
	return math.Float32frombits(atomic.LoadUint32(&hdr.Confidence))
}
