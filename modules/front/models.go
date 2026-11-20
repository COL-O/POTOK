package front

import (
	"math/bits"
	"sync/atomic"
	"time"

	"organism/modules/bus"
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

// HDCPrototype — эталонный образ ситуации в таблице RCU.
type HDCPrototype struct {
	Pattern     bus.HDCSlot
	ClassID     uint32
	BaseMask    uint32
	MinSim      float32
	GenealogyID uint64
}

type HDCTable struct {
	Version    uint64
	Prototypes []HDCPrototype
}

type HDCOrgan struct {
	table atomic.Pointer[HDCTable]
}

func NewHDCOrgan(initial []HDCPrototype) *HDCOrgan {
	o := &HDCOrgan{}
	o.table.Store(&HDCTable{Version: 1, Prototypes: initial})
	return o
}

func (o *HDCOrgan) UpdateTableRCU(next *HDCTable) { o.table.Store(next) }

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
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDHDC, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	tbl := o.table.Load()
	const totalBits = float32(bus.HDCWords * 64)
	var bestSim float32
	var bestProto *HDCPrototype
	for i := range tbl.Prototypes {
		p := &tbl.Prototypes[i]
		var diffBits int
		for w := 0; w < bus.HDCWords; w++ {
			diffBits += bits.OnesCount64(view.HDC.Words[w] ^ p.Pattern.Words[w])
		}
		sim := 1.0 - (float32(diffBits) / totalBits)
		if sim > bestSim {
			bestSim = sim
			bestProto = p
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if bestProto == nil || bestSim < bestProto.MinSim {
		return bus.Verdict{OrganID: OrganIDHDC, Status: bus.StatusOutOfBounds, Confidence: bestSim},
			bus.OrganMetrics{LatencyNs: lat, Confidence: bestSim, Status: bus.StatusOutOfBounds}
	}
	return bus.Verdict{
		OrganID:       OrganIDHDC,
		Status:        bus.StatusOK,
		Confidence:    bestSim,
		ActionCode:    bestProto.ClassID,
		SuggestedMask: bestProto.BaseMask,
	}, bus.OrganMetrics{LatencyNs: lat, Confidence: bestSim, Status: bus.StatusOK}
}

// TsetlinClause — булева клауза.
type TsetlinClause struct {
	IncludeFlags uint32
	ExcludeFlags uint32
	MaxTTS       uint32
	AddMask      uint32
	DropMask     uint32
	ActionCode   uint32
	Weight       float32
}

type TsetlinTable struct {
	Version uint64
	Clauses []TsetlinClause
}

type TsetlinOrgan struct {
	table atomic.Pointer[TsetlinTable]
}

func NewTsetlinOrgan(clauses []TsetlinClause) *TsetlinOrgan {
	o := &TsetlinOrgan{}
	o.table.Store(&TsetlinTable{Version: 1, Clauses: clauses})
	return o
}

func (o *TsetlinOrgan) UpdateTableRCU(next *TsetlinTable) { o.table.Store(next) }

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
	start := time.Now().UnixNano()
	flags := atomic.LoadUint32(&view.Header.StateFlags)
	tts := view.Header.TTS
	tbl := o.table.Load()
	mask := BaseRoutineMask
	var matched int
	var maxWeight float32
	var action uint32
	for i := range tbl.Clauses {
		c := &tbl.Clauses[i]
		if (flags&c.IncludeFlags) == c.IncludeFlags && (flags&c.ExcludeFlags) == 0 {
			if c.MaxTTS == 0 || tts <= c.MaxTTS {
				mask = (mask | c.AddMask) &^ c.DropMask
				matched++
				if c.Weight > maxWeight {
					maxWeight = c.Weight
					action = c.ActionCode
				}
			}
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if matched == 0 {
		return bus.Verdict{OrganID: OrganIDTsetlin, Status: bus.StatusAbstain, Confidence: 0.0},
			bus.OrganMetrics{LatencyNs: lat, Status: bus.StatusAbstain}
	}
	return bus.Verdict{
		OrganID:       OrganIDTsetlin,
		Status:        bus.StatusOK,
		Confidence:    maxWeight,
		ActionCode:    action,
		SuggestedMask: mask,
	}, bus.OrganMetrics{LatencyNs: lat, Confidence: maxWeight, Status: bus.StatusOK}
}

// TTLOrgan — контроль бюджета времени жизни импульса.
type TTLOrgan struct {
	MinSafeTTS uint32
}

func (o *TTLOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDTTL,
		Name:        "TTL",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 1 * time.Microsecond,
		HardTimeout: 50 * time.Microsecond,
	}
}

func (o *TTLOrgan) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *TTLOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *TTLOrgan) CoolDown()                        {}

func (o *TTLOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	tts := view.Header.TTS
	lat := uint64(time.Now().UnixNano() - start)
	if tts == 0 {
		return bus.Verdict{OrganID: OrganIDTTL, Status: bus.StatusOutOfBounds, Confidence: 0.0},
			bus.OrganMetrics{LatencyNs: lat, Status: bus.StatusOutOfBounds}
	}
	if tts < o.MinSafeTTS {
		return bus.Verdict{OrganID: OrganIDTTL, Status: bus.StatusOK, Confidence: 0.70, SuggestedMask: BitDStarLite},
			bus.OrganMetrics{LatencyNs: lat, Confidence: 0.70, Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDTTL, Status: bus.StatusOK, Confidence: 0.99},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.99, Status: bus.StatusOK}
}

// CapacityOrgan — проверка пропускной способности узлов.
type CapacityOrgan struct {
	MaxEdgeLoad float32
}

func (o *CapacityOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDCapacity,
		Name:        "Capacity",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneGraph),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 3 * time.Microsecond,
		HardTimeout: 15 * time.Microsecond,
	}
}

