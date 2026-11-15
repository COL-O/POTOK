// Package core — ядро организма: слои, манифесты, модули, шина, реестр.
//
// Структура пакета по авторскому фундаменту:
// layer.go (Layer, Role), manifest.go (Manifest модуля),
// module.go (Module, Env), eventbus.go (Event, EventBus),
// registry.go (Registry), errors.go (ошибки манифеста/реестра).
package core

// Layer — слой организма (у каждого своё характерное время).
type Layer string

const (
	LayerFront   Layer = "front"   // Рефлекс: наносекунды–миллисекунды
	LayerPolygon Layer = "polygon" // Иммунитет: минуты–часы
	LayerGrowth  Layer = "growth"  // Рост: часы–дни
	LayerStratos Layer = "stratos" // Память: годы–века
)

// Role — роль слоя в организме.
type Role string

const (
	RoleReflex   Role = "reflex"
	RoleImmunity Role = "immunity"
	RoleGrowth   Role = "growth"
	RoleMemory   Role = "memory"
)
