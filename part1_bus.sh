#!/usr/bin/env bash
set -euo pipefail

echo "=== [ЧАСТЬ 1/4] СОЗДАНИЕ КАРКАСА И НУЛЕВОГО ЭТАЖА (bus/) ==="

mkdir -p bus front polygon growth stratos network cmd/organism examples/simple examples/drone deploy/prometheus deploy/grafana/provisioning/datasources deploy/grafana/provisioning/dashboards deploy/k8s bin

cat << 'EOF' > go.mod
module organism

go 1.21

require golang.org/x/sys v0.27.0
EOF

cat << 'EOF' > bootstrap.sh
#!/usr/bin/env bash
set -euo pipefail
MODE="${1:---check}"
echo "=== ПРОВЕРКА И СБОРКА ОРГАНИЗМА (РЕЖИМ: ${MODE}) ==="
command -v go >/dev/null 2>&1 || { echo "[ERR] Требуется Go 1.21+"; exit 1; }
echo "[ENV] $(go version)"
go mod tidy
echo "[TEST] Запуск всех тестов с детектором гонок (-race)..."
go test -v -race ./...
echo "[TEST] Проверка Zero-GC (0.0 аллокаций)..."
go test -v -run=TestZero ./bus/... ./front/...
echo "[CHECK] Проверка примеров..."
go build -o /dev/null ./examples/simple/...
go build -o /dev/null ./examples/drone/...
echo "[BUILD] Сборка bin/organism..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/organism ./cmd/organism/...
echo "=== УСПЕШНО ЗАВЕРШЕНО ==="
EOF
chmod +x bootstrap.sh

cat << 'EOF' > bus/errors.go
package bus

import "errors"

var (
	ErrInvalidLane     = errors.New("bus: invalid lane id")
	ErrLaneNotMapped   = errors.New("bus: lane region not mapped")
	ErrOutOfBounds     = errors.New("bus: arena offset out of bounds")
	ErrChecksum        = errors.New("bus: crc32 checksum mismatch")
	ErrEpochMismatch   = errors.New("bus: slot sequence epoch overwritten")
	ErrSlotNotReady    = errors.New("bus: slot not ready")
	ErrInvalidOrganID  = errors.New("bus: organ id out of range [0..31]")
	ErrMmapFailed      = errors.New("bus: unix.Mmap failed")
)
EOF

cat << 'EOF' > bus/types.go
package bus

const (
	MaxLanes        = 10
	MaxActiveOrgans = 32
	HDCWords        = 160 // 160 * 64 = 10 240 бит
	NullOffset      = ^uint32(0)
)

type LayerID uint8

const (
	LayerBus     LayerID = 0
	LayerFront   LayerID = 1
	LayerPolygon LayerID = 2
	LayerGrowth  LayerID = 3
	LayerStratos LayerID = 4
)

type LaneID uint8

const (
	LanePulse  LaneID = 0 // 64B EventHeader
	LaneHDC    LaneID = 1 // 1280B HDCSlot
	LaneGraph  LaneID = 2 // CSR Graph Arena
	LaneTensor LaneID = 3 // 4D Tensor Arena
	LaneMeta   LaneID = 4 // 64B MetaSlot
	LaneError  LaneID = 5 // 128B ErrorSlot
	LaneFree1  LaneID = 6 // Резерв КОН 8
	LaneVector LaneID = 7 // 1D Vector Arena
	LaneAudio  LaneID = 8 // PCM16 Audio Arena
	LaneFree2  LaneID = 9 // Резерв КОН 8
)

type LaneMask uint32

func (m LaneMask) With(id LaneID) LaneMask { return m | (1 << id) }
func (m LaneMask) Has(id LaneID) bool      { return (m & (1 << id)) != 0 }

type StateFlags uint32

const (
	FlagReady            StateFlags = 1 << 0
	FlagSurprise         StateFlags = 1 << 1
	FlagVetoApplied      StateFlags = 1 << 2
	FlagEscalateToGrowth StateFlags = 1 << 3
	FlagHintDrift        StateFlags = 1 << 4
	FlagCommitted        StateFlags = 1 << 5
)

type StatusCode uint8

const (
	StatusOK          StatusCode = 1
	StatusAbstain     StatusCode = 2
	StatusOutOfBounds StatusCode = 3
	StatusVetoed      StatusCode = 4
	StatusError       StatusCode = 5
)

