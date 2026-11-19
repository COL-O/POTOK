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
