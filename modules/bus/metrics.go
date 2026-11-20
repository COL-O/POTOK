package bus

import "sync/atomic"

// BusMetrics — lock-free счётчики состояния и здоровья шины.
type BusMetrics struct {
	PublishedCount        atomic.Uint64
	RingOverrunCount      atomic.Uint64
	BackpressureCount     atomic.Uint64
	BoundsErrorCount      atomic.Uint64
	ChecksumMismatchCount atomic.Uint64
	PrefetchCount         atomic.Uint64
	EvictCount            atomic.Uint64
}

// BusStatsSnapshot — неизменяемый снимок метрик.
type BusStatsSnapshot struct {
	Published      uint64
	RingOverruns   uint64
	Backpressure   uint64
	BoundsErrors   uint64
	ChecksumErrors uint64
	Prefetches     uint64
	Evictions      uint64
}

func (m *BusMetrics) Snapshot() BusStatsSnapshot {
	return BusStatsSnapshot{
		Published:      m.PublishedCount.Load(),
		RingOverruns:   m.RingOverrunCount.Load(),
		Backpressure:   m.BackpressureCount.Load(),
		BoundsErrors:   m.BoundsErrorCount.Load(),
		ChecksumErrors: m.ChecksumMismatchCount.Load(),
		Prefetches:     m.PrefetchCount.Load(),
		Evictions:      m.EvictCount.Load(),
	}
}