// EventHeader — ровно 64 байта (1 кэш-линия CPU, выравнивание SOA).
type EventHeader struct {
	EventID     uint64     // 0..8
	Timestamp   int64      // 8..16
	Sequence    uint32     // 16..20 (Epoch Fencing)
	Flags       uint32     // 20..24 (StateFlags)
	ActiveMask  uint32     // 24..28
	VetoMask    uint32     // 28..32
	Confidence  float32    // 32..36
	TTS         uint32     // 36..40
	GraphOffset uint32     // 40..44
	TensorOff   uint32     // 44..48
	VectorOff   uint32     // 48..52
	AudioOff    uint32     // 52..56
	ActionCode  uint32     // 56..60
	OwnerLayer  uint8      // 60..61
	Priority    uint8      // 61..62
	ErrorCode   uint16     // 62..64
}

// HDCSlot — 1280 байт (20 кэш-линий, 10 240 бит).
type HDCSlot struct {
	Words [HDCWords]uint64
}
EOF

cat << 'EOF' > bus/lane.go
package bus

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type ResidencyTier uint8

const (
	ResidencyPinned ResidencyTier = 1
	ResidencyWarm   ResidencyTier = 2
	ResidencyCold   ResidencyTier = 3
)

type MmapRegion struct {
	data   []byte
	ptr    unsafe.Pointer
	size   int
	pinned bool
}

func NewMmapRegion(size int, pin bool) (*MmapRegion, error) {
	if size <= 0 {
		size = 4096
	}
	pageSize := unix.Getpagesize()
	aligned := (size + pageSize - 1) &^ (pageSize - 1)

	b, err := unix.Mmap(-1, 0, aligned, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		return nil, ErrMmapFailed
	}

	r := &MmapRegion{
		data: b,
		ptr:  unsafe.Pointer(&b[0]),
		size: aligned,
	}
	if pin {
		if err := unix.Mlock(b); err == nil {
			r.pinned = true
		}
	}
	return r, nil
}

func (r *MmapRegion) Prefetch(offset, length int) error {
	if r == nil || offset < 0 || offset+length > r.size || length <= 0 {
		return ErrOutOfBounds
	}
	return unix.Madvise(r.data[offset:offset+length], unix.MADV_WILLNEED)
}

func (r *MmapRegion) Evict(offset, length int) error {
	if r == nil || offset < 0 || offset+length > r.size || length <= 0 {
		return ErrOutOfBounds
	}
	return unix.Madvise(r.data[offset:offset+length], unix.MADV_DONTNEED)
}

func (r *MmapRegion) Close() error {
	if r == nil || r.data == nil {
		return nil
	}
	if r.pinned {
		_ = unix.Munlock(r.data)
	}
	err := unix.Munmap(r.data)
	r.data = nil
	r.ptr = nil
	return err
}
EOF

cat << 'EOF' > bus/metrics.go
package bus

import "sync/atomic"

type BusStatsSnapshot struct {
	Published      uint64 `json:"published"`
	Committed      uint64 `json:"committed"`
	ChecksumErrors uint64 `json:"checksum_errors"`
	BoundsErrors   uint64 `json:"bounds_errors"`
	Backpressure   uint64 `json:"backpressure"`
}

type BusMetrics struct {
	PublishedCount atomic.Uint64
	CommittedCount atomic.Uint64
	ChecksumErrors atomic.Uint64
	BoundsErrors   atomic.Uint64
	Backpressure   atomic.Uint64
}

func (m *BusMetrics) Snapshot() BusStatsSnapshot {
	return BusStatsSnapshot{
		Published:      m.PublishedCount.Load(),
		Committed:      m.CommittedCount.Load(),
		ChecksumErrors: m.ChecksumErrors.Load(),
		BoundsErrors:   m.BoundsErrors.Load(),
		Backpressure:   m.Backpressure.Load(),
	}
}
EOF

cat << 'EOF' > bus/arena.go
package bus

import (
	"encoding/binary"
	"hash/crc32"
	"sync/atomic"
	"unsafe"
)

type CSRView struct {
	NumNodes    uint32
	NumEdges    uint32
	RowOffsets  []uint32
	ColIndices  []uint32
	EdgeWeights []float32
}

type CSRArena struct {
	region  *MmapRegion
	head    atomic.Uint64
	metrics *BusMetrics
}

func NewCSRArena(size int, m *BusMetrics) (*CSRArena, error) {
	reg, err := NewMmapRegion(size, false)
	if err != nil {
		return nil, err
	}
	return &CSRArena{region: reg, metrics: m}, nil
}

func (a *CSRArena) Close() error { return a.region.Close() }

