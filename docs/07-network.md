# 07 — Восприятие (Сенсорный шлюз)

**`network/`** — UDP-шлюз (сырые пакеты → `HDCSlot` + `CSRArena`). Z-score нормализация. 128 базисов. `MaxPacketSize` = 4640 байт.

**Роль**: «рецепторный шлюз» организма. `network` принимает сырую телеметрию с датчиков и превращает её в импульсы на Шине без аллокаций на горячем пути.

**КОН**: КОН 1 (движение, не деньги) — шлюз перекачивает чистые импульсы. КОН 2 (факт, не суждение) — Z-score нормализация не «оценивает», а «приводит к общей шкале». КОН 10 (честность) — NaN/Inf отбраковываются, `PacketsDropped` «считается».

---

## 1. Что это за пакет

**`network`** — не «модель» и не «слой времени». Это «восприятие» — то, что соединяет реальный мир (датчики, дроны, заводы) с организмом.

**Два типа пакетов**:
- **`FrameMagicTensor`** (**`ORG1`**) — поток признаков (Tensor + HDC).
- **`FrameMagicGraph`** (**`ORG2`**) — граф топологии (CSR Graph + HDC).

---

## 2. Константы

| Константа | Значение | Назначение |
|---|---|---|
| `FrameMagicTensor` | `0x4F524731` | «ORG1» — поток признаков |
| `FrameMagicGraph` | `0x4F524732` | «ORG2» — граф топологии |
| `FrameHeaderSize` | `24` | Размер заголовка пакета |
| `MaxFeatures` | `64` | Максимум признаков в тензоре |
| `MaxGraphNodes` | `128` | Максимум узлов в графе |
| `MaxGraphEdges` | `512` | Максимум рёбер в графе |
| `MaxPacketSize` | `4640` | `FrameHeaderSize + 4 + (MaxGraphNodes+1)*4 + MaxGraphEdges*8` |

---

## 3. Заголовок пакета (24 байта)

| Поле | Тип | Смещение | Назначение |
|---|---|---|---|
| `Magic` | `uint32` | [0..3] | `FrameMagicTensor` или `FrameMagicGraph` |
| `SourceID` | `uint32` | [4..7] | ID источника (датчик, дрон, завод) |
| `TTS` | `uint32` | [8..11] | Бюджет времени жизни события |
| `Priority` | `uint8` | [12] | Приоритет (0..255) |
| `FeatureCount` | `uint8` | [13] | Число признаков (для Tensor) |
| `Reserved` | `[2]byte` | [14..15] | Выравнивание |
| `Timestamp` | `uint64` | [16..23] | Время отправки (наносекунды) |

---

## 4. Два типа пакетов

### 4.1. `FrameMagicTensor` (ORG1) — Поток признаков

**Формат payload**: `[FeatureCount]float32` (признаки).

**Что делает шлюз**:
1. Читает `FeatureCount` признаков из payload.
2. Проверяет на `NaN` / `Inf` — если есть, `PacketsDropped` инкрементируется, пакет отбрасывается.
3. Применяет Z-score нормализацию: `normFeatures[i] = (val - Mean[i]) / Std[i]`.
4. Записывает `normFeatures` в `TensorArena` (Дорожка 3) — чтобы `HDC` и `polygon` / `growth` видели единый масштаб.
5. Кодирует `normFeatures` в `HDCSlot` через `encodeFeaturesToHDC`.
6. Публикует импульс на Шину (`Publish`).
7. Вызывает `front.Router.Step(eventID)` — рефлекс «срабатывает».

**Z-score нормализация** — на стеке, без аллокаций. `FeatureStats` (`Mean`, `Std`) обновляется через `atomic.Pointer` (RCU).

**Смысл**: HDC «видит» все признаки равно (температура 60±10 и давление 2±0.5 не «забивают» друг друга).

---

### 4.2. `FrameMagicGraph` (ORG2) — Граф топологии

**Формат payload**:
- `NumNodes` (`uint16`), `NumEdges` (`uint16`).
- `RowOffsets` (`[NumNodes+1]uint32`).
- `ColIndices` (`[NumEdges]uint32`).
- `Weights` (`[NumEdges]float32`).

**Что делает шлюз**:
1. Читает `NumNodes`, `NumEdges`.
2. Проверяет границы: `NumNodes` < 2 или `NumNodes` > `MaxGraphNodes` (128) или `NumEdges` <= 0 или `NumEdges` > `MaxGraphEdges` (512) → `PacketsDropped`.
3. Проверяет `len(raw)` >= `needBytes` (`FrameHeaderSize + 4 + (NumNodes+1)*4 + NumEdges*8`).
4. Читает `RowOffsets`, `ColIndices`, `Weights`.
5. Проверяет `dst < NumNodes` — если нет, `PacketsDropped`.
6. Проверяет `Weights` на `NaN` / `Inf` — если есть, `PacketsDropped`.
7. Записывает CSR в `CSRArena` (Дорожка 2).
8. Кодирует CSR в `HDCSlot` через `encodeGraphToHDC`.
9. Публикует импульс на Шину (`Publish`).
10. Вызывает `front.Router.Step(eventID)`.

