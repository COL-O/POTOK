package links

import (
	"organism/core"
	"organism/modules/front"
	"organism/modules/polygon"
)

// ConnectPolygonFront — мостик polygon ↔ front: polygon публикует Antibody,
// front подписан на шине и применяет правило рефлексом. Отдельного канала
// не нужно: связь реализуется через EventBus (тема Antibody).
func ConnectPolygonFront(p *polygon.Module, f *front.Module, bus core.EventBus) {
	_ = p
	_ = f
	_ = bus
}
