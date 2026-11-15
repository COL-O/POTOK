package contracts

// RollbackKind — вид отката: 300 — лечение, 200 — рециклинг.
type RollbackKind string

const (
	Rollback300 RollbackKind = "rollback_300"
	Rollback200 RollbackKind = "rollback_200"
)

// RollbackEvent — событие отката (пострадавший узел/изделие):
// polygon решает — госпиталь (300) или рециклинг (200).
type RollbackEvent struct {
	Kind      RollbackKind `json:"kind"`
	FactoryID uint16       `json:"factory_id"`
	ProductID uint32       `json:"product_id"`
}
