package bus

// VerdictStatus — статус выполнения органа.
type VerdictStatus uint8

const (
	StatusOK          VerdictStatus = 1
	StatusAbstain     VerdictStatus = 2
	StatusOutOfBounds VerdictStatus = 3
	StatusTimeout     VerdictStatus = 4
	StatusError       VerdictStatus = 5
)

// Verdict — унифицированный ответ любого из органов.
type Verdict struct {
	OrganID       uint8
	Status        VerdictStatus
	Confidence    float32
	ActionCode    uint32
	SuggestedMask uint32
	ResultOffset  uint32
}

// OrganMetrics — телеметрия такта органа.
type OrganMetrics struct {
	LatencyNs   uint64
	ScratchUsed uint32
	Confidence  float32
	Status      VerdictStatus
}
