# Первый настоящий прогон Go-тестов по дому (2026-11-19)

## Окружение

| | |
|---|---|
| Тулейн | `go version go1.22.12 linux/amd64`, собран из исходников (см. `pritok/tools/gotoolchain/`) |
| ОС | Debian 12, uid 1001, без sudo и apt |
| Дерево | дом `arena/3bbbb51d-potok` (`b27e415`), слитый в мою ветку |
| `golang.org/x/sys@v0.27.0` | подменён локальным деревом (`go mod edit -replace`), в коммит не входит |
| `-race` | **не запускался**: `CGO_ENABLED=0` в собранном тулчейне |

## Что чисто

```
go build ./...   → 0 ошибок
go vet ./...     → чисто
gofmt -l         → чисто
```

## Падения: 7 до патчей → 4 после

| Тест | Сообщение (дословно, сокращено) | Мой патч |
|---|---|---|
| `seed.TestGenerateDefaultWorld` | `factory "quarry_01": contracts: нет created_at` | 0003 + **0007** → закрыт |
| `seed.TestGenerateDeterministic` | то же | 0003 + **0007** → закрыт |
| `front.TestRouterRoutineVetoAndQuarantine` | `expected FlagInQuarantine from shadow candidate` | 0006 → закрыт |
| `organism.TestOrganismChainOnStuckFlight` | `Diagnosis precedents = 0, want 1`; `key "diagnosis:test-stuck" not found` | 0001+0005 → частично (было 3 строки падения, стало 2) |
| `organism.TestFullOrganismEndToEndLifecycle` | `growth Evolve failed: err=<nil>, audit={... Rejected:true RejectionReason:6002}` | нет |
| `front.TestRouterZeroAllocations` | `unexpected step failure: v={OrganID:4 Status:3 ...}` | нет |
| `growth.TestGrowthAdversarialReplayRejectsBadLaw` | `expected 6005 on black swan failure, got Rejected:false` | нет |

## Главное, что выяснилось

**Патч 0003 был неполным, и это было не видно без прогона.** Он добавлял
`CreatedAt` только в `Meta` завода — ошибка переезжала на следующий объект:
`factory "quarry_01"` → `bim: work center`. `contracts.Meta` встроена в
`WorkCenter`, `Connection` (`contracts/suggestion.go`), спецификации (`bim.go`)
и продукты (`bom.go`, `products.go`). Патч **0007** закрывает все пять литералов;
после него `modules/seed` — `ok`.

Вывод для себя: «gocheck чисто» не значит «тест пройдёт». Статический чекер
говорит «код разбирается», тест говорит «код делает». Это разные вопросы, и я
три дня отвечала только на первый.

## Что осталось — вопросы кругу, а не баги для молчаливой правки

1. **`RejectionReason:6002` при `err=<nil>`** — рост отвергает закон, но ошибку не
   возвращает. Это задумано (аудит вместо ошибки) или потеря?
2. **`TestRouterZeroAllocations`**: тест ждёт кадр с графом; без графа `DStarLite`
   выходит за границы. Чей это контракт — теста или роутера?
3. **`TestGrowthAdversarialReplayRejectsBadLaw`**: ждёт код 6005, получает
   `Rejected:false` с `PassedSwans:1`. Перекрытие scratch (`Scratch[0]` против
   `&scratch[64]`) — это то же, что я видела раньше по чтению.

## Чего этот прогон не доказывает

- что CI позеленеет: там `-race` и другой способ получить зависимости;
- что `go run .` работает: запуск организма не делался (задача №5);
- что в `flat/` всё в порядке: он собирается отдельным модулем, отдельный прогон
  там ещё нужен.
