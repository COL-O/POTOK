package core

import "errors"

// Ошибки манифеста и реестра.
var (
	// ErrDuplicateModule — модуль с таким именем уже зарегистрирован.
	ErrDuplicateModule = errors.New("core: duplicate module")
	// ErrEmptyModuleName — модуль без имени.
	ErrEmptyModuleName = errors.New("core: empty module name")
	// ErrEmptyManifestName — манифест без имени.
	ErrEmptyManifestName = errors.New("core: empty manifest name")
	// ErrManifestNameMismatch — имя в манифесте не совпало с именем модуля.
	ErrManifestNameMismatch = errors.New("core: manifest name mismatch")
	// ErrEmptyLayer — у модуля не указан слой.
	ErrEmptyLayer = errors.New("core: empty layer")
	// ErrEmptyRole — у модуля не указана роль.
	ErrEmptyRole = errors.New("core: empty role")
)
