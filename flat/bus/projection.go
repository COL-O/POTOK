package bus

import "sync/atomic"

type TickView struct {
	EventID uint64
	Header  *EventHeader
	HDC     *HDCSlot
	Graph   CSRView
	Tensor  TensorView
	Audio   []int16
	Scratch []byte
}

type ScratchPool struct {
	buffers  [][]byte
	slotSize int
	inUse    []atomic.Uint32
}

func NewScratchPool(slots, slotSize int) (*ScratchPool, error) {
	p := &ScratchPool{
		buffers:  make([][]byte, slots),
		slotSize: slotSize,
		inUse:    make([]atomic.Uint32, slots),
	}
	for i := 0; i < slots; i++ {
		p.buffers[i] = make([]byte, slotSize)
	}
	return p, nil
}

func (p *ScratchPool) Acquire() ([]byte, int) {
	if p == nil {
		return nil, -1
	}
	for i := range p.inUse {
		if p.inUse[i].CompareAndSwap(0, 1) {
			return p.buffers[i], i
		}
	}
	return make([]byte, p.slotSize), -1
}

func (p *ScratchPool) Release(idx int) {
	if p != nil && idx >= 0 && idx < len(p.inUse) {
		p.inUse[idx].Store(0)
	}
}

func (p *ScratchPool) Close() {}
