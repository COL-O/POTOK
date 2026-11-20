package contracts

import (
	"fmt"
	"time"
)

// FactoryRole — роль завода.
type FactoryRole uint8

const (
	RoleQuarry FactoryRole = iota
	RoleShop
	RoleAssembly
	RoleHospital300
	RoleRecycler
	RoleWarehouse
)

func (r FactoryRole) String() string {
	switch r {
	case RoleQuarry:
		return "quarry"
	case RoleShop:
		return "shop"
	case RoleAssembly:
		return "assembly"
	case RoleHospital300:
		return "hospital_300"
	case RoleRecycler:
		return "recycler"
	case RoleWarehouse:
		return "warehouse"
	default:
		return "unknown"
	}
}

// Factory — завод в организме: каноническое определение в factory.go
// (расширенная структура). Здесь не дублируется: лёгкая версия из первой
// редакции каталога снята при подготовке к первому запуску, чтобы пакет
// собирался; все использования (seed, growth) опираются на расширенную.

// ProductLayer — слой изделия в DAG.
type ProductLayer uint8

const (
	LayerRaw ProductLayer = iota
	LayerUnit
	LayerSubassembly
	LayerProduct
)

func (l ProductLayer) String() string {
	switch l {
	case LayerRaw:
		return "raw"
	case LayerUnit:
		return "unit"
	case LayerSubassembly:
		return "subassembly"
	case LayerProduct:
		return "product"
	default:
		return "unknown"
	}
}

// ProductUnit — единица измерения.
type ProductUnit uint8

const (
	UnitPcs ProductUnit = iota
	UnitKg
	UnitL
	UnitM
)

func (u ProductUnit) String() string {
	switch u {
	case UnitPcs:
		return "pcs"
	case UnitKg:
		return "kg"
	case UnitL:
		return "l"
	case UnitM:
		return "m"
	default:
		return "unknown"
	}
}

// Product — изделие в справочнике.
type Product struct {
	Meta
	ID       uint32       `json:"id"`
	SKU      string       `json:"sku"`
	Layer    ProductLayer `json:"layer"`
	Category string       `json:"category"`
	Unit     ProductUnit  `json:"unit"`
	Active   bool         `json:"active"`
}

func (p *Product) Validate() error {
	if err := p.Meta.Validate(); err != nil {
		return err
	}
	if p.ID == 0 {
		return fmt.Errorf("product: id = 0")
	}
	if p.SKU == "" {
		return fmt.Errorf("product: нет sku")
	}
	if p.Layer > LayerProduct {
		return fmt.Errorf("product: неизвестный layer %d", p.Layer)
	}
	if p.Category == "" {
		return fmt.Errorf("product: нет category")
	}
	if p.Unit > UnitM {
		return fmt.Errorf("product: неизвестный unit %d", p.Unit)
	}
	return nil
}

func (p *Product) IsHelp() bool { return true }

// BOMEdge — связь «из чего собирается».
type BOMEdge struct {
	Meta
	ParentID   uint32  `json:"parent_id"`
	ChildID    uint32  `json:"child_id"`
	FactoryID  uint16  `json:"factory_id"`
	QtyPerUnit float64 `json:"qty_per_unit"`
	TransitSec int32   `json:"transit_sec"`
}

func (e *BOMEdge) Validate() error {
	if err := e.Meta.Validate(); err != nil {
		return err
	}
	if e.ParentID == 0 {
		return fmt.Errorf("bom: parent_id = 0")
	}
	if e.ChildID == 0 {
		return fmt.Errorf("bom: child_id = 0")
	}
	if e.ParentID == e.ChildID {
		return fmt.Errorf("bom: parent_id == child_id (%d)", e.ParentID)
	}
	if e.FactoryID == 0 {
		return fmt.Errorf("bom: factory_id = 0")
	}
	if e.QtyPerUnit <= 0 {
		return fmt.Errorf("bom: qty_per_unit <= 0 (%f)", e.QtyPerUnit)
	}
	if e.TransitSec < 0 {
		return fmt.Errorf("bom: transit_sec < 0 (%d)", e.TransitSec)
	}
	return nil
}

func (e *BOMEdge) IsHelp() bool { return true }

// IsAdjacentLayer — проверка, что parent и child на соседних слоях.
func IsAdjacentLayer(parent, child ProductLayer) bool {
	if parent == child+1 {
		return true
	}
	if parent == LayerProduct && child == LayerRaw {
		return true
	}
	return false
}

// ValidateBOMChain — проверка цепочки BOM.
func ValidateBOMChain(edges []BOMEdge, products map[uint32]Product) error {
	for _, e := range edges {
		parent, ok := products[e.ParentID]
		if !ok {
			return fmt.Errorf("bom: parent %d не найден", e.ParentID)
		}
		child, ok := products[e.ChildID]
		if !ok {
			return fmt.Errorf("bom: child %d не найден", e.ChildID)
		}
		if !IsAdjacentLayer(parent.Layer, child.Layer) {
			return fmt.Errorf("bom: parent %d (layer %s) и child %d (layer %s) не соседние",
				e.ParentID, parent.Layer, e.ChildID, child.Layer)
		}
	}
	return nil
}

// ExampleFactory — пример завода (для тестов).
func ExampleFactory(id uint16, code string, role FactoryRole, region string) Factory {
	return Factory{
		Meta:   Meta{Source: "seed", Targets: []string{"front", "polygon", "growth"}, CreatedAt: time.Now(), Branch: "main"},
		ID:     id,
		Code:   code,
		Role:   role,
		Region: region,
		Active: true,
	}
}

// ExampleProduct — пример изделия (для тестов).
func ExampleProduct(id uint32, sku string, layer ProductLayer, category string) Product {
	return Product{
		Meta:     Meta{Source: "seed", Targets: []string{"front", "polygon", "growth"}, CreatedAt: time.Now(), Branch: "main"},
		ID:       id,
		SKU:      sku,
		Layer:    layer,
		Category: category,
		Unit:     UnitPcs,
		Active:   true,
	}
}
