package seed

import (
	"fmt"

	"organism/contracts"
)

// generateBIM — генерация рабочих центров и маршрутов.
func (g *Generator) generateBIM() error {
	// Рабочие центры: для каждого завода с ролью цех/сборка — 2–5 центров.
	wcID := uint16(1)
	factoryWCs := make(map[uint16][]uint16)
	for _, f := range g.factories {
		if f.Role != contracts.RoleShop && f.Role != contracts.RoleAssembly {
			continue
		}
		n := 2 + g.rng.Intn(4)
		for i := 0; i < n; i++ {
			wc := contracts.WorkCenter{
				Meta:      contracts.Meta{Source: "seed", CreatedAt: g.createdAt, Targets: []string{"projector", "front"}, Branch: "main"},
				ID:        wcID,
				FactoryID: f.ID,
				Type:      "line",
				Capacity:  50 + g.rng.Float64()*150,
				Load:      g.rng.Float64() * 0.9,
			}
			if err := wc.Validate(); err != nil {
				return fmt.Errorf("work center: %w", err)
			}
			g.workCenters = append(g.workCenters, wc)
			factoryWCs[f.ID] = append(factoryWCs[f.ID], wcID)
			wcID++
		}
	}

	// Маршруты: внутри завода и между заводами.
	for _, wcs := range factoryWCs {
		if len(wcs) < 2 {
			continue
		}
		for i := 0; i < len(wcs)-1; i++ {
			c := contracts.Connection{
				Meta:       contracts.Meta{Source: "seed", CreatedAt: g.createdAt, Targets: []string{"front", "bim"}, Branch: "main"},
				FromID:     wcs[i],
				ToID:       wcs[i+1],
				DistanceM:  int32(10 + g.rng.Intn(90)),
				TransitSec: int32(g.rng.Intn(300)),
			}
			if err := c.Validate(); err != nil {
				continue
			}
			g.connections = append(g.connections, c)
		}
	}
	return nil
}
