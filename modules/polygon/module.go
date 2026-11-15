// Package polygon — слой «Иммунитет»: неспешный разбор отклонений front.
// Ставит диагноз, готовит антитело для front, предложения — в growth,
// всё случившееся — прецедентом в stratos (через шину).
package polygon

import (
	"context"
	"encoding/json"

	"github.com/COL-O/POTOK/contracts"
	"github.com/COL-O/POTOK/core"
	"github.com/COL-O/POTOK/modules/graph"
	"github.com/COL-O/POTOK/modules/store"
)

// Module — модуль слоя polygon.
type Module struct {
	store  store.Store
	bus    core.EventBus
	bom    *graph.BOMGraph
	unsubs []func()
}

// New — новый модуль polygon.
func New() *Module { return &Module{} }

// Name — имя модуля в реестре.
func (m *Module) Name() string { return "polygon" }

// Init — получить шину из окружения.
func (m *Module) Init(ctx context.Context, env *core.Env) error {
	m.bus = env.Bus
	return nil
}

// Start — подписки: сбои и низкий TTS от front.
func (m *Module) Start(ctx context.Context) error {
	u1, err := m.bus.Subscribe(ctx, "polygon", "FlightStuck", m.onStuck)
	if err != nil {
		return err
	}
	u2, err := m.bus.Subscribe(ctx, "polygon", "TTSLow", m.onTTSLow)
	if err != nil {
		return err
	}
	m.unsubs = append(m.unsubs, u1, u2)
	return nil
}

// Stop — отписаться от всех.
func (m *Module) Stop(ctx context.Context) error {
	for _, u := range m.unsubs {
		u()
	}
	m.unsubs = nil
	return nil
}

// SetStore — мостик: хранилище модуля.
func (m *Module) SetStore(s store.Store) { m.store = s }

// SetBOM — мостик: BOM-граф для диагнозов по структуре изделий.
func (m *Module) SetBOM(b *graph.BOMGraph) { m.bom = b }

func (m *Module) onStuck(ctx context.Context, ev core.Event) error {
	flight, ok := ev.Payload.(contracts.FlightEvent)
	if !ok {
		return nil
	}

	// Диагноз: застрял на маршруте.
	diag := map[string]any{
		"flight_id":  flight.FlightID,
		"factory_id": flight.FactoryID,
		"reason":     "stuck_on_route",
	}

	// Запись в store (diagnosis:*).
	if m.store != nil {
		key := []byte("diagnosis:" + flight.FlightID)
		val, _ := json.Marshal(diag)
		_ = m.store.Put(ctx, key, val)
	}

	// Публикуем диагноз для growth.
	return m.bus.Publish(ctx, "polygon",
		[]string{"growth", "stratos"}, "Diagnosis", diag)
}

func (m *Module) onTTSLow(ctx context.Context, ev core.Event) error {
	// Антитело: перенаправить на соседний завод.
	antibody := map[string]any{
		"action": "redirect_to_neighbor",
		"event":  ev.Payload,
	}
	return m.bus.Publish(ctx, "polygon", []string{"front"}, "Antibody", antibody)
}
