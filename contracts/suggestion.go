package contracts

import "fmt"

// Deficit — чего не хватает, чтобы собрать изделие (обратный отсчёт projector-а).
type Deficit struct {
	ProductID uint32  `json:"product_id"`
	FactoryID uint16  `json:"factory_id,omitempty"`
	Qty       float64 `json:"qty"`
	Reason    string  `json:"reason,omitempty"`
}

// SuggestionSource — откуда пришла подсказка projector-а.
type SuggestionSource string

const (
	SourceReverseBOM SuggestionSource = "reverse_bom"
)

// Suggestion — предложение projector-а: что можно собрать (can_build)
// и где дефицит. Read-only подсказка слою growth.
type Suggestion struct {
	Meta         Meta             `json:"meta"`
	SuggestionID string           `json:"suggestion_id"`
	ProductID    uint32           `json:"product_id"`
	CanBuild     float64          `json:"can_build"`
	Deficits     []Deficit        `json:"deficits,omitempty"`
	Source       SuggestionSource `json:"source"`
	Confidence   float64          `json:"confidence"`
}

// WorkCenter — рабочий центр завода (источник capacity, слой BIM).
type WorkCenter struct {
	Meta      Meta    `json:"meta"`
	ID        uint16  `json:"id"`
	FactoryID uint16  `json:"factory_id"`
	Type      string  `json:"type"`
	Capacity  float64 `json:"capacity"` // операций/час
	Load      float64 `json:"load"`     // 0..1
}

// AvailableCapacity — свободная мощность центра.
func (w WorkCenter) AvailableCapacity() float64 {
	if w.Load >= 1 {
		return 0
	}
	return w.Capacity * (1 - w.Load)
}

// Validate — минимальная проверка рабочего центра.
func (w WorkCenter) Validate() error {
	if w.FactoryID == 0 {
		return fmt.Errorf("work center %d: empty factory", w.ID)
	}
	if w.Load < 0 || w.Load > 1 {
		return fmt.Errorf("work center %d: load out of range", w.ID)
	}
	return nil
}

// Connection — связь между рабочими центрами (маршрут).
type Connection struct {
	Meta       Meta   `json:"meta"`
	FromID     uint16 `json:"from_id"`
	ToID       uint16 `json:"to_id"`
	DistanceM  int32  `json:"distance_m"`
	TransitSec int32  `json:"transit_sec"`
}

// Validate — связь не может вести в саму себя.
func (c Connection) Validate() error {
	if c.FromID == c.ToID {
		return fmt.Errorf("connection: from == to (%d)", c.FromID)
	}
	return nil
}
