package stratos

import "testing"

// Цепочка: блоки связаны по PrevHash, хеши различны для разных payload,
// целостность проверяется, поиск по теме работает.
func TestChainAppendAndVerify(t *testing.T) {
	c := NewChain()
	b1 := c.Append("FlightEvent", "front", map[string]any{"flight_id": "f1"})
	b2 := c.Append("FlightEvent", "front", map[string]any{"flight_id": "f2"})
	c.Append("Diagnosis", "polygon", map[string]any{"reason": "stuck_on_route"})

	if b1.Hash == b2.Hash {
		t.Error("different payloads produced same hash")
	}
	if b2.PrevHash != b1.Hash {
		t.Errorf("b2.PrevHash = %s, want %s", b2.PrevHash, b1.Hash)
	}
	if !c.Verify() {
		t.Error("chain does not verify")
	}
	if got := len(c.ByTopic("FlightEvent")); got != 2 {
		t.Errorf("ByTopic(FlightEvent) = %d, want 2", got)
	}
	if got := c.Len(); got != 3 {
		t.Errorf("Len = %d, want 3", got)
	}
}
