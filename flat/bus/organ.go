package bus

import (
	"sync"
	"sync/atomic"
	"time"
)

type OrganSpec struct {
	ID          uint8
	Name        string
	Layer       LayerID
	ReadLanes   LaneMask
	WriteLane   LaneID
	Residency   ResidencyTier
	SoftTimeout time.Duration
	HardTimeout time.Duration
}

type Organ interface {
	Spec() OrganSpec
	InBounds(view TickView) bool
	Warmup(region *MmapRegion) error
	Process(view TickView) (Verdict, OrganMetrics)
	CoolDown()
}

type organTable [MaxActiveOrgans]Organ

type OrganRegistry struct {
	mu    sync.Mutex
	table atomic.Pointer[organTable]
}

func NewOrganRegistry() *OrganRegistry {
	r := &OrganRegistry{}
	var empty organTable
	r.table.Store(&empty)
	return r
}

func (r *OrganRegistry) HotSwap(o Organ) error {
	if o == nil {
		return ErrInvalidOrganID
	}
	spec := o.Spec()
	if spec.ID >= MaxActiveOrgans {
		return ErrInvalidOrganID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.table.Load()
	next := *cur
	next[spec.ID] = o
	r.table.Store(&next)
	return nil
}

func (r *OrganRegistry) Get(id uint8) (Organ, bool) {
	if id >= MaxActiveOrgans {
		return nil, false
	}
	o := r.table.Load()[id]
	return o, o != nil
}
