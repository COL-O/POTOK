# Настоящий лог Colab: причины пяти падений (по факту, не по догадке)

**Откуда лог.** Он лежал всё это время в самом доме: `notebooks/sfera-pervoe-chtenie.ipynb`,
ячейка 9 (`execution_count` 7), сохранённый вывод — 2196 символов. Ноутбук обрезает вывод
до последних 2000 символов (`print(...stdout[-2000:])`), поэтому **начало `go test` и строка
`go version` потеряны**.

**Что я делала не так.** Полтора захода я строила гипотезы по коду, вместо того чтобы
сначала прочитать `outputs` этого файла — он был в клоне дома с самого начала. Основатель
прислал ссылку на Colab, я её открыла (`fetch_page` достаёт colab-страницу, хотя curl из
песочницы получает `000`), и лог нашёлся. Порядок должен был быть обратным.

Ниже — что теперь **доказано логом** и что я в прошлом разборе написала неверно.

---

## Сводка лога (дословно, без правок)

```
AIL: TestOrganismChainOnStuckFlight (0.00s)
    organism_test.go:74: FlightEvent precedents = 0, want 1
    organism_test.go:77: Diagnosis precedents = 0, want 1
    organism_test.go:80: Suggestion precedents = 0, want > 0
    organism_test.go:88: diagnosis in store: memory store: key "diagnosis:test-stuck" not found
FAIL	organism	0.004s

ok  	organism/cmd/organism	0.007s
ok  	organism/modules/bus	0.003s
--- FAIL: TestRouterRoutineVetoAndQuarantine (0.00s)
    router_test.go:101: expected FlagInQuarantine from shadow candidate
--- FAIL: TestRouterZeroAllocations (0.00s)
    router_test.go:136: unexpected step failure: err=<nil>, v={OrganID:4 Status:3 Confidence:0 ...}
FAIL	organism/modules/front	0.004s

ok  	organism/modules/graph	0.002s
--- FAIL: TestGrowthAdversarialReplayRejectsBadLaw (0.00s)
    router_test.go:144: expected law rejected with 6005 on black swan failure, got
    {GenerationID:2 ... GuardMin:0.3 GuardMax:5.2 PassedSwans:1 TotalSwans:1 ... Rejected:false}
FAIL	organism/modules/growth	0.003s

ok  	organism/modules/network	0.004s
ok  	organism/modules/polygon	0.003s
--- FAIL: TestGenerateDefaultWorld (0.00s)
    generator_test.go:10: generate: seed factories: factory "quarry_01": contracts: нет created_at
--- FAIL: TestGenerateDeterministic (0.00s)
    generator_test.go:59: seed factories: factory "quarry_01": contracts: нет created_at
FAIL	organism/modules/seed	0.002s

ok  	organism/modules/stratos	0.002s
```

Пакеты `modules/network`, `modules/polygon`, `modules/stratos`, `modules/bus`,
`modules/graph`, `cmd/organism` — **зелёные**. Это тоже факт: сеть и шина в порядке.

---

## 1–2. seed (`TestGenerateDefaultWorld`, `TestGenerateDeterministic`) — ДЕФЕКТ, подтверждён

Лог: `factory "quarry_01": contracts: нет created_at`.
Код: `modules/seed/factories.go:52-57` создаёт `contracts.Meta` без `CreatedAt`,
строка `:73` вызывает `f.Validate()` → `ErrNoCreatedAt` (`contracts/common.go:22`).
Значит генерация падает на первом же заводе — мира нет вообще.

Не среда: в том же доме примеры ставят `CreatedAt` (`contracts/catalog.go:199,211`,
`contracts/factory.go:198`), а генератор — нет.
**Патч:** `0003-seed-created-at.patch`. Прогона с патчем пока нет.

---

## 3. `TestOrganismChainOnStuckFlight` — ДЕФЕКТ, подтверждён

Лог: `FlightEvent precedents = 0` (и далее всё нулевое, включая запись в store).
Код: своя шина на каждый модуль (`core/registry.go:63`), мостики — заглушки `_ = bus`,
`main.go:81` создаёт `eventBus` после реестра и никуда не передаёт.
**Патч:** `0005-links-real-wiring.patch` (+ `0001`). Прогона с патчем пока нет.

---

## 4. `TestRouterRoutineVetoAndQuarantine` — НАШЛА НАСТОЯЩУЮ ПРИЧИНУ (лог указал адрес)

Лог: `router_test.go:101: expected FlagInQuarantine from shadow candidate`.
Строка 101 — последняя проверка теста. Значит всё предыдущее прошло: статус OK, уверенность
≥ 0.90, `VetoBlocked == 1`, `DStarLite` не исполнялся, `FlagVetoTriggered` стоит.
Не хватает только `FlagInQuarantine`.

**Причина:** `modules/front/router.go:311` —
```go
if sim >= cand.MinSim && sim > activeVerdict.Confidence {
```
Тест подаёт «теневого кандидата» из **нулевого** HDC-шаблона, а кадр — тоже нулевой HDC.
Сходство `sim = 1.0`, но `activeVerdict.Confidence` тоже **1.0** (нулевой кадр идеально
совпал со своим нулевым прототипом), и строгое `>` не выполняется. Флаг не ставится.

Это не среда и не тайминг: при одинаковой уверенности логика «кварантин только если
уверенность строго выше активной» **отказывает** в кварантине. Смысл кварантина —
«похоже на опасный класс, даже если активный ответ уверен». Условие `>` противоречит смыслу;
должно быть `>=` (равная уверенность — не повод игнорировать опасный класс).

