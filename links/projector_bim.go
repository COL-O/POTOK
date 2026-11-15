package links

import (
	"github.com/COL-O/POTOK/modules/bim"
	"github.com/COL-O/POTOK/modules/growth/projector"
)

// ConnectProjectorBIM — мостик projector ↔ bim: projector получает BIM
// при создании (projector.New) и берёт оттуда ограничения capacity.
// Отдельной настройки мостик не требует.
func ConnectProjectorBIM(p *projector.Projector, b *bim.BIM) {
	_ = p
	_ = b
}
