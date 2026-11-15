package contracts

// FlightStatus — статус рейса (перемещения) между заводами.
type FlightStatus string

const (
	StatusMoving FlightStatus = "moving"
	StatusStuck  FlightStatus = "stuck"
)

// FlightEvent — рейс: продукт едет с завода на завод.
// Слой рейсов: ETA, стыковка, TTS, правило (см. концепт слоёв).
type FlightEvent struct {
	Meta      Meta         `json:"meta"`
	FlightID  string       `json:"flight_id"`
	TraceID   string       `json:"trace_id"`
	FactoryID uint16       `json:"factory_id"`
	ToFactory uint16       `json:"to_factory"`
	ProductID uint32       `json:"product_id"`
	Qty       float64      `json:"qty"`
	Status    FlightStatus `json:"status"`
}
