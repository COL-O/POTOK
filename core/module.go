package core

import "context"

// Module — любой модуль организма.
type Module interface {
	Name() string
	Init(ctx context.Context, env *Env) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Env — окружение, которое модуль получает при старте.
type Env struct {
	Registry Registry
	Bus      EventBus
}
