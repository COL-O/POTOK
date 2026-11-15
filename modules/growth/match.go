package growth

// Hypothesis — гипотеза: кому и сколько продукта нужно.
// Source — автор гипотезы (например, "engineer_petrov").
type Hypothesis struct {
	ProductID uint32  `json:"product_id"`
	Need      float64 `json:"need"`
	Source    string  `json:"source"`
}

// Match — стыковка излишка с гипотезой: тот же продукт, количества хватает.
func Match(s Surplus, h Hypothesis) bool {
	return s.ProductID == h.ProductID && s.Qty >= h.Need
}
