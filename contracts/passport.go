package contracts

import (
	"fmt"
	"time"
)

// PassportResult — результат ОТК.
type PassportResult uint8

const (
	PassportPassed PassportResult = iota
	PassportFailed
	PassportRework
)

func (r PassportResult) String() string {
	switch r {
	case PassportPassed:
		return "passed"
	case PassportFailed:
		return "failed"
	case PassportRework:
		return "rework"
	default:
		return "unknown"
	}
}

// QualityPassport — паспорт качества.
type QualityPassport struct {
	Meta

	PassportID  string         `json:"passport_id"`
	FlightID    string         `json:"flight_id"`
	TraceID     string         `json:"trace_id"`
	Result      PassportResult `json:"result"`
	Reason      string         `json:"reason,omitempty"`
	InspectedAt time.Time      `json:"inspected_at"`
	Inspector   string         `json:"inspector,omitempty"`
}

func (p *QualityPassport) Validate() error {
	if err := p.Meta.Validate(); err != nil {
		return err
	}
	if p.PassportID == "" {
		return ErrNoPassport
	}
	if p.FlightID == "" {
		return ErrNoFlightID
	}
	if p.Result > PassportRework {
		return fmt.Errorf("passport: неизвестный result %d", p.Result)
	}
	return nil
}

func (p *QualityPassport) IsHelp() bool { return true }
