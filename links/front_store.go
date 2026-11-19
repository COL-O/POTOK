// Package links — мостики между модулями организма.
//
// Большая часть связей идёт через общую шину событий (core.EventBus):
// мостик здесь — явная точка подключения, а не скрытая зависимость.
// Мостики не содержат логики слоёв, только стыковку.
package links

import (
	"organism/modules/front"
	"organism/modules/store"
)

// ConnectFrontStore — мостик front ↔ store: рейсы и паспорта пишутся в store.
func ConnectFrontStore(f *front.Module, s store.Store) { f.SetStore(s) }
