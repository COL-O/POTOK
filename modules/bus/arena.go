package bus

import (
	"encoding/binary"
	"hash/crc32"
	"sync/atomic"
	"unsafe"
)

const (
	csrHeaderSize    = 16
	tensorHeaderSize = 64
	vectorHeaderSize = 16
	audioHeaderSize  = 16
)

// CSRView — Zero-Copy проекция графа в формате CSR.
type CSRView struct {
	NumNodes    uint32
	NumEdges    uint32
	RowOffsets  []uint32
	ColIndices  []uint32
	EdgeWeights []float32
}

type CSRArena struct {
	region  *MmapRegion
	cursor  atomic.Uint64
	metrics *BusMetrics
}

func NewCSRArena(region *MmapRegion, metrics *BusMetrics) *CSRArena {
	return &CSRArena{region: region, metrics: metrics}
}

func (a *CSRArena) Write(seq uint32, rowOffsets, colIndices []uint32, weights []float32) (uint32, error) {
	if len(rowOffsets) < 2 || len(colIndices) != len(weights) {
		return NullOffset, ErrBoundsExceeded
	}
	numNodes := uint32(len(rowOffsets) - 1)
	numEdges := uint32(len(colIndices))

	payloadBytes := (len(rowOffsets) + len(colIndices) + len(weights)) * 4
	totalBytes := alignUp(csrHeaderSize+payloadBytes, CacheLineSize)
	if totalBytes > a.region.Size() {
		a.metrics.BoundsErrorCount.Add(1)
		return NullOffset, ErrBoundsExceeded
	}
	offset := a.reserve(totalBytes)
	buf, ok := a.region.Bytes(offset, totalBytes)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return NullOffset, ErrBoundsExceeded
	}
	body := buf[csrHeaderSize : csrHeaderSize+payloadBytes]
	dstU32 := unsafe.Slice((*uint32)(unsafe.Pointer(&body[0])), len(rowOffsets)+len(colIndices))
	copy(dstU32, rowOffsets)
	copy(dstU32[len(rowOffsets):], colIndices)

	weightsOffset := (len(rowOffsets) + len(colIndices)) * 4
	dstF32 := unsafe.Slice((*float32)(unsafe.Pointer(&body[weightsOffset])), len(weights))
	copy(dstF32, weights)

	checksum := crc32.ChecksumIEEE(body)

	binary.LittleEndian.PutUint32(buf[0:4], numNodes)
	binary.LittleEndian.PutUint32(buf[4:8], numEdges)
	binary.LittleEndian.PutUint32(buf[12:16], checksum)
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&buf[8])), seq)

	return uint32(offset), nil
}

func (a *CSRArena) Read(offset uint32, expectedSeq uint32) (CSRView, error) {
	if offset == NullOffset {
		return CSRView{}, ErrBoundsExceeded
	}
	hdr, ok := a.region.Bytes(int(offset), csrHeaderSize)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return CSRView{}, ErrBoundsExceeded
	}
	seqBefore := atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[8])))
	if seqBefore != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return CSRView{}, ErrEpochMismatch
	}
	numNodes := binary.LittleEndian.Uint32(hdr[0:4])
	numEdges := binary.LittleEndian.Uint32(hdr[4:8])
	expectedCRC := binary.LittleEndian.Uint32(hdr[12:16])

	rowLen := int(numNodes) + 1
	payloadBytes := (rowLen + int(numEdges)*2) * 4
	if payloadBytes <= 0 {
		a.metrics.BoundsErrorCount.Add(1)
		return CSRView{}, ErrBoundsExceeded
	}
	body, ok := a.region.Bytes(int(offset)+csrHeaderSize, payloadBytes)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return CSRView{}, ErrBoundsExceeded
	}
	if crc32.ChecksumIEEE(body) != expectedCRC {
		a.metrics.ChecksumMismatchCount.Add(1)
		return CSRView{}, ErrChecksumMismatch
	}
	if atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[8]))) != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return CSRView{}, ErrEpochMismatch
	}
	basePtr := unsafe.Pointer(&body[0])
	rowOffsets := unsafe.Slice((*uint32)(basePtr), rowLen)
	colIndices := unsafe.Slice((*uint32)(unsafe.Add(basePtr, rowLen*4)), int(numEdges))
	weights := unsafe.Slice((*float32)(unsafe.Add(basePtr, (rowLen+int(numEdges))*4)), int(numEdges))

	return CSRView{
		NumNodes:    numNodes,
		NumEdges:    numEdges,
		RowOffsets:  rowOffsets,
		ColIndices:  colIndices,
		EdgeWeights: weights,
	}, nil
}

