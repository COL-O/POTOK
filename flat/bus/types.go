package bus

const (
	MaxLanes        = 10
	MaxActiveOrgans = 32
	HDCWords        = 160 // 160 * 64 = 10 240 бит
	NullOffset      = ^uint32(0)
)

type LayerID uint8

const (
	LayerBus     LayerID = 0
	LayerFront   LayerID = 1
	LayerPolygon LayerID = 2
	LayerGrowth  LayerID = 3
	LayerStratos LayerID = 4
)

type LaneID uint8

const (
	LanePulse  LaneID = 0 // 64B EventHeader
	LaneHDC    LaneID = 1 // 1280B HDCSlot
	LaneGraph  LaneID = 2 // CSR Graph Arena
	LaneTensor LaneID = 3 // 4D Tensor Arena
	LaneMeta   LaneID = 4 // 64B MetaSlot
	LaneError  LaneID = 5 // 128B ErrorSlot
	LaneFree1  LaneID = 6 // Резерв КОН 8
	LaneVector LaneID = 7 // 1D Vector Arena
	LaneAudio  LaneID = 8 // PCM16 Audio Arena
	LaneFree2  LaneID = 9 // Резерв КОН 8
)

type LaneMask uint32

func (m LaneMask) With(id LaneID) LaneMask { return m | (1 << id) }
func (m LaneMask) Has(id LaneID) bool      { return (m & (1 << id)) != 0 }

type StateFlags uint32

const (
	FlagReady            StateFlags = 1 << 0
	FlagSurprise         StateFlags = 1 << 1
	FlagVetoApplied      StateFlags = 1 << 2
	FlagEscalateToGrowth StateFlags = 1 << 3
	FlagHintDrift        StateFlags = 1 << 4
	FlagCommitted        StateFlags = 1 << 5
)

type StatusCode uint8

const (
	StatusOK          StatusCode = 1
	StatusAbstain     StatusCode = 2
	StatusOutOfBounds StatusCode = 3
	StatusVetoed      StatusCode = 4
	StatusError       StatusCode = 5
)

// EventHeader — ровно 64 байта (1 кэш-линия CPU, выравнивание SOA).
type EventHeader struct {
	EventID     uint64     // 0..8
	Timestamp   int64      // 8..16
	Sequence    uint32     // 16..20 (Epoch Fencing)
	Flags       uint32     // 20..24 (StateFlags)
	ActiveMask  uint32     // 24..28
	VetoMask    uint32     // 28..32
	Confidence  float32    // 32..36
	TTS         uint32     // 36..40
	GraphOffset uint32     // 40..44
	TensorOff   uint32     // 44..48
	VectorOff   uint32     // 48..52
	AudioOff    uint32     // 52..56
	ActionCode  uint32     // 56..60
	OwnerLayer  uint8      // 60..61
	Priority    uint8      // 61..62
	ErrorCode   uint16     // 62..64
}

// HDCSlot — 1280 байт (20 кэш-линий, 10 240 бит).
type HDCSlot struct {
	Words [HDCWords]uint64
}
