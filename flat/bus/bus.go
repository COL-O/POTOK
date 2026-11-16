package bus

import (
	"sync/atomic"
	"time"
	"unsafe"
)

type BusConfig struct {
	PulseRingSize   int
	HDCRingSize     int
	GraphArenaSize  int
	TensorArenaSize int
	MetaRingSize    int
	ErrorRingSize   int
	VectorArenaSize int
	AudioArenaSize  int
	PinFrontInRAM   bool
}

type Bus struct {
	cfg      BusConfig
	pulseReg *MmapRegion
	hdcReg   *MmapRegion
	pulses   []EventHeader
	hdcs     []HDCSlot
	ringMask uint64

	Graphs  *CSRArena
	Tensors *TensorArena
	Metrics BusMetrics

	nextEventID atomic.Uint64
}

func nextPow2(v int) int {
	if v < 64 {
		v = 64
	}
	p := 1
	for p < v {
		p <<= 1
	}
	return p
}

func NewBus(cfg BusConfig) (*Bus, error) {
	slots := nextPow2(cfg.PulseRingSize)
	pReg, err := NewMmapRegion(slots*int(unsafe.Sizeof(EventHeader{})), cfg.PinFrontInRAM)
	if err != nil {
		return nil, err
	}
	hReg, err := NewMmapRegion(slots*int(unsafe.Sizeof(HDCSlot{})), cfg.PinFrontInRAM)
	if err != nil {
		_ = pReg.Close()
		return nil, err
	}
	gArena, err := NewCSRArena(cfg.GraphArenaSize, &BusMetrics{})
	if err != nil {
		_ = pReg.Close()
		_ = hReg.Close()
		return nil, err
	}
	tArena, err := NewTensorArena(cfg.TensorArenaSize, &BusMetrics{})
	if err != nil {
		_ = pReg.Close()
		_ = hReg.Close()
		_ = gArena.Close()
		return nil, err
	}
	b := &Bus{
		cfg:      cfg,
		pulseReg: pReg,
		hdcReg:   hReg,
		pulses:   unsafe.Slice((*EventHeader)(pReg.ptr), slots),
		hdcs:     unsafe.Slice((*HDCSlot)(hReg.ptr), slots),
		ringMask: uint64(slots - 1),
		Graphs:   gArena,
		Tensors:  tArena,
	}
	gArena.metrics = &b.Metrics
	tArena.metrics = &b.Metrics
	return b, nil
}

func (b *Bus) Publish(hdc *HDCSlot, graphOff, tensorOff, tts uint32, priority uint8) uint64 {
	id := b.nextEventID.Add(1)
	idx := id & b.ringMask
	seq := uint32(id >> 10)
	if hdc != nil {
		b.hdcs[idx] = *hdc
	} else {
		b.hdcs[idx] = HDCSlot{}
	}
	hdr := &b.pulses[idx]
	atomic.StoreUint32(&hdr.Flags, 0)
	hdr.EventID = id
	hdr.Timestamp = time.Now().UnixNano()
	hdr.Sequence = seq
	hdr.ActiveMask = 0
	hdr.VetoMask = 0
	hdr.Confidence = 0
	hdr.TTS = tts
	hdr.GraphOffset = graphOff
	hdr.TensorOff = tensorOff
	hdr.VectorOff = NullOffset
	hdr.AudioOff = NullOffset
	hdr.ActionCode = 0
	hdr.OwnerLayer = uint8(LayerFront)
	hdr.Priority = priority
	hdr.ErrorCode = 0
	atomic.StoreUint32(&hdr.Flags, uint32(FlagReady))
	b.Metrics.PublishedCount.Add(1)
	return id
}

func (b *Bus) ViewFor(eventID uint64, lanes LaneMask, scratch []byte) (TickView, error) {
	idx := eventID & b.ringMask
	hdr := &b.pulses[idx]
	flags := StateFlags(atomic.LoadUint32(&hdr.Flags))
	if (flags & FlagReady) == 0 {
		return TickView{}, ErrSlotNotReady
	}
	if hdr.EventID != eventID {
		b.Metrics.Backpressure.Add(1)
		return TickView{}, ErrEpochMismatch
	}
	tv := TickView{
		EventID: eventID,
		Header:  hdr,
		Scratch: scratch,
	}
	if lanes.Has(LaneHDC) {
		tv.HDC = &b.hdcs[idx]
	}
	if lanes.Has(LaneGraph) && hdr.GraphOffset != NullOffset {
		g, err := b.Graphs.Read(hdr.GraphOffset)
		if err != nil {
			return TickView{}, err
		}
		tv.Graph = g
	}
	if lanes.Has(LaneTensor) && hdr.TensorOff != NullOffset {
		t, err := b.Tensors.Read4D(hdr.TensorOff)
		if err != nil {
			return TickView{}, err
		}
		tv.Tensor = t
	}
	if hdr.EventID != eventID {
		b.Metrics.Backpressure.Add(1)
		return TickView{}, ErrEpochMismatch
	}
	return tv, nil
}

func (b *Bus) CommitVerdict(eventID uint64, v Verdict, extraFlags StateFlags) error {
	idx := eventID & b.ringMask
	hdr := &b.pulses[idx]
	if hdr.EventID != eventID {
		b.Metrics.Backpressure.Add(1)
		return ErrEpochMismatch
	}
	hdr.Confidence = v.Confidence
	hdr.ActionCode = v.ActionCode
	for {
		old := atomic.LoadUint32(&hdr.Flags)
		next := old | uint32(extraFlags|FlagCommitted)
		if atomic.CompareAndSwapUint32(&hdr.Flags, old, next) {
			break
		}
	}
	b.Metrics.CommittedCount.Add(1)
	return nil
}

func (b *Bus) Close() error {
	_ = b.Tensors.Close()
	_ = b.Graphs.Close()
	_ = b.hdcReg.Close()
	return b.pulseReg.Close()
}
