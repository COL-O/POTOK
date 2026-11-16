package front

import (
	"sync"
	"sync/atomic"
	"time"

	"organism/bus"
)

type PrefetchConfig struct {
	DiskBandwidthMBps   int
	WindowDuration      time.Duration
	MaxTokensPerWindow  int
	CriticalTTSThresh   uint32
	MinPriorityOverride uint8
}

type GraphPrefetcher struct {
	cfg    PrefetchConfig
	region *bus.MmapRegion
	tokens atomic.Int64
	stopCh chan struct{}
	wg     sync.WaitGroup

	PrefetchedCount atomic.Uint64
	DroppedCount    atomic.Uint64
}

func NewGraphPrefetcher(cfg PrefetchConfig, region *bus.MmapRegion) *GraphPrefetcher {
	if cfg.MaxTokensPerWindow <= 0 {
		cfg.MaxTokensPerWindow = 4
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 10 * time.Millisecond
	}
	p := &GraphPrefetcher{
		cfg:    cfg,
		region: region,
		stopCh: make(chan struct{}),
	}
	p.tokens.Store(int64(cfg.MaxTokensPerWindow))
	p.wg.Add(1)
	go p.refillLoop()
	return p
}

func (p *GraphPrefetcher) refillLoop() {
	defer p.wg.Done()
	t := time.NewTicker(p.cfg.WindowDuration)
	defer t.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-t.C:
			p.tokens.Store(int64(p.cfg.MaxTokensPerWindow))
		}
	}
}

func (p *GraphPrefetcher) Request(offset, length int, tts uint32, priority uint8) bool {
	critical := (p.cfg.CriticalTTSThresh > 0 && tts <= p.cfg.CriticalTTSThresh) ||
		(p.cfg.MinPriorityOverride > 0 && priority >= p.cfg.MinPriorityOverride)
	if !critical {
		if p.tokens.Add(-1) < 0 {
			p.tokens.Add(1)
			p.DroppedCount.Add(1)
			return false
		}
	}
	if p.region != nil {
		_ = p.region.Prefetch(offset, length)
	}
	p.PrefetchedCount.Add(1)
	return true
}

func (p *GraphPrefetcher) Cancel(offset, length int) {
	if p.region != nil {
		_ = p.region.Evict(offset, length)
	}
}

func (p *GraphPrefetcher) Close() {
	close(p.stopCh)
	p.wg.Wait()
}
