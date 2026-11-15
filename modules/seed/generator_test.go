package seed

import "testing"

// Мир по умолчанию: 100 заводов, изделия по слоям, BOM только между
// соседними слоями, ID заводов уникальны.
func TestGenerateDefaultWorld(t *testing.T) {
	g := NewGenerator(DefaultConfig())
	if err := g.Generate(); err != nil {
		t.Fatalf("generate: %v", err)
	}

	if got := len(g.Factories()); got != 100 {
		t.Errorf("factories = %d, want 100", got)
	}

	seen := make(map[uint16]bool)
	for _, f := range g.Factories() {
		if seen[f.ID] {
			t.Errorf("duplicate factory id %d", f.ID)
		}
		seen[f.ID] = true
		if len(f.Modules) < 1 || len(f.Modules) > 3 {
			t.Errorf("factory %s: modules = %d, want 1..3", f.Code, len(f.Modules))
		}
	}

	cfg := DefaultConfig()
	wantProducts := cfg.NumRaw + cfg.NumUnits + cfg.NumSubassembly + cfg.NumProducts
	if got := len(g.Products()); got != wantProducts {
		t.Errorf("products = %d, want %d", got, wantProducts)
	}

	if got := len(g.BOMEdges()); got == 0 {
		t.Error("bom edges = 0, want > 0")
	}

	// BOM: родитель всегда слоем выше ребёнка ровно на 1.
	layerOf := make(map[uint32]int)
	for _, p := range g.Products() {
		layerOf[p.ID] = int(p.Layer)
	}
	for _, e := range g.BOMEdges() {
		if layerOf[e.ParentID] != layerOf[e.ChildID]+1 {
			t.Errorf("bom edge %d→%d: layers %d and %d, want adjacent",
				e.ParentID, e.ChildID, layerOf[e.ParentID], layerOf[e.ChildID])
		}
	}

	if got := len(g.WorkCenters()); got == 0 {
		t.Error("work centers = 0, want > 0")
	}
}

// Детерминированность: два генератора с одним Seed дают один мир.
func TestGenerateDeterministic(t *testing.T) {
	g1 := NewGenerator(DefaultConfig())
	if err := g1.Generate(); err != nil {
		t.Fatal(err)
	}
	g2 := NewGenerator(DefaultConfig())
	if err := g2.Generate(); err != nil {
		t.Fatal(err)
	}
	if len(g1.BOMEdges()) != len(g2.BOMEdges()) {
		t.Errorf("bom edges differ: %d vs %d", len(g1.BOMEdges()), len(g2.BOMEdges()))
	}
	for i := range g1.Factories() {
		if g1.Factories()[i].Code != g2.Factories()[i].Code {
			t.Fatalf("factory %d differs: %s vs %s", i, g1.Factories()[i].Code, g2.Factories()[i].Code)
		}
	}
}
