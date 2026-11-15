// Package memory — in-memory реализация store.Store.
//
// Временная замена Pebble из авторского скелета (внешние зависимости
// в окружении сборки недоступны). Семантика та же: Put/Get/Delete и
// итерация по префиксу в порядке ключей. Данные живут в памяти процесса.
package memory

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryStore — хранилище в памяти.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// New — новое пустое хранилище.
func New() *MemoryStore {
	return &MemoryStore{data: make(map[string][]byte)}
}

// Put — записать значение по ключу (копия).
func (s *MemoryStore) Put(_ context.Context, key, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[string(key)] = cp
	return nil
}

// Get — прочитать значение (копия). Отсутствие ключа — ошибка.
func (s *MemoryStore) Get(_ context.Context, key []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[string(key)]
	if !ok {
		return nil, fmt.Errorf("memory store: key %q not found", key)
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

// Delete — удалить ключ (отсутствие не считается ошибкой).
func (s *MemoryStore) Delete(_ context.Context, key []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, string(key))
	return nil
}

// Iter — обойти все ключи с префиксом в лексикографическом порядке.
func (s *MemoryStore) Iter(_ context.Context, prefix []byte, fn func(k, v []byte) error) error {
	s.mu.RLock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		if bytes.HasPrefix([]byte(k), prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	snapshot := make(map[string][]byte, len(keys))
	for _, k := range keys {
		snapshot[k] = s.data[k]
	}
	s.mu.RUnlock()

	for _, k := range keys {
		if err := fn([]byte(k), snapshot[k]); err != nil {
			return err
		}
	}
	return nil
}

// Close — для in-memory нечего закрывать.
func (s *MemoryStore) Close() error { return nil }
