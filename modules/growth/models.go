package growth

import (
	"math"
	"time"
	"unsafe"

	"github.com/COL-O/POTOK/modules/bus"
)

const (
	OrganIDGraphSAGE       uint8 = 13
	OrganIDActiveInference uint8 = 14
	OrganIDSINDy           uint8 = 15
	OrganIDLUTKAN          uint8 = 16
	OrganIDGrowthMCTS      uint8 = 17
	OrganIDTDMPC2          uint8 = 18

	BitGraphSAGE       uint32 = 1 << OrganIDGraphSAGE
	BitActiveInference uint32 = 1 << OrganIDActiveInference
	BitSINDy           uint32 = 1 << OrganIDSINDy
	BitLUTKAN          uint32 = 1 << OrganIDLUTKAN
	BitGrowthMCTS      uint32 = 1 << OrganIDGrowthMCTS
	BitTDMPC2          uint32 = 1 << OrganIDTDMPC2
)

type GraphSAGEOrgan struct {
	MaxDegreeChange float32
}

func (o *GraphSAGEOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDGraphSAGE,
		Name:        "GraphSAGE",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneGraph),
		WriteLane:   bus.LaneGraph,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 200 * time.Millisecond,
		HardTimeout: 2 * time.Second,
	}
}

func (o *GraphSAGEOrgan) InBounds(view bus.TickView) bool {
	return view.Graph.NumNodes >= 2 && len(view.Graph.RowOffsets) >= int(view.Graph.NumNodes)+1
}
func (o *GraphSAGEOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GraphSAGEOrgan) CoolDown()                    {}

func (o *GraphSAGEOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDGraphSAGE, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	var maxEmbed float32
	var bottleneckNode uint32
	for node := uint32(0); node < view.Graph.NumNodes; node++ {
		s := view.Graph.RowOffsets[node]
		e := view.Graph.RowOffsets[node+1]
		deg := float32(e - s)
		if deg == 0 {
			continue
		}
		var sumWeight float32
		for idx := s; idx < e; idx++ {
			sumWeight += view.Graph.EdgeWeights[idx]
		}
		embed := (sumWeight / deg) * float32(math.Log1p(float64(deg)))
		if embed > maxEmbed {
			maxEmbed = embed
			bottleneckNode = node
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDGraphSAGE, Status: bus.StatusOK, Confidence: 0.90, ActionCode: bottleneckNode, SuggestedMask: BitActiveInference},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.90, Status: bus.StatusOK}
}

type ActiveInferenceOrgan struct {
	ExpectedBaseline float32
	SurpriseThresh   float32
}

func (o *ActiveInferenceOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDActiveInference,
		Name:        "ActiveInference",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 200 * time.Millisecond,
		HardTimeout: 2 * time.Second,
	}
}

func (o *ActiveInferenceOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) >= 4 }
func (o *ActiveInferenceOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *ActiveInferenceOrgan) CoolDown()                        {}

func (o *ActiveInferenceOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDActiveInference, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	var freeEnergy float32
	for _, v := range view.Tensor.Data {
		diff := v - o.ExpectedBaseline
		freeEnergy += diff * diff
	}
	freeEnergy = float32(math.Sqrt(float64(freeEnergy / float32(len(view.Tensor.Data)))))
	lat := uint64(time.Now().UnixNano() - start)
	action := uint32(1)
	nextMask := BitSINDy | BitLUTKAN
	if freeEnergy > o.SurpriseThresh*2.0 {
		action = 2
		nextMask |= BitGrowthMCTS
	}
	conf := 1.0 / (1.0 + freeEnergy)
	if conf < 0.75 {
		conf = 0.75
	}
	return bus.Verdict{OrganID: OrganIDActiveInference, Status: bus.StatusOK, Confidence: conf, ActionCode: action, SuggestedMask: nextMask},
		bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
}

type SINDyEquation struct {
	C0         float32
	C1         float32
	C2         float32
	FitError   float32
	EquationID uint64
}

type SINDyOrgan struct {
	MaxFitError float32
}

func (o *SINDyOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDSINDy,
		Name:        "SINDy",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneTensor),
		WriteLane:   bus.LaneTensor,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 500 * time.Millisecond,
		HardTimeout: 5 * time.Second,
	}
}

func (o *SINDyOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) >= 6 }
func (o *SINDyOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *SINDyOrgan) CoolDown()                        {}

func (o *SINDyOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDSINDy, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	series := view.Tensor.Data
	n := len(series)
	var sumSlope, sumX, sumXSlope, sumX2 float32
	for i := 1; i < n; i++ {
		dx := series[i] - series[i-1]
		x := series[i-1]
		sumSlope += dx
		sumX += x
		sumXSlope += x * dx
		sumX2 += x * x
	}
	steps := float32(n - 1)
	denom := steps*sumX2 - sumX*sumX
	var c0, c1 float32
	if math.Abs(float64(denom)) > 1e-6 {
		c1 = (steps*sumXSlope - sumX*sumSlope) / denom
		c0 = (sumSlope - c1*sumX) / steps
	} else {
		c0 = sumSlope / steps
	}
	var mse float32
	for i := 1; i < n; i++ {
		predDx := c0 + c1*series[i-1]
		actualDx := series[i] - series[i-1]
		err := actualDx - predDx
		mse += err * err
	}
	rmse := float32(math.Sqrt(float64(mse / steps)))
	if len(view.Scratch) >= int(unsafe.Sizeof(SINDyEquation{})) {
		eq := (*SINDyEquation)(unsafe.Pointer(&view.Scratch[0]))
		eq.C0 = c0
		eq.C1 = c1
		eq.C2 = 0
		eq.FitError = rmse
		eq.EquationID = view.EventID
	}
	lat := uint64(time.Now().UnixNano() - start)
	if o.MaxFitError > 0 && rmse > o.MaxFitError {
		return bus.Verdict{OrganID: OrganIDSINDy, Status: bus.StatusOutOfBounds, Confidence: 0.20},
			bus.OrganMetrics{LatencyNs: lat, Confidence: 0.20, Status: bus.StatusOutOfBounds}
	}
	conf := 1.0 / (1.0 + rmse)
	return bus.Verdict{OrganID: OrganIDSINDy, Status: bus.StatusOK, Confidence: conf, ActionCode: uint32(view.EventID), SuggestedMask: BitLUTKAN},
		bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
}

