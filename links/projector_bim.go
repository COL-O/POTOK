package links

import (
	"organism/modules/bim"
	"organism/modules/growth/projector"
)

// ConnectProjectorBIM — мостик projector ↔ bim: projector получает BIM
// при создании (projector.New) и берёт оттуда ограничения capacity.
// Отдельной настройки мостик не требует.
func ConnectProjectorBIM(p *projector.Projector, b *bim.BIM) {
	_ = p
	_ = b
}
