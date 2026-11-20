package front

import (
	"sync/atomic"
	"time"

	"organism/modules/bus"
)

// MmapSlotDescriptor описывает расположение весов тяжёлой модели в MmapRegion.
type MmapSlotDescriptor struct {
	OrganID uint8
	Region  *bus.MmapRegion
	Offset  int
	Length  int
}

// PrefetchConfig задаёт бюджет шины памяти.
type PrefetchConfig struct {
	DiskBandwidthMBps   uint64
	WindowDuration      time.Duration
	MaxTokensPerWindow  int64
	CriticalTTSThresh   uint32
	MinPriorityOverride uint8
}

type prefetchCommand struct {
	classID  uint32
	isCancel bool
}

// GraphPrefetcher реализует интерфейс Prefetcher для роутера front.
type GraphPrefetcher struct {
	cfg      PrefetchConfig
	graphMap atomic.Pointer[map[uint32][]MmapSlotDescriptor]

	tokens       atomic.Int64
	lastRefillNs atomic.Int64

	ring     [256]prefetchCommand
	writeIdx atomic.Uint64
	readIdx  atomic.Uint64
	notify   chan struct{}
	stopCh   chan struct{}

	IssuedCount   atomic.Uint64
	ThrottledCnt  atomic.Uint64
	CanceledCount atomic.Uint64
	WarmBitmap    atomic.Uint32
}

func NewGraphPrefetcher(cfg PrefetchConfig, initialGraph map[uint32][]MmapSlotDescriptor) *GraphPrefetcher {
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 10 * time.Millisecond
	}
	if cfg.MaxTokensPerWindow <= 0 {
		calc := int64(cfg.DiskBandwidthMBps / 1500)
		if calc < 1 {
			calc = 1
		}
		cfg.MaxTokensPerWindow = calc
	}
	p := &GraphPrefetcher{
		cfg:    cfg,
		notify: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
	p.tokens.Store(cfg.MaxTokensPerWindow)
	p.lastRefillNs.Store(time.Now().UnixNano())
	if initialGraph == nil {
		initialGraph = make(map[uint32][]MmapSlotDescriptor)
	}
	p.graphMap.Store(&initialGraph)
	go p.workerLoop()
	return p
}

func (p *GraphPrefetcher) UpdateGraphRCU(next map[uint32][]MmapSlotDescriptor) {
	p.graphMap.Store(&next)
}

func (p *GraphPrefetcher) HintDrift(classID uint32, similarity float32, tts uint32, priority uint8) {
	now := time.Now().UnixNano()
	p.refillTokens(now)
	isCritical := (tts > 0 && tts <= p.cfg.CriticalTTSThresh) || (priority >= p.cfg.MinPriorityOverride)
	if !p.tryConsumeToken(isCritical) {
		p.ThrottledCnt.Add(1)
		return
	}
	p.enqueue(prefetchCommand{classID: classID, isCancel: false})
}

func (p *GraphPrefetcher) Cancel(classID uint32) {
	if p.WarmBitmap.Load() == 0 {
		return
	}
	p.enqueue(prefetchCommand{classID: classID, isCancel: true})
}

func (p *GraphPrefetcher) Close() {
	close(p.stopCh)
}

func (p *GraphPrefetcher) refillTokens(nowNs int64) {
	last := p.lastRefillNs.Load()
	if nowNs-last < p.cfg.WindowDuration.Nanoseconds() {
		return
	}
	if p.lastRefillNs.CompareAndSwap(last, nowNs) {
		p.tokens.Store(p.cfg.MaxTokensPerWindow)
	}
}

func (p *GraphPrefetcher) tryConsumeToken(isCritical bool) bool {
	for {
		cur := p.tokens.Load()
		if cur <= 0 {
			if isCritical && cur > -2 {
				if p.tokens.CompareAndSwap(cur, cur-1) {
					return true
				}
				continue
			}
			return false
		}
		if p.tokens.CompareAndSwap(cur, cur-1) {
			return true
		}
	}
}

func (p *GraphPrefetcher) enqueue(cmd prefetchCommand) {
	idx := p.writeIdx.Add(1) - 1
	p.ring[idx&255] = cmd
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

func (p *GraphPrefetcher) workerLoop() {
	for {
		select {
		case <-p.stopCh:
			return
		case <-p.notify:
			p.drainCommands()
		}
	}
}

func (p *GraphPrefetcher) drainCommands() {
	for {
		rIdx := p.readIdx.Load()
		wIdx := p.writeIdx.Load()
		if rIdx >= wIdx {
			return
		}
		if wIdx-rIdx > 256 {
			rIdx = wIdx - 256
			p.readIdx.Store(rIdx)
		}
		cmd := p.ring[rIdx&255]
		p.readIdx.Add(1)
		p.executeCommand(cmd)
	}
}

func (p *GraphPrefetcher) executeCommand(cmd prefetchCommand) {
	gMap := *p.graphMap.Load()
	slots, ok := gMap[cmd.classID]
	if !ok {
		return
	}
	for i := range slots {
		s := &slots[i]
		if s.Region == nil || s.Length <= 0 {
			continue
		}
		bit := uint32(1) << s.OrganID
		if cmd.isCancel {
			if (p.WarmBitmap.Load() & bit) != 0 {
				if err := s.Region.Evict(s.Offset, s.Length); err == nil {
					p.clearWarmBit(bit)
					p.CanceledCount.Add(1)
				}
			}
			continue
		}
		if (p.WarmBitmap.Load() & bit) == 0 {
			if err := s.Region.Prefetch(s.Offset, s.Length); err == nil {
				p.setWarmBit(bit)
				p.IssuedCount.Add(1)
			}
		}
	}
}

func (p *GraphPrefetcher) setWarmBit(bit uint32) {
	for {
		old := p.WarmBitmap.Load()
		if p.WarmBitmap.CompareAndSwap(old, old|bit) {
			return
		}
	}
}

func (p *GraphPrefetcher) clearWarmBit(bit uint32) {
	for {
		old := p.WarmBitmap.Load()
		if p.WarmBitmap.CompareAndSwap(old, old&^bit) {
			return
		}
	}
}
