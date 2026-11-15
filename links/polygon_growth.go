package links

import (
	"organism/core"
	"organism/modules/growth"
	"organism/modules/polygon"
)

// ConnectPolygonGrowth — мостик polygon ↔ growth: polygon публикует Diagnosis,
// growth подписан на шине и ловит излишки. Связь — через EventBus (тема Diagnosis).
func ConnectPolygonGrowth(p *polygon.Module, g *growth.Module, bus core.EventBus) {
	_ = p
	_ = g
	_ = bus
}
