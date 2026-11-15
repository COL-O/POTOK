# 08 — Сборка (Оркестратор)

**`cmd/organism/`** — главный оркестратор организма. `main.go` (`OrganismBridge`, LIFO `defer`) + `dashboard.go` (`/health`, `/metrics`, `/prometheus`).

**Роль**: «сборка» всех слоёв в одну программу. `main.go` инициализирует Шину, `front`, `polygon`, `growth`, `stratos`, `network`, `dashboard` — в строгом порядке. `OrganismBridge` связывает этажи через RCU без блокировок.

**КОН**: КОН 11 (КОН первичен) — сборка «рождается» из слоёв, а не «подстраивается» под инструменты. КОН 10 (честность) — `dashboard` показывает факт.

---

## 1. `main.go` — Главный оркестратор

### 1.1. `OrganismBridge` — мост между этажами

**Что делает**: связывает этажи (`front` ↔ `polygon` ↔ `growth`) через атомарный RCU. Реализует `polygon.FrontPolicyUpdater` и `growth.LowerLayerUpdater` без блокировок на чтение.

**Поля**:

| Поле | Тип | Назначение |
|---|---|---|
| `mu` | `sync.Mutex` | Сериализует редкие операции записи/обновления RCU |
| `frontRouter` | `*front.Router` | Роутер front |
| `hdcOrgan` | `*front.HDCOrgan` | HDC-орган (для RCU-обновления таблицы) |
| `tsetlinOrgan` | `*front.TsetlinOrgan` | Tsetlin-орган |
| `kanOrgan` | `*polygon.KANOrgan` | KAN-орган (для RCU-обновления таблицы) |
| `genCounter` | `atomic.Uint64` | Счётчик поколений генома |
| `kanVersion` | `atomic.Uint64` | Версия KAN-таблицы |
| `frontVersion` | `atomic.Uint64` | Версия front-политики |
| `activeProtos` | `[]front.HDCPrototype` | Активные HDC-прототипы (накопительно) |
| `activeVetoRules` | `[]front.VetoRule` | Активные VetoMask-правила |
| `quarantinePool` | `[]front.HDCPrototype` | Кандидаты в Карантине |
| `currentKAN` | `*polygon.KANRuleTable` | Текущая KAN-таблица |
| `snapshots` | `map[uint64]organismGenomeState` | Снимки генома по поколениям |

**Методы**:

| Метод | Что делает |
|---|---|
| `ProposeQuarantineReflex` | Помещает новый рефлекс от polygon/growth в теневой Карантин front |
| `ProposeFrontReflex` | Накопительно добавляет новый рефлекс в основную таблицу HDC |
| `ApplyVetoRule` | Выставляет запрет VetoMask (контур боли) в front |
| `RehabilitateVeto` | Законно снимает запрет с указанной маски органов после проверки в growth |
| `DeployPolygonKAN` | Атомарно обновляет таблицу формул с паспортом границ в polygon |
| `SnapshotCurrentGenome` | Сохраняет снимок текущего генома |
| `RestoreGenomeSnapshot` | Полностью восстанавливает политики front и таблицы polygon |

---

### 1.2. Порядок инициализации (строгий)

**`main.go`** инициализирует всё в строгом порядке:

1. **`stratos.NewStore`** — кристалл долгой памяти (WAL на NVMe).
2. **`bus.NewBus`** — кровеносная система (10 дорожек).
3. **`bus.NewScratchPool`** — пул черновиков.
4. **`front.NewHDCOrgan`**, **`front.NewTsetlinOrgan`** — рефлексы.
5. **`bus.NewOrganRegistry`** — реестр органов.
6. **`registry.HotSwap`** — регистрация 7 органов front.
7. **`front.NewGraphPrefetcher`** — упреждающий прогрев.
8. **`front.NewRouter`** — роутер front.
9. **`polygon.NewKANOrgan`**, **`polygon.NewPolygonTsetlinOrgan`** — иммунитет.
10. **`NewOrganismBridge`** — мост между этажами.
11. **`polygon.NewRouter`** — роутер polygon.
12. **`growth.NewRouter`** — роутер growth.
13. **`dashboard.Start()`** — HTTP-дашборд прозрачности (синхронно через `net.Listen`).
14. **`gateway.StartUDP(":9000")`** — UDP-шлюз (после готовности дашборда).

**Смысл**: дашборд «готов» до `gateway`. Наблюдаемость «работает» с первого пакета.

---

### 1.3. LIFO `defer` (строгий порядок завершения)

**Порядок `defer`** (LIFO — последний пришёл, первый ушёл):

1. **`gateway.Stop()`** — останавливает приём UDP первым.
2. **`dashboard.Shutdown(ctx)`** — останавливает HTTP-сервер.
3. **`prefetcher.Close()`** — останавливает прогрев.
4. **`scratchPool.Close()`** — освобождает пул черновиков.
5. **`coreBus.Close()`** — освобождает регионы `mmap`.
6. **`memoryStore.Close()`** — закрывает WAL на NVMe.

**Смысл**: `gateway` «останавливает» приём до того, как `coreBus` «закрывается». Нет «гонки» при завершении.

---

### 1.4. Живой прогон (4 такта)

**`main.go`** демонстрирует 4 такта жизни организма:

