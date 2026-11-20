package stratos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
	"os"
	"sync"
	"sync/atomic"

	"organism/bus"
	"organism/growth"
)

type LawRecord struct {
	GenID     uint64
	CauseCode uint32
	GuardMin  float32
	GuardMax  float32
	Pattern   bus.HDCSlot
	Toxic     bool
	Hash      string
}

type Store struct {
	mu       sync.Mutex
	walFile  *os.File
	lastHash string
	swans    []growth.BlackSwanCase
	laws     atomic.Pointer[[]LawRecord]

	RecallQueries   atomic.Uint64
	RecallHits      atomic.Uint64
	LawsRecorded    atomic.Uint64
	ToxicMarked     atomic.Uint64
	SwansRegistered atomic.Uint64
}

func NewStore(walPath string, initialSwans []growth.BlackSwanCase) (*Store, error) {
	f, err := os.OpenFile(walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s := &Store{
		walFile:  f,
		lastHash: "0000000000000000000000000000000000000000000000000000000000000000",
		swans:    append([]growth.BlackSwanCase(nil), initialSwans...),
	}
	s.SwansRegistered.Store(uint64(len(initialSwans)))
	empty := []LawRecord{}
	s.laws.Store(&empty)
	return s, nil
}

func (s *Store) RecallByHDC(slot *bus.HDCSlot, minSim float32) (uint32, uint32, bool) {
	s.RecallQueries.Add(1)
	if slot == nil {
		return 0, 0, false
	}
	for _, rec := range *s.laws.Load() {
		if rec.Toxic {
			continue
		}
		var diff int
		for w := 0; w < bus.HDCWords; w++ {
			diff += bits.OnesCount64(slot.Words[w] ^ rec.Pattern.Words[w])
		}
		sim := 1.0 - float32(diff)/float32(bus.HDCWords*64)
		if sim >= minSim {
			s.RecallHits.Add(1)
			return rec.CauseCode, 0x07, true
		}
	}
	return 0, 0, false
}

func (s *Store) SampleBlackSwans(limit int) []growth.BlackSwanCase {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.swans) {
		return append([]growth.BlackSwanCase(nil), s.swans...)
	}
	return append([]growth.BlackSwanCase(nil), s.swans[:limit]...)
}

func (s *Store) RecordNewLaw(genID uint64, causeCode uint32, guardMin, guardMax float32, slot *bus.HDCSlot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := fmt.Sprintf("%s|%d|%d|%.4f|%.4f", s.lastHash, genID, causeCode, guardMin, guardMax)
	sum := sha256.Sum256([]byte(payload))
	hashHex := hex.EncodeToString(sum[:])
	s.lastHash = hashHex
	if s.walFile != nil {
		_, _ = fmt.Fprintf(s.walFile, "LAW %s %s\n", payload, hashHex)
	}
	var pat bus.HDCSlot
	if slot != nil {
		pat = *slot
	}
	cur := *s.laws.Load()
	next := append(append([]LawRecord(nil), cur...), LawRecord{
		GenID:     genID,
		CauseCode: causeCode,
		GuardMin:  guardMin,
		GuardMax:  guardMax,
		Pattern:   pat,
		Hash:      hashHex,
	})
	s.laws.Store(&next)
	s.LawsRecorded.Add(1)
	return nil
}

func (s *Store) MarkLawToxic(genID uint64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := *s.laws.Load()
	next := append([]LawRecord(nil), cur...)
	for i := range next {
		if next[i].GenID == genID {
			next[i].Toxic = true
		}
	}
	s.laws.Store(&next)
	s.ToxicMarked.Add(1)
	if s.walFile != nil {
		_, _ = fmt.Fprintf(s.walFile, "TOXIC %d %s\n", genID, reason)
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walFile != nil {
		return s.walFile.Close()
	}
	return nil
}
