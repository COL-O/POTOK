package bus

import "unsafe"

const (
	CacheLineSize = 64
	HDCWords      = 160
	NullOffset    = uint32(0xFFFFFFFF)
)

// LaneID — идентификатор дорожки (до 32 дорожек).
type LaneID uint8

const (
	LanePulse  LaneID = 0
	LaneHDC    LaneID = 1
	LaneGraph  LaneID = 2
	LaneTensor LaneID = 3
	LaneMeta   LaneID = 4
	LaneError  LaneID = 5
	LaneFree1  LaneID = 6
	LaneVector LaneID = 7
	LaneAudio  LaneID = 8
	LaneFree2  LaneID = 9
)

// LaneMask — битовая маска доступа к дорожкам (до 32 дорожек).
type LaneMask uint32

func (m LaneMask) Has(id LaneID) bool {
	return (m & (1 << id)) != 0
}

func (m LaneMask) With(id LaneID) LaneMask {
	return m | (1 << id)
}

// StateFlags — атомарные флаги состояния слота в Дорожке 0.
const (
	FlagReady            uint32 = 1 << 0
	FlagSurprise         uint32 = 1 << 1
	FlagEscalateToGrowth uint32 = 1 << 2
	FlagVetoTriggered    uint32 = 1 << 3
	FlagInQuarantine     uint32 = 1 << 4
	FlagHasError         uint32 = 1 << 5
	FlagCommitted        uint32 = 1 << 6
)

// LayerID — временной контур организма.
type LayerID uint8

const (
	LayerNone    LayerID = 0
	LayerFront   LayerID = 1
	LayerPolygon LayerID = 2
	LayerGrowth  LayerID = 3
	LayerStratos LayerID = 4
)

// EventHeader — Дорожка 0 («Пульс»).
type EventHeader struct {
	EventID      uint64
	Timestamp    int64
	Sequence     uint32
	StateFlags   uint32
	ActiveMask   uint32
	VetoMask     uint32
	Confidence   uint32
	TTS          uint32
	GraphOffset  uint32
	TensorOffset uint32
	MetaOffset   uint32
	OwnerLayer   LayerID
	Priority     uint8
	ErrorCode    uint16
	Reserved     [8]byte
}

var _ [64]byte = [unsafe.Sizeof(EventHeader{})]byte{}

// HDCSlot — Дорожка 1 («Нервный импульс»).
type HDCSlot struct {
	Words [HDCWords]uint64
}

var _ [1280]byte = [unsafe.Sizeof(HDCSlot{})]byte{}

// MetadataSlot — Дорожка 4 («Метаданные и эволюция»).
type MetadataSlot struct {
	EventID        uint64
	GenealogyID    uint64
	RuleVersion    uint32
	HitCount       uint32
	DecayScore     uint32
	VetoReasonCode uint32
	OriginOrganID  uint16
	Reserved       [30]byte
}

var _ [64]byte = [unsafe.Sizeof(MetadataSlot{})]byte{}

// ErrorSlot — Дорожка 5 («Контур честности»).
type ErrorSlot struct {
	EventID   uint64
	Timestamp int64
	OrganID   uint16
	ErrorCode uint16
	Severity  uint8
	MsgLen    uint8
	Message   [106]byte
}

var _ [128]byte = [unsafe.Sizeof(ErrorSlot{})]byte{}
