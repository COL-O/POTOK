package stratos

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/bits"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/COL-O/POTOK/modules/bus"
)

const (
	OrganIDHDCIndex   uint8 = 19
	OrganIDLSHHNSW    uint8 = 20
	OrganIDRetNet     uint8 = 21
	OrganIDMPIndex    uint8 = 22
	OrganIDSHA256Seal uint8 = 23

	BitHDCIndex   uint32 = 1 << OrganIDHDCIndex
	BitLSHHNSW    uint32 = 1 << OrganIDLSHHNSW
	BitRetNet     uint32 = 1 << OrganIDRetNet
	BitMPIndex    uint32 = 1 << OrganIDMPIndex
	BitSHA256Seal uint32 = 1 << OrganIDSHA256Seal
)

// HDCRecord — запись прецедента в долгой памяти.
type HDCRecord struct {
	RecordID        uint64
	Timestamp       int64
	Pattern         bus.HDCSlot
	CauseCode       uint32
	AntibodyAction  uint32
	RecommendedMask uint32
	Confidence      float32
	IsToxic         bool
}

type HDCIndexOrgan struct {
	records atomic.Pointer[[]HDCRecord]
	MinSim  float32
}

func NewHDCIndexOrgan(initial []HDCRecord, minSim float32) *HDCIndexOrgan {
	if minSim <= 0 {
		minSim = 0.85
	}
	o := &HDCIndexOrgan{MinSim: minSim}
	o.records.Store(&initial)
	return o
}

func (o *HDCIndexOrgan) UpdateIndexRCU(next []HDCRecord) { o.records.Store(&next) }

func (o *HDCIndexOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDHDCIndex,
		Name:        "HDCIndex",
		Layer:       bus.LayerStratos,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 5 * time.Millisecond,
		HardTimeout: 20 * time.Millisecond,
	}
}

func (o *HDCIndexOrgan) InBounds(view bus.TickView) bool { return view.HDC != nil }
func (o *HDCIndexOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *HDCIndexOrgan) CoolDown()                        {}

func (o *HDCIndexOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDHDCIndex, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	recs := *o.records.Load()
	const totalBits = float32(bus.HDCWords * 64)
	var bestSim float32
	var bestRec *HDCRecord
	for i := range recs {
		r := &recs[i]
		if r.IsToxic {
			continue
		}
		var diff int
		for w := 0; w < bus.HDCWords; w++ {
			diff += bits.OnesCount64(view.HDC.Words[w] ^ r.Pattern.Words[w])
		}
		sim := 1.0 - (float32(diff) / totalBits)
		if sim > bestSim {
			bestSim = sim
			bestRec = r
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if bestRec == nil || bestSim < o.MinSim {
		return bus.Verdict{OrganID: OrganIDHDCIndex, Status: bus.StatusAbstain, Confidence: bestSim},
			bus.OrganMetrics{LatencyNs: lat, Confidence: bestSim, Status: bus.StatusAbstain}
	}
	if len(view.Scratch) >= 8 {
		*(*uint64)(unsafe.Pointer(&view.Scratch[0])) = bestRec.RecordID
	}
	return bus.Verdict{OrganID: OrganIDHDCIndex, Status: bus.StatusOK, Confidence: bestSim * bestRec.Confidence, ActionCode: bestRec.AntibodyAction, SuggestedMask: bestRec.RecommendedMask},
		bus.OrganMetrics{LatencyNs: lat, Confidence: bestSim * bestRec.Confidence, Status: bus.StatusOK}
}

// HNSWNode — узел графа навигации.
type HNSWNode struct {
	NodeID    uint32
	RecordID  uint64
	BucketLSH uint32
	Vector    []float32
	Neighbors []uint32
}

type LSHHNSWOrgan struct {
	graph  atomic.Pointer[[]HNSWNode]
	MinCos float32
}

func NewLSHHNSWOrgan(nodes []HNSWNode, minCos float32) *LSHHNSWOrgan {
	if minCos <= 0 {
		minCos = 0.80
	}
	o := &LSHHNSWOrgan{MinCos: minCos}
	o.graph.Store(&nodes)
	return o
}

func (o *LSHHNSWOrgan) UpdateGraphRCU(next []HNSWNode) { o.graph.Store(&next) }

func (o *LSHHNSWOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDLSHHNSW,
		Name:        "LSH+HNSW",
		Layer:       bus.LayerStratos,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneVector),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 10 * time.Millisecond,
		HardTimeout: 50 * time.Millisecond,
	}
}

