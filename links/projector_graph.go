package links

import (
	"github.com/COL-O/POTOK/modules/graph"
	"github.com/COL-O/POTOK/modules/growth/projector"
)

// ConnectProjectorGraph — мостик projector ↔ graph: projector строится
// поверх BOM-графа (см. projector.New в main), обратный индекс графа —
// его рабочий инструмент. Отдельной настройки мостик не требует.
func ConnectProjectorGraph(p *projector.Projector, b *graph.BOMGraph) {
	_ = p
	_ = b
}
