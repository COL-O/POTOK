package contracts

import (
	"fmt"
	"time"
)

// FlightStatus — статус рейса.
type FlightStatus uint8

const (
	StatusPlanned     FlightStatus = iota
	StatusMoving
	StatusArrived
	StatusConsumed
	StatusReturned300
	StatusScrapped200
	StatusStuck
)

func (s FlightStatus) String() string {
	switch s {
	case StatusPlanned:
		return "planned"
	case StatusMoving:
		return "moving"
	case StatusArrived:
		return "arrived"
	case StatusConsumed:
		return "consumed"
	case StatusReturned300:
		return "returned_300"
	case StatusScrapped200:
		return "scrapped_200"
	case StatusStuck:
		return "stuck"
	default:
		return "unknown"
	}
}

// FlightEvent — рейс материального потока.
type FlightEvent struct {
	Meta

	FlightID     string `json:"flight_id"`
	TraceID      string `json:"trace_id"`
	ParentFlight string `json:"parent_flight,omitempty"`

	FactoryID uint16 `json:"factory_id"`
	ToFactory uint16 `json:"to_factory"`
	ProductID uint32 `json:"product_id"`

	Qty    float64      `json:"qty"`
	Status FlightStatus `json:"status"`
	ETA    time.Time    `json:"eta,omitempty"`
	Note   string       `json:"note,omitempty"`
}

func (e *FlightEvent) Validate() error {
	if err := e.Meta.Validate(); err != nil {
		return err
	}
	if e.FlightID == "" {
		return ErrNoFlightID
	}
	if e.TraceID == "" {
		return ErrNoTraceID
	}
	if e.FactoryID == 0 {
		return ErrNoFactory
	}
	if e.ProductID == 0 {
		return ErrNoProduct
	}
	if e.Qty < 0 {
		return fmt.Errorf("flight: qty < 0 (%f)", e.Qty)
	}
	if e.Status > StatusStuck {
		return ErrBadStatus
	}
	return nil
}

func (e *FlightEvent) IsHelp() bool { return true }
