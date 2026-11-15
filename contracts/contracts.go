// Package contracts — общие типы организма POTOK.
//
// Это минимальное ядро контрактов, восстановленное по скелету автора
// (часть описания «от окончания»): только те типы и поля, которые реально
// используются модулями seed/graph/bim/front/polygon/growth/stratos.
// Точные формулы и расширенные поля — в следующих частях описания.
package contracts

import "fmt"

// Meta — общие метаданные любой сущности/сообщения организма.
type Meta struct {
	Source  string   `json:"source"`
	Targets []string `json:"targets,omitempty"`
	Branch  string   `json:"branch,omitempty"`
}

// ---- Заводы ----

// FactoryRole — роль завода в организме.
type FactoryRole string

const (
	RoleQuarry      FactoryRole = "quarry"
	RoleShop        FactoryRole = "shop"
	RoleAssembly    FactoryRole = "assembly"
	RoleHospital300 FactoryRole = "hospital300"
	RoleRecycler    FactoryRole = "recycler"
	RoleWarehouse   FactoryRole = "warehouse"
)

func (r FactoryRole) String() string { return string(r) }

// Criticality — критичность завода для организма.
type Criticality string

const (
	CriticalityCritical  Criticality = "critical"
	CriticalityImportant Criticality = "important"
	CriticalityOptional  Criticality = "optional"
)

// Zone — зона развёртывания.
type Zone string

const (
	ZoneA Zone = "A"
	ZoneB Zone = "B"
	ZoneC Zone = "C"
)

// FactoryMode — режим завода.
type FactoryMode string

const (
	ModeOperate FactoryMode = "operate"
	ModeDeploy  FactoryMode = "deploy"
)

// FactoryModule — модуль внутри завода (линия, пресс, печь и т.д.).
type FactoryModule struct {
	ID       uint16  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Capacity float64 `json:"capacity"` // операций/час
}

// Factory — завод общего поля.
type Factory struct {
	Meta            Meta            `json:"meta"`
	ID              uint16          `json:"id"`
	Code            string          `json:"code"`
	Role            FactoryRole     `json:"role"`
	Region          string          `json:"region"`
	Zone            Zone            `json:"zone"`
	Mode            FactoryMode     `json:"mode"`
	AutomationLevel uint8           `json:"automation_level"`
	CapitalCost     float64         `json:"capital_cost"`
	Criticality     Criticality     `json:"criticality"`
	DeployTimeSec   int32           `json:"deploy_time_sec"`
	DuplicateOf     uint16          `json:"duplicate_of,omitempty"`
	Modules         []FactoryModule `json:"modules,omitempty"`
	Shared          []uint32        `json:"shared,omitempty"`
	Exclusive       []uint32        `json:"exclusive,omitempty"`
	Active          bool            `json:"active"`
}

// Validate — минимальная проверка завода перед регистрацией в seed.
func (f Factory) Validate() error {
	if f.Code == "" {
		return fmt.Errorf("factory: empty code")
	}
	if f.Role == "" {
		return fmt.Errorf("factory %q: empty role", f.Code)
	}
	return nil
}

// ---- Изделия и BOM ----

// ProductLayer — слой изделия: сырьё → узлы → подузлы → изделия.
type ProductLayer uint8

const (
	LayerRaw ProductLayer = iota
	LayerUnit
	LayerSubassembly
	LayerProduct
)

// Unit — единица измерения.
type Unit string

const (
	UnitKg  Unit = "kg"
	UnitPcs Unit = "pcs"
)

// Product — изделие/сырьё общего поля.
type Product struct {
	Meta     Meta         `json:"meta"`
	ID       uint32       `json:"id"`
	SKU      string       `json:"sku"`
	Layer    ProductLayer `json:"layer"`
	Category string       `json:"category"`
	Unit     Unit         `json:"unit"`
	Active   bool         `json:"active"`
}