func (a *CSRArena) Write(seq uint32, rowOff []uint32, colIdx []uint32, weights []float32) (uint32, error) {
	if len(rowOff) < 2 || len(colIdx) != len(weights) {
		a.metrics.BoundsErrors.Add(1)
		return NullOffset, ErrOutOfBounds
	}
	numNodes := uint32(len(rowOff) - 1)
	numEdges := uint32(len(colIdx))
	payloadBytes := int((numNodes+1)*4 + numEdges*4 + numEdges*4)
	totalBytes := (16 + payloadBytes + 7) &^ 7

	if totalBytes > a.region.size {
		a.metrics.BoundsErrors.Add(1)
		return NullOffset, ErrOutOfBounds
	}

	pos := a.head.Add(uint64(totalBytes)) - uint64(totalBytes)
	off := int(pos % uint64(a.region.size))
	if off+totalBytes > a.region.size {
		pos = a.head.Add(uint64(totalBytes)) - uint64(totalBytes)
		off = 0
	}

	buf := a.region.data[off : off+totalBytes]
	binary.LittleEndian.PutUint32(buf[0:4], numNodes)
	binary.LittleEndian.PutUint32(buf[4:8], numEdges)
	binary.LittleEndian.PutUint32(buf[8:12], seq)

	cur := 16
	for _, r := range rowOff {
		binary.LittleEndian.PutUint32(buf[cur:cur+4], r)
		cur += 4
	}
	for _, c := range colIdx {
		binary.LittleEndian.PutUint32(buf[cur:cur+4], c)
		cur += 4
	}
	for _, w := range weights {
		binary.LittleEndian.PutUint32(buf[cur:cur+4], *(*uint32)(unsafe.Pointer(&w)))
		cur += 4
	}

	sum := crc32.ChecksumIEEE(buf[16 : 16+payloadBytes])
	binary.LittleEndian.PutUint32(buf[12:16], sum)
	return uint32(off), nil
}

func (a *CSRArena) Read(offset uint32) (CSRView, error) {
	off := int(offset)
	if off < 0 || off+16 > a.region.size {
		a.metrics.BoundsErrors.Add(1)
		return CSRView{}, ErrOutOfBounds
	}
	hdr := a.region.data[off : off+16]
	numNodes := binary.LittleEndian.Uint32(hdr[0:4])
	numEdges := binary.LittleEndian.Uint32(hdr[4:8])
	wantSum := binary.LittleEndian.Uint32(hdr[12:16])

	payloadBytes := int((numNodes+1)*4 + numEdges*8)
	if off+16+payloadBytes > a.region.size {
		a.metrics.BoundsErrors.Add(1)
		return CSRView{}, ErrOutOfBounds
	}

	payload := a.region.data[off+16 : off+16+payloadBytes]
	if crc32.ChecksumIEEE(payload) != wantSum {
		a.metrics.ChecksumErrors.Add(1)
		return CSRView{}, ErrChecksum
	}

	base := unsafe.Pointer(&payload[0])
	rows := unsafe.Slice((*uint32)(base), numNodes+1)
	colsPtr := unsafe.Add(base, uintptr((numNodes+1)*4))
	cols := unsafe.Slice((*uint32)(colsPtr), numEdges)
	wPtr := unsafe.Add(colsPtr, uintptr(numEdges*4))
	weights := unsafe.Slice((*float32)(wPtr), numEdges)

	return CSRView{
		NumNodes:    numNodes,
		NumEdges:    numEdges,
		RowOffsets:  rows,
		ColIndices:  cols,
		EdgeWeights: weights,
	}, nil
}

type TensorView struct {
	Dims [4]uint16
	Data []float32
}

type TensorArena struct {
	region  *MmapRegion
	head    atomic.Uint64
	metrics *BusMetrics
}

func NewTensorArena(size int, m *BusMetrics) (*TensorArena, error) {
	reg, err := NewMmapRegion(size, false)
	if err != nil {
		return nil, err
	}
	return &TensorArena{region: reg, metrics: m}, nil
}

func (a *TensorArena) Close() error { return a.region.Close() }

func (a *TensorArena) Write4D(seq uint32, dims [4]uint16, data []float32) (uint32, error) {
	payloadBytes := len(data) * 4
	totalBytes := (64 + payloadBytes + 63) &^ 63
	if totalBytes > a.region.size || len(data) == 0 {
		a.metrics.BoundsErrors.Add(1)
		return NullOffset, ErrOutOfBounds
	}
	pos := a.head.Add(uint64(totalBytes)) - uint64(totalBytes)
	off := int(pos % uint64(a.region.size))
	if off+totalBytes > a.region.size {
		pos = a.head.Add(uint64(totalBytes)) - uint64(totalBytes)
		off = 0
	}
	buf := a.region.data[off : off+totalBytes]
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], dims[i])
	}
	binary.LittleEndian.PutUint32(buf[8:12], uint32(len(data)))
	binary.LittleEndian.PutUint32(buf[12:16], seq)
	dst := unsafe.Slice((*float32)(unsafe.Pointer(&buf[64])), len(data))
	copy(dst, data)
	return uint32(off), nil
}

