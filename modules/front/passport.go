package front

import (
	"context"
	"encoding/json"

	"github.com/COL-O/POTOK/contracts"
)

// RecordPassport — запись паспорта качества (Pasport из набора front).
func (m *Module) RecordPassport(ctx context.Context, p contracts.QualityPassport) error {
	if m.store == nil {
		return nil
	}
	key := []byte("passport:" + p.PassportID)
	val, _ := json.Marshal(p)
	return m.store.Put(ctx, key, val)
}
