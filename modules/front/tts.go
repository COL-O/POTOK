package front

import (
	"sync"
)

// TTS — Time-To-Survive: часы до остановки завода.
// remain — запас (в условных единицах потока), rate — скорость потребления.
// Точная формула TTS — в следующих частях описания; здесь скелет автора.
type TTS struct {
	mu     sync.RWMutex
	remain map[uint16]float64 // factoryID → остаток
	rate   map[uint16]float64 // factoryID → скорость потребления
}

// NewTTS — новый счётчик TTS.
func NewTTS() *TTS {
	return &TTS{remain: map[uint16]float64{}, rate: map[uint16]float64{}}
}

// Update — приход потока на завод: запас растёт, скорость по умолчанию 1.0.
func (t *TTS) Update(factoryID uint16, delta float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.remain[factoryID] += delta
	if t.rate[factoryID] == 0 {
		t.rate[factoryID] = 1.0
	}
}

// HoursToStop — сколько часов завод протянет на текущем запасе.
func (t *TTS) HoursToStop(factoryID uint16) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rate := t.rate[factoryID]
	if rate == 0 {
		return 0
	}
	return t.remain[factoryID] / rate
}
