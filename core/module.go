package core

import "context"

// Env — окружение, которое модуль получает при Init:
// шина общего поля и реестр (чтобы знать остальных — «знаю других»).
type Env struct {
	Bus      EventBus
	Registry *Registry
}

// Module — модуль организма (front, polygon, growth, stratos, seed, ...).
// Жизненный цикл: Init → Start → Stop.
type Module interface {
	Name() string
	Init(ctx context.Context, env *Env) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
