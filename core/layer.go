package core

// Layer — к какому времени относится модуль.
type Layer uint8

const (
	LayerFront   Layer = 1
	LayerPolygon Layer = 2
	LayerGrowth  Layer = 3
	LayerStratos Layer = 4
	LayerCross   Layer = 0
)

func (l Layer) String() string {
	switch l {
	case LayerFront:
		return "front"
	case LayerPolygon:
		return "polygon"
	case LayerGrowth:
		return "growth"
	case LayerStratos:
		return "stratos"
	case LayerCross:
		return "cross"
	default:
		return "unknown"
	}
}

// Role — что модуль делает в организме.
type Role uint8

const (
	RoleReflex Role = iota
	RoleImmunity
	RoleGrowth
	RoleMemory
	RoleStore
	RoleGraph
	RoleBIM
	RoleSeed
	RoleProjector
)

func (r Role) String() string {
	switch r {
	case RoleReflex:
		return "reflex"
	case RoleImmunity:
		return "immunity"
	case RoleGrowth:
		return "growth"
	case RoleMemory:
		return "memory"
	case RoleStore:
		return "store"
	case RoleGraph:
		return "graph"
	case RoleBIM:
		return "bim"
	case RoleSeed:
		return "seed"
	case RoleProjector:
		return "projector"
	default:
		return "unknown"
	}
}
