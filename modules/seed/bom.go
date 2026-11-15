package seed

import (
	"github.com/COL-O/POTOK/contracts"
)

// generateBOM — генерация BOM-связей по слоям.
// Каждый узел слоя N — 2–5 детей из слоя N-1. Только между соседними слоями.
func (g *Generator) generateBOM() error {
	byLayer := make(map[contracts.ProductLayer][]contracts.Product)
	for _, p := range g.products {
		byLayer[p.Layer] = append(byLayer[p.Layer], p)
	}

	// Слой 1 (узлы) ← слой 0 (сырьё).
	g.linkLayers(byLayer[contracts.LayerUnit], byLayer[contracts.LayerRaw], g.rng.Intn(2))
	// Слой 2 (подузлы) ← слой 1 (узлы).
	g.linkLayers(byLayer[contracts.LayerSubassembly], byLayer[contracts.LayerUnit], 0)
	// Слой 3 (изделия) ← слой 2 (подузлы).
	g.linkLayers(byLayer[contracts.LayerProduct], byLayer[contracts.LayerSubassembly], 0)

	return nil
}

// linkLayers — связать parent-слой с child-слоем.
// rawDirect — сколько product→raw напрямую (0 — нет). В текущей генерации
// прямые связи сырьё→изделие не строятся, параметр оставлен из скелета.
func (g *Generator) linkLayers(parents, children []contracts.Product, rawDirect int) {
	_ = rawDirect
	if len(children) == 0 || len(parents) == 0 {
		return
	}
	factoryIDs := g.factoryIDs()
	if len(factoryIDs) == 0 {
		return
	}

	for _, parent := range parents {
		n := g.cfg.MinChildren + g.rng.Intn(g.cfg.MaxChildren-g.cfg.MinChildren+1)
		chosen := make(map[uint32]bool)
		for i := 0; i < n; i++ {
			child := children[g.rng.Intn(len(children))]
			if chosen[child.ID] {
				continue
			}
			chosen[child.ID] = true

			fid := factoryIDs[g.rng.Intn(len(factoryIDs))]
			qty := 0.1 + g.rng.Float64()*9.9
			transit := int32(0)
			if g.rng.Intn(2) == 0 {
				transit = int32(3600 + g.rng.Intn(86400))
			}
			edge := contracts.BOMEdge{
				Meta:       contracts.Meta{Source: "seed", Targets: []string{"graph", "projector"}, Branch: "main"},
				ParentID:   parent.ID,
				ChildID:    child.ID,
				FactoryID:  fid,
				QtyPerUnit: qty,
				TransitSec: transit,
			}
			g.bomEdges = append(g.bomEdges, edge)
			g.parentToChildren[parent.ID] = append(g.parentToChildren[parent.ID], child.ID)
		}
	}
}

// factoryIDs — список ID заводов для BOM.
func (g *Generator) factoryIDs() []uint16 {
	out := make([]uint16, 0, len(g.factories))
	for _, f := range g.factories {
		out = append(out, f.ID)
	}
	return out
}
