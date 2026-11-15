package contracts

import "fmt"

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

// ProductLayer — слой изделия: сырьё → узлы → подузлы → изделия.
type ProductLayer uint8

const (
	LayerRaw ProductLayer = iota
	LayerUnit
	LayerSubassembly
	LayerProduct
)

// ProductUnit — единица измерения изделия.
type ProductUnit string

// Unit — короткое имя того же типа (использовалось в скелете).
type Unit = ProductUnit

const (
	UnitKg  ProductUnit = "kg"
	UnitPcs ProductUnit = "pcs"
)

// Product — изделие/сырьё общего поля («атом» мира, справочник изделий).
type Product struct {
	Meta     Meta         `json:"meta"`
	ID       uint32       `json:"id"`
	SKU      string       `json:"sku"`
	Layer    ProductLayer `json:"layer"`
	Category string       `json:"category"`
	Unit     ProductUnit  `json:"unit"`
	Active   bool         `json:"active"`
}

// BOMEdge — связь «родитель собирается из ребёнка» на конкретном заводе
// («молекула»: как изделия собираются из деталей).
type BOMEdge struct {
	Meta       Meta    `json:"meta"`
	ParentID   uint32  `json:"parent_id"`
	ChildID    uint32  `json:"child_id"`
	FactoryID  uint16  `json:"factory_id"`
	QtyPerUnit float64 `json:"qty_per_unit"`
	TransitSec int32   `json:"transit_sec"`
}

// IsAdjacentLayer — BOM-связь допустима только между соседними слоями:
// родитель ровно на слой выше ребёнка.
func IsAdjacentLayer(parent, child ProductLayer) bool {
	return parent == child+1
}

// ValidateBOMChain — проверка цепочки BOM по каталогу изделий:
// известные изделия, соседние слои, без самосвязей.
// Проверка на цикл — отдельно (нужен граф, см. seed.validateDAG и modules/graph).
func ValidateBOMChain(products []Product, edges []BOMEdge) error {
	layerOf := make(map[uint32]ProductLayer, len(products))
	for _, p := range products {
		layerOf[p.ID] = p.Layer
	}
	for _, e := range edges {
		parentLayer, ok := layerOf[e.ParentID]
		if !ok {
			return fmt.Errorf("%w: parent %d", ErrUnknownProduct, e.ParentID)
		}
		childLayer, ok := layerOf[e.ChildID]
		if !ok {
			return fmt.Errorf("%w: child %d", ErrUnknownProduct, e.ChildID)
		}
		if e.ParentID == e.ChildID {
			return fmt.Errorf("%w: self edge on product %d", ErrBOMCycle, e.ParentID)
		}
		if !IsAdjacentLayer(parentLayer, childLayer) {
			return fmt.Errorf("%w: %d (layer %d) -> %d (layer %d)",
				ErrNonAdjacentLayer, e.ParentID, parentLayer, e.ChildID, childLayer)
		}
	}
	return nil
}
