package core

import "errors"

var (
	ErrManifestNoName  = errors.New("манифест: нет имени")
	ErrManifestNoLayer = errors.New("манифест: нет слоя")
)
