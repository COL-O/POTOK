// Package growth — слой «Рост»: ищет возможности, стыкует излишки
// с гипотезами, отпочковывает почки. Предложения уходят прецедентами
// в stratos; projector считает «что можно собрать» в обратном BOM-проходе.
package growth

import (
	"context"
	"encoding/json"

	"organism/contracts"
	"organism/core"
	"organism/modules/growth/projector"
	"organism/modules/store"
)

// Module — модуль слоя growth.
type Module struct {
	store  store.Store
	bus    core.EventBus
	proj   *projector.Projector
	unsubs []func()
}

// New — новый модуль growth с projector-ом (может быть nil).
func New(p *projector.Projector) *Module { return &Module{proj: p} }

// Name — имя модуля в реестре.
func (m *Module) Name() string { return "growth" }

// Init — получить шину из окружения.
func (m *Module) Init(ctx context.Context, env *core.Env) error {
	m.bus = env.Bus
	return nil
}

// Start — подписка на диагнозы polygon.
func (m *Module) Start(ctx context.Context) error {
	u, err := m.bus.Subscribe(ctx, "growth", "Diagnosis", m.onDiagnosis)
	if err != nil {
		return err
	}
	m.unsubs = append(m.unsubs, u)
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

func (m *Module) onDiagnosis(ctx context.Context, ev core.Event) error {
	// Ловец излишков: если диагноз = stuck, значит ресурс застрял.
	diag, ok := ev.Payload.(map[string]any)
	if !ok {
		return nil
	}
	if diag["reason"] != "stuck_on_route" {
		return nil
	}

	// Стыковка с гипотезой: предложить почку.
	if m.proj != nil {
		sugg := m.proj.Suggest()
		if len(sugg) > 0 {
			for _, s := range sugg {
				if err := m.storeSuggestion(ctx, s); err != nil {
					return err
				}
				_ = m.bus.Publish(ctx, "growth", []string{"stratos"}, "Suggestion", s)
			}
		}
	}
	return nil
}

func (m *Module) storeSuggestion(ctx context.Context, s contracts.Suggestion) error {
	if m.store == nil {
		return nil
	}
	key := []byte("suggestion:" + s.SuggestionID)
	val, _ := json.Marshal(s)
	return m.store.Put(ctx, key, val)
}
