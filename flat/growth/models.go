package growth

import "organism/bus"

const (
	OrganIDGraphSAGE       uint8 = 13
	OrganIDActiveInference uint8 = 14
	OrganIDSINDy           uint8 = 15
	OrganIDLUTKAN          uint8 = 16
	OrganIDGrowthMCTS      uint8 = 17
	OrganIDTDMPC2          uint8 = 18
)

type GraphSAGEOrgan struct{ MaxDegreeChange float32 }

func (o *GraphSAGEOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDGraphSAGE, Name: "GraphSAGE", Layer: bus.LayerGrowth}
}
func (o *GraphSAGEOrgan) InBounds(v bus.TickView) bool { return true }
func (o *GraphSAGEOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GraphSAGEOrgan) CoolDown()                    {}
func (o *GraphSAGEOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDGraphSAGE, Status: bus.StatusOK, Confidence: 0.90}, bus.OrganMetrics{Status: bus.StatusOK}
}

type ActiveInferenceOrgan struct {
	ExpectedBaseline float32
	SurpriseThresh   float32
}

func (o *ActiveInferenceOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDActiveInference, Name: "ActiveInference", Layer: bus.LayerGrowth}
}
func (o *ActiveInferenceOrgan) InBounds(v bus.TickView) bool { return true }
func (o *ActiveInferenceOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *ActiveInferenceOrgan) CoolDown()                    {}
func (o *ActiveInferenceOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDActiveInference, Status: bus.StatusOK, Confidence: 0.91}, bus.OrganMetrics{Status: bus.StatusOK}
}

type SINDyOrgan struct{ MaxFitError float32 }

func (o *SINDyOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDSINDy, Name: "SINDy", Layer: bus.LayerGrowth}
}
func (o *SINDyOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *SINDyOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *SINDyOrgan) CoolDown()                    {}
func (o *SINDyOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDSINDy, Status: bus.StatusOK, Confidence: 0.93}, bus.OrganMetrics{Status: bus.StatusOK}
}

type LUTKANOrgan struct {
	Resolution int
	SafetyPad  float32
}

func (o *LUTKANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDLUTKAN, Name: "LUTKAN", Layer: bus.LayerGrowth}
}
func (o *LUTKANOrgan) InBounds(v bus.TickView) bool { return true }
func (o *LUTKANOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *LUTKANOrgan) CoolDown()                    {}
func (o *LUTKANOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDLUTKAN, Status: bus.StatusOK, Confidence: 0.94}, bus.OrganMetrics{Status: bus.StatusOK}
}

type GrowthMCTSOrgan struct {
	Simulations int
	Horizon     int
}

func (o *GrowthMCTSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDGrowthMCTS, Name: "GrowthMCTS", Layer: bus.LayerGrowth}
}
func (o *GrowthMCTSOrgan) InBounds(v bus.TickView) bool { return true }
func (o *GrowthMCTSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *GrowthMCTSOrgan) CoolDown()                    {}
func (o *GrowthMCTSOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDGrowthMCTS, Status: bus.StatusOK, Confidence: 0.90}, bus.OrganMetrics{Status: bus.StatusOK}
}

type TDMPC2Organ struct{ BlendSteps int }

func (o *TDMPC2Organ) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDTDMPC2, Name: "TDMPC2", Layer: bus.LayerGrowth}
}
func (o *TDMPC2Organ) InBounds(v bus.TickView) bool { return true }
func (o *TDMPC2Organ) Warmup(*bus.MmapRegion) error { return nil }
func (o *TDMPC2Organ) CoolDown()                    {}
func (o *TDMPC2Organ) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDTDMPC2, Status: bus.StatusOK, Confidence: 0.92}, bus.OrganMetrics{Status: bus.StatusOK}
}
