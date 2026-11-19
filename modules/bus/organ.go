package bus

import (
	"sync/atomic"
	"time"
)

const MaxActiveOrgans = 32

// ResidencyPolicy — политика удержания весов органа в памяти.
type ResidencyPolicy uint8

const (
	ResidencyPinned   ResidencyPolicy = 1
	ResidencyWarmMmap ResidencyPolicy = 2
	ResidencyColdMmap ResidencyPolicy = 3
)

// OrganSpec — статический паспорт органа при регистрации.
type OrganSpec struct {
	ID          uint8
	Name        string
	Layer       LayerID
	ReadLanes   LaneMask
	WriteLane   LaneID
	Residency   ResidencyPolicy
	SoftTimeout time.Duration
	HardTimeout time.Duration
}

// Organ — единый контракт подключения любой модели к Шине.
type Organ interface {
	Spec() OrganSpec
	InBounds(view TickView) bool
	Warmup(mem *MmapRegion) error
	Process(view TickView) (Verdict, OrganMetrics)
	CoolDown()
}

type organTable struct {
	slots   [MaxActiveOrgans]Organ
	version uint64
}

// OrganRegistry — реестр органов с поддержкой атомарной горячей подмены.
type OrganRegistry struct {
	current atomic.Pointer[organTable]
}

func NewOrganRegistry() *OrganRegistry {
	reg := &OrganRegistry{}
	reg.current.Store(&organTable{version: 1})
	return reg
}

func (r *OrganRegistry) Get(id uint8) (Organ, bool) {
	if id >= MaxActiveOrgans {
		return nil, false
	}
	tbl := r.current.Load()
	org := tbl.slots[id]
	return org, org != nil
}

func (r *OrganRegistry) HotSwap(org Organ) error {
	spec := org.Spec()
	if spec.ID >= MaxActiveOrgans {
		return ErrOrganNotFound
	}
	for {
		oldTbl := r.current.Load()
		newTbl := *oldTbl
		newTbl.slots[spec.ID] = org
		newTbl.version = oldTbl.version + 1
		if r.current.CompareAndSwap(oldTbl, &newTbl) {
			return nil
		}
	}
}
