package contracts

import "fmt"

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
// Модульность — основа самовосстановления организма.
type FactoryModule struct {
	ID       uint16  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Capacity float64 `json:"capacity"` // операций/час
}

// Factory — завод общего поля (расширенная карточка).
// Shared — изделия, которые завод разделяет с другими (единое общество),
// Exclusive — только свои. DuplicateOf — кого дублирует (самовосстановление).
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
		return fmt.Errorf("%w: factory code", ErrEmptyCode)
	}
	if f.Role == "" {
		return fmt.Errorf("factory %q: empty role", f.Code)
	}
	return nil
}
