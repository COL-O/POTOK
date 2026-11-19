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
