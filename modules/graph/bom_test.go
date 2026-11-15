package graph

import (
	"testing"

	"organism/contracts"
)

func products(ids ...uint32) []contracts.Product {
	out := make([]contracts.Product, 0, len(ids))
	for _, id := range ids {
		out = append(out, contracts.Product{ID: id, SKU: "sku", Active: true})
	}
	return out
}

// DAG: топосорт проходит, обратный индекс полон.
func TestToposortDAG(t *testing.T) {
	b, err := NewBOM(products(1, 2, 3), []contracts.BOMEdge{
		{ParentID: 1, ChildID: 2},
		{ParentID: 1, ChildID: 3},
		{ParentID: 2, ChildID: 3},
	})
	if err != nil {
		t.Fatalf("new bom: %v", err)
	}
	if err := b.Toposort(); err != nil {
		t.Fatalf("toposort: %v", err)
	}
	if got := len(b.Reverse()[3]); got != 2 {
		t.Errorf("reverse[3] = %d parents, want 2", got)
	}
}

// Цикл в BOM — ошибка: собирать по кругу нельзя.
func TestToposortCycle(t *testing.T) {
	b, err := NewBOM(products(1, 2), []contracts.BOMEdge{
		{ParentID: 1, ChildID: 2},
		{ParentID: 2, ChildID: 1},
	})
	if err != nil {
		t.Fatalf("new bom: %v", err)
	}
	if err := b.Toposort(); err == nil {
		t.Error("toposort on cycle = nil, want error")
	}
}

// Связь на неизвестное изделие — ошибка построения.
func TestNewBOMUnknownProduct(t *testing.T) {
	if _, err := NewBOM(products(1), []contracts.BOMEdge{{ParentID: 1, ChildID: 99}}); err == nil {
		t.Error("NewBOM with unknown child = nil, want error")
	}
}
