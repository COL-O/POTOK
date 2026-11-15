package growth

import "github.com/COL-O/POTOK/contracts"

// SpawnPilot — создание почки (Завод 100+1): новый завод-пилот
// в режиме развёртывания на ветке pilot:100+1.
func SpawnPilot(parentFactory uint16, hypothesisID string) contracts.Factory {
	return contracts.Factory{
		Meta:          contracts.Meta{Source: "growth", Branch: "pilot:100+1"},
		Code:          "pilot_" + hypothesisID,
		Role:          contracts.RoleAssembly,
		Zone:          contracts.ZoneB,
		Mode:          contracts.ModeDeploy,
		Criticality:   contracts.CriticalityOptional,
		DeployTimeSec: 86400,
		Active:        true,
	}
}
