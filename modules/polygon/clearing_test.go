package polygon

import "testing"

// Встречные потоки сворачиваются: A→B 10 и B→A 4 дают нетто A→B 6.
func TestClearNetting(t *testing.T) {
	out := Clear([]Clearing{
		{From: 1, To: 2, Qty: 10},
		{From: 2, To: 1, Qty: 4},
	})
	if len(out) != 1 {
		t.Fatalf("clear = %d flows, want 1: %+v", len(out), out)
	}
	if out[0].From != 1 || out[0].To != 2 || out[0].Qty != 6 {
		t.Errorf("net flow = %+v, want 1→2 qty 6", out[0])
	}
}

// Лечение: 300 — в госпиталь, остальное — в рециклинг.
func TestHeal(t *testing.T) {
	if got := Heal(contracts.RollbackEvent{Kind: contracts.Rollback300}); got != "sent_to_hospital" {
		t.Errorf("Heal(300) = %q, want sent_to_hospital", got)
	}
	if got := Heal(contracts.RollbackEvent{Kind: contracts.Rollback200}); got != "sent_to_recycler" {
		t.Errorf("Heal(200) = %q, want sent_to_recycler", got)
	}
}
