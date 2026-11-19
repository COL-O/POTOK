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
