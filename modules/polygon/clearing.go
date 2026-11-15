package polygon

// Clearing — свёртка встречных потоков (безденежный клиринг организма).
type Clearing struct {
	From      uint16  `json:"from"`
	To        uint16  `json:"to"`
	Qty       float64 `json:"qty"`
	Unsettled float64 `json:"unsettled"`
}

// Clear — простая свёртка: A→B и B→A компенсируются,
// в остатке только нетто-потоки. Точная механика клиринга — позже.
func Clear(flows []Clearing) []Clearing {
	out := []Clearing{}
	pairs := map[[2]uint16]float64{}
	for _, f := range flows {
		key := [2]uint16{f.From, f.To}
		rev := [2]uint16{f.To, f.From}
		if pairs[rev] > 0 {
			comp := minf(pairs[rev], f.Qty)
			pairs[rev] -= comp
			f.Qty -= comp
		}
		if f.Qty > 0 {
			pairs[key] += f.Qty
		}
	}
	for k, v := range pairs {
		if v > 0 {
			out = append(out, Clearing{From: k[0], To: k[1], Qty: v, Unsettled: v})
		}
	}
	return out
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
