package main

import (
	"context"
	"testing"

	"github.com/COL-O/POTOK/contracts"
	"github.com/COL-O/POTOK/core"
	"github.com/COL-O/POTOK/modules/bim"
	"github.com/COL-O/POTOK/modules/front"
	"github.com/COL-O/POTOK/modules/graph"
	"github.com/COL-O/POTOK/modules/growth"
	"github.com/COL-O/POTOK/modules/growth/projector"
	"github.com/COL-O/POTOK/modules/polygon"
	"github.com/COL-O/POTOK/modules/store/memory"
	"github.com/COL-O/POTOK/modules/stratos"
)

// Сквозная цепочка организма: застрявший рейс front рождает FlightStuck,
// polygon ставит Diagnosis, growth предлагает Suggestion, stratos пишет
// всё прецедентами в целостную цепочку.
func TestOrganismChainOnStuckFlight(t *testing.T) {
	ctx := context.Background()

	bom, err := graph.NewBOM(
		[]contracts.Product{{ID: 1, SKU: "product_1"}, {ID: 2, SKU: "unit_1"}},
		[]contracts.BOMEdge{{ParentID: 1, ChildID: 2, FactoryID: 1, QtyPerUnit: 2}},
	)
	if err != nil {
		t.Fatalf("bom: %v", err)
	}

	st := memory.New()
	defer st.Close()

	frontMod := front.New()
	polygonMod := polygon.New()
	growthMod := growth.New(projector.New(bom, bim.New(nil, nil)))
	stratosMod := stratos.New()

	frontMod.SetStore(st)
	polygonMod.SetStore(st)
	polygonMod.SetBOM(bom)
	growthMod.SetStore(st)
	stratosMod.SetStore(st)

	reg := core.NewRegistry()
	for _, m := range []core.Module{frontMod, polygonMod, growthMod, stratosMod} {
		if err := reg.Register(m, core.Manifest{Name: m.Name(), Enabled: true}); err != nil {
			t.Fatalf("register %s: %v", m.Name(), err)
		}
	}
	if err := reg.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer reg.StopAll(ctx)

	stuck := contracts.FlightEvent{
		FlightID:  "test-stuck",
		FactoryID: 1, ToFactory: 2, ProductID: 2, Qty: 10,
		Status: contracts.StatusStuck,
	}
	if err := frontMod.Publish(ctx, stuck); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if got := len(stratosMod.Recall("FlightEvent")); got != 1 {
		t.Errorf("FlightEvent precedents = %d, want 1", got)
	}
	if got := len(stratosMod.Recall("Diagnosis")); got != 1 {
		t.Errorf("Diagnosis precedents = %d, want 1", got)
	}
	if got := len(stratosMod.Recall("Suggestion")); got == 0 {
		t.Error("Suggestion precedents = 0, want > 0")
	}
	if !stratosMod.Chain().Verify() {
		t.Error("stratos chain does not verify")
	}

	// Диагноз записан polygon в store.
	if _, err := st.Get(ctx, []byte("diagnosis:test-stuck")); err != nil {
		t.Errorf("diagnosis in store: %v", err)
	}
}
