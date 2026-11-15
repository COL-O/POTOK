package links

import (
	"github.com/COL-O/POTOK/core"
	"github.com/COL-O/POTOK/modules/growth"
	"github.com/COL-O/POTOK/modules/polygon"
)

// ConnectPolygonGrowth — мостик polygon ↔ growth: polygon публикует Diagnosis,
// growth подписан на шине и ловит излишки. Связь — через EventBus (тема Diagnosis).
func ConnectPolygonGrowth(p *polygon.Module, g *growth.Module, bus core.EventBus) {
	_ = p
	_ = g
	_ = bus
}
