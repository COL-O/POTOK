// Package contracts — общие типы организма POTOK.
//
// Структура пакета по авторскому фундаменту:
// common.go (Meta, ошибки), flight.go, passport.go, rollback.go,
// catalog.go (роли, изделия, BOM), suggestion.go (подсказки, мощности),
// interning.go (словарь код → ID), factory.go (расширенный завод).
package contracts

import "errors"

// Meta — общие метаданные любой сущности/сообщения организма.
type Meta struct {
	Source    string   `json:"source"`
	Targets   []string `json:"targets,omitempty"`
	CreatedAt int64    `json:"created_at,omitempty"` // unix-секунды, 0 — не задано
	Branch    string   `json:"branch,omitempty"`
}

// Общие ошибки контрактов.
var (
	// ErrUnknownProduct — BOM-связь ссылается на неизвестное изделие.
	ErrUnknownProduct = errors.New("contracts: unknown product in BOM edge")
	// ErrNonAdjacentLayer — BOM-связь не между соседними слоями.
	ErrNonAdjacentLayer = errors.New("contracts: BOM layers are not adjacent")
	// ErrBOMCycle — в BOM обнаружен цикл.
	ErrBOMCycle = errors.New("contracts: BOM contains a cycle")
	// ErrEmptyCode — пустой код завода или SKU изделия.
	ErrEmptyCode = errors.New("contracts: empty code")
)
