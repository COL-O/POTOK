package polygon

import (
	"sync/atomic"
	"time"

	"organism/bus"
)

const (
	OrganIDMatrixProfile uint8 = 7
	OrganIDKAN           uint8 = 8
	OrganIDCfC           uint8 = 9
	OrganIDCBS           uint8 = 10
	OrganIDESN           uint8 = 11
	OrganIDPolyTsetlin   uint8 = 12
)

type KANRuleTable struct {
	Version   uint64
	MinVal    float32
	MaxVal    float32
	Knots     []float32
	CauseCode uint32
}

type KANOrgan struct {
	table atomic.Pointer[KANRuleTable]
}

func NewKANOrgan(t *KANRuleTable) *KANOrgan {
	o := &KANOrgan{}
	o.table.Store(t)
	return o
}

func (o *KANOrgan) UpdateTableRCU(next *KANRuleTable) {
	if next != nil {
		o.table.Store(next)
	}
}

func (o *KANOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDKAN, Name: "KAN", Layer: bus.LayerPolygon, ReadLanes: bus.LaneMask(0).With(bus.LaneTensor), WriteLane: bus.LanePulse}
}
func (o *KANOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *KANOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *KANOrgan) CoolDown()                    {}

func (o *KANOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(v) {
		return bus.Verdict{OrganID: OrganIDKAN, Status: bus.StatusAbstain}, bus.OrganMetrics{Status: bus.StatusAbstain}
	}
	tbl := o.table.Load()
	last := v.Tensor.Data[len(v.Tensor.Data)-1]
	// Domain Guard (КОН 10): честный отказ при выходе за паспорт применимости
	if last < tbl.MinVal || last > tbl.MaxVal {
		return bus.Verdict{
			OrganID:    OrganIDKAN,
			Status:     bus.StatusOutOfBounds,
			Confidence: 0.2,
			ActionCode: tbl.CauseCode,
		}, bus.OrganMetrics{Status: bus.StatusOutOfBounds}
	}
	norm := (last - tbl.MinVal) / (tbl.MaxVal - tbl.MinVal + 1e-6)
	return bus.Verdict{
		OrganID:    OrganIDKAN,
		Status:     bus.StatusOK,
		Confidence: 0.90,
		ActionCode: tbl.CauseCode,
	}, bus.OrganMetrics{Confidence: norm, Status: bus.StatusOK}
}

type MatrixProfileOrgan struct {
	WindowSize    int
	DiscordThresh float32
}

func (o *MatrixProfileOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDMatrixProfile, Name: "MatrixProfile", Layer: bus.LayerPolygon}
}
func (o *MatrixProfileOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) >= 4 }
func (o *MatrixProfileOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *MatrixProfileOrgan) CoolDown()                    {}
func (o *MatrixProfileOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDMatrixProfile, Status: bus.StatusOK, Confidence: 0.92}, bus.OrganMetrics{Status: bus.StatusOK}
}

type CfCOrgan struct {
	DecayTau    float32
	HorizonStep float32
}

func (o *CfCOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDCfC, Name: "CfC", Layer: bus.LayerPolygon}
}
func (o *CfCOrgan) InBounds(v bus.TickView) bool { return len(v.Tensor.Data) > 0 }
func (o *CfCOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CfCOrgan) CoolDown()                    {}
func (o *CfCOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDCfC, Status: bus.StatusOK, Confidence: 0.89}, bus.OrganMetrics{Status: bus.StatusOK}
}

type CBSOrgan struct{ MaxNodeCapacity float32 }

func (o *CBSOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDCBS, Name: "CBS", Layer: bus.LayerPolygon}
}
func (o *CBSOrgan) InBounds(v bus.TickView) bool { return true }
func (o *CBSOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *CBSOrgan) CoolDown()                    {}
func (o *CBSOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDCBS, Status: bus.StatusOK, Confidence: 0.91}, bus.OrganMetrics{Status: bus.StatusOK}
}

type ESNOrgan struct{ LeakRate float32 }

func (o *ESNOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDESN, Name: "ESN", Layer: bus.LayerPolygon}
}
func (o *ESNOrgan) InBounds(v bus.TickView) bool { return true }
func (o *ESNOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *ESNOrgan) CoolDown()                    {}
func (o *ESNOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	return bus.Verdict{OrganID: OrganIDESN, Status: bus.StatusOK, Confidence: 0.88}, bus.OrganMetrics{Status: bus.StatusOK}
}

type AntibodyRule struct {
	CauseCode      uint32
	AntibodyAction uint32
	ProposeMask    uint32
	Confidence     float32
}

type PolygonTsetlinOrgan struct {
	rules []AntibodyRule
}

func NewPolygonTsetlinOrgan(r []AntibodyRule) *PolygonTsetlinOrgan {
	return &PolygonTsetlinOrgan{rules: r}
}

func (o *PolygonTsetlinOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDPolyTsetlin, Name: "PolyTsetlin", Layer: bus.LayerPolygon}
}
func (o *PolygonTsetlinOrgan) InBounds(v bus.TickView) bool { return true }
func (o *PolygonTsetlinOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *PolygonTsetlinOrgan) CoolDown()                    {}
func (o *PolygonTsetlinOrgan) Process(v bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if len(o.rules) > 0 {
		r := o.rules[0]
		return bus.Verdict{
			OrganID:       OrganIDPolyTsetlin,
			Status:        bus.StatusOK,
			Confidence:    r.Confidence,
			ActionCode:    r.AntibodyAction,
			SuggestedMask: r.ProposeMask,
		}, bus.OrganMetrics{Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDPolyTsetlin, Status: bus.StatusOK, Confidence: 0.85, ActionCode: 800}, bus.OrganMetrics{Status: bus.StatusOK}
}

var _ = time.Second
