package links

import (
	"github.com/COL-O/POTOK/modules/front"
	"github.com/COL-O/POTOK/modules/store"
)

// ConnectFrontParquet — мостик front ↔ Parquet: закрытые смены front
// сбрасываются в колоночный архив для stratos (длинные волны).
// Пока не реализовано: в скелете front пишет в store (flight:*, passport:*),
// Parquet — в списке «что осталось» у автора. Мостик зарезервирован.
func ConnectFrontParquet(f *front.Module, s store.Store) {
	_ = f
	_ = s
}
