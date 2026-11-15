package stratos

import (
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/COL-O/POTOK/modules/bus"
	"github.com/COL-O/POTOK/modules/growth"
	"github.com/COL-O/POTOK/modules/polygon"
)

// LawArchiveEntry — запись выведенного закона в долгой памяти.
type LawArchiveEntry struct {
	GenealogyID uint64
	Timestamp   int64
	Equation    growth.SINDyEquation
	LUT         growth.CompiledLUT
	IsToxic     bool
	ToxicReason uint32
	SealHash    [32]byte
}

type storeState struct {
	version    uint64
	precedents []HDCRecord
	blackSwans []growth.BlackSwanCase
	laws       []LawArchiveEntry
}

// Store — центральное хранилище stratos.
type Store struct {
	mu       sync.Mutex
	state    atomic.Pointer[storeState]
	hdcIndex *HDCIndexOrgan
	lshHnsw  *LSHHNSWOrgan
	mpIndex  *MatrixProfileIndexOrgan
	retNet   *RetNetOrgan
	seal     *SHA256SealOrgan

	walFile   *os.File
	lastChain [32]byte

	RecallQueries   atomic.Uint64
	RecallHits      atomic.Uint64
	LawsRecorded    atomic.Uint64
	ToxicMarked     atomic.Uint64
	SwansRegistered atomic.Uint64
}

