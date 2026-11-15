package seed

import (
	"fmt"

	"github.com/COL-O/POTOK/contracts"
)

// generateFactories — генерация модульных заводов.
// Принципы: функциональность (каждая роль нужна), модульность (модули),
// дублёры (критичные — минимум 2), зоны (A/B/C), единое общество.
func (g *Generator) generateFactories() error {
	// Роли и количество (функциональная необходимость).
	roles := []struct {
		role   contracts.FactoryRole
		count  int
		crit   contracts.Criticality
		zone   contracts.Zone
		deploy int32
	}{
		{contracts.RoleQuarry, g.cfg.NumQuarries, contracts.CriticalityCritical, contracts.ZoneA, 3600},
		{contracts.RoleShop, g.cfg.NumShops, contracts.CriticalityImportant, contracts.ZoneB, 7200},
		{contracts.RoleAssembly, g.cfg.NumAssembly, contracts.CriticalityCritical, contracts.ZoneB, 5400},
		{contracts.RoleHospital300, g.cfg.NumHospitals, contracts.CriticalityCritical, contracts.ZoneA, 1800},
		{contracts.RoleRecycler, g.cfg.NumRecyclers, contracts.CriticalityImportant, contracts.ZoneC, 10800},
		{contracts.RoleWarehouse, g.cfg.NumWarehouses, contracts.CriticalityOptional, contracts.ZoneA, 1800},
	}

	regions := []string{"Север", "Юг", "Запад", "Восток", "Центр"}

	for _, r := range roles {
		for i := 0; i < r.count; i++ {
			code := fmt.Sprintf("%s_%02d", r.role.String(), i+1)
			id, err := g.intern.InternFactory(code)
			if err != nil {
				return fmt.Errorf("intern factory %q: %w", code, err)
			}

			// Модули: 1–3 на завод (по роли).
			modules := g.generateModules(r.role)

			// Дублёр: для критичных — минимум 2.
			var duplicateOf uint16
			if r.crit == contracts.CriticalityCritical && i > 0 {
				prevCode := fmt.Sprintf("%s_%02d", r.role.String(), i)
				if prevID, err := g.intern.InternFactory(prevCode); err == nil {
					duplicateOf = prevID
				}
			}

			f := contracts.Factory{
				Meta: contracts.Meta{
					Source:  "seed",
					Targets: []string{"front", "polygon", "growth"},
					Branch:  "main",
				},
				ID:              id,
				Code:            code,
				Role:            r.role,
				Region:          regions[i%len(regions)],
				Zone:            r.zone,
				Mode:            contracts.ModeOperate,
				AutomationLevel: g.automationLevel(r.role),
				CapitalCost:     g.capitalCost(r.role),
				Criticality:     r.crit,
				DeployTimeSec:   r.deploy,
				DuplicateOf:     duplicateOf,
				Modules:         modules,
				Shared:          []uint32{}, // заполним в products.go
				Exclusive:       []uint32{},
				Active:          true,
			}
			if err := f.Validate(); err != nil {
				return fmt.Errorf("factory %q: %w", code, err)
			}
			g.factories = append(g.factories, f)
		}
	}
	return nil
}

// generateModules — модули завода (по роли).
func (g *Generator) generateModules(role contracts.FactoryRole) []contracts.FactoryModule {
	n := 1 + g.rng.Intn(3) // 1–3 модуля
	modules := make([]contracts.FactoryModule, 0, n)
	for i := 0; i < n; i++ {
		typ := "line"
		switch role {
		case contracts.RoleQuarry:
			typ = "excavator"
		case contracts.RoleShop:
			typ = "press"
		case contracts.RoleAssembly:
			typ = "assembly"
		case contracts.RoleHospital300:
			typ = "repair"
		case contracts.RoleRecycler:
			typ = "furnace"
		case contracts.RoleWarehouse:
			typ = "storage"
		}
		modules = append(modules, contracts.FactoryModule{
			ID:       uint16(len(modules) + 1),
			Name:     fmt.Sprintf("%s_%d", typ, i+1),
			Type:     typ,
			Capacity: 50 + g.rng.Float64()*150, // 50–200 операций/час
		})
	}
	return modules
}

// automationLevel — уровень автоматизации (гибрид).
func (g *Generator) automationLevel(role contracts.FactoryRole) uint8 {
	switch role {
	case contracts.RoleAssembly, contracts.RoleShop:
		return uint8(g.rng.Intn(3)) // 0–2 (dark factory возможен)
	default:
		return uint8(g.rng.Intn(2)) // 0–1 (люди + роботы)
	}
}

// capitalCost — стоимость развёртывания.
func (g *Generator) capitalCost(role contracts.FactoryRole) float64 {
	base := 100.0
	if role == contracts.RoleAssembly {
		base = 330.0 // dark factory дорого
	}
	return base * (0.5 + g.rng.Float64())
}
