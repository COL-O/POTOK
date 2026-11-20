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
	// Заглушка опасна: она обещает связь, которой нет. Мостик обязан
	// отдать модулям одну и ту же шину, иначе подписка и публикация
	// окажутся в разных мирах.
	p.SetBus(bus)
	f.SetBus(bus)
}
