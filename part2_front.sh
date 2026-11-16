#!/usr/bin/env bash
set -euo pipefail

echo "=== [ЧАСТЬ 2/4] СОЗДАНИЕ ПЕРВОГО ЭТАЖА (front/) ==="

cat << 'EOF' > front/models.go
package front

import (
	"math/bits"
	"sync/atomic"
	"time"

	"organism/bus"
)

const (
	OrganIDHDC       uint8 = 0
	OrganIDTsetlin   uint8 = 1
	OrganIDTTL       uint8 = 2
	OrganIDCapacity  uint8 = 3
	OrganIDDStarLite uint8 = 4
	OrganIDMCTS      uint8 = 5
	OrganIDPasport   uint8 = 6

	BitHDC       uint32 = 1 << OrganIDHDC
	BitTsetlin   uint32 = 1 << OrganIDTsetlin
	BitTTL       uint32 = 1 << OrganIDTTL
	BitCapacity  uint32 = 1 << OrganIDCapacity
	BitDStarLite uint32 = 1 << OrganIDDStarLite
	BitMCTS      uint32 = 1 << OrganIDMCTS
	BitPasport   uint32 = 1 << OrganIDPasport

	BaseRoutineMask uint32 = BitHDC | BitTsetlin | BitPasport
)

type HDCPrototype struct {
	Pattern  bus.HDCSlot
	ClassID  uint32
	BaseMask uint32
	MinSim   float32
}

type HDCTable struct {
	Version    uint64
	Prototypes []HDCPrototype
}

type HDCOrgan struct {
	table atomic.Pointer[HDCTable]
}

func NewHDCOrgan(protos []HDCPrototype) *HDCOrgan {
	o := &HDCOrgan{}
	o.table.Store(&HDCTable{Version: 1, Prototypes: protos})
	return o
}

func (o *HDCOrgan) UpdateTableRCU(next *HDCTable) {
	if next != nil {
		o.table.Store(next)
	}
}

func (o *HDCOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDHDC,
		Name:        "HDC",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 2 * time.Microsecond,
		HardTimeout: 10 * time.Microsecond,
	}
}

func (o *HDCOrgan) InBounds(view bus.TickView) bool { return view.HDC != nil }
func (o *HDCOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *HDCOrgan) CoolDown()                        {}

func (o *HDCOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDHDC, Status: bus.StatusOutOfBounds}, bus.OrganMetrics{Status: bus.StatusOutOfBounds}
	}
	tbl := o.table.Load()
	var bestSim float32
	var bestProto *HDCPrototype

	for i := range tbl.Prototypes {
		p := &tbl.Prototypes[i]
		var diffBits int
		for w := 0; w < bus.HDCWords; w++ {
			diffBits += bits.OnesCount64(view.HDC.Words[w] ^ p.Pattern.Words[w])
		}
		sim := 1.0 - float32(diffBits)/float32(bus.HDCWords*64)
		if sim > bestSim {
			bestSim = sim
			bestProto = p
		}
	}

	if bestProto == nil || bestSim < bestProto.MinSim {
		return bus.Verdict{
			OrganID:    OrganIDHDC,
			Status:     bus.StatusOutOfBounds,
			Confidence: bestSim,
		}, bus.OrganMetrics{Confidence: bestSim, Status: bus.StatusOutOfBounds}
	}

	return bus.Verdict{
		OrganID:       OrganIDHDC,
		Status:        bus.StatusOK,
		Confidence:    bestSim,
		ActionCode:    bestProto.ClassID,
		SuggestedMask: bestProto.BaseMask,
	}, bus.OrganMetrics{Confidence: bestSim, Status: bus.StatusOK}
}

type TsetlinClause struct {
	IncludeFlags bus.StateFlags
	ExcludeFlags bus.StateFlags
	MaxTTS       uint32
	AddMask      uint32
	DropMask     uint32
	ActionCode   uint32
	Weight       float32
}

type TsetlinOrgan struct {
	clauses atomic.Pointer[[]TsetlinClause]
}

func NewTsetlinOrgan(clauses []TsetlinClause) *TsetlinOrgan {
	o := &TsetlinOrgan{}
	cp := append([]TsetlinClause(nil), clauses...)
	o.clauses.Store(&cp)
	return o
}

func (o *TsetlinOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDTsetlin,
		Name:        "Tsetlin",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 2 * time.Microsecond,
		HardTimeout: 10 * time.Microsecond,
	}
}

func (o *TsetlinOrgan) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *TsetlinOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *TsetlinOrgan) CoolDown()                        {}