func NewStore(walPath string, initialSwans []growth.BlackSwanCase) (*Store, error) {
	s := &Store{
		hdcIndex: NewHDCIndexOrgan(nil, 0.85),
		lshHnsw:  NewLSHHNSWOrgan(nil, 0.80),
		mpIndex:  NewMatrixProfileIndexOrgan(nil),
		retNet:   &RetNetOrgan{Gamma: 0.95},
		seal:     NewSHA256SealOrgan(),
	}
	swansCopy := make([]growth.BlackSwanCase, len(initialSwans))
	copy(swansCopy, initialSwans)
	s.state.Store(&storeState{version: 1, blackSwans: swansCopy})
	if walPath != "" {
		f, err := os.OpenFile(walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		s.walFile = f
	}
	return s, nil
}

func (s *Store) RecallByHDC(hdc *bus.HDCSlot, minSim float32) (polygon.Precedent, bool) {
	if hdc == nil {
		return polygon.Precedent{}, false
	}
	s.RecallQueries.Add(1)
	st := s.state.Load()
	const totalBits = float32(bus.HDCWords * 64)
	var bestSim float32
	var best *HDCRecord
	for i := range st.precedents {
		rec := &st.precedents[i]
		if rec.IsToxic {
			continue
		}
		var diff int
		for w := 0; w < bus.HDCWords; w++ {
			diff += bits.OnesCount64(hdc.Words[w] ^ rec.Pattern.Words[w])
		}
		sim := 1.0 - (float32(diff) / totalBits)
		if sim > bestSim {
			bestSim = sim
			best = rec
		}
	}
	if best == nil || bestSim < minSim {
		return polygon.Precedent{}, false
	}
	s.RecallHits.Add(1)
	return polygon.Precedent{
		PrecedentID:     best.RecordID,
		CauseCode:       best.CauseCode,
		AntibodyAction:  best.AntibodyAction,
		RecommendedMask: best.RecommendedMask,
		Confidence:      bestSim * best.Confidence,
	}, true
}

func (s *Store) FetchBlackSwans(limit int) []growth.BlackSwanCase {
	st := s.state.Load()
	if limit <= 0 || limit >= len(st.blackSwans) {
		out := make([]growth.BlackSwanCase, len(st.blackSwans))
		copy(out, st.blackSwans)
		return out
	}
	start := len(st.blackSwans) - limit
	out := make([]growth.BlackSwanCase, limit)
	copy(out, st.blackSwans[start:])
	return out
}

func (s *Store) RecordNewLaw(eq growth.SINDyEquation, lut growth.CompiledLUT, genealogyID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixNano()
	sealHash := s.computeChainSeal(genealogyID, now, eq.C0, eq.C1)
	entry := LawArchiveEntry{GenealogyID: genealogyID, Timestamp: now, Equation: eq, LUT: lut, IsToxic: false, SealHash: sealHash}
	oldSt := s.state.Load()
	nextLaws := make([]LawArchiveEntry, len(oldSt.laws)+1)
	copy(nextLaws, oldSt.laws)
	nextLaws[len(oldSt.laws)] = entry
	nextSt := &storeState{version: oldSt.version + 1, precedents: oldSt.precedents, blackSwans: oldSt.blackSwans, laws: nextLaws}
	s.state.Store(nextSt)
	s.LawsRecorded.Add(1)
	s.appendWALRecord(0x01, genealogyID, now, sealHash)
}

func (s *Store) MarkLawToxic(genealogyID uint64, reason uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldSt := s.state.Load()
	nextLaws := make([]LawArchiveEntry, len(oldSt.laws))
	copy(nextLaws, oldSt.laws)
	for i := range nextLaws {
		if nextLaws[i].GenealogyID == genealogyID {
			nextLaws[i].IsToxic = true
			nextLaws[i].ToxicReason = reason
		}
	}
	nextPrecs := make([]HDCRecord, len(oldSt.precedents))
	copy(nextPrecs, oldSt.precedents)
	for i := range nextPrecs {
		if nextPrecs[i].RecordID == genealogyID {
			nextPrecs[i].IsToxic = true
		}
	}
	nextSt := &storeState{version: oldSt.version + 1, precedents: nextPrecs, blackSwans: oldSt.blackSwans, laws: nextLaws}
	s.state.Store(nextSt)
	s.hdcIndex.UpdateIndexRCU(nextPrecs)
	s.ToxicMarked.Add(1)
	s.appendWALRecord(0x02, genealogyID, time.Now().UnixNano(), s.lastChain)
}

func (s *Store) RegisterPrecedent(rec HDCRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.Timestamp == 0 {
		rec.Timestamp = time.Now().UnixNano()
	}
	oldSt := s.state.Load()
	nextPrecs := make([]HDCRecord, len(oldSt.precedents)+1)
	copy(nextPrecs, oldSt.precedents)
	nextPrecs[len(oldSt.precedents)] = rec
	nextSt := &storeState{version: oldSt.version + 1, precedents: nextPrecs, blackSwans: oldSt.blackSwans, laws: oldSt.laws}
	s.state.Store(nextSt)
	s.hdcIndex.UpdateIndexRCU(nextPrecs)
	seal := s.computeChainSeal(rec.RecordID, rec.Timestamp, rec.Confidence, 0)
	s.appendWALRecord(0x03, rec.RecordID, rec.Timestamp, seal)
}

func (s *Store) RegisterBlackSwan(swan growth.BlackSwanCase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldSt := s.state.Load()
	nextSwans := make([]growth.BlackSwanCase, len(oldSt.blackSwans)+1)
	copy(nextSwans, oldSt.blackSwans)
	nextSwans[len(oldSt.blackSwans)] = swan
	nextSt := &storeState{version: oldSt.version + 1, precedents: oldSt.precedents, blackSwans: nextSwans, laws: oldSt.laws}
	s.state.Store(nextSt)
	s.SwansRegistered.Add(1)
	s.appendWALRecord(0x04, swan.CaseID, time.Now().UnixNano(), s.lastChain)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.walFile != nil {
		err := s.walFile.Close()
		s.walFile = nil
		return err
	}
	return nil
}

func (s *Store) computeChainSeal(id uint64, ts int64, p1, p2 float32) [32]byte {
	h := sha256.New()
	h.Write(s.lastChain[:])
	var buf [24]byte
	binary.LittleEndian.PutUint64(buf[0:8], id)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(ts))
	binary.LittleEndian.PutUint32(buf[16:20], *(*uint32)(unsafe.Pointer(&p1)))
	binary.LittleEndian.PutUint32(buf[20:24], *(*uint32)(unsafe.Pointer(&p2)))
	h.Write(buf[:])
	var out [32]byte
	h.Sum(out[:0])
	copy(s.lastChain[:], out[:])
	return out
}

func (s *Store) appendWALRecord(recType uint8, id uint64, ts int64, seal [32]byte) {
	if s.walFile == nil {
		return
	}
	var frame [56]byte
	frame[0] = recType
	binary.LittleEndian.PutUint64(frame[8:16], id)
	binary.LittleEndian.PutUint64(frame[16:24], uint64(ts))
	copy(frame[24:56], seal[:])
	_, _ = s.walFile.Write(frame[:])
}
