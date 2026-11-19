package front

import (
	"math/bits"
	"sync/atomic"
	"time"

	"organism/bus"
)

type VetoLevel uint8

const (
	VetoHard      VetoLevel = 1
	VetoSoftTTS   VetoLevel = 2
	VetoTemporary VetoLevel = 3
)

type VetoRule struct {
	Mask       uint32
	Level      VetoLevel
	ReasonCode uint16
	ExpiresAt  int64
}

type RouterPolicy struct {
	Version    uint64
	VetoRules  []VetoRule
	Quarantine []HDCPrototype
}

type RouterConfig struct {
	BaseLeaseDuration time.Duration
	MaxFlapsInWindow  int
	FlapWindowNs      int64
	QuarantineTrials  int
}

type Router struct {
	cfg        RouterConfig
	coreBus    *bus.Bus
	registry   *bus.OrganRegistry
	pool       *bus.ScratchPool
	hdc        *HDCOrgan
	tsetlin    *TsetlinOrgan
	prefetcher *GraphPrefetcher

	policy          atomic.Pointer[RouterPolicy]
	inSurpriseLease atomic.Bool
	leaseUntilNs    atomic.Int64
	Heatmap         [bus.MaxActiveOrgans]atomic.Uint64
	SurpriseCount   atomic.Uint64
	VetoBlocked     atomic.Uint64
	EscalatedGrowth atomic.Uint64
	DemotedReflexes atomic.Uint64
}

func NewRouter(
	cfg RouterConfig,
	b *bus.Bus,
	reg *bus.OrganRegistry,
	pool *bus.ScratchPool,
	hdc *HDCOrgan,
	ts *TsetlinOrgan,
	pf *GraphPrefetcher,
) *Router {
	r := &Router{
		cfg:        cfg,
		coreBus:    b,
		registry:   reg,
		pool:       pool,
		hdc:        hdc,
		tsetlin:    ts,
		prefetcher: pf,
	}
	r.policy.Store(&RouterPolicy{Version: 1})
	return r
}

func (r *Router) UpdatePolicyRCU(next *RouterPolicy) {
	if next != nil {
		r.policy.Store(next)
		r.inSurpriseLease.Store(false)
	}
}

func (r *Router) Step(eventID uint64) (bus.Verdict, error) {
	view, err := r.coreBus.ViewFor(eventID, bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC), nil)
	if err != nil {
		return bus.Verdict{Status: bus.StatusError}, err
	}
	r.Heatmap[OrganIDHDC].Add(1)
	hdcVerdict, _ := r.hdc.Process(view)
	// Проверка Карантина, если основной HDC не узнал паттерн
	if hdcVerdict.Status != bus.StatusOK {
		pol := r.policy.Load()
		for i := range pol.Quarantine {
			p := &pol.Quarantine[i]
			var diff int
			for w := 0; w < bus.HDCWords; w++ {
				diff += bits.OnesCount64(view.HDC.Words[w] ^ p.Pattern.Words[w])
			}
			sim := 1.0 - float32(diff)/float32(bus.HDCWords*64)
			if sim >= p.MinSim {
				hdcVerdict = bus.Verdict{
					OrganID:       OrganIDHDC,
					Status:        bus.StatusOK,
					Confidence:    sim,
					ActionCode:    p.ClassID,
					SuggestedMask: p.BaseMask,
				}
				break
			}
		}
	}
	if hdcVerdict.Status != bus.StatusOK {
		r.SurpriseCount.Add(1)
		r.inSurpriseLease.Store(true)
		r.leaseUntilNs.Store(time.Now().Add(r.cfg.BaseLeaseDuration).UnixNano())
		_ = r.coreBus.CommitVerdict(eventID, hdcVerdict, bus.FlagSurprise)
		return hdcVerdict, nil
	}
	r.Heatmap[OrganIDTsetlin].Add(1)
	tsVerdict, _ := r.tsetlin.Process(view)
	activeMask := hdcVerdict.SuggestedMask | tsVerdict.SuggestedMask
	var vetoMask uint32
	nowNs := time.Now().UnixNano()
	pol := r.policy.Load()
	for _, vr := range pol.VetoRules {
		if vr.ExpiresAt > 0 && nowNs > vr.ExpiresAt {
			continue
		}
		vetoMask |= vr.Mask
	}
	if (activeMask & vetoMask) != 0 {
		r.VetoBlocked.Add(1)
	}
	finalMask := activeMask &^ vetoMask
	finalVerdict := bus.Verdict{
		OrganID:       OrganIDHDC,
		Status:        bus.StatusOK,
		Confidence:    hdcVerdict.Confidence,
		ActionCode:    tsVerdict.ActionCode,
		SuggestedMask: finalMask,
	}
	if finalVerdict.ActionCode == 0 {
		finalVerdict.ActionCode = hdcVerdict.ActionCode
	}
	// Запускаем остальные выбранные органы из finalMask (биты 2..31)
	for id := uint8(2); id < bus.MaxActiveOrgans; id++ {
		if (finalMask & (1 << id)) == 0 {
			continue
		}
		org, ok := r.registry.Get(id)
		if !ok {
			continue
		}
		r.Heatmap[id].Add(1)
		v, _ := org.Process(view)
		if v.Status == bus.StatusOutOfBounds {
			r.SurpriseCount.Add(1)
			finalVerdict.Status = bus.StatusOutOfBounds
			_ = r.coreBus.CommitVerdict(eventID, finalVerdict, bus.FlagSurprise)
			return finalVerdict, nil
		}
		if v.ActionCode != 0 {
			finalVerdict.ActionCode = v.ActionCode
		}
	}
	_ = r.coreBus.CommitVerdict(eventID, finalVerdict, 0)
	return finalVerdict, nil
}
