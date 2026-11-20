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
