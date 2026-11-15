package bus

import (
	"math"
	"sync/atomic"
	"time"
	"unsafe"
)

// BusConfig задаёт индивидуальные размеры колец для каждой дорожки.
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

// Bus — главный координатор Типизированной Шины.
type Bus struct {
	cfg     BusConfig
	nextID  atomic.Uint64
	Metrics BusMetrics

	pulseReg  *MmapRegion
	hdcReg    *MmapRegion
	graphReg  *MmapRegion
	tensorReg *MmapRegion
	metaReg   *MmapRegion
	errorReg  *MmapRegion
	vectorReg *MmapRegion
	audioReg  *MmapRegion

	headers []EventHeader
	hdcs    []HDCSlot
	metas   []MetadataSlot
	errors  []ErrorSlot

	Graphs  *CSRArena
	Tensors *TensorArena
	Vectors *VectorArena
	Audios  *AudioArena

	pulseMask   uint64
	pulseShift  uint32
	hdcMask     uint64
	metaMask    uint64
	errorMask   uint64
	errorCursor atomic.Uint64
}

func isPowerOfTwo(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

func NewBus(cfg BusConfig) (*Bus, error) {
	if !isPowerOfTwo(cfg.PulseRingSize) || !isPowerOfTwo(cfg.HDCRingSize) ||
		!isPowerOfTwo(cfg.MetaRingSize) || !isPowerOfTwo(cfg.ErrorRingSize) {
		return nil, ErrAlignmentInvalid
	}
	b := &Bus{
		cfg:       cfg,
		pulseMask: uint64(cfg.PulseRingSize - 1),
		hdcMask:   uint64(cfg.HDCRingSize - 1),
		metaMask:  uint64(cfg.MetaRingSize - 1),
		errorMask: uint64(cfg.ErrorRingSize - 1),
	}
	for s := cfg.PulseRingSize; s > 1; s >>= 1 {
		b.pulseShift++
	}
	var err error
	if b.pulseReg, err = AllocRegion(cfg.PulseRingSize*int(unsafe.Sizeof(EventHeader{})), cfg.PinFrontInRAM); err != nil {
		return nil, err
	}
	if b.hdcReg, err = AllocRegion(cfg.HDCRingSize*int(unsafe.Sizeof(HDCSlot{})), cfg.PinFrontInRAM); err != nil {
		b.Close()
		return nil, err
	}
	if b.graphReg, err = AllocRegion(cfg.GraphArenaSize, false); err != nil {
		b.Close()
		return nil, err
	}
	if b.tensorReg, err = AllocRegion(cfg.TensorArenaSize, false); err != nil {
		b.Close()
		return nil, err
	}
	if b.metaReg, err = AllocRegion(cfg.MetaRingSize*int(unsafe.Sizeof(MetadataSlot{})), false); err != nil {
		b.Close()
		return nil, err
	}
	if b.errorReg, err = AllocRegion(cfg.ErrorRingSize*int(unsafe.Sizeof(ErrorSlot{})), false); err != nil {
		b.Close()
		return nil, err
	}
	if b.vectorReg, err = AllocRegion(cfg.VectorArenaSize, false); err != nil {
		b.Close()
		return nil, err
	}
	if b.audioReg, err = AllocRegion(cfg.AudioArenaSize, false); err != nil {
		b.Close()
		return nil, err
	}

	b.headers = unsafe.Slice((*EventHeader)(b.pulseReg.BasePtr()), cfg.PulseRingSize)
	b.hdcs = unsafe.Slice((*HDCSlot)(b.hdcReg.BasePtr()), cfg.HDCRingSize)
	b.metas = unsafe.Slice((*MetadataSlot)(b.metaReg.BasePtr()), cfg.MetaRingSize)
	b.errors = unsafe.Slice((*ErrorSlot)(b.errorReg.BasePtr()), cfg.ErrorRingSize)

	b.Graphs = NewCSRArena(b.graphReg, &b.Metrics)
	b.Tensors = NewTensorArena(b.tensorReg, &b.Metrics)
	b.Vectors = NewVectorArena(b.vectorReg, &b.Metrics)
	b.Audios = NewAudioArena(b.audioReg, &b.Metrics)

	return b, nil
}

func (b *Bus) Publish(hdc *HDCSlot, graphOff, tensorOff uint32, tts uint32, priority uint8) uint64 {
	id := b.nextID.Add(1)
	pSlot := id & b.pulseMask
	hSlot := id & b.hdcMask
	seq := uint32(id >> b.pulseShift)

	hdr := &b.headers[pSlot]
	atomic.StoreUint32(&hdr.StateFlags, 0)
	atomic.StoreUint32(&hdr.Sequence, seq)

	if hdc != nil {
		b.hdcs[hSlot] = *hdc
	}
	hdr.EventID = id
	hdr.Timestamp = time.Now().UnixNano()
	hdr.ActiveMask = 0
	hdr.VetoMask = 0
	hdr.Confidence = 0
	hdr.TTS = tts
	hdr.GraphOffset = graphOff
	hdr.TensorOffset = tensorOff
	hdr.MetaOffset = NullOffset
	hdr.OwnerLayer = LayerFront
	hdr.Priority = priority
	hdr.ErrorCode = 0

	atomic.StoreUint32(&hdr.StateFlags, FlagReady)
	b.Metrics.PublishedCount.Add(1)
	return id
}

func (b *Bus) ViewFor(eventID uint64, lanes LaneMask, scratch []byte) (TickView, error) {
	pSlot := eventID & b.pulseMask
	hdr := &b.headers[pSlot]

	flags := atomic.LoadUint32(&hdr.StateFlags)
	if (flags & FlagReady) == 0 {
		return TickView{}, ErrSlotNotReady
	}
	expectedSeq := uint32(eventID >> b.pulseShift)
	if atomic.LoadUint32(&hdr.Sequence) != expectedSeq || hdr.EventID != eventID {
		b.Metrics.BackpressureCount.Add(1)
		return TickView{}, ErrEpochMismatch
	}
	view := TickView{
		EventID:  eventID,
		Sequence: expectedSeq,
		Header:   hdr,
		Scratch:  scratch,
	}
	if lanes.Has(LaneHDC) {
		view.HDC = &b.hdcs[eventID&b.hdcMask]
	}
	if lanes.Has(LaneGraph) && hdr.GraphOffset != NullOffset {
		g, err := b.Graphs.Read(hdr.GraphOffset, expectedSeq)
		if err != nil {
			return TickView{}, err
		}
		view.Graph = g
	}
	if lanes.Has(LaneTensor) && hdr.TensorOffset != NullOffset {
		t, err := b.Tensors.Read4D(hdr.TensorOffset, expectedSeq)
		if err != nil {
			return TickView{}, err
		}
		view.Tensor = t
	}
	if atomic.LoadUint32(&hdr.Sequence) != expectedSeq {
		b.Metrics.BackpressureCount.Add(1)
		return TickView{}, ErrEpochMismatch
	}
	return view, nil
}

func (b *Bus) CommitVerdict(eventID uint64, v Verdict) error {
	pSlot := eventID & b.pulseMask
	hdr := &b.headers[pSlot]
	expectedSeq := uint32(eventID >> b.pulseShift)
	if atomic.LoadUint32(&hdr.Sequence) != expectedSeq || hdr.EventID != eventID {
		return ErrEpochMismatch
	}
	atomic.StoreUint32(&hdr.Confidence, math.Float32bits(v.Confidence))
	if v.Status == StatusOutOfBounds || v.Status == StatusTimeout || v.Status == StatusError {
		for {
			old := atomic.LoadUint32(&hdr.StateFlags)
			if atomic.CompareAndSwapUint32(&hdr.StateFlags, old, old|FlagSurprise) {
				break
			}
		}
	}
	return nil
}

func (b *Bus) Close() {
	for _, r := range []*MmapRegion{
		b.pulseReg, b.hdcReg, b.graphReg, b.tensorReg,
		b.metaReg, b.errorReg, b.vectorReg, b.audioReg,
	} {
		if r != nil {
			_ = r.Close()
		}
	}
}
