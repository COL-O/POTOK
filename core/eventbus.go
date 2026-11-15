package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Event — событие на шине (общее поле, «нервы» организма).
// IsHelp — событие-помощь: модуль публикует не для себя, а другому слою
// (уровень «помогаю»). Priority — приоритет доставки при перегрузке поля.
type Event struct {
	Source   string
	Targets  []string
	Topic    string
	Payload  any
	IsHelp   bool
	Priority int
}

// Handler — обработчик события модуля.
type Handler func(ctx context.Context, ev Event) error

// EventBus — шина событий организма.
type EventBus interface {
	Publish(ctx context.Context, source string, targets []string, topic string, payload any) error
	Subscribe(ctx context.Context, subscriber string, topic string, h Handler) (unsubscribe func(), err error)
}

// ---- In-memory шина ----

type subscription struct {
	subscriber string
	handler    Handler
}

// MemoryBus — синхронная шина в памяти.
//
// Правило доставки: событие получают подписчики темы, которые либо указаны
// в Targets, либо являются самим источником (модуль слышит собственные
// события — так front обрабатывает свой FlightEvent и рождает FlightStuck).
// Обработчики вызываются синхронно, без удержания блокировки.
type MemoryBus struct {
	mu   sync.RWMutex
	subs map[string][]subscription
}

// NewMemoryBus — новая шина.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{subs: make(map[string][]subscription)}
}

// Subscribe — подписка модуля на тему; возвращает функцию отписки.
func (b *MemoryBus) Subscribe(_ context.Context, subscriber string, topic string, h Handler) (func(), error) {
	if h == nil {
		return nil, fmt.Errorf("bus: nil handler for %q/%q", subscriber, topic)
	}
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], subscription{subscriber: subscriber, handler: h})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			list := b.subs[topic]
			for i, s := range list {
				if s.subscriber == subscriber {
					b.subs[topic] = append(list[:i], list[i+1:]...)
					break
				}
			}
		})
	}, nil
}

// Publish — доставка события подписчикам (синхронно).
func (b *MemoryBus) Publish(ctx context.Context, source string, targets []string, topic string, payload any) error {
	return b.publish(ctx, Event{Source: source, Targets: targets, Topic: topic, Payload: payload})
}

// PublishHelp — публикация события-помощи другому слою (IsHelp=true).
func (b *MemoryBus) PublishHelp(ctx context.Context, source string, targets []string, topic string, payload any) error {
	return b.publish(ctx, Event{Source: source, Targets: targets, Topic: topic, Payload: payload, IsHelp: true})
}

func (b *MemoryBus) publish(ctx context.Context, ev Event) error {
	b.mu.RLock()
	list := append([]subscription(nil), b.subs[ev.Topic]...)
	b.mu.RUnlock()

	var errs []error
	for _, s := range list {
		if !addressed(s.subscriber, ev.Source, ev.Targets) {
			continue
		}
		if err := s.handler(ctx, ev); err != nil {
			errs = append(errs, fmt.Errorf("bus: %s on %s: %w", s.subscriber, ev.Topic, err))
		}
	}
	return errors.Join(errs...)
}

func addressed(subscriber, source string, targets []string) bool {
	if subscriber == source {
		return true // модуль слышит собственные события
	}
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == subscriber {
			return true
		}
	}
	return false
}