func (o *LSHHNSWOrgan) InBounds(view bus.TickView) bool { return len(view.Vector) > 0 }
func (o *LSHHNSWOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *LSHHNSWOrgan) CoolDown()                        {}

func (o *LSHHNSWOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDLSHHNSW, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	nodes := *o.graph.Load()
	if len(nodes) == 0 {
		return bus.Verdict{OrganID: OrganIDLSHHNSW, Status: bus.StatusAbstain},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusAbstain}
	}
	queryBucket := computeSignLSH(view.Vector)
	entryIdx := 0
	for i := range nodes {
		if nodes[i].BucketLSH == queryBucket {
			entryIdx = i
			break
		}
	}
	curIdx := entryIdx
	bestCos := cosineSim(view.Vector, nodes[curIdx].Vector)
	for step := 0; step < 32; step++ {
		improved := false
		for _, nb := range nodes[curIdx].Neighbors {
			if int(nb) >= len(nodes) {
				continue
			}
			sim := cosineSim(view.Vector, nodes[nb].Vector)
			if sim > bestCos {
				bestCos = sim
				curIdx = int(nb)
				improved = true
			}
		}
		if !improved {
			break
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if bestCos < o.MinCos {
		return bus.Verdict{OrganID: OrganIDLSHHNSW, Status: bus.StatusAbstain, Confidence: bestCos},
			bus.OrganMetrics{LatencyNs: lat, Confidence: bestCos, Status: bus.StatusAbstain}
	}
	return bus.Verdict{OrganID: OrganIDLSHHNSW, Status: bus.StatusOK, Confidence: bestCos, ActionCode: uint32(nodes[curIdx].RecordID)},
		bus.OrganMetrics{LatencyNs: lat, Confidence: bestCos, Status: bus.StatusOK}
}

func computeSignLSH(v []float32) uint32 {
	var h uint32
	limit := len(v)
	if limit > 32 {
		limit = 32
	}
	for i := 0; i < limit; i++ {
		if v[i] >= 0 {
			h |= 1 << i
		}
	}
	return h
}

func cosineSim(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := float32(math.Sqrt(float64(normA * normB)))
	if denom < 1e-6 {
		return 0
	}
	return dot / denom
}

// RetNetOrgan — память с экспоненциальным затуханием.
type RetNetOrgan struct {
	Gamma float32
}

func (o *RetNetOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDRetNet,
		Name:        "RetNet",
		Layer:       bus.LayerStratos,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyWarmMmap,
		SoftTimeout: 5 * time.Millisecond,
		HardTimeout: 25 * time.Millisecond,
	}
}

func (o *RetNetOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) > 0 }
func (o *RetNetOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *RetNetOrgan) CoolDown()                        {}

func (o *RetNetOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDRetNet, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	gamma := o.Gamma
	if gamma <= 0 || gamma >= 1 {
		gamma = 0.95
	}
	var retentionState float32
	for _, x := range view.Tensor.Data {
		retentionState = gamma*retentionState + x
	}
	lat := uint64(time.Now().UnixNano() - start)
	action := uint32(1)
	if math.Abs(float64(retentionState)) > 5.0 {
		action = 2
	}
	return bus.Verdict{OrganID: OrganIDRetNet, Status: bus.StatusOK, Confidence: 0.90, ActionCode: action},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.90, Status: bus.StatusOK}
}

// HistoricalMotif — исторический мотив.
type HistoricalMotif struct {
	MotifID    uint64
	Pattern    []float32
	MaxDist    float32
	IsDisaster bool
}

