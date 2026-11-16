// POTOK — саморазвивающийся организм управления системой заводов.
// Точка сборки: seed мира → store → BOM-граф → BIM → модули слоёв →
// мостики → тестовые рейсы. Скелет: работает на синтетике, не «продакшн».
package main

import (
	"context"
	"fmt"
	"log"

	"organism/contracts"
	"organism/core"
	"organism/links"
	"organism/modules/bim"
	"organism/modules/front"
	"organism/modules/graph"
	"organism/modules/growth"
	"organism/modules/growth/projector"
	"organism/modules/polygon"
	"organism/modules/seed"
	"organism/modules/store/memory"
	"organism/modules/stratos"
)

func main() {
	ctx := context.Background()

	// 1. Seed: генерация blueprint.
	gen := seed.NewGenerator(seed.DefaultConfig())
	if err := gen.Generate(); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Printf("[main] factories=%d products=%d bom=%d workCenters=%d\n",
		len(gen.Factories()), len(gen.Products()), len(gen.BOMEdges()), len(gen.WorkCenters()))

	// 2. Store. В авторском скелете здесь Pebble; в окружении без внешних
	// зависимостей работает in-memory реализация того же интерфейса.
	st := memory.New()
	defer st.Close()

	// 3. Graph (BOM).
	bom, err := graph.NewBOM(gen.Products(), gen.BOMEdges())
	if err != nil {
		log.Fatalf("graph: %v", err)
	}
	if err := bom.Toposort(); err != nil {
		log.Fatalf("toposort: %v", err)
	}
	fmt.Println("[main] BOM DAG valid")

	// 4. BIM.
	b := bim.New(gen.WorkCenters(), gen.Connections())

	// 5. Модули.
	frontMod := front.New()
	polygonMod := polygon.New()
	proj := projector.New(bom, b)
	growthMod := growth.New(proj)
	stratosMod := stratos.New()

	// 6. Мостики (до старта: модули должны стартовать уже состыкованными).
	links.ConnectFrontStore(frontMod, st)
	polygonMod.SetStore(st)
	polygonMod.SetBOM(bom)
	growthMod.SetStore(st)
	stratosMod.SetStore(st)
	links.ConnectProjectorGraph(proj, bom)
	links.ConnectProjectorBIM(proj, b)

	// 7. Реестр.
	reg := core.NewRegistry()
	_ = reg.Register(frontMod, core.Manifest{Name: "front", Role: core.RoleReflex, Layer: core.LayerFront, Enabled: true})
	_ = reg.Register(polygonMod, core.Manifest{Name: "polygon", Role: core.RoleImmunity, Layer: core.LayerPolygon, Enabled: true})
	_ = reg.Register(growthMod, core.Manifest{Name: "growth", Role: core.RoleGrowth, Layer: core.LayerGrowth, Enabled: true})
	_ = reg.Register(stratosMod, core.Manifest{Name: "stratos", Role: core.RoleMemory, Layer: core.LayerStratos, Enabled: true})

	// Мостики через общую шину событий.
	eventBus := core.NewEventBus()
	links.ConnectPolygonFront(polygonMod, frontMod, eventBus)
	links.ConnectPolygonGrowth(polygonMod, growthMod, eventBus)
	links.ConnectGrowthStratos(growthMod, stratosMod, eventBus)
	links.ConnectStratosAll(stratosMod, eventBus)
	links.ConnectFrontParquet(frontMod, st)

	// 8. Старт.
	if err := reg.StartAll(ctx); err != nil {
		log.Fatalf("start: %v", err)
	}
	defer reg.StopAll(ctx)

	// 9. Тестовый рейс (штатный).
	testFlight := contracts.FlightEvent{
		Meta:      contracts.Meta{Source: "front", Targets: []string{"polygon", "growth", "stratos"}, Branch: "main"},
		FlightID:  "test-001",
		TraceID:   "trace-001",
		FactoryID: 1, ToFactory: 2, ProductID: 1, Qty: 100,
		Status: contracts.StatusMoving,
	}
	if err := frontMod.Publish(ctx, testFlight); err != nil {
		log.Printf("publish: %v", err)
	}

	// 10. Тестовый сбой: рейс застрял — цепочка front→polygon→growth→stratos.
	stuckFlight := contracts.FlightEvent{
		Meta:      contracts.Meta{Source: "front", Targets: []string{"polygon", "growth", "stratos"}, Branch: "main"},
		FlightID:  "test-002",
		TraceID:   "trace-002",
		FactoryID: 1, ToFactory: 2, ProductID: 1, Qty: 50,
		Status: contracts.StatusStuck,
	}
	if err := frontMod.Publish(ctx, stuckFlight); err != nil {
		log.Printf("publish stuck: %v", err)
	}

	fmt.Printf("[main] precedents: total=%d diagnosis=%d suggestion=%d flights=%d chainValid=%v\n",
		stratosMod.Chain().Len(),
		len(stratosMod.Recall("Diagnosis")),
		len(stratosMod.Recall("Suggestion")),
		len(stratosMod.Recall("FlightEvent")),
		stratosMod.Chain().Verify(),
	)
	fmt.Println("[main] organism started")
}
