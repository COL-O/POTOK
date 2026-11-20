# 11 — Как добавить новый орган (КОН 8)

**POTOK** расширяется по закону КОН 8 (Деление, не строительство). Мы не переписываем ядро Шины и не ломаем существующие контуры. Новый орган (начиная со слота `ID` = 24 и до `31` в активном реестре) подключается как независимый эксперт через контракт `bus.Organ` и проходит обязательное испытание в Карантине.

---

## 1. Золотые правила при создании органа

### 1.1. Zero-GC на горячем пути (`front` и `polygon`)

Внутри метода `Process(view bus.TickView)` запрещено выделять память в куче Go (`make`, `new`, конкатенация строк, замыкания, захваченные в кучу). Используйте только стек и предвыделенный буфер `view.Scratch`.

### 1.2. Честность границ (`InBounds` — КОН 10)

Орган обязан проверять применимость входных данных до начала расчётов. Если вход вышел за рамки обучения или формата — верните `bus.StatusOutOfBounds` (триггер `Surprise`), а не галлюцинируйте результат.

### 1.3. Разделение природ (SOA)

Читайте только те дорожки Шины (`ReadLanes`), которые соответствуют математической природе вашей модели (биты — `LaneHDC`, графы — `LaneGraph`, тензоры — `LaneTensor`, векторы — `LaneVector`, аудио — `LaneAudio`).

### 1.4. Lock-Free обновление параметров

Если у органа есть обучаемые веса или таблицы правил, храните их в неизменяемой структуре под `atomic.Pointer[T]` (паттерн RCU).

---

## 2. Пошаговая инструкция: добавляем 25-й орган (`ID = 24`)

### Шаг 1. Определите слот и битовую маску

Слоты `0..23` заняты базовыми 24 органами:

- **`0..6`** — слой `front` (`HDC`, `Tsetlin`, `TTL`, `Capacity`, `DStarLite`, `MCTS`, `Pasport`).
- **`7..12`** — слой `polygon` (`MatrixProfile`, `KAN`, `CfC`, `CBS`, `ESN`, `PolyTsetlin`).
- **`13..18`** — слой `growth` (`GraphSAGE`, `ActiveInference`, `SINDy`, `LUTKAN`, `GrowthMCTS`, `TDMPC2`).
- **`19..23`** — слой `stratos` (`HDCIndex`, `LSHHNSW`, `RetNet`, `MPIndex`, `SHA256Seal`).

Свободные слоты для доменных органов (транспорт, дроны, акустика, склад): `24..31`.

```go
const (
    OrganIDAcousticAnomaly uint8  = 24
    BitAcousticAnomaly     uint32 = 1 << OrganIDAcousticAnomaly
)
```

---

### Шаг 2. Реализуйте интерфейс `bus.Organ`

Файл `custom/acoustic.go`:

```go
package custom

import (
    "time"
    "organism/bus"
)

type AcousticOrgan struct {
    PeakThreshold int16
}

func (o *AcousticOrgan) Spec() bus.OrganSpec {
    return bus.OrganSpec{
        ID:          OrganIDAcousticAnomaly,
        Name:        "AcousticAnomaly",
        Layer:       bus.LayerFront,
        ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneAudio),
        WriteLane:   bus.LanePulse,
        Residency:   bus.ResidencyPinned,
        SoftTimeout: 5 * time.Microsecond,
        HardTimeout: 20 * time.Microsecond,
    }
}

func (o *AcousticOrgan) InBounds(view bus.TickView) bool {
    return len(view.Audio) > 0
}

func (o *AcousticOrgan) Warmup(*bus.MmapRegion) error { return nil }
func (o *AcousticOrgan) CoolDown()                    {}

func (o *AcousticOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
    start := time.Now().UnixNano()
    if !o.InBounds(view) {
        return bus.Verdict{OrganID: OrganIDAcousticAnomaly, Status: bus.StatusOutOfBounds},
            bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusOutOfBounds}
    }

    var maxAmp int16
    for _, sample := range view.Audio {
        if sample < 0 {
            sample = -sample
        }
        if sample > maxAmp {
            maxAmp = sample
        }
    }

    lat := uint64(time.Now().UnixNano() - start)
    if maxAmp >= o.PeakThreshold {
        return bus.Verdict{
            OrganID:    OrganIDAcousticAnomaly,
            Status:     bus.StatusOK,
            Confidence: 0.94,
            ActionCode: 901,
        }, bus.OrganMetrics{LatencyNs: lat, Confidence: 0.94, Status: bus.StatusOK}
    }

    return bus.Verdict{
        OrganID:    OrganIDAcousticAnomaly,
        Status:     bus.StatusOK,
        Confidence: 0.98,
        ActionCode: 0,
    }, bus.OrganMetrics{LatencyNs: lat, Confidence: 0.98, Status: bus.StatusOK}
}
```

---

### Шаг 3. Обязательный тест на Zero-GC и границы

Файл `custom/acoustic_test.go`:

```go
package custom

import (
    "testing"
    "organism/bus"
)

func TestAcousticOrganZeroGC(t *testing.T) {
    org := &AcousticOrgan{PeakThreshold: 12000}
    samples := []int16{100, -250, 15000, 300}
    view := bus.TickView{EventID: 1, Audio: samples}

    allocs := testing.AllocsPerRun(500, func() {
        v, _ := org.Process(view)
        if v.Status != bus.StatusOK || v.ActionCode != 901 {
            t.Fatalf("unexpected verdict: %+v", v)
        }
    })
    if allocs > 0 {
        t.Fatalf("Zero-GC violated: got %f allocs/op, want 0", allocs)
    }
}
```

---

### Шаг 4. Ввод в организм через Карантин и RCU HotSwap

1. Зарегистрируйте орган в реестре через атомарную подмену:

   ```go
   _ = registry.HotSwap(&AcousticOrgan{PeakThreshold: 12000})
   ```

2. Добавьте правило активации бита `BitAcousticAnomaly` в `TsetlinClause` или `HDCPrototype` через Карантин `front` (`ProposeQuarantineReflex`).

3. После прохождения теневого прогона (`QuarantineTrials`) рефлекс автоматически перейдёт в основную RCU-таблицу без остановки конвейера.

---

## 3. Чек-лист перед коммитом нового органа

- [ ] **`go test -v -race ./...`** проходит без гонок данных.
- [ ] **`testing.AllocsPerRun`** для `Process()` равен `0` (для слоёв `front` и `polygon`).
- [ ] **Метод `InBounds()`** защищает от пустых срезов, `NaN` / `Inf` и выхода за пределы диапазонов обучения.
- [ ] **Время выполнения** укладывается в заявленный `SoftTimeout` в `OrganSpec`.

---

## 4. Что дальше

**После `docs/11-contributing.md`** — **`docs/12-first-run.md`** («первый запуск»: `./bootstrap.sh --check`, `make run`, `curl /health`, UDP-пакет).