func (o *CapacityOrgan) InBounds(view bus.TickView) bool { return view.Graph.NumNodes > 0 }
func (o *CapacityOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *CapacityOrgan) CoolDown()                        {}

func (o *CapacityOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDCapacity, Status: bus.StatusAbstain},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusAbstain}
	}
	var peak float32
	for _, w := range view.Graph.EdgeWeights {
		if w > peak {
			peak = w
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if peak > o.MaxEdgeLoad {
		return bus.Verdict{OrganID: OrganIDCapacity, Status: bus.StatusOK, Confidence: 0.65, SuggestedMask: BitDStarLite},
			bus.OrganMetrics{LatencyNs: lat, Confidence: 0.65, Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDCapacity, Status: bus.StatusOK, Confidence: 0.95},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.95, Status: bus.StatusOK}
}

// DStarLiteOrgan — локальный графовый обход завала.
type DStarLiteOrgan struct{}

func (o *DStarLiteOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDDStarLite,
		Name:        "DStarLite",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneGraph),
		WriteLane:   bus.LaneGraph,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 5 * time.Microsecond,
		HardTimeout: 25 * time.Microsecond,
	}
}

func (o *DStarLiteOrgan) InBounds(view bus.TickView) bool {
	return view.Graph.NumNodes >= 2 && view.Graph.NumEdges > 0
}
func (o *DStarLiteOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *DStarLiteOrgan) CoolDown()                    {}

func (o *DStarLiteOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDDStarLite, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	endIdx := view.Graph.RowOffsets[1]
	if endIdx == 0 {
		return bus.Verdict{
			OrganID:       OrganIDDStarLite,
			Status:        bus.StatusOutOfBounds,
			Confidence:    0.20,
			SuggestedMask: BitMCTS,
		}, bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Confidence: 0.20, Status: bus.StatusOutOfBounds}
	}
	bestEdge := uint32(0)
	bestWeight := view.Graph.EdgeWeights[0]
	for i := uint32(1); i < endIdx; i++ {
		if view.Graph.EdgeWeights[i] < bestWeight {
			bestWeight = view.Graph.EdgeWeights[i]
			bestEdge = view.Graph.ColIndices[i]
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDDStarLite, Status: bus.StatusOK, Confidence: 0.90, ActionCode: bestEdge},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.90, Status: bus.StatusOK}
}

// MCTSOrgan — быстрая разведка веток при развилке.
type MCTSOrgan struct {
	MaxRollouts int
}

func (o *MCTSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDMCTS,
		Name:        "MCTS",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneGraph),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 10 * time.Microsecond,
		HardTimeout: 50 * time.Microsecond,
	}
}

func (o *MCTSOrgan) InBounds(view bus.TickView) bool { return view.Graph.NumNodes > 0 }
func (o *MCTSOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *MCTSOrgan) CoolDown()                        {}

func (o *MCTSOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDMCTS, Status: bus.StatusAbstain},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusAbstain}
	}
	rollouts := o.MaxRollouts
	if rollouts <= 0 {
		rollouts = 16
	}
	reachable := uint32(0)
	for step := 0; step < rollouts && reachable < view.Graph.NumNodes; step++ {
		if view.Graph.RowOffsets[reachable+1] > view.Graph.RowOffsets[reachable] {
			reachable++
		} else {
			break
		}
	}
	conf := float32(reachable) / float32(view.Graph.NumNodes)
	if conf > 1.0 {
		conf = 1.0
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDMCTS, Status: bus.StatusOK, Confidence: conf, ActionCode: reachable},
		bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
}

// PasportOrgan — фиксация факта такта.
type PasportOrgan struct {
	Committed atomic.Uint64
}

func (o *PasportOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDPasport,
		Name:        "Pasport",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse),
		WriteLane:   bus.LaneMeta,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 1 * time.Microsecond,
		HardTimeout: 50 * time.Microsecond,
	}
}

func (o *PasportOrgan) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *PasportOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *PasportOrgan) CoolDown()                        {}

func (o *PasportOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	o.Committed.Add(1)
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDPasport, Status: bus.StatusOK, Confidence: 1.0, ActionCode: uint32(view.EventID)},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 1.0, Status: bus.StatusOK}
}
