package links

import (
	"github.com/COL-O/POTOK/core"
	"github.com/COL-O/POTOK/modules/stratos"
)

// ConnectStratosAll — мостик stratos ↔ все: stratos подписан на шине на
// Diagnosis, Suggestion и FlightEvent и пишет прецеденты; остальные слои
// читают через stratos.Recall. Связь — через EventBus и прямой Recall.
func ConnectStratosAll(s *stratos.Module, bus core.EventBus) {
	_ = s
	_ = bus
}
