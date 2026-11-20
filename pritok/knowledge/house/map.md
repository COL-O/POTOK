# Карта дома по факту (мои адреса, не документов)

Проверено чтением ветки дома `arena/3bbbb51d-potok` (голова `eb3a346`, затем `50b0cf7`
с CI на `arena/**`). Дата: 2026-11-19. Всё, что ниже, — адрес + что там на самом деле.

## Две раскладки

| | `main` (`478864d`) | дом (`arena/3bbbb51d-potok`) |
|---|---|---|
| Модуль | `module github.com/COL-O/POTOK` | `module organism` |
| `.go` файлов | 54 | 114 |
| Воркфлоу | нет | `ci.yml`, `codeql.yml`, `sfera-inbox.yml`, `ally-bridge.yml`, `synthes-mention.yml` |
| Инбокс | нет | `inbox/` + `scripts/sfera_inbox.py` |

**Вывод:** всё, что я делаю «для дома», надо адресовать ко второй колонке. Первая — история.

## Организм: где что

| Что | Адрес | Факт |
|---|---|---|
| Сборка мира | `main.go` (корень) | seed → store → BOM → BIM → модули → мостики → реестр |
| Сборка «живого» | `cmd/organism/main.go` | шина `modules/bus`, дашборд `:8080`, UDP `:9000` |
| Реестр модулей | `core/registry.go:63` | своя шина каждому модулю (закрыто патчем 0001) |
| Шина событий | `core/eventbus.go` | отписка невозможна, `Publish` через `go func` (патч 0002) |
| Герметичная шина | `modules/bus/bus.go` | кольца, `Publish`/`ViewFor`/`CommitVerdict`, арены |
| Датчик мира | `modules/network/gateway.go` | UDP-кадры Tensor/Graph, пул буферов, счётчики потерь |
| Память | `modules/stratos/store.go` | WAL 56 байт без состояния, replay нет (патч 0004) |
| Мостики | `links/*.go` | до патча 0005 — заглушки `_ = bus` |

## Где слова расходятся с кодом

| Документ говорит | Код говорит | Проверено |
|---|---|---|
| «долгая память», «стража» | WAL хранит только `recType/id/ts/seal`, состояния нет | чтением, 2026-11-18 |
| «цепочка работает» (`docs/PROJECT-STATUS.md`) | своя шина на модуль → доставки нет | чтением + логом Colab |
| «PR №7 — 233 файла» | 2834 файла | GitHub API, `docfact.mjs` |
| «первый запуск пройден» (`docs/13-roadmap.md`) | `docs/ruslo-lab/RUN-LOG.md` пуст | `docfact.mjs` |

## Тесты: где болит (все пять — с причиной)

| Тест | Причина | Чем закрыто |
|---|---|---|
| `TestGenerateDefaultWorld`, `TestGenerateDeterministic` | `Meta` без `CreatedAt` (`modules/seed/factories.go:52-57`) | патч 0003 |
| `TestOrganismChainOnStuckFlight` | одна шина на всех (`core/registry.go:63`) | 0005 + 0001 |
| `TestRouterRoutineVetoAndQuarantine` | `>` вместо `>=` (`modules/front/router.go:311`) | 0006 |
| `TestRouterZeroAllocations` | кадр без графа → `DStarLite` вне границ → ранний выход | вопрос круга |
| `TestGrowthAdversarialReplayRejectsBadLaw` | LUT пишется с `Scratch[0]`, читается с `scratch[64]` | вопрос круга |
