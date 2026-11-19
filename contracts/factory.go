package contracts

import (
	"fmt"
	"time"
)

// FactoryMode — режим работы завода.
type FactoryMode uint8

const (
	ModeDeploy FactoryMode = iota
	ModeOperate
)

func (m FactoryMode) String() string {
	switch m {
	case ModeDeploy:
		return "deploy"
	case ModeOperate:
		return "operate"
	default:
		return "unknown"
	}
}

// Criticality — насколько завод критичен для организма.
type Criticality uint8

const (
	CriticalityOptional Criticality = iota
	CriticalityImportant
	CriticalityCritical
)

func (c Criticality) String() string {
	switch c {
	case CriticalityOptional:
		return "optional"
	case CriticalityImportant:
		return "important"
	case CriticalityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Zone — зона развёртывания.
type Zone uint8

const (
	ZoneA Zone = iota
	ZoneB
	ZoneC
)

func (z Zone) String() string {
	switch z {
	case ZoneA:
		return "A"
	case ZoneB:
		return "B"
	case ZoneC:
		return "C"
	default:
		return "unknown"
	}
}

// FactoryModule — модуль завода (цех, линия, станок).
type FactoryModule struct {
	ID       uint16  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Capacity float64 `json:"capacity"`
}

func (m *FactoryModule) Validate() error {
	if m.ID == 0 {
		return fmt.Errorf("module: id = 0")
	}
	if m.Name == "" {
		return fmt.Errorf("module: нет name")
	}
	if m.Type == "" {
		return fmt.Errorf("module: нет type")
	}
	if m.Capacity <= 0 {
		return fmt.Errorf("module: capacity <= 0 (%f)", m.Capacity)
	}
	return nil
}

// Factory — завод в организме (расширенная структура).
type Factory struct {
	Meta

	ID   uint16 `json:"id"`
	Code string `json:"code"`

	Role   FactoryRole `json:"role"`
	Region string      `json:"region"`
	Zone   Zone        `json:"zone"`

	Mode            FactoryMode `json:"mode"`
	AutomationLevel uint8       `json:"automation_level"`
	CapitalCost     float64     `json:"capital_cost"`

	Criticality   Criticality     `json:"criticality"`
	DeployTimeSec int32           `json:"deploy_time_sec"`
	DuplicateOf   uint16          `json:"duplicate_of,omitempty"`
	Modules       []FactoryModule `json:"modules"`

	Shared    []uint32 `json:"shared,omitempty"`
	Exclusive []uint32 `json:"exclusive,omitempty"`

	Active bool `json:"active"`
}

func (f *Factory) Validate() error {
	if err := f.Meta.Validate(); err != nil {
		return err
	}
	if f.ID == 0 {
		return fmt.Errorf("factory: id = 0")
	}
	if f.Code == "" {
		return fmt.Errorf("factory: нет code")
	}
	if f.Role > RoleWarehouse {
		return fmt.Errorf("factory: неизвестный role %d", f.Role)
	}
	if f.Region == "" {
		return fmt.Errorf("factory: нет region")
	}
	if f.Zone > ZoneC {
		return fmt.Errorf("factory: неизвестный zone %d", f.Zone)
	}
	if f.Mode > ModeOperate {
		return fmt.Errorf("factory: неизвестный mode %d", f.Mode)
	}
	if f.AutomationLevel > 2 {
		return fmt.Errorf("factory: automation_level вне [0,2] (%d)", f.AutomationLevel)
	}
	if f.Criticality > CriticalityCritical {
		return fmt.Errorf("factory: неизвестный criticality %d", f.Criticality)
	}
	if f.DeployTimeSec < 0 {
		return fmt.Errorf("factory: deploy_time_sec < 0 (%d)", f.DeployTimeSec)
	}
	for i := range f.Modules {
		if err := f.Modules[i].Validate(); err != nil {
			return fmt.Errorf("factory: module[%d]: %w", i, err)
		}
	}
	return nil
}

func (f *Factory) IsHelp() bool { return true }

func (f *Factory) TotalCapacity() float64 {
	var total float64
	for _, m := range f.Modules {
		total += m.Capacity
	}
	return total
}

func (f *Factory) CanDeploy() bool { return len(f.Modules) > 0 }

func (f *Factory) CanOperate() bool { return f.Active && len(f.Modules) > 0 }

func (f *Factory) IsDuplicate() bool { return f.DuplicateOf != 0 }

func (f *Factory) HasSharedResource(productID uint32) bool {
	for _, id := range f.Shared {
		if id == productID {
			return true
		}
	}
	return false
}

func (f *Factory) HasExclusiveResource(productID uint32) bool {
	for _, id := range f.Exclusive {
		if id == productID {
			return true
		}
		return false
	}
	return false
}

// ExampleExtendedFactory — пример расширенного завода (для тестов).
func ExampleExtendedFactory(id uint16, code string, role FactoryRole, zone Zone) Factory {
	return Factory{
		Meta:            Meta{Source: "seed", Targets: []string{"front", "polygon", "growth"}, CreatedAt: time.Now(), Branch: "main"},
		ID:              id,
		Code:            code,
		Role:            role,
		Region:          "Центр",
		Zone:            zone,
		Mode:            ModeOperate,
		AutomationLevel: 1,
		CapitalCost:     100.0,
		Criticality:     CriticalityImportant,
		DeployTimeSec:   3600,
		Modules: []FactoryModule{
			{ID: 1, Name: "line_1", Type: "press", Capacity: 100},
		},
		Active: true,
	}
}
