// Package graph — BOM-граф организма: изделия и связи «из чего собирается».
//
// В авторском скелете использовалась библиотека dominikbraun/graph;
// здесь та же семантика на стандартной библиотеке (внешние зависимости
// в окружении сборки недоступны): вершины-изделия, прямой и обратный
// индексы, топологическая сортировка с обнаружением циклов.
package graph

import (
	"fmt"
	"sort"

	"github.com/COL-O/POTOK/contracts"
)

// BOMGraph — направленный граф BOM. Ребро parent → child означает
// «parent собирается из child».
type BOMGraph struct {
	products map[uint32]contracts.Product
	children map[uint32][]uint32 // parent → []child
	reverse  map[uint32][]uint32 // child → []parent
}

// NewBOM — построить граф по изделиям и связям.
// Связь на неизвестное изделие — ошибка: мир должен быть цельным.
func NewBOM(products []contracts.Product, edges []contracts.BOMEdge) (*BOMGraph, error) {
	b := &BOMGraph{
		products: make(map[uint32]contracts.Product, len(products)),
		children: make(map[uint32][]uint32),
		reverse:  make(map[uint32][]uint32),
	}
	for _, p := range products {
		b.products[p.ID] = p
	}
	for _, e := range edges {
		if _, ok := b.products[e.ParentID]; !ok {
			return nil, fmt.Errorf("graph: edge parent %d: unknown product", e.ParentID)
		}
		if _, ok := b.products[e.ChildID]; !ok {
			return nil, fmt.Errorf("graph: edge child %d: unknown product", e.ChildID)
		}
		b.children[e.ParentID] = append(b.children[e.ParentID], e.ChildID)
		b.reverse[e.ChildID] = append(b.reverse[e.ChildID], e.ParentID)
	}
	return b, nil
}

// Toposort — топологическая сортировка (алгоритм Kahn).
// Возвращает ошибку, если в BOM есть цикл: собирать по кругу нельзя.
func (b *BOMGraph) Toposort() error {
	_, err := b.sorted()
	return err
}

// Sorted — вершины в топологическом порядке (родители раньше детей
// в порядке обхода от корней; детерминировано по возрастанию ID).
func (b *BOMGraph) Sorted() ([]uint32, error) {
	return b.sorted()
}

func (b *BOMGraph) sorted() ([]uint32, error) {
	indeg := make(map[uint32]int, len(b.products))
	for id := range b.products {
		indeg[id] = 0
	}
	for _, childs := range b.children {
		for _, c := range childs {
			indeg[c]++
		}
	}

	queue := make([]uint32, 0, len(indeg))
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i] < queue[j] })

	out := make([]uint32, 0, len(indeg))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		next := append([]uint32(nil), b.children[id]...)
		sort.Slice(next, func(i, j int) bool { return next[i] < next[j] })
		for _, c := range next {
			indeg[c]--
			if indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if len(out) != len(b.products) {
		return nil, fmt.Errorf("graph: BOM contains a cycle (%d of %d vertices sorted)", len(out), len(b.products))
	}
	return out, nil
}

// Reverse — обратный индекс: ребёнок → все родители (для projector-а:
// «что можно собрать из этого излишка»).
func (b *BOMGraph) Reverse() map[uint32][]uint32 { return b.reverse }

// Children — прямой индекс: родитель → дети.
func (b *BOMGraph) Children() map[uint32][]uint32 { return b.children }

// Product — изделие по ID.
func (b *BOMGraph) Product(id uint32) (contracts.Product, bool) {
	p, ok := b.products[id]
	return p, ok
}