func (a *CSRArena) reserve(alignedBytes int) int {
	capBytes := uint64(a.region.Size())
	for {
		cur := a.cursor.Load()
		off := cur % capBytes
		if off+uint64(alignedBytes) > capBytes {
			next := cur + (capBytes - off) + uint64(alignedBytes)
			if a.cursor.CompareAndSwap(cur, next) {
				return 0
			}
			continue
		}
		if a.cursor.CompareAndSwap(cur, cur+uint64(alignedBytes)) {
			return int(off)
		}
	}
}

// TensorView — Zero-Copy проекция 4D-тензора.
type TensorView struct {
	Dims [4]uint16
	Data []float32
}

type TensorArena struct {
	region  *MmapRegion
	cursor  atomic.Uint64
	metrics *BusMetrics
}

func NewTensorArena(region *MmapRegion, metrics *BusMetrics) *TensorArena {
	return &TensorArena{region: region, metrics: metrics}
}

func (a *TensorArena) Write4D(seq uint32, dims [4]uint16, data []float32) (uint32, error) {
	elemCount := int(dims[0]) * int(dims[1]) * int(dims[2]) * int(dims[3])
	if elemCount <= 0 || elemCount != len(data) {
		a.metrics.BoundsErrorCount.Add(1)
		return NullOffset, ErrBoundsExceeded
	}
	payloadBytes := elemCount * 4
	totalBytes := alignUp(tensorHeaderSize+payloadBytes, CacheLineSize)
	if totalBytes > a.region.Size() {
		a.metrics.BoundsErrorCount.Add(1)
		return NullOffset, ErrBoundsExceeded
	}
	offset := a.reserve(totalBytes)
	buf, ok := a.region.Bytes(offset, totalBytes)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return NullOffset, ErrBoundsExceeded
	}
	body := buf[tensorHeaderSize : tensorHeaderSize+payloadBytes]
	dst := unsafe.Slice((*float32)(unsafe.Pointer(&body[0])), elemCount)
	copy(dst, data)
	checksum := crc32.ChecksumIEEE(body)

	binary.LittleEndian.PutUint16(buf[0:2], dims[0])
	binary.LittleEndian.PutUint16(buf[2:4], dims[1])
	binary.LittleEndian.PutUint16(buf[4:6], dims[2])
	binary.LittleEndian.PutUint16(buf[6:8], dims[3])
	binary.LittleEndian.PutUint32(buf[8:12], uint32(elemCount))
	binary.LittleEndian.PutUint32(buf[12:16], checksum)
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&buf[16])), seq)

	return uint32(offset), nil
}

func (a *TensorArena) Read4D(offset uint32, expectedSeq uint32) (TensorView, error) {
	if offset == NullOffset {
		return TensorView{}, ErrBoundsExceeded
	}
	hdr, ok := a.region.Bytes(int(offset), tensorHeaderSize)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return TensorView{}, ErrBoundsExceeded
	}
	if atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[16]))) != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return TensorView{}, ErrEpochMismatch
	}
	dims := [4]uint16{
		binary.LittleEndian.Uint16(hdr[0:2]),
		binary.LittleEndian.Uint16(hdr[2:4]),
		binary.LittleEndian.Uint16(hdr[4:6]),
		binary.LittleEndian.Uint16(hdr[6:8]),
	}
	elemCount := int(binary.LittleEndian.Uint32(hdr[8:12]))
	expectedCRC := binary.LittleEndian.Uint32(hdr[12:16])
	if elemCount != int(dims[0])*int(dims[1])*int(dims[2])*int(dims[3]) || elemCount <= 0 {
		a.metrics.BoundsErrorCount.Add(1)
		return TensorView{}, ErrBoundsExceeded
	}
	body, ok := a.region.Bytes(int(offset)+tensorHeaderSize, elemCount*4)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return TensorView{}, ErrBoundsExceeded
	}
	if crc32.ChecksumIEEE(body) != expectedCRC {
		a.metrics.ChecksumMismatchCount.Add(1)
		return TensorView{}, ErrChecksumMismatch
	}
	if atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[16]))) != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return TensorView{}, ErrEpochMismatch
	}
	data := unsafe.Slice((*float32)(unsafe.Pointer(&body[0])), elemCount)
	return TensorView{Dims: dims, Data: data}, nil
}

