package polygon

import (
	"math"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/COL-O/POTOK/modules/bus"
)

const (
	OrganIDMatrixProfile uint8 = 7
	OrganIDKAN           uint8 = 8
	OrganIDCfC           uint8 = 9
	OrganIDCBS           uint8 = 10
	OrganIDESN           uint8 = 11
	OrganIDPolyTsetlin   uint8 = 12

	BitMatrixProfile uint32 = 1 << OrganIDMatrixProfile
	BitKAN           uint32 = 1 << OrganIDKAN
	BitCfC           uint32 = 1 << OrganIDCfC
	BitCBS           uint32 = 1 << OrganIDCBS
	BitESN           uint32 = 1 << OrganIDESN
	BitPolyTsetlin   uint32 = 1 << OrganIDPolyTsetlin
)

// MatrixProfileOrgan — поиск аномалий во временных рядах.
type MatrixProfileOrgan struct {
	WindowSize    int
	DiscordThresh float32
}

func (o *MatrixProfileOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDMatrixProfile,
		Name:        "MatrixProfile",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 10 * time.Millisecond,
		HardTimeout: 50 * time.Millisecond,
	}
}

func (o *MatrixProfileOrgan) InBounds(view bus.TickView) bool {
	w := o.WindowSize
	if w <= 0 {
		w = 8
	}
	return len(view.Tensor.Data) >= w*2
}
func (o *MatrixProfileOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *MatrixProfileOrgan) CoolDown()                    {}

func (o *MatrixProfileOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	w := o.WindowSize
	if w <= 0 {
		w = 8
	}
	series := view.Tensor.Data
	n := len(series) - w + 1
	lastIdx := n - 1
	minDistSq := float32(math.MaxFloat32)
	for i := 0; i < n-w; i++ {
		var distSq float32
		for k := 0; k < w; k++ {
			d := series[lastIdx+k] - series[i+k]
			distSq += d * d
		}
		if distSq < minDistSq {
			minDistSq = distSq
		}
	}
	dist := float32(math.Sqrt(float64(minDistSq)))
	lat := uint64(time.Now().UnixNano() - start)
	if dist >= o.DiscordThresh {
		conf := dist / (dist + o.DiscordThresh)
		if conf < 0.80 {
			conf = 0.80
		}
		return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusOK, Confidence: conf, ActionCode: 1, SuggestedMask: BitKAN},
			bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusAbstain, Confidence: 0.20, ActionCode: 0},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.20, Status: bus.StatusAbstain}
}

// KANRuleTable — RCU-таблица сплайновых коэффициентов и паспорта границ.
type KANRuleTable struct {
	Version     uint64
	GenealogyID uint64
	MinVal      float32
	MaxVal      float32
	Knots       []float32
	CauseCode   uint32
}

type KANOrgan struct {
	table atomic.Pointer[KANRuleTable]
}

func NewKANOrgan(initial *KANRuleTable) *KANOrgan {
	o := &KANOrgan{}
	o.table.Store(initial)
	return o
}

func (o *KANOrgan) UpdateTableRCU(next *KANRuleTable) { o.table.Store(next) }

func (o *KANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDKAN,
		Name:        "KAN",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 15 * time.Millisecond,
		HardTimeout: 60 * time.Millisecond,
	}
}

func (o *KANOrgan) InBounds(view bus.TickView) bool {
	tbl := o.table.Load()
	if tbl == nil || len(tbl.Knots) < 2 || len(view.Tensor.Data) == 0 {
		return false
	}
	x := view.Tensor.Data[len(view.Tensor.Data)-1]
	return x >= tbl.MinVal && x <= tbl.MaxVal
}
func (o *KANOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *KANOrgan) CoolDown()                    {}

func (o *KANOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDKAN, Status: bus.StatusOutOfBounds, Confidence: 0.0},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	tbl := o.table.Load()
	x := view.Tensor.Data[len(view.Tensor.Data)-1]
	norm := (x - tbl.MinVal) / (tbl.MaxVal - tbl.MinVal)
	scaled := norm * float32(len(tbl.Knots)-1)
	idx := int(scaled)
	if idx >= len(tbl.Knots)-1 {
		idx = len(tbl.Knots) - 2
	}
	frac := scaled - float32(idx)
	severity := tbl.Knots[idx]*(1.0-frac) + tbl.Knots[idx+1]*frac
	conf := severity
	if conf > 1.0 {
		conf = 1.0
	}
	if conf < 0.0 {
		conf = 0.0
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDKAN, Status: bus.StatusOK, Confidence: conf, ActionCode: tbl.CauseCode, SuggestedMask: BitCfC},
		bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
}

// CfCOrgan — прогноз развития сбоя во времени.
type CfCOrgan struct {
	DecayTau    float32
	HorizonStep float32
}

func (o *CfCOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDCfC,
		Name:        "CfC",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 10 * time.Millisecond,
		HardTimeout: 50 * time.Millisecond,
	}
}

func (o *CfCOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) >= 2 }
func (o *CfCOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *CfCOrgan) CoolDown()                        {}

