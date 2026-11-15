package links

import (
	"organism/modules/graph"
	"organism/modules/growth/projector"
)

// ConnectProjectorGraph — мостик projector ↔ graph: projector строится
// поверх BOM-графа (см. projector.New в main), обратный индекс графа —
// его рабочий инструмент. Отдельной настройки мостик не требует.
func ConnectProjectorGraph(p *projector.Projector, b *graph.BOMGraph) {
	_ = p
	_ = b
}