func (a *TensorArena) reserve(alignedBytes int) int {
	capBytes := uint64(a.region.Size())
	for {
		cur := a.cursor.Load()
		off := cur % capBytes
		if off+uint64(alignedBytes) > capBytes {
			next := cur + (capBytes - off) + uint64(alignedBytes)
			if a.cursor.CompareAndSwap(cur, next) {
				return 0
			}
			continue
		}
		if a.cursor.CompareAndSwap(cur, cur+uint64(alignedBytes)) {
			return int(off)
		}
	}
}

// VectorArena — Дорожка 7: 1D-эмбеддинги float32.
type VectorArena struct {
	region  *MmapRegion
	cursor  atomic.Uint64
	metrics *BusMetrics
}

func NewVectorArena(region *MmapRegion, metrics *BusMetrics) *VectorArena {
	return &VectorArena{region: region, metrics: metrics}
}

func (a *VectorArena) Read1D(offset uint32, expectedSeq uint32) ([]float32, error) {
	if offset == NullOffset {
		return nil, ErrBoundsExceeded
	}
	hdr, ok := a.region.Bytes(int(offset), vectorHeaderSize)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return nil, ErrBoundsExceeded
	}
	if atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[4]))) != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return nil, ErrEpochMismatch
	}
	dim := int(binary.LittleEndian.Uint32(hdr[0:4]))
	expectedCRC := binary.LittleEndian.Uint32(hdr[8:12])
	if dim <= 0 {
		return nil, ErrBoundsExceeded
	}
	body, ok := a.region.Bytes(int(offset)+vectorHeaderSize, dim*4)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return nil, ErrBoundsExceeded
	}
	if crc32.ChecksumIEEE(body) != expectedCRC {
		a.metrics.ChecksumMismatchCount.Add(1)
		return nil, ErrChecksumMismatch
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&body[0])), dim), nil
}

// AudioArena — Дорожка 8: PCM16 аудио.
type AudioArena struct {
	region  *MmapRegion
	cursor  atomic.Uint64
	metrics *BusMetrics
}

func NewAudioArena(region *MmapRegion, metrics *BusMetrics) *AudioArena {
	return &AudioArena{region: region, metrics: metrics}
}

func (a *AudioArena) ReadPCM16(offset uint32, expectedSeq uint32) ([]int16, uint32, error) {
	if offset == NullOffset {
		return nil, 0, ErrBoundsExceeded
	}
	hdr, ok := a.region.Bytes(int(offset), audioHeaderSize)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return nil, 0, ErrBoundsExceeded
	}
	if atomic.LoadUint32((*uint32)(unsafe.Pointer(&hdr[8]))) != expectedSeq {
		a.metrics.BackpressureCount.Add(1)
		return nil, 0, ErrEpochMismatch
	}
	count := int(binary.LittleEndian.Uint32(hdr[0:4]))
	sampleRate := binary.LittleEndian.Uint32(hdr[4:8])
	expectedCRC := binary.LittleEndian.Uint32(hdr[12:16])
	if count <= 0 {
		return nil, 0, ErrBoundsExceeded
	}
	body, ok := a.region.Bytes(int(offset)+audioHeaderSize, count*2)
	if !ok {
		a.metrics.BoundsErrorCount.Add(1)
		return nil, 0, ErrBoundsExceeded
	}
	if crc32.ChecksumIEEE(body) != expectedCRC {
		a.metrics.ChecksumMismatchCount.Add(1)
		return nil, 0, ErrChecksumMismatch
	}
	return unsafe.Slice((*int16)(unsafe.Pointer(&body[0])), count), sampleRate, nil
}
