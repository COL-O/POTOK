package bus

import "sync/atomic"

// ScratchPool — lock-free пул предвыделенных черновиков поверх MmapRegion.
type ScratchPool struct {
	region   *MmapRegion
	slotSize int
	numSlots uint64
	mask     uint64
	allocIdx atomic.Uint64
}

func NewScratchPool(numSlots int, slotSize int) (*ScratchPool, error) {
	if numSlots <= 0 || (numSlots&(numSlots-1)) != 0 {
		return nil, ErrAlignmentInvalid
	}
	alignedSlot := alignUp(slotSize, CacheLineSize)
	reg, err := AllocRegion(numSlots*alignedSlot, false)
	if err != nil {
		return nil, err
	}
	return &ScratchPool{
		region:   reg,
		slotSize: alignedSlot,
		numSlots: uint64(numSlots),
		mask:     uint64(numSlots - 1),
	}, nil
}

func (p *ScratchPool) Acquire() ([]byte, bool) {
	if p == nil || p.region == nil {
		return nil, false
	}
	idx := p.allocIdx.Add(1) - 1
	offset := int(idx&p.mask) * p.slotSize
	return p.region.Bytes(offset, p.slotSize)
}

func (p *ScratchPool) Close() error {
	if p == nil || p.region == nil {
		return nil
	}
	return p.region.Close()
}

// TickView — Zero-Copy окно просмотра Шины.
type TickView struct {
	EventID    uint64
	Sequence   uint32
	Header     *EventHeader
	HDC        *HDCSlot
	Graph      CSRView
	Tensor     TensorView
	Vector     []float32
	Audio      []int16
	SampleRate uint32
	Scratch    []byte
}