func (a *TensorArena) Read4D(offset uint32) (TensorView, error) {
	off := int(offset)
	if off < 0 || off+64 > a.region.size {
		a.metrics.BoundsErrors.Add(1)
		return TensorView{}, ErrOutOfBounds
	}
	buf := a.region.data[off:]
	var dims [4]uint16
	for i := 0; i < 4; i++ {
		dims[i] = binary.LittleEndian.Uint16(buf[i*2 : i*2+2])
	}
	count := int(binary.LittleEndian.Uint32(buf[8:12]))
	if off+64+count*4 > a.region.size {
		a.metrics.BoundsErrors.Add(1)
		return TensorView{}, ErrOutOfBounds
	}
	data := unsafe.Slice((*float32)(unsafe.Pointer(&buf[64])), count)
	return TensorView{Dims: dims, Data: data}, nil
}
EOF

cat << 'EOF' > bus/projection.go
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
EOF

cat << 'EOF' > bus/verdict.go
package bus

type Verdict struct {
	OrganID       uint8
	Status        StatusCode
	Confidence    float32
	ActionCode    uint32
	SuggestedMask uint32
}

type OrganMetrics struct {
	LatencyNs  uint64
	Confidence float32
	Status     StatusCode
}
EOF

cat << 'EOF' > bus/organ.go
package bus

import (
	"sync"
	"sync/atomic"
	"time"
)

type OrganSpec struct {
	ID          uint8
	Name        string
	Layer       LayerID
	ReadLanes   LaneMask
	WriteLane   LaneID
	Residency   ResidencyTier
	SoftTimeout time.Duration
	HardTimeout time.Duration
}

type Organ interface {
	Spec() OrganSpec
	InBounds(view TickView) bool
	Warmup(region *MmapRegion) error
	Process(view TickView) (Verdict, OrganMetrics)
	CoolDown()
}

type organTable [MaxActiveOrgans]Organ

type OrganRegistry struct {
	mu    sync.Mutex
	table atomic.Pointer[organTable]
}

func NewOrganRegistry() *OrganRegistry {
	r := &OrganRegistry{}
	var empty organTable
	r.table.Store(&empty)
	return r
}

func (r *OrganRegistry) HotSwap(o Organ) error {
	if o == nil {
		return ErrInvalidOrganID
	}
	spec := o.Spec()
	if spec.ID >= MaxActiveOrgans {
		return ErrInvalidOrganID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.table.Load()
	next := *cur
	next[spec.ID] = o
	r.table.Store(&next)
	return nil
}

func (r *OrganRegistry) Get(id uint8) (Organ, bool) {
	if id >= MaxActiveOrgans {
		return nil, false
	}
	o := r.table.Load()[id]
	return o, o != nil
}
EOF

cat << 'EOF' > bus/bus.go
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
EOF

cat << 'EOF' > bus/bus_test.go
package bus

import (
	"testing"
	"unsafe"
)

func TestBusAlignmentZeroGCAndEpochFencing(t *testing.T) {
	if got := unsafe.Sizeof(EventHeader{}); got != 64 {
		t.Fatalf("EventHeader size = %d, want 64", got)
	}
	if got := unsafe.Sizeof(HDCSlot{}); got != 1280 {
		t.Fatalf("HDCSlot size = %d, want 1280", got)
	}
	b, err := NewBus(BusConfig{
		PulseRingSize:   64,
		HDCRingSize:     64,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}
	defer b.Close()
	var slot HDCSlot
	slot.Words[0] = 0xDEADBEEF
	// Проверка Zero-GC (0.0 аллокаций на горячем пути Publish -> ViewFor -> CommitVerdict)
	allocs := testing.AllocsPerRun(500, func() {
		id := b.Publish(&slot, NullOffset, NullOffset, 100, 10)
		v, err := b.ViewFor(id, LaneMask(0).With(LanePulse).With(LaneHDC), nil)
		if err != nil || v.HDC.Words[0] != 0xDEADBEEF {
			t.Fatalf("ViewFor failed: %v", err)
		}
		_ = b.CommitVerdict(id, Verdict{Status: StatusOK, Confidence: 0.99, ActionCode: 1}, 0)
	})
	if allocs > 0 {
		t.Fatalf("Zero-GC violated on Bus hot path: got %f allocs/op", allocs)
	}
	// Проверка Epoch Fencing при обороте кольца (64 слота)
	firstID := b.Publish(&slot, NullOffset, NullOffset, 100, 10)
	for i := 0; i < 65; i++ {
		b.Publish(&slot, NullOffset, NullOffset, 100, 10)
	}
	if _, err := b.ViewFor(firstID, LaneMask(0).With(LanePulse), nil); err != ErrEpochMismatch {
		t.Fatalf("expected ErrEpochMismatch after ring wrap, got %v", err)
	}
}
EOF

echo "=== [ЧАСТЬ 1/4 ЗАВЕРШЕНА] Жду команду на Часть 2/4 (Этаж 1: front/) ==="