func (o *CfCOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDCfC, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	n := len(view.Tensor.Data)
	curr := view.Tensor.Data[n-1]
	prev := view.Tensor.Data[n-2]
	velocity := curr - prev
	tau := o.DecayTau
	if tau <= 0 {
		tau = 1.0
	}
	dt := o.HorizonStep
	if dt <= 0 {
		dt = 1.0
	}
	decay := float32(math.Exp(float64(-dt / tau)))
	predicted := curr*decay + velocity*dt
	var nextMask uint32 = BitESN
	var action uint32 = 1
	if predicted < 0 || predicted > curr*1.5 {
		nextMask |= BitCBS
		action = 2
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDCfC, Status: bus.StatusOK, Confidence: 0.88, ActionCode: action, SuggestedMask: nextMask},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.88, Status: bus.StatusOK}
}

// CBSOrgan — развод конфликтов ресурсов на графе.
type CBSOrgan struct {
	MaxNodeCapacity float32
}

func (o *CBSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDCBS,
		Name:        "CBS",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneGraph),
		WriteLane:   bus.LaneGraph,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 25 * time.Millisecond,
		HardTimeout: 100 * time.Millisecond,
	}
}

func (o *CBSOrgan) InBounds(view bus.TickView) bool {
	return view.Graph.NumNodes >= 2 && len(view.Graph.RowOffsets) >= int(view.Graph.NumNodes)+1
}
func (o *CBSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CBSOrgan) CoolDown()                    {}

func (o *CBSOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDCBS, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	bestNode := uint32(0)
	minLoad := float32(math.MaxFloat32)
	for node := uint32(0); node < view.Graph.NumNodes; node++ {
		s := view.Graph.RowOffsets[node]
		e := view.Graph.RowOffsets[node+1]
		var load float32
		for idx := s; idx < e; idx++ {
			load += view.Graph.EdgeWeights[idx]
		}
		if load < minLoad {
			minLoad = load
			bestNode = node
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if o.MaxNodeCapacity > 0 && minLoad > o.MaxNodeCapacity {
		return bus.Verdict{OrganID: OrganIDCBS, Status: bus.StatusOutOfBounds, Confidence: 0.15},
			bus.OrganMetrics{LatencyNs: lat, Confidence: 0.15, Status: bus.StatusOutOfBounds}
	}
	return bus.Verdict{OrganID: OrganIDCBS, Status: bus.StatusOK, Confidence: 0.91, ActionCode: bestNode, SuggestedMask: BitPolyTsetlin},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.91, Status: bus.StatusOK}
}

// ESNOrgan — прогноз ритма и пиков загрузки.
type ESNOrgan struct {
	LeakRate float32
}

func (o *ESNOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDESN,
		Name:        "ESN",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 15 * time.Millisecond,
		HardTimeout: 60 * time.Millisecond,
	}
}

func (o *ESNOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) > 0 }
func (o *ESNOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *ESNOrgan) CoolDown()                        {}

func (o *ESNOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDESN, Status: bus.StatusAbstain},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusAbstain}
	}
	alpha := o.LeakRate
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}
	var state float32
	for _, v := range view.Tensor.Data {
		state = (1.0-alpha)*state + alpha*float32(math.Tanh(float64(v)))
	}
	lat := uint64(time.Now().UnixNano() - start)
	action := uint32(1)
	if state > 0.7 {
		action = 2
	}
	return bus.Verdict{OrganID: OrganIDESN, Status: bus.StatusOK, Confidence: 0.85, ActionCode: action, SuggestedMask: BitPolyTsetlin},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.85, Status: bus.StatusOK}
}

// AntibodyRule — правило синтеза антитела.
type AntibodyRule struct {
	CauseCode      uint32
	TrendCode      uint32
	AntibodyAction uint32
	ProposeMask    uint32
	VetoMask       uint32
	VetoLevel      uint8
	Confidence     float32
}

type PolygonTsetlinOrgan struct {
	rules atomic.Pointer[[]AntibodyRule]
}

func NewPolygonTsetlinOrgan(initial []AntibodyRule) *PolygonTsetlinOrgan {
	o := &PolygonTsetlinOrgan{}
	o.rules.Store(&initial)
	return o
}

func (o *PolygonTsetlinOrgan) UpdateRulesRCU(next []AntibodyRule) { o.rules.Store(&next) }

func (o *PolygonTsetlinOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDPolyTsetlin,
		Name:        "PolygonTsetlin",
		Layer:       bus.LayerPolygon,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 5 * time.Millisecond,
		HardTimeout: 20 * time.Millisecond,
	}
}

func (o *PolygonTsetlinOrgan) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *PolygonTsetlinOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *PolygonTsetlinOrgan) CoolDown()                        {}

func (o *PolygonTsetlinOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	rules := *o.rules.Load()
	var causeCode, trendCode uint32
	if len(view.Scratch) >= 8 {
		data := unsafe.Slice((*uint32)(unsafe.Pointer(&view.Scratch[0])), 2)
		causeCode = data[0]
		trendCode = data[1]
	}
	for i := range rules {
		r := &rules[i]
		if r.CauseCode == causeCode && (r.TrendCode == 0 || r.TrendCode == trendCode) {
			lat := uint64(time.Now().UnixNano() - start)
			return bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOK, Confidence: r.Confidence, ActionCode: r.AntibodyAction, SuggestedMask: r.ProposeMask},
				bus.OrganMetrics{LatencyNs: lat, Confidence: r.Confidence, Status: bus.StatusOK}
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusAbstain, Confidence: 0.0},
		bus.OrganMetrics{LatencyNs: lat, Status: bus.StatusAbstain}
}
