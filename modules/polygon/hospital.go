package polygon

import "github.com/COL-O/POTOK/contracts"

// Heal — лечение раненого (300) или рециклинг (200).
func Heal(r contracts.RollbackEvent) string {
	if r.Kind == contracts.Rollback300 {
		return "sent_to_hospital"
	}
	return "sent_to_recycler"
}
