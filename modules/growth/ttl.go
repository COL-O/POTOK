package growth

// TTL — время жизни почки: пилот либо успевает доказать пользу,
// либо умирает по истечении срока (механизм слоя, не модель).
type TTL struct {
	StartedAt   int64 `json:"started_at"`
	DurationSec int64 `json:"duration_sec"`
}

// Expired — срок жизни истёк к моменту now (unix-секунды).
func (t TTL) Expired(now int64) bool { return now-t.StartedAt > t.DurationSec }
