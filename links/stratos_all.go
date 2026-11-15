package links

import (
	"organism/core"
	"organism/modules/stratos"
)

// ConnectStratosAll — мостик stratos ↔ все: stratos подписан на шине на
// Diagnosis, Suggestion и FlightEvent и пишет прецеденты; остальные слои
// читают через stratos.Recall. Связь — через EventBus и прямой Recall.
func ConnectStratosAll(s *stratos.Module, bus core.EventBus) {
	_ = s
	_ = bus
}
