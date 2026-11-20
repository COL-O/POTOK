package bus

type Verdict struct {
	OrganID       uint8
	Status        StatusCode
	Confidence    float32
	ActionCode    uint32
	SuggestedMask uint32
}

type OrganMetrics struct {
	LatencyNs  uint64
	Confidence float32
	Status     StatusCode
}
