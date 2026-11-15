package growth

// Surplus — метка излишка: продукт лежит на заводе дольше порога —
// это приглашение к росту (КОН 7).
type Surplus struct {
	ProductID uint32  `json:"product_id"`
	FactoryID uint16  `json:"factory_id"`
	Qty       float64 `json:"qty"`
	Days      int     `json:"days"`
}

// IsGrowth — излишек «созрел» в возможность роста (от 14 дней).
func (s Surplus) IsGrowth() bool { return s.Days >= 14 }
