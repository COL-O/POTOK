package core

import (
	"context"
	"sync"
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
}

type subscriber struct {
	module  string
	handler func(ctx context.Context, ev Event) error
}

func NewEventBus() EventBus {
	return &bus{handlers: make(map[string][]subscriber)}
}

func (b *bus) Publish(ctx context.Context, source string, targets []string, topic string, payload any) error {
	ev := Event{Source: source, Targets: targets, Topic: topic, Payload: payload}
	b.mu.RLock()
	subs := b.handlers[topic]
	b.mu.RUnlock()
	for _, s := range subs {
		if len(targets) > 0 && !contains(targets, s.module) {
			continue
		}
		go func(s subscriber) {
			_ = s.handler(ctx, ev)
		}(s)
	}
	return nil
}

func (b *bus) Subscribe(ctx context.Context, moduleName, topic string, handler func(ctx context.Context, ev Event) error) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], subscriber{module: moduleName, handler: handler})
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.handlers[topic]
		for i, s := range subs {
			if s.module == moduleName && &s.handler == &handler {
				b.handlers[topic] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
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
