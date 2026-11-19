package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Registry interface {
	Register(m Module, manifest Manifest) error
	StartAll(ctx context.Context) error
	StopAll(ctx context.Context) error
	Enabled(name string) bool
	Get(name string) (Module, bool)
	BodyMap() []Manifest
}

type registry struct {
	mu        sync.RWMutex
	modules   map[string]Module
	manifests map[string]Manifest
	order     []string
}

func NewRegistry() Registry {
	return &registry{
		modules:   make(map[string]Module),
		manifests: make(map[string]Manifest),
	}
}

func (r *registry) Register(m Module, manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := manifest.Name
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("модуль %q уже зарегистрирован", name)
	}
	r.modules[name] = m
	r.manifests[name] = manifest
	return nil
}

func (r *registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	order, err := r.resolveOrder()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.order = order
	r.mu.Unlock()
	for _, name := range order {
		m := r.modules[name]
		manifest := r.manifests[name]
		if !manifest.Enabled {
			continue
		}
		env := &Env{Registry: r, Bus: NewEventBus()}
		if err := m.Init(ctx, env); err != nil {
			return fmt.Errorf("Init %q: %w", name, err)
		}
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("Start %q: %w", name, err)
		}
	}
	return nil
}

func (r *registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	order := append([]string(nil), r.order...)
	r.mu.RUnlock()
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if m, ok := r.modules[name]; ok {
			_ = m.Stop(ctx)
		}
	}
	return nil
}

func (r *registry) Enabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.manifests[name]
	return ok && m.Enabled
}

func (r *registry) Get(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modules[name]
	return m, ok
}

func (r *registry) BodyMap() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Manifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Layer != list[j].Layer {
			return list[i].Layer < list[j].Layer
		}
		return list[i].Name < list[j].Name
	})
	return list
}

func (r *registry) resolveOrder() ([]string, error) {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var order []string
	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("цикл зависимостей: %q", name)
		}
		visiting[name] = true
		manifest, ok := r.manifests[name]
		if !ok {
			return fmt.Errorf("модуль %q не зарегистрирован", name)
		}
		for _, dep := range manifest.Depends {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		order = append(order, name)
		return nil
	}
	for name := range r.manifests {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}
