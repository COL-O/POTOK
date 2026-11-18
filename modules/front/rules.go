package front

import (
	"context"

	"github.com/COL-O/POTOK/contracts"
	"github.com/COL-O/POTOK/core"
)

// RunRule — запуск правила при сбое (антитело от polygon применяется здесь).
// Если TTS завода ниже порога — срочно звать polygon (событие TTSLow).
func (m *Module) RunRule(ctx context.Context, flight contracts.FlightEvent) error {
	tts := m.tts.HoursToStop(flight.FactoryID)
	if tts < 2 {
		return m.bus.Publish(ctx, "front", []string{"polygon"}, "TTSLow",
			map[string]any{"factory_id": flight.FactoryID, "tts": tts})
	}
	return nil
}

var _ = core.LayerFront // слой модуля — front (Рефлекс)
