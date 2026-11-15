package core

import "fmt"

// Manifest — паспорт модуля в реестре организма.
//
// Помимо Name/Role/Layer модуль честно заявляет о себе всё, что нужно
// общему полю для «осознания целого»: от кого зависит и кого слушает,
// что публикует и кому помогает, что делает и чего не делает.
// Does/DoesNot — граница ответственности: организм знает, чего от модуля
// ждать нельзя, и не зовёт его не по делу.
type Manifest struct {
	Name    string
	Role    Role
	Layer   Layer
	Depends []string // модули, без которых этот не работает
	Provides []string // что модуль даёт общему полю
	Targets []string // кому адресованы его публикации по умолчанию

	Enabled bool

	// Взаимодействие на шине.
	ListensTo    []string // темы, которые модуль слушает
	ExpectsFrom  []string // от кого ждёт события
	Publishes    []string // темы, которые модуль публикует
	Helps        []string // кому модуль помогает (уровень «помогаю»)
	FeedbackFrom []string // от кого принимает обратную связь

	Criticality string // критичность модуля для организма

	Does    []string // что модуль делает
	DoesNot []string // чего модуль не делает (граница ответственности)
}

// Validate — манифест должен совпадать с модулем и иметь слой и роль.
func (mf Manifest) Validate(moduleName string) error {
	if mf.Name == "" {
		return fmt.Errorf("%w: empty manifest name for module %q", ErrEmptyManifestName, moduleName)
	}
	if mf.Name != moduleName {
		return fmt.Errorf("%w: manifest %q vs module %q", ErrManifestNameMismatch, mf.Name, moduleName)
	}
	if mf.Layer == "" {
		return fmt.Errorf("%w: module %q has no layer", ErrEmptyLayer, moduleName)
	}
	if mf.Role == "" {
		return fmt.Errorf("%w: module %q has no role", ErrEmptyRole, moduleName)
	}
	return nil
}