func (o *TsetlinOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDTsetlin, Status: bus.StatusOutOfBounds}, bus.OrganMetrics{Status: bus.StatusOutOfBounds}
	}
	flags := bus.StateFlags(view.Header.Flags)
	var addMask, dropMask, action uint32
	var bestWeight float32 = 0.85

	for _, c := range *o.clauses.Load() {
		if c.IncludeFlags != 0 && (flags&c.IncludeFlags) != c.IncludeFlags {
			continue
		}
		if c.ExcludeFlags != 0 && (flags&c.ExcludeFlags) != 0 {
			continue
		}
		if c.MaxTTS > 0 && view.Header.TTS > c.MaxTTS {
			continue
		}
		addMask |= c.AddMask
		dropMask |= c.DropMask
		if c.Weight >= bestWeight {
			bestWeight = c.Weight
			action = c.ActionCode
		}
	}

	return bus.Verdict{
		OrganID:       OrganIDTsetlin,
		Status:        bus.StatusOK,
		Confidence:    bestWeight,
		ActionCode:    action,
		SuggestedMask: addMask &^ dropMask,
	}, bus.OrganMetrics{Confidence: bestWeight, Status: bus.StatusOK}
}

type TTLOrgan struct{ MinSafeTTS uint32 }

func (o *TTLOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDTTL, Name: "TTL", Layer: bus.LayerFront, ReadLanes: bus.LaneMask(0).With(bus.LanePulse), WriteLane: bus.LanePulse, Residency: bus.ResidencyPinned}
}
func (o *TTLOrgan) InBounds(v bus.TickView) bool { return v.Header != nil }
func (o *TTLOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *TTLOrgan) CoolDown()                    {}
func (o *TTLOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if v.Header.TTS < o.MinSafeTTS {
		return bus.Verdict{OrganID: OrganIDTTL, Status: bus.StatusOK, Confidence: 0.90, SuggestedMask: BitDStarLite}, bus.OrganMetrics{Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDTTL, Status: bus.StatusOK, Confidence: 0.95}, bus.OrganMetrics{Status: bus.StatusOK}
}

type CapacityOrgan struct{ MaxEdgeLoad float32 }

func (o *CapacityOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDCapacity, Name: "Capacity", Layer: bus.LayerFront, ReadLanes: bus.LaneMask(0).With(bus.LaneGraph), WriteLane: bus.LanePulse, Residency: bus.ResidencyPinned}
}
func (o *CapacityOrgan) InBounds(v bus.TickView) bool { return true }
func (o *CapacityOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CapacityOrgan) CoolDown()                    {}
func (o *CapacityOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	for _, w := range v.Graph.EdgeWeights {
		if w > o.MaxEdgeLoad {
			return bus.Verdict{OrganID: OrganIDCapacity, Status: bus.StatusOutOfBounds, Confidence: 0.4}, bus.OrganMetrics{Status: bus.StatusOutOfBounds}
		}
	}
	return bus.Verdict{OrganID: OrganIDCapacity, Status: bus.StatusOK, Confidence: 0.95}, bus.OrganMetrics{Status: bus.StatusOK}
}

type DStarLiteOrgan struct{}

func (o *DStarLiteOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDDStarLite, Name: "DStarLite", Layer: bus.LayerFront, ReadLanes: bus.LaneMask(0).With(bus.LaneGraph), WriteLane: bus.LanePulse, Residency: bus.ResidencyPinned}
}
func (o *DStarLiteOrgan) InBounds(v bus.TickView) bool { return true }
func (o *DStarLiteOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *DStarLiteOrgan) CoolDown()                    {}
func (o *DStarLiteOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDDStarLite, Status: bus.StatusOK, Confidence: 0.92, ActionCode: 301}, bus.OrganMetrics{Status: bus.StatusOK}
}

type MCTSOrgan struct{ MaxRollouts int }

func (o *MCTSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDMCTS, Name: "MCTS", Layer: bus.LayerFront, ReadLanes: bus.LaneMask(0).With(bus.LaneGraph), WriteLane: bus.LanePulse, Residency: bus.ResidencyPinned}
}
func (o *MCTSOrgan) InBounds(v bus.TickView) bool { return true }
func (o *MCTSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *MCTSOrgan) CoolDown()                    {}
func (o *MCTSOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDMCTS, Status: bus.StatusOK, Confidence: 0.91, ActionCode: 302}, bus.OrganMetrics{Status: bus.StatusOK}
}

type PasportOrgan struct{}

func (o *PasportOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDPasport, Name: "Pasport", Layer: bus.LayerFront, ReadLanes: bus.LaneMask(0).With(bus.LanePulse), WriteLane: bus.LanePulse, Residency: bus.ResidencyPinned}
}
func (o *PasportOrgan) InBounds(v bus.TickView) bool { return v.Header != nil }
func (o *PasportOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *PasportOrgan) CoolDown()                    {}
func (o *PasportOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDPasport, Status: bus.StatusOK, Confidence: 0.99}, bus.OrganMetrics{Status: bus.StatusOK}
}
EOF

cat << 'EOF' > front/prefetch.go
package front

