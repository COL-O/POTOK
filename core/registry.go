package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Registry — реестр модулей организма: кто есть, кто включён, кто кому помогает.
// Через реестр организм «осознаёт целое»: BodyMap — карта тела,
// Understanding — сводка трёх уровней (часть → знаю других → помогаю),
// Field — общее поле (шина), в котором все встречаются.
type Registry struct {
	bus       EventBus
	modules   []Module
	byName    map[string]Module
	manifests map[string]Manifest
}

// NewRegistry — реестр с собственной in-memory шиной.
func NewRegistry() *Registry {
	return &Registry{
		bus:       NewMemoryBus(),
		byName:    make(map[string]Module),
		manifests: make(map[string]Manifest),
	}
}

// Bus — шина реестра.
func (r *Registry) Bus() EventBus { return r.bus }

// Field — общее поле организма («Сфера Вернадского» в коде — шина событий).
func (r *Registry) Field() EventBus { return r.bus }

// Register — зарегистрировать модуль с манифестом.
// Выключенный (Enabled=false) модуль не регистрируется.
func (r *Registry) Register(m Module, mf Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: nil module", ErrEmptyModuleName)
	}
	name := m.Name()
	if name == "" {
		return fmt.Errorf("%w: module with empty name", ErrEmptyModuleName)
	}
	if !mf.Enabled {
		return nil
	}
	if err := mf.Validate(name); err != nil {
		return err
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicateModule, name)
	}
	r.byName[name] = m
	r.manifests[name] = mf
	r.modules = append(r.modules, m)
	return nil
}

// StartAll — Init + Start всех модулей в порядке регистрации.
func (r *Registry) StartAll(ctx context.Context) error {
	env := &Env{Bus: r.bus, Registry: r}
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

// Enabled — имена включённых модулей (в порядке регистрации).
func (r *Registry) Enabled() []string {
	out := make([]string, 0, len(r.modules))
	for _, m := range r.modules {
		out = append(out, m.Name())
	}
	return out
}

// Get — модуль по имени.
func (r *Registry) Get(name string) (Module, bool) {
	m, ok := r.byName[name]
	return m, ok
}

// BodyMap — карта тела организма: имя модуля → его манифест.
func (r *Registry) BodyMap() map[string]Manifest {
	out := make(map[string]Manifest, len(r.manifests))
	for k, v := range r.manifests {
		out[k] = v
	}
	return out
}

// Understanding — осознание целого в трёх уровнях:
// 1) каждый модуль — часть тела; 2) реестр знает всех и их слои;
// 3) по манифестам видно, кто кому помогает (Helps).
// Возвращает человекочитаемую сводку, детерминированную по именам.
func (r *Registry) Understanding() string {
	names := r.Enabled()
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("уровень 1 (часть): организм состоит из модулей: ")
	b.WriteString(strings.Join(names, ", "))
	b.WriteString("\nуровень 2 (знаю других): слои — ")
	layers := make([]string, 0, len(names))
	for _, n := range names {
		layers = append(layers, fmt.Sprintf("%s=%s/%s", n, r.manifests[n].Layer, r.manifests[n].Role))
	}
	b.WriteString(strings.Join(layers, ", "))
	b.WriteString("\nуровень 3 (помогаю): ")
	helps := make([]string, 0)
	for _, n := range names {
		if h := r.manifests[n].Helps; len(h) > 0 {
			helps = append(helps, fmt.Sprintf("%s→%s", n, strings.Join(h, "+")))
		}
	}
	if len(helps) == 0 {
		b.WriteString("помощь в манифестах пока не заявлена")
	} else {
		b.WriteString(strings.Join(helps, ", "))
	}
	return b.String()
}
