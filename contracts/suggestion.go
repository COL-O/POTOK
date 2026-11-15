package contracts

import (
	"fmt"
	"time"
)

// Deficit — недостача ресурса для сборки.
type Deficit struct {
	Meta
	ProductID uint32  `json:"product_id"`
	Need      float64 `json:"need"`
	Have      float64 `json:"have"`
	Short     float64 `json:"short"`
}

func (d *Deficit) Validate() error {
	if err := d.Meta.Validate(); err != nil {
		return err
	}
	if d.ProductID == 0 {
		return fmt.Errorf("deficit: product_id = 0")
	}
	if d.Need < 0 {
		return fmt.Errorf("deficit: need < 0 (%f)", d.Need)
	}
	if d.Have < 0 {
		return fmt.Errorf("deficit: have < 0 (%f)", d.Have)
	}
	if d.Short < 0 {
		return fmt.Errorf("deficit: short < 0 (%f)", d.Short)
	}
	expected := d.Need - d.Have
	if expected > 0 && d.Short != expected {
		return fmt.Errorf("deficit: short (%f) != need - have (%f)", d.Short, expected)
	}
	return nil
}

func (d *Deficit) IsHelp() bool { return true }

// SuggestionSource — откуда пришла возможность.
type SuggestionSource uint8

const (
	SourceReverseBOM SuggestionSource = iota
	SourceWhatIf
	SourceHistory
)

func (s SuggestionSource) String() string {
	switch s {
	case SourceReverseBOM:
		return "reverse_bom"
	case SourceWhatIf:
		return "what_if"
	case SourceHistory:
		return "history"
	default:
		return "unknown"
	}
}

// Suggestion — возможность (не решение).
type Suggestion struct {
	Meta
	SuggestionID string           `json:"suggestion_id"`
	ProductID    uint32           `json:"product_id"`
	CanBuild     float64          `json:"can_build"`
	Deficit      []Deficit        `json:"deficit,omitempty"`
	WorkCenters  []uint16         `json:"work_centers,omitempty"`
	Source       SuggestionSource `json:"source"`
	Confidence   float64          `json:"confidence"`
	ExpiresAt    time.Time        `json:"expires_at"`
}

func (s *Suggestion) Validate() error {
	if err := s.Meta.Validate(); err != nil {
		return err
	}
	if s.SuggestionID == "" {
		return fmt.Errorf("suggestion: нет suggestion_id")
	}
	if s.ProductID == 0 {
		return fmt.Errorf("suggestion: product_id = 0")
	}
	if s.CanBuild < 0 {
		return fmt.Errorf("suggestion: can_build < 0 (%f)", s.CanBuild)
	}
	if s.Source > SourceHistory {
		return fmt.Errorf("suggestion: неизвестный source %d", s.Source)
	}
	if s.Confidence < 0 || s.Confidence > 1 {
		return fmt.Errorf("suggestion: confidence вне [0,1] (%f)", s.Confidence)
	}
	for i := range s.Deficit {
		if err := s.Deficit[i].Validate(); err != nil {
			return fmt.Errorf("suggestion: deficit[%d]: %w", i, err)
		}
	}
	return nil
}

func (s *Suggestion) IsHelp() bool { return true }

func (s *Suggestion) HasDeficit() bool { return len(s.Deficit) > 0 }

func (s *Suggestion) TotalShort() float64 {
	var total float64
	for _, d := range s.Deficit {
		total += d.Short
	}
	return total
}

func (s *Suggestion) IsFullyBuildable() bool {
	return !s.HasDeficit() && s.CanBuild >= 1
}

// WorkCenter — рабочий центр (BIM).
type WorkCenter struct {
	Meta
	ID        uint16  `json:"id"`
	FactoryID uint16  `json:"factory_id"`
	Type      string  `json:"type"`
	Capacity  float64 `json:"capacity"`
	Load      float64 `json:"load"`
}

func (w *WorkCenter) Validate() error {
	if err := w.Meta.Validate(); err != nil {
		return err
	}
	if w.ID == 0 {
		return fmt.Errorf("work_center: id = 0")
	}
	if w.FactoryID == 0 {
		return fmt.Errorf("work_center: factory_id = 0")
	}
	if w.Type == "" {
		return fmt.Errorf("work_center: нет type")
	}
	if w.Capacity <= 0 {
		return fmt.Errorf("work_center: capacity <= 0 (%f)", w.Capacity)
	}
	if w.Load < 0 || w.Load > 1 {
		return fmt.Errorf("work_center: load вне [0,1] (%f)", w.Load)
	}
	return nil
}

func (w *WorkCenter) AvailableCapacity() float64 {
	return w.Capacity * (1 - w.Load)
}

func (w *WorkCenter) IsHelp() bool { return true }

// Connection — физический маршрут между рабочими центрами.
type Connection struct {
	Meta
	FromID     uint16 `json:"from_id"`
	ToID       uint16 `json:"to_id"`
	DistanceM  int32  `json:"distance_m"`
	TransitSec int32  `json:"transit_sec"`
}

func (c *Connection) Validate() error {
	if err := c.Meta.Validate(); err != nil {
		return err
	}
	if c.FromID == 0 || c.ToID == 0 {
		return fmt.Errorf("connection: from_id или to_id = 0")
	}
	if c.FromID == c.ToID {
		return fmt.Errorf("connection: from_id == to_id (%d)", c.FromID)
	}
	if c.DistanceM < 0 {
		return fmt.Errorf("connection: distance_m < 0 (%d)", c.DistanceM)
	}
	if c.TransitSec < 0 {
		return fmt.Errorf("connection: transit_sec < 0 (%d)", c.TransitSec)
	}
	return nil
}

func (c *Connection) IsHelp() bool { return true }