type MatrixProfileIndexOrgan struct {
	motifs atomic.Pointer[[]HistoricalMotif]
}

func NewMatrixProfileIndexOrgan(initial []HistoricalMotif) *MatrixProfileIndexOrgan {
	o := &MatrixProfileIndexOrgan{}
	o.motifs.Store(&initial)
	return o
}

func (o *MatrixProfileIndexOrgan) UpdateMotifsRCU(next []HistoricalMotif) { o.motifs.Store(&next) }

func (o *MatrixProfileIndexOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDMPIndex,
		Name:        "MatrixProfileIndex",
		Layer:       bus.LayerStratos,
		ReadLanes:   bus.LaneMask(0).With(bus.LaneTensor),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyColdMmap,
		SoftTimeout: 15 * time.Millisecond,
		HardTimeout: 60 * time.Millisecond,
	}
}

func (o *MatrixProfileIndexOrgan) InBounds(view bus.TickView) bool { return len(view.Tensor.Data) >= 4 }
func (o *MatrixProfileIndexOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *MatrixProfileIndexOrgan) CoolDown()                        {}

func (o *MatrixProfileIndexOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDMPIndex, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	motifs := *o.motifs.Load()
	series := view.Tensor.Data
	for i := range motifs {
		m := &motifs[i]
		w := len(m.Pattern)
		if w == 0 || len(series) < w {
			continue
		}
		tail := series[len(series)-w:]
		var distSq float32
		for k := 0; k < w; k++ {
			d := tail[k] - m.Pattern[k]
			distSq += d * d
		}
		dist := float32(math.Sqrt(float64(distSq)))
		if dist <= m.MaxDist {
			lat := uint64(time.Now().UnixNano() - start)
			conf := 1.0 - (dist / (m.MaxDist + 1e-5))
			return bus.Verdict{OrganID: OrganIDMPIndex, Status: bus.StatusOK, Confidence: conf, ActionCode: uint32(m.MotifID)},
				bus.OrganMetrics{LatencyNs: lat, Confidence: conf, Status: bus.StatusOK}
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDMPIndex, Status: bus.StatusAbstain},
		bus.OrganMetrics{LatencyNs: lat, Status: bus.StatusAbstain}
}

// SHA256SealOrgan — криптографическая печать целостности.
type SHA256SealOrgan struct {
	lastHash atomic.Value
}

func NewSHA256SealOrgan() *SHA256SealOrgan {
	o := &SHA256SealOrgan{}
	o.lastHash.Store([32]byte{})
	return o
}

func (o *SHA256SealOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDSHA256Seal,
		Name:        "SHA-256",
		Layer:       bus.LayerStratos,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneHDC),
		WriteLane:   bus.LaneMeta,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 2 * time.Millisecond,
		HardTimeout: 10 * time.Millisecond,
	}
}

func (o *SHA256SealOrgan) InBounds(view bus.TickView) bool { return view.Header != nil }
func (o *SHA256SealOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *SHA256SealOrgan) CoolDown()                        {}

func (o *SHA256SealOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDSHA256Seal, Status: bus.StatusOutOfBounds},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
	}
	prev := o.lastHash.Load().([32]byte)
	h := sha256.New()
	h.Write(prev[:])
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], view.Header.EventID)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(view.Header.Timestamp))
	h.Write(buf[:])
	if view.HDC != nil {
		rawHDC := unsafe.Slice((*byte)(unsafe.Pointer(&view.HDC.Words[0])), bus.HDCWords*8)
		h.Write(rawHDC)
	}
	var nextHash [32]byte
	h.Sum(nextHash[:0])
	o.lastHash.Store(nextHash)
	sealCode := binary.LittleEndian.Uint32(nextHash[0:4])
	lat := uint64(time.Now().UnixNano() - start)
	return bus.Verdict{OrganID: OrganIDSHA256Seal, Status: bus.StatusOK, Confidence: 1.0, ActionCode: sealCode},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 1.0, Status: bus.StatusOK}
}
