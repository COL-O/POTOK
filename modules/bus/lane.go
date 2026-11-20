package bus

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// MmapRegion — непрерывный участок памяти вне кучи Go (Zero-GC).
type MmapRegion struct {
	data          []byte
	size          int
	prefetchCount atomic.Uint64
	evictCount    atomic.Uint64
}

func alignUp(size, align int) int {
	return (size + align - 1) &^ (align - 1)
}

// AllocRegion выделяет регион памяти через unix.Mmap.
func AllocRegion(requestedSize int, lockInRAM bool) (*MmapRegion, error) {
	if requestedSize <= 0 {
		return nil, fmt.Errorf("bus: invalid region size %d", requestedSize)
	}
	pageSize := unix.Getpagesize()
	alignedSize := alignUp(requestedSize, pageSize)

	data, err := unix.Mmap(
		-1,
		0,
		alignedSize,
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_ANONYMOUS|unix.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("bus: mmap failed (size=%d): %w", alignedSize, err)
	}
	if lockInRAM {
		if err := unix.Mlock(data); err != nil {
			_ = unix.Munmap(data)
			return nil, fmt.Errorf("bus: mlock failed (size=%d): %w", alignedSize, err)
		}
	}
	return &MmapRegion{data: data, size: alignedSize}, nil
}

func (r *MmapRegion) BasePtr() unsafe.Pointer {
	if r == nil || len(r.data) == 0 {
		return nil
	}
	return unsafe.Pointer(&r.data[0])
}

func (r *MmapRegion) Bytes(offset, length int) ([]byte, bool) {
	if r == nil || offset < 0 || length < 0 || offset+length > r.size {
		return nil, false
	}
	return r.data[offset : offset+length], true
}

func (r *MmapRegion) Size() int {
	if r == nil {
		return 0
	}
	return r.size
}

func (r *MmapRegion) Prefetch(offset, length int) error {
	slice, ok := r.alignRange(offset, length)
	if !ok {
		return ErrBoundsExceeded
	}
	r.prefetchCount.Add(1)
	return unix.Madvise(slice, unix.MADV_WILLNEED)
}

func (r *MmapRegion) Evict(offset, length int) error {
	slice, ok := r.alignRange(offset, length)
	if !ok {
		return ErrBoundsExceeded
	}
	r.evictCount.Add(1)
	return unix.Madvise(slice, unix.MADV_DONTNEED)
}

func (r *MmapRegion) Close() error {
	if r == nil || r.data == nil {
		return nil
	}
	err := unix.Munmap(r.data)
	r.data = nil
	r.size = 0
	return err
}

func (r *MmapRegion) alignRange(offset, length int) ([]byte, bool) {
	if r == nil || offset < 0 || length <= 0 || offset+length > r.size {
		return nil, false
	}
	pageSize := unix.Getpagesize()
	start := offset &^ (pageSize - 1)
	end := alignUp(offset+length, pageSize)
	if end > r.size {
		end = r.size
	}
	return r.data[start:end], true
}
