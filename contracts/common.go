package contracts

import (
	"errors"
	"time"
)

// Meta — общая «шапка» любого контракта.
// Знает: от кого, для кого, когда, в какой ветке.
type Meta struct {
	Source    string    `json:"source"`
	Targets   []string  `json:"targets,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Branch    string    `json:"branch"`
}

// Validate — проверка метаданных.
func (m Meta) Validate() error {
	if m.Source == "" {
		return ErrNoSource
	}
	if m.CreatedAt.IsZero() {
		return ErrNoCreatedAt
	}
	if m.Branch == "" {
		m.Branch = "main"
	}
	return nil
}

var (
	ErrNoSource    = errors.New("contracts: нет source")
	ErrNoCreatedAt = errors.New("contracts: нет created_at")
	ErrNoFlightID  = errors.New("contracts: нет flight_id")
	ErrNoTraceID   = errors.New("contracts: нет trace_id")
	ErrNoFactory   = errors.New("contracts: нет factory_id")
	ErrNoProduct   = errors.New("contracts: нет product_id")
	ErrBadStatus   = errors.New("contracts: неизвестный status")
	ErrNoPassport  = errors.New("contracts: нет passport_id")
	ErrNoRollback  = errors.New("contracts: нет rollback_id")
)
