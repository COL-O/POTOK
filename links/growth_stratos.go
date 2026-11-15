package links

import (
	"organism/core"
	"organism/modules/growth"
	"organism/modules/stratos"
)

// ConnectGrowthStratos — мостик growth ↔ stratos: growth публикует Suggestion,
// stratos подписан на шине и кладёт предложение в цепочку прецедентов.
// Связь — через EventBus (тема Suggestion).
func ConnectGrowthStratos(g *growth.Module, s *stratos.Module, bus core.EventBus) {
	_ = g
	_ = s
	_ = bus
}