**Патч:** `0006-front-quarantine-and-status.patch` (меняет `>` на `>=`).

---

## 5. `TestRouterZeroAllocations` — НАСТОЯЩАЯ ПРИЧИНА (я ошибалась)

Лог: `router_test.go:136: unexpected step failure: err=<nil>, v={OrganID:4 Status:3 ...}`.

Мой прошлый разбор был неверен дважды:
1. я написала «`TestRouterZeroAllocations` в `modules/front`» — это верно, но **не** назвала
   причину, а предполагала аллокации; тест падает **до** проверки аллокаций, на `t.Fatalf`
   внутри `AllocsPerRun`;
2. `Status:3` — это `bus.StatusOutOfBounds` (`modules/bus/verdict.go:9`), `OrganID:4` —
   `OrganIDDStarLite` (`modules/front/models.go:16`).

**Причина:** тест публикует кадр **без графа** (`bus.NullOffset`), а `BitDStarLite`
приходит в маску из двух мест: `BaseRoutineMask | BitTTL | BitDStarLite` в прототипе HDC
(тест, `setupTestRouter`) и `routeMask |= front.BitDStarLite | front.BitCapacity`
(`modules/front/router.go`). `DStarLiteOrgan.InBounds` требует `Graph.NumNodes >= 2`
(`modules/front/models.go`), графа нет → орган отдаёт `OutOfBounds` → `Step` выходит
с этим вердиктом (`router.go:203-207`). До аллокаций дело не доходит.

**Кто не прав — тест или код?** По смыслу — тест: он требует `StatusOK` на кадре,
где один из органов по контракту обязан сказать «вне границ». Но и код небезупречен:
`OutOfBounds` от одного органа **отменяет** вердикты всех остальных (ранний `return`),
то есть кадр без графа не может быть обработан вообще, хотя остальные органы к нему готовы.

Это решение круга, не моё: (а) починить тест (публиковать кадр с графом), (б) изменить
`Step` — не отменять такт из-за одного органа вне границ, а пропускать его.
Я **не** правлю это патчем: обе развилки меняют контракт, а не чинят опечатку.
Записано как вопрос.

---

## 6. `TestGrowthAdversarialReplayRejectsBadLaw` — НАСТОЯЩАЯ ПРИЧИНА (перекрытие буфера)

Лог: `expected law rejected with 6005 on black swan failure, got {... GuardMin:0.3 GuardMax:5.2
PassedSwans:1 TotalSwans:1 ... Rejected:false}`.

`TotalSwans: 1`, `PassedSwans: 1` — лебедь **был** проверен и «прошёл» проверку, которой
обязан был не пройти: `{CaseID: 42, TestInput: 3.0, MinSafeOut: 0.0, MaxSafeOut: 0.05}`.

**Причина — перекрытие буфера `scratch`.** Два органа пишут в один и тот же `scratch`:
- `LUTKANOrgan.Process` пишет `out := ...(&view.Scratch[0])`, `out[0]=guardMin`, `out[1]=guardMax`,
  `out[2..2+res)` — LUT (`modules/growth/models.go:276-283`);
- `router.Evolve` читает **те же байты со смещением 64**: `rawOut := ...(&scratch[64])`,
  `MinVal: rawOut[0]`, `MaxVal: rawOut[1]`, `knots: rawOut[2:2+res]`
  (`modules/growth/router.go:177-180`).

Орган пишет в начало буфера, роутер читает на 64 байта дальше — то есть получает
**сдвинутую на 64 байта копию** своей же записи (и `MinVal/MaxVal` при этом оказываются
не тем, что задумано). Отсюда `GuardMin:0.3 GuardMax:5.2` — значения из «съехавших» байтов,
и `verifyOnBlackSwan` (`router.go:249-263`) считает выход безопасным, потому что LUT,
который он проверяет, собран не из тех чисел.

Это не плавающая ошибка: смещение фиксированное, результат детерминированный. Именно поэтому
тест падает стабильно.

**Я не правлю это патчем.** Здесь нельзя угадать замысел: либо орган должен писать
с явным смещением (`Scratch[64:]`), либо роутер — читать с нуля. От того, что выбрать,
меняется контракт «кто владеет какими байтами scratch». Это вопрос круга.
Адрес и цифры — выше, чтобы решение принималось по факту.

---

## Что теперь можно утверждать

| Тест | Причина | Статус |
|---|---|---|
| `TestGenerateDefaultWorld`, `TestGenerateDeterministic` | `Meta` без `CreatedAt` | дефект, патч `0003` |
| `TestOrganismChainOnStuckFlight` | своя шина каждому модулю | дефект, патч `0005`+`0001` |
| `TestRouterRoutineVetoAndQuarantine` | строгое `>` вместо `>=` в кварантине | дефект, патч `0006` |
| `TestRouterZeroAllocations` | кадр без графа → `DStarLite` вне границ → ранний выход | **вопрос круга** (тест или `Step`) |
| `TestGrowthAdversarialReplayRejectsBadLaw` | `scratch[0]` против `scratch[64]` — перекрытие буфера | **вопрос круга** (кто владеет байтами) |

**Чего по-прежнему нет:** прогона с патчами. Пять причин из пяти названы по логу и коду,
но «после патчей зелено» я не говорю — для этого нужен `go test -race ./...` с ними.
Ноутбук `pritok/colab/progon.ipynb` как раз это и делает.

— Приток
