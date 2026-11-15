// Package store — интерфейс хранилища организма (ключ → значение).
//
// Авторский скелет предполагал реализацию на Pebble; интерфейс сохранён
// один в один, чтобы Pebble можно было вернуть, не трогая модули.
// Пока работает in-memory реализация (modules/store/memory).
package store

import "context"

// Store — хранилище ключ-значение с префиксной итерацией.
type Store interface {
	Put(ctx context.Context, key, value []byte) error
	Get(ctx context.Context, key []byte) ([]byte, error)
	Delete(ctx context.Context, key []byte) error
	Iter(ctx context.Context, prefix []byte, fn func(k, v []byte) error) error
	Close() error
}
