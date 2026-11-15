// Package bim — BIM-слой организма: рабочие центры заводов и их мощность.
// CBS из polygon берёт отсюда capacity; projector — ограничения.
package bim

import "github.com/COL-O/POTOK/contracts"

// BIM — индекс рабочих центров по заводам.
type BIM struct {
	workCenters map[uint16]contracts.WorkCenter
	byFactory   map[uint16][]uint16
	connections []contracts.Connection
}

// New — построить BIM по рабочим центрам и связям.
func New(wcs []contracts.WorkCenter, conns []contracts.Connection) *BIM {
	b := &BIM{
		workCenters: make(map[uint16]contracts.WorkCenter, len(wcs)),
		byFactory:   make(map[uint16][]uint16),
		connections: append([]contracts.Connection(nil), conns...),
	}
	for _, wc := range wcs {
		b.workCenters[wc.ID] = wc
		b.byFactory[wc.FactoryID] = append(b.byFactory[wc.FactoryID], wc.ID)
	}
	return b
}

// AvailableCapacity — суммарная свободная мощность завода.
func (b *BIM) AvailableCapacity(factoryID uint16) float64 {
	var total float64
	for _, id := range b.byFactory[factoryID] {
		if wc, ok := b.workCenters[id]; ok {
			total += wc.AvailableCapacity()
		}
	}
	return total
}

// WorkCenters — рабочие центры завода.
func (b *BIM) WorkCenters(factoryID uint16) []contracts.WorkCenter {
	out := make([]contracts.WorkCenter, 0, len(b.byFactory[factoryID]))
	for _, id := range b.byFactory[factoryID] {
		if wc, ok := b.workCenters[id]; ok {
			out = append(out, wc)
		}
	}
	return out
}

// Connections — маршруты между рабочими центрами.
func (b *BIM) Connections() []contracts.Connection {
	return append([]contracts.Connection(nil), b.connections...)
}
