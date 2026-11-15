// Package core — ядро организма: шина событий, реестр модулей, слои.
//
// Минимальная реализация под скелет автора: синхронная in-memory шина
// (доставка в том же вызове Publish — так цепочка front→polygon→growth→
// stratos проходит за один проход main) и реестр модулей с жизненным
// циклом Init → Start → Stop.
package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Event — событие на шине. Topic — тема, Source — опубликовавший модуль,
// Targets — кому адресовано, Payload — данные (типы из contracts).
type Event struct {
	Topic   string
	Source  string
	Targets []string
	Payload any
}

// Handler — обработчик события модуля.
type Handler func(ctx context.Context, ev Event) error

// EventBus — шина событий организма (общее поле обмена между слоями).
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
	b.mu.RLock()
	list := append([]subscription(nil), b.subs[topic]...)
	b.mu.RUnlock()

	ev := Event{Topic: topic, Source: source, Targets: targets, Payload: payload}
	var errs []error
	for _, s := range list {
		if !addressed(s.subscriber, source, targets) {
			continue
		}
		if err := s.handler(ctx, ev); err != nil {
			errs = append(errs, fmt.Errorf("bus: %s on %s: %w", s.subscriber, topic, err))
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

// ---- Модули и слои ----

// Role — роль слоя в организме.
type Role string

const (
	RoleReflex   Role = "reflex"
	RoleImmunity Role = "immunity"
	RoleGrowth   Role = "growth"
	RoleMemory   Role = "memory"
)

// Layer — слой организма.
type Layer string

const (
	LayerFront   Layer = "front"
	LayerPolygon Layer = "polygon"
	LayerGrowth  Layer = "growth"
	LayerStratos Layer = "stratos"
)

// Manifest — паспорт модуля в реестре.
type Manifest struct {
	Name    string
	Role    Role
	Layer   Layer
	Enabled bool
}

// Env — окружение, которое модуль получает при Init.
type Env struct {
	Bus EventBus
}

// Module — модуль организма (front, polygon, growth, stratos, ...).
type Module interface {
	Name() string
	Init(ctx context.Context, env *Env) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Registry — реестр модулей: регистрация, старт всех, остановка всех.
type Registry struct {
	bus     EventBus
	modules []Module
	byName  map[string]Module
}

// NewRegistry — реестр с собственной in-memory шиной.
func NewRegistry() *Registry {
	return &Registry{bus: NewMemoryBus(), byName: make(map[string]Module)}
}

// Bus — шина реестра (для мостиков и внешних публикаций).
func (r *Registry) Bus() EventBus { return r.bus }

// Register — зарегистрировать модуль с манифестом.
func (r *Registry) Register(m Module, mf Manifest) error {
	if m == nil {
		return fmt.Errorf("registry: nil module")
	}
	name := m.Name()
	if name == "" {
		return fmt.Errorf("registry: module with empty name")
	}
	if !mf.Enabled {
		return nil // выключенный модуль не регистрируем
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("registry: duplicate module %q", name)
	}
	r.byName[name] = m
	r.modules = append(r.modules, m)
	return nil
}

// StartAll — Init + Start всех модулей в порядке регистрации.
func (r *Registry) StartAll(ctx context.Context) error {
	env := &Env{Bus: r.bus}
	for _, m := range r.modules {
		if err := m.Init(ctx, env); err != nil {
			return fmt.Errorf("registry: init %s: %w", m.Name(), err)
		}
	}
	for _, m := range r.modules {
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("registry: start %s: %w", m.Name(), err)
		}
	}
	return nil
}

// StopAll — Stop всех модулей в обратном порядке. Ошибки собираются.
func (r *Registry) StopAll(ctx context.Context) error {
	var errs []error
	for i := len(r.modules) - 1; i >= 0; i-- {
		if err := r.modules[i].Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("registry: stop %s: %w", r.modules[i].Name(), err))
		}
	}
	return errors.Join(errs...)
}
