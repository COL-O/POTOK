package core

import (
	"context"
	"sync"
	"sync/atomic"
)

type Event struct {
	Source  string
	Targets []string
	Topic   string
	Payload any
}

type EventBus interface {
	Publish(ctx context.Context, source string, targets []string, topic string, payload any) error
	Subscribe(ctx context.Context, moduleName, topic string, handler func(ctx context.Context, ev Event) error) (unsubscribe func(), err error)
}

type bus struct {
	mu       sync.RWMutex
	handlers map[string][]subscriber
	nextID   atomic.Uint64
}

type subscriber struct {
	id      uint64
	module  string
	handler func(ctx context.Context, ev Event) error
}

func NewEventBus() EventBus {
	return &bus{handlers: make(map[string][]subscriber)}
}

func (b *bus) Publish(ctx context.Context, source string, targets []string, topic string, payload any) error {
	ev := Event{Source: source, Targets: targets, Topic: topic, Payload: payload}
	b.mu.RLock()
	subs := append([]subscriber(nil), b.handlers[topic]...)
	b.mu.RUnlock()
	for _, s := range subs {
		if len(targets) > 0 && !contains(targets, s.module) {
			continue
		}
		// Синхронный вызов: при `go func` горутина переживала тест и работала
		// после Stop, что давало гонки под -race и «висящие» обработчики.
		_ = s.handler(ctx, ev)
	}
	return nil
}

func (b *bus) Subscribe(ctx context.Context, moduleName, topic string, handler func(ctx context.Context, ev Event) error) (func(), error) {
	id := b.nextID.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], subscriber{id: id, module: moduleName, handler: handler})
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			subs := b.handlers[topic]
			for i, s := range subs {
				// Отписка по идентификатору подписки: сравнение адресов
				// (&s.handler == &handler) не могло быть истинным никогда —
				// в срезе лежит копия, и подписки не снимались.
				if s.id == id {
					b.handlers[topic] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
		})
	}, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