**Смысл**: `front` «видит» топологию (маршруты, заводы, цеха) и может «реагировать» на изменения.

---

## 5. Z-score нормализация (RCU)

**`FeatureStats`** — параметры Z-score нормализации:

| Поле | Тип | Назначение |
|---|---|---|
| `Mean` | `[MaxFeatures]float32` | Среднее по каждому признаку |
| `Std` | `[MaxFeatures]float32` | Стандартное отклонение по каждому признаку |

**`UpdateStatsRCU`** — атомарно обновляет статистики нормализации без остановки приёма UDP.

**По умолчанию**: `Mean` = 0, `Std` = 1 (единичный масштаб).

**Смысл**: Z-score приводит разномасштабные признаки к общей шкале. `HDC` «видит» их равно.

---

## 6. 128 базисов (изолированные)

**Два изолированных пространства базисов**:
- **`featureBasis`** (`[MaxFeatures]bus.HDCSlot`, 64) — для признаков.
- **`graphBasis`** (`[MaxGraphNodes]bus.HDCSlot`, 128) — для узлов графа.

**`initBasisVectors`** — два семейства (`seed` 1 и `seed` 2). `SplitMix64`.

**Смысл**: `encodeFeaturesToHDC` и `encodeGraphToHDC` не «пересекаются» (разные базисы).

---

## 7. Кодирование в HDC

### 7.1. `encodeFeaturesToHDC`

**Для каждого слова `w` из 160**:
- Для каждого бита `bit` из 64:
  - `acc` = сумма `±val` по всем признакам `f`, где `featureBasis[f].Words[w] & mask` даёт знак.
  - Если `acc >= 0` — `word |= mask`.

**Проверка остановки**: каждые 32 слова — `if g.isStopping() { return false }`.

**Проверка границ**: `if f >= len(g.featureBasis) { break }`.

---

### 7.2. `encodeGraphToHDC`

**Для каждого слова `w` из 160**:
- Для каждого бита `bit` из 64:
  - Для каждого `src` узла:
    - Для каждого ребра `e` из `rowOff[src]` в `rowOff[src+1]`:
      - `dst` = `colIdx[e]`, `val` = `weights[e]`.
      - `srcBasis` = `graphBasis[src].Words[w]`.
      - `dstBasis` = циклический сдвиг `graphBasis[dst].Words[w]` на 1 бит.
      - Если `((srcBasis ^ dstBasis) & mask) != 0` — `acc += val`, иначе `acc -= val`.
  - Если `acc >= 0` — `word |= mask`.

**Проверка остановки**: каждые 32 слова — `if g.isStopping() { return false }`.

**Проверка границ**: `if dst < 0 || dst >= len(g.graphBasis) { continue }`.

**Смысл**: направленное ребро (`src -> dst != dst -> src`) различается HDC.

---

## 8. Буферы (`bufPool`)

**`MaxPacketSize`** = 4640 байт — под максимальный граф (512 рёбер).

**`bufPool`** — `sync.Pool` с буферами `MaxPacketSize`. Zero-GC на приёме.

**Смысл**: крупные графы не обрезаются при чтении из сокета.

---

## 9. Остановка (`Stop`)

**`Stop`** — безопасно закрывает UDP-сокет, сохраняет `closeErr` и ждёт завершения `readLoop` с ограничением 5 секунд.

**Что делает**:
1. `g.stopping.Store(true)` (`atomic.Bool`).
2. `g.udpConn.SetReadDeadline(time.Now())` — «пробуждает» `readLoop`.
3. `closeErr = g.udpConn.Close()`.
4. `select`: `<-done` (`readLoop` завершился) → `return closeErr`; `<-time.After(5 * time.Second)` → `return errors.Join(timeoutErr, closeErr)`.

**Смысл**: `Stop` не «висит» вечно. Ошибка `Close` не «теряется».

---

## 10. Счётчики телеметрии (КОН 10)

| Счётчик | Назначение |
|---|---|
| `PacketsReceived` | Всего принято UDP-пакетов |
| `PacketsDropped` | Отбраковано (NaN, границы, битые) |
| `ImpulsesPushed` | Опубликовано импульсов на Шину |
| `GraphsIngested` | Принято графовых пакетов |

**Смысл**: `/metrics` и `/prometheus` показывают «здоровье» шлюза.

---

## 11. Что дальше

**После `docs/07-network.md`** — **`docs/08-organism.md`** («сборка»: `main.go`, `dashboard.go`, LIFO).
