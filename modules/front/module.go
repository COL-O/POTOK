// Package front — слой «Рефлекс»: миллисекундная реакция на события поля.
// Пишет рейсы и паспорта в store, считает TTS, при сбое сразу зовёт polygon.
package front

import (
	"context"
	"encoding/json"

	"github.com/COL-O/POTOK/contracts"
	"github.com/COL-O/POTOK/core"
	"github.com/COL-O/POTOK/modules/store"
)

// Module — модуль слоя front.
type Module struct {
	store  store.Store
	bus    core.EventBus
	tts    *TTS
	unsubs []func()
}

// New — новый модуль front со своим счётчиком TTS.
func New() *Module { return &Module{tts: NewTTS()} }

// Name — имя модуля в реестре.
func (m *Module) Name() string { return "front" }

// Init — получить шину из окружения.
func (m *Module) Init(ctx context.Context, env *core.Env) error {
	m.bus = env.Bus
	return nil
}

// Start — подписки: свои рейсы и антитела от polygon.
func (m *Module) Start(ctx context.Context) error {
	u1, err := m.bus.Subscribe(ctx, "front", "FlightEvent", m.onFlightEvent)
	if err != nil {
		return err
	}
	m.unsubs = append(m.unsubs, u1)

	u2, err := m.bus.Subscribe(ctx, "front", "Antibody", m.onAntibody)
	if err != nil {
		return err
	}
	m.unsubs = append(m.unsubs, u2)
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

// SetStore — мостик links.ConnectFrontStore: хранилище модуля.
func (m *Module) SetStore(s store.Store) { m.store = s }

// TTS — доступ к счётчику времени жизни (для правил и тестов).
func (m *Module) TTS() *TTS { return m.tts }

func (m *Module) onFlightEvent(ctx context.Context, ev core.Event) error {
	flight, ok := ev.Payload.(contracts.FlightEvent)
	if !ok {
		return nil
	}

	// Запись в store.
	if m.store != nil {
		key := []byte("flight:" + flight.FlightID)
		val, _ := json.Marshal(flight)
		_ = m.store.Put(ctx, key, val)
	}

	// Обновление TTS (в памяти).
	m.tts.Update(flight.FactoryID, flight.Qty)

	// Если сбой — публикуем для polygon.
	if flight.Status == contracts.StatusStuck {
		return m.bus.Publish(ctx, "front",
			[]string{"polygon"}, "FlightStuck", flight)
	}
	return nil
}

func (m *Module) onAntibody(ctx context.Context, ev core.Event) error {
	// Антитело от polygon применено: если в payload рейс — перепроверить правило по TTS.
	if flight, ok := ev.Payload.(contracts.FlightEvent); ok {
		return m.RunRule(ctx, flight)
	}
	return nil
}

// Publish — публикация рейса (для внешних вызовов).
func (m *Module) Publish(ctx context.Context, flight contracts.FlightEvent) error {
	return m.bus.Publish(ctx, "front",
		[]string{"polygon", "growth", "stratos"}, "FlightEvent", flight)
}