**Такт 1** — Штатный импульс (отрабатывает только `front` за наносекунды).

**Такт 2** — Неизвестная аномалия → `Surprise` → Диагноз `polygon` → Карантин `front`.

**Такт 3** — Выход за границы паспорта KAN → Эскалация в `growth`.

**Такт 4** — Повторение сложного события → `front` отрабатывает как врождённый рефлекс.

**Смысл**: организм проходит полный цикл от рефлекса до эволюции.

---

### 1.5. Итоговый отчёт честности (КОН 10)

**`main.go`** печатает итоговый отчёт по всем счётчикам:

```
=== ИТОГОВЫЙ ТЕПЛОВОЙ ОТЧЁТ ОРГАНИЗМА (КОН 10) ===
Шина (Bus):     Опубликовано=%d | Ошибок CRC32=%d | Пропусков эпохи=%d
Слой front:     Surprise=%d | Заблокировано Veto=%d | Эскалаций в growth=%d
Слой polygon:   Диагнозов=%d | Антител в Карантин=%d | Выдано Veto=%d
Слой growth:    Новых законов=%d | Забраковано=%d | Реабилитаций Veto=%d
Слой stratos:   Записей законов в WAL=%d | Токсичных меток=%d
```

**Смысл**: организм «видит» себя честно.

---

## 2. `dashboard.go` — HTTP-дашборд прозрачности

### 2.1. Что это

**`DashboardServer`** — HTTP-сервер с тремя эндпоинтами:

| Эндпоинт | Формат | Назначение |
|---|---|---|
| `/health` | JSON | Статус целостности Шины (`healthy` / `degraded`) |
| `/metrics` | JSON | Полный снимок счётчиков всех 4 контуров + Тепловая карта |
| `/prometheus` | `text/plain` | Метрики в формате Prometheus |

**Zero-Interference**: дашборд читает только `atomic.Uint64` счётчики и снимки `BusStatsSnapshot`, не ставя ни одного мьютекса на кровоток `Bus`.

---

### 2.2. `OrganismTelemetry` — полный снимок

**`OrganismTelemetry`** — JSON-структура со всеми счётчиками:

| Поле | Тип | Назначение |
|---|---|---|
| `TimestampNs` | `int64` | Время снимка |
| `Bus` | `bus.BusStatsSnapshot` | Счётчики Шины |
| `Front` | `FrontStats` | Счётчики front |
| `Polygon` | `PolygonStats` | Счётчики polygon |
| `Growth` | `GrowthStats` | Счётчики growth |
| `Stratos` | `StratosStats` | Счётчики stratos |
| `Heatmap` | `[]OrganHeatEntry` | Тепловая карта (24 органа) |

**`CollectTelemetry`** — собирает согласованный срез всех счётчиков и отсортированной тепловой карты без блокировок.

**Смысл**: `/metrics` показывает «пульс» организма в реальном времени.

---

### 2.3. `/prometheus` — нативный формат

**`handlePrometheus`** отдаёт метрики в стандартном текстовом формате Prometheus:

```
# TYPE organism_bus_published_total counter
organism_bus_published_total 5
# TYPE organism_front_surprise_total counter
organism_front_surprise_total 1
# TYPE organism_organ_hits_total counter
organism_organ_hits_total{id="0",layer="front",organ="HDC"} 1
...
```

**Смысл**: Prometheus «видит» организм без JSON. Grafana «рисует» тепловую карту.

---

### 2.4. `Start()` — синхронный через `net.Listen`

**`Start()`** синхронно занимает TCP-порт через `net.Listen` и запускает HTTP-сервер дашборда:

```go
func (d *DashboardServer) Start() error {
    ln, err := net.Listen("tcp", d.srv.Addr)
    if err != nil {
        return err
    }
    go func() {
        defer ln.Close()
        _ = d.srv.Serve(ln)
    }()
    return nil
}
```

**Смысл**: дашборд «готов» до `gateway` (`net.Listen` «занимает» порт синхронно).

---

## 3. Схема взаимодействия (main.go)

```
+-------------------------------------------------------------+
| 1. stratos.NewStore (WAL на NVMe)                           |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 2. bus.NewBus (10 дорожек)                                   |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 3. bus.NewScratchPool (пул черновиков)                       |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 4. front.NewHDCOrgan + front.NewTsetlinOrgan                 |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 5. bus.NewOrganRegistry + registry.HotSwap (7 органов front) |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 6. front.NewGraphPrefetcher + front.NewRouter                |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 7. polygon.NewKANOrgan + polygon.NewPolygonTsetlinOrgan      |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 8. NewOrganismBridge (мост между этажами)                    |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 9. polygon.NewRouter + growth.NewRouter                     |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 10. dashboard.Start() (синхронно через net.Listen)           |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 11. gateway.StartUDP(":9000") (после готовности дашборда)    |
+-------------------------------------------------------------+
                            |
                            v
+-------------------------------------------------------------+
| 12. Живой прогон (4 такта) + итоговый отчёт (КОН 10)        |
+-------------------------------------------------------------+
```

---

## 4. Что дальше

**После `docs/08-organism.md`** — **`docs/09-infrastructure.md`** («инфраструктура»: `bootstrap.sh`, `Makefile`, `Dockerfile`, `deploy/k8s/*`, `deploy/prometheus/`, `deploy/grafana/`).
