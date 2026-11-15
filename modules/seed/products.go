package seed

import (
	"fmt"

	"github.com/COL-O/POTOK/contracts"
)

// generateProducts — генерация изделий по слоям.
func (g *Generator) generateProducts() error {
	// Слой 0: сырьё.
	rawCategories := []string{"iron_ore", "steel", "aluminum", "bearing", "electronics", "packaging"}
	rawPerCat := g.cfg.NumRaw / len(rawCategories)

	layerRaw := make([]contracts.Product, 0, g.cfg.NumRaw)
	for _, cat := range rawCategories {
		for i := 0; i < rawPerCat; i++ {
			sku := fmt.Sprintf("%s_%03d", cat, i+1)
			id, err := g.intern.InternProduct(sku)
			if err != nil {
				return err
			}
			p := contracts.Product{
				Meta:     contracts.Meta{Source: "seed", Targets: []string{"front", "polygon", "growth"}, Branch: "main"},
				ID:       id,
				SKU:      sku,
				Layer:    contracts.LayerRaw,
				Category: cat,
				Unit:     contracts.UnitKg,
				Active:   true,
			}
			layerRaw = append(layerRaw, p)
		}
	}
	g.products = append(g.products, layerRaw...)

	// Слой 1: узлы.
	layerUnit := g.generateLayerProducts("unit", g.cfg.NumUnits, contracts.LayerUnit)
	g.products = append(g.products, layerUnit...)

	// Слой 2: подузлы.
	layerSub := g.generateLayerProducts("subassembly", g.cfg.NumSubassembly, contracts.LayerSubassembly)
	g.products = append(g.products, layerSub...)

	// Слой 3: изделия.
	layerProd := g.generateLayerProducts("product", g.cfg.NumProducts, contracts.LayerProduct)
	g.products = append(g.products, layerProd...)

	return nil
}

func (g *Generator) generateLayerProducts(prefix string, n int, layer contracts.ProductLayer) []contracts.Product {
	out := make([]contracts.Product, 0, n)
	for i := 0; i < n; i++ {
		sku := fmt.Sprintf("%s_%05d", prefix, i+1)
		id, err := g.intern.InternProduct(sku)
		if err != nil {
			continue
		}
		out = append(out, contracts.Product{
			Meta:     contracts.Meta{Source: "seed", Targets: []string{"front", "polygon", "growth"}, Branch: "main"},
			ID:       id,
			SKU:      sku,
			Layer:    layer,
			Category: prefix,
			Unit:     contracts.UnitPcs,
			Active:   true,
		})
	}
	return out
}