type CompiledLUT struct {
	MinVal      float32
	MaxVal      float32
	Knots       []float32
	EquationID  uint64
	SafetyScore float32
}

type LUTKANOrgan struct {
	Resolution int
	SafetyPad  float32
}

func (o *LUTKANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDLUTKAN,
		Name:        "LUT-KAN",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneTensor),
		WriteLane:   bus.LaneTensor,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 200 * time.Millisecond,
		HardTimeout: 2 * time.Second,
	}
}

func (o *LUTKANOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) >= 2 }
func (o *LUTKANOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *LUTKANOrgan) CoolDown()                        {}

func (o *LUTKANOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDLUTKAN, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	minV, maxV := view.Tensor.Data[0], view.Tensor.Data[0]
	for _, v := range view.Tensor.Data {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	span := maxV - minV
	if span < 1e-3 {
		span = 1.0
	}
	pad := o.SafetyPad
	if pad <= 0 {
		pad = 0.2
	}
	guardMin := minV - span*pad
	guardMax := maxV + span*pad
	var c0, c1 float32 = 0.1, 0.5
	if len(view.Scratch) >= int(unsafe.Sizeof(SINDyEquation{})) {
		eq := (*SINDyEquation)(unsafe.Pointer(&view.Scratch[0]))
		c0, c1 = eq.C0, eq.C1
	}
	res := o.Resolution
	if res <= 1 {
		res = 8
	}
	neededBytes := (res + 2) * 4
	if len(view.Scratch) >= neededBytes {
		out := unsafe.Slice((*float32)(unsafe.Pointer(&view.Scratch[0])), res+2)
		out[0] = guardMin
		out[1] = guardMax
		for i := 0; i < res; i++ {
			x := guardMin + (guardMax-guardMin)*float32(i)/float32(res-1)
			val := float32(math.Abs(float64(c0 + c1*x)))
			if val > 1.0 {
				val = 1.0
			}
			out[i+2] = val
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDLUTKAN, Status: bus.StatusOK, Confidence: 0.93, ActionCode: uint32(res), SuggestedMask: BitGrowthMCTS},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.93, Status: bus.StatusOK}
}

type GrowthMCTSOrgan struct {
	Simulations int
	Horizon     int
}

func (o *GrowthMCTSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDGrowthMCTS,
		Name:        "GrowthMCTS",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneGraph),
		WriteLane:   bus.LaneGraph,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 1 * time.Second,
		HardTimeout: 10 * time.Second,
	}
}

func (o *GrowthMCTSOrgan) InBounds(view bus.TickView) bool {
	return view.Graph.NumNodes >= 2 && len(view.Graph.RowOffsets) >= int(view.Graph.NumNodes)+1
}
func (o *GrowthMCTSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GrowthMCTSOrgan) CoolDown()                    {}

func (o *GrowthMCTSOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDGrowthMCTS, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	bestStartNode := uint32(0)
	bestCost := float32(math.MaxFloat32)
	for startNode := uint32(0); startNode < view.Graph.NumNodes; startNode++ {
		cur := startNode
		var totalCost float32
		for step := 0; step < o.Horizon; step++ {
			s := view.Graph.RowOffsets[cur]
			e := view.Graph.RowOffsets[cur+1]
			if s == e {
				totalCost += 1000.0
				break
			}
			minW := view.Graph.EdgeWeights[s]
			nextNode := view.Graph.ColIndices[s]
			for idx := s + 1; idx < e; idx++ {
				if view.Graph.EdgeWeights[idx] < minW {
					minW = view.Graph.EdgeWeights[idx]
					nextNode = view.Graph.ColIndices[idx]
				}
			}
			totalCost += minW
			cur = nextNode
		}
		if totalCost < bestCost {
			bestCost = totalCost
			bestStartNode = startNode
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDGrowthMCTS, Status: bus.StatusOK, Confidence: 0.91, ActionCode: bestStartNode, SuggestedMask: BitTDMPC2},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.91, Status: bus.StatusOK}
}

type TDMPC2Organ struct {
	BlendSteps int
}

func (o *TDMPC2Organ) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDTDMPC2,
		Name:        "TD-MPC2",
		Layer:       bus.LayerGrowth,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 500 * time.Millisecond,
		HardTimeout: 5 * time.Second,
	}
}

func (o *TDMPC2Organ) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *TDMPC2Organ) Warmup(*bus.MmapRegion) error     { return nil }
func (o *TDMPC2Organ) CoolDown()                        {}

func (o *TDMPC2Organ) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	steps := o.BlendSteps
	if steps <= 0 {
		steps = 4
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDTDMPC2, Status: bus.StatusOK, Confidence: 0.94, ActionCode: uint32(steps)},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.94, Status: bus.StatusOK}
}
