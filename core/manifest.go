package core

// Manifest — «паспорт» модуля.
type Manifest struct {
	Name     string
	Role     Role
	Layer    Layer
	Depends  []string
	Provides []string
	Targets  []string
	Enabled  bool
}

func (m Manifest) Validate() error {
	if m.Name == "" {
		return ErrManifestNoName
	}
	if m.Layer == 0 && m.Role != RoleStore && m.Role != RoleGraph &&
		m.Role != RoleBIM && m.Role != RoleSeed {
		return ErrManifestNoLayer
	}
	return nil
}
