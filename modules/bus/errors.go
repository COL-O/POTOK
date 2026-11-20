package bus

import "errors"

var (
	ErrBoundsExceeded   = errors.New("bus: arena bounds exceeded")
	ErrEpochMismatch    = errors.New("bus: slot sequence (epoch) mismatch")
	ErrChecksumMismatch = errors.New("bus: arena block crc32 checksum mismatch")
	ErrAlignmentInvalid = errors.New("bus: invalid memory alignment")
	ErrRingFull         = errors.New("bus: ring buffer overrun")
	ErrOrganNotFound    = errors.New("bus: organ not found in registry")
	ErrSlotNotReady     = errors.New("bus: event slot not ready")
)
