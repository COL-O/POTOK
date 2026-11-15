package polygon

// Antibody — готовое правило для front.
// polygon думает и готовит антитело; front не думает — применяет.
type Antibody struct {
	Action  string `json:"action"`
	Payload any    `json:"payload"`
}