// BOMEdge — связь «родитель собирается из ребёнка» на конкретном заводе.
type BOMEdge struct {
	Meta       Meta    `json:"meta"`
	ParentID   uint32  `json:"parent_id"`
	ChildID    uint32  `json:"child_id"`
	FactoryID  uint16  `json:"factory_id"`
	QtyPerUnit float64 `json:"qty_per_unit"`
	TransitSec int32   `json:"transit_sec"`
}

// ---- BIM: рабочие центры и связи ----

// WorkCenter — рабочий центр завода (источник capacity).
type WorkCenter struct {
	Meta      Meta    `json:"meta"`
	ID        uint16  `json:"id"`
	FactoryID uint16  `json:"factory_id"`
	Type      string  `json:"type"`
	Capacity  float64 `json:"capacity"` // операций/час
	Load      float64 `json:"load"`     // 0..1
}

// AvailableCapacity — свободная мощность центра.
func (w WorkCenter) AvailableCapacity() float64 {
	if w.Load >= 1 {
		return 0
	}
	return w.Capacity * (1 - w.Load)
}

// Validate — минимальная проверка рабочего центра.
func (w WorkCenter) Validate() error {
	if w.FactoryID == 0 {
		return fmt.Errorf("work center %d: empty factory", w.ID)
	}
	if w.Load < 0 || w.Load > 1 {
		return fmt.Errorf("work center %d: load out of range", w.ID)
	}
	return nil
}

// Connection — связь между рабочими центрами (маршрут).
type Connection struct {
	Meta       Meta   `json:"meta"`
	FromID     uint16 `json:"from_id"`
	ToID       uint16 `json:"to_id"`
	DistanceM  int32  `json:"distance_m"`
	TransitSec int32  `json:"transit_sec"`
}

// Validate — связь не может вести в саму себя.
func (c Connection) Validate() error {
	if c.FromID == c.ToID {
		return fmt.Errorf("connection: from == to (%d)", c.FromID)
	}
	return nil
}

// ---- События front / polygon / growth ----

// FlightStatus — статус рейса (перемещения) между заводами.
type FlightStatus string

const (
	StatusMoving FlightStatus = "moving"
	StatusStuck  FlightStatus = "stuck"
)

// FlightEvent — рейс: продукт едет с завода на завод.
type FlightEvent struct {
	Meta      Meta         `json:"meta"`
	FlightID  string       `json:"flight_id"`
	TraceID   string       `json:"trace_id"`
	FactoryID uint16       `json:"factory_id"`
	ToFactory uint16       `json:"to_factory"`
	ProductID uint32       `json:"product_id"`
	Qty       float64      `json:"qty"`
	Status    FlightStatus `json:"status"`
}

// QualityPassport — паспорт качества (Pasport из слоя front).
type QualityPassport struct {
	PassportID string `json:"passport_id"`
	FactoryID  uint16 `json:"factory_id"`
	ProductID  uint32 `json:"product_id"`
	Grade      string `json:"grade"`
	IssuedAt   int64  `json:"issued_at"`
}

// RollbackKind — вид отката: 300 — лечение, 200 — рециклинг.
type RollbackKind string

const (
	Rollback300 RollbackKind = "rollback_300"
	Rollback200 RollbackKind = "rollback_200"
)

// RollbackEvent — событие отката (пострадавший узел/изделие).
type RollbackEvent struct {
	Kind      RollbackKind `json:"kind"`
	FactoryID uint16       `json:"factory_id"`
	ProductID uint32       `json:"product_id"`
}

// SuggestionSource — откуда пришла подсказка projector-а.
type SuggestionSource string

const (
	SourceReverseBOM SuggestionSource = "reverse_bom"
)

// Suggestion — предложение projector-а: что можно собрать (can_build).
type Suggestion struct {
	Meta         Meta             `json:"meta"`
	SuggestionID string           `json:"suggestion_id"`
	ProductID    uint32           `json:"product_id"`
	CanBuild     float64          `json:"can_build"`
	Source       SuggestionSource `json:"source"`
	Confidence   float64          `json:"confidence"`
}
