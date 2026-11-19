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
