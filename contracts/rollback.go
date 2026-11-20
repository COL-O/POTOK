package contracts

import (
	"fmt"
	"time"
)

// RollbackKind — тип отката.
type RollbackKind uint8

const (
	Rollback300 RollbackKind = iota
	Rollback200
)

func (k RollbackKind) String() string {
	switch k {
	case Rollback300:
		return "300_repair"
	case Rollback200:
		return "200_recycle"
	default:
		return "unknown"
	}
}

// RollbackEvent — откат потока (Вены).
type RollbackEvent struct {
	Meta

	RollbackID  string       `json:"rollback_id"`
	FlightID    string       `json:"flight_id"`
	TraceID     string       `json:"trace_id"`
	Kind        RollbackKind `json:"kind"`
	FromFactory uint16       `json:"from_factory"`
	ToFactory   uint16       `json:"to_factory"`
	Qty         float64      `json:"qty"`
	Reason      string       `json:"reason,omitempty"`
	ReturnAt    time.Time    `json:"return_at,omitempty"`
}

func (r *RollbackEvent) Validate() error {
	if err := r.Meta.Validate(); err != nil {
		return err
	}
	if r.RollbackID == "" {
		return ErrNoRollback
	}
	if r.FlightID == "" {
		return ErrNoFlightID
	}
	if r.FromFactory == 0 || r.ToFactory == 0 {
		return ErrNoFactory
	}
	if r.Kind > Rollback200 {
		return fmt.Errorf("rollback: неизвестный kind %d", r.Kind)
	}
	if r.Qty <= 0 {
		return fmt.Errorf("rollback: qty <= 0 (%f)", r.Qty)
	}
	return nil
}

func (r *RollbackEvent) IsHelp() bool { return true }
