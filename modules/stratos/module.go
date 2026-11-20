// Package stratos — слой «Память»: неизменяемая цепочка прецедентов.
// Ничего не командует — хранит и отдаёт по запросу (Recall), учит все слои.
package stratos

import (
	"context"

	"organism/core"
	"organism/modules/store"
)

// Module — модуль слоя stratos.
type Module struct {
	store  store.Store
	bus    core.EventBus
	chain  *Chain
	unsubs []func()
}

// New — новый модуль stratos со своей цепочкой.
func New() *Module { return &Module{chain: NewChain()} }

// SetBus — мостик links: модуль работает на общей шине организма.
func (m *Module) SetBus(b core.EventBus) { m.bus = b }

// Name — имя модуля в реестре.
func (m *Module) Name() string { return "stratos" }

// Init — получить шину из окружения.
func (m *Module) Init(ctx context.Context, env *core.Env) error {
	m.bus = env.Bus
	return nil
}

// Start — подписки на все значимые события слоёв.
func (m *Module) Start(ctx context.Context) error {
	topics := []string{"Diagnosis", "Suggestion", "FlightEvent"}
	for _, t := range topics {
		topic := t
		u, err := m.bus.Subscribe(ctx, "stratos", topic, m.onEvent)
		if err != nil {
			return err
		}
		m.unsubs = append(m.unsubs, u)
	}
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

func (m *Module) onEvent(ctx context.Context, ev core.Event) error {
	// Запись прецедента (append-only, с хешем).
	block := m.chain.Append(ev.Topic, ev.Source, ev.Payload)
	return m.storeBlock(ctx, block)
}

func (m *Module) storeBlock(ctx context.Context, b Block) error {
	if m.store == nil {
		return nil
	}
	key := []byte("precedent:" + b.Hash)
	return m.store.Put(ctx, key, b.Bytes())
}

// Recall — поиск прецедентов по теме (для других модулей):
// stratos не выбирает — отдаёт по запросу, слои решают сами.
func (m *Module) Recall(topic string) []Block { return m.chain.ByTopic(topic) }

// Chain — доступ к цепочке (состав, проверка целостности).
func (m *Module) Chain() *Chain { return m.chain }