import (
	"sync"
	"sync/atomic"
	"time"

	"organism/bus"
)

type PrefetchConfig struct {
	DiskBandwidthMBps   int
	WindowDuration      time.Duration
	MaxTokensPerWindow  int
	CriticalTTSThresh   uint32
	MinPriorityOverride uint8
}

type GraphPrefetcher struct {
	cfg    PrefetchConfig
	region *bus.MmapRegion
	tokens atomic.Int64
	stopCh chan struct{}
	wg     sync.WaitGroup

	PrefetchedCount atomic.Uint64
	DroppedCount    atomic.Uint64
}

func NewGraphPrefetcher(cfg PrefetchConfig, region *bus.MmapRegion) *GraphPrefetcher {
	if cfg.MaxTokensPerWindow <= 0 {
		cfg.MaxTokensPerWindow = 4
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 10 * time.Millisecond
	}
	p := &GraphPrefetcher{
		cfg:    cfg,
		region: region,
		stopCh: make(chan struct{}),
	}
	p.tokens.Store(int64(cfg.MaxTokensPerWindow))
	p.wg.Add(1)
	go p.refillLoop()
	return p
}

func (p *GraphPrefetcher) refillLoop() {
	defer p.wg.Done()
	t := time.NewTicker(p.cfg.WindowDuration)
	defer t.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-t.C:
			p.tokens.Store(int64(p.cfg.MaxTokensPerWindow))
		}
	}
}

func (p *GraphPrefetcher) Request(offset, length int, tts uint32, priority uint8) bool {
	critical := (p.cfg.CriticalTTSThresh > 0 && tts <= p.cfg.CriticalTTSThresh) ||
		(p.cfg.MinPriorityOverride > 0 && priority >= p.cfg.MinPriorityOverride)
	if !critical {
		if p.tokens.Add(-1) < 0 {
			p.tokens.Add(1)
			p.DroppedCount.Add(1)
			return false
		}
	}
	if p.region != nil {
		_ = p.region.Prefetch(offset, length)
	}
	p.PrefetchedCount.Add(1)
	return true
}

func (p *GraphPrefetcher) Cancel(offset, length int) {
	if p.region != nil {
		_ = p.region.Evict(offset, length)
	}
}

func (p *GraphPrefetcher) Close() {
	close(p.stopCh)
	p.wg.Wait()
}
EOF

cat << 'EOF' > front/router.go
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
EOF

cat << 'EOF' > front/router_test.go
package front

import (
	"testing"
	"time"

	"organism/bus"
)

func TestFrontRouterZeroGCAndVetoMask(t *testing.T) {
	b, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   128,
		HDCRingSize:     128,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer b.Close()
	var pat bus.HDCSlot
	pat.Words[0] = 0x12345678
	hdc := NewHDCOrgan([]HDCPrototype{
		{Pattern: pat, ClassID: 7, BaseMask: BaseRoutineMask | BitDStarLite, MinSim: 0.9},
	})
	ts := NewTsetlinOrgan([]TsetlinClause{
		{IncludeFlags: bus.FlagReady, ActionCode: 55, Weight: 0.95},
	})
	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(ts)
	_ = reg.HotSwap(&DStarLiteOrgan{})
	_ = reg.HotSwap(&PasportOrgan{})
	r := NewRouter(RouterConfig{BaseLeaseDuration: 10 * time.Millisecond}, b, reg, nil, hdc, ts, nil)
	// Проверка Zero-GC на полном такте front.Router.Step
	allocs := testing.AllocsPerRun(500, func() {
		id := b.Publish(&pat, bus.NullOffset, bus.NullOffset, 100, 10)
		v, err := r.Step(id)
		if err != nil || v.Status != bus.StatusOK || v.ActionCode != 301 {
			t.Fatalf("unexpected Step result: err=%v, v=%+v", err, v)
		}
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated in front.Router.Step: got %f allocs/op", allocs)
	}
	// Проверка VetoMask (блокировка D* Lite -> остаётся действие Tsetlin = 55)
	r.UpdatePolicyRCU(&RouterPolicy{
		Version:   2,
		VetoRules: []VetoRule{{Mask: BitDStarLite, Level: VetoHard, ReasonCode: 99}},
	})
	id2 := b.Publish(&pat, bus.NullOffset, bus.NullOffset, 100, 10)
	v2, _ := r.Step(id2)
	if v2.ActionCode != 55 || r.VetoBlocked.Load() != 1 {
		t.Fatalf("expected VetoMask to block DStarLite, got action=%d, blocked=%d", v2.ActionCode, r.VetoBlocked.Load())
	}
}
EOF

echo "=== [ЧАСТЬ 2/4 ЗАВЕРШЕНА] Жду команду на Часть 3/4 (Этажи 2, 3, 4: polygon/, growth/, stratos/) ==="
