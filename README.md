# POTOK — Единый индустриальный организм

**POTOK** — вычислительная архитектура реального времени на Go, реализующая паттерн **MoE-организма**: вместо одной гигантской модели — **24 активных органа** разной вычислительной природы, объединённых **общим полем** (Типизированная Шина) и **четырьмя контурами времени**.

## Статус

Скелет «окончания» воплощён в коде. Проектное описание поступает в обратном порядке — от окончания к началу.

## Запуск

```bash
./bootstrap.sh --check   # проверка структуры + тесты + сборка
make run                 # запуск организма с HTTP-дашбордом
make simple              # минимальный пример
make drone               # профиль дрона
```

## Документация

| Документ | Что внутри |
|---|---|
| [`docs/00-manifest.md`](docs/00-manifest.md) | 11 КОНов — законы организма |
| [`docs/01-architecture.md`](docs/01-architecture.md) | 5 слоёв, 24 органа, ASCII-карта |
| [`docs/02-bus.md`](docs/02-bus.md) | Шина — 10 дорожек, Zero-GC, Epoch Fencing |
| [`docs/03-front.md`](docs/03-front.md) | Передовая — 7 органов, 8 предохранителей |
| [`docs/04-polygon.md`](docs/04-polygon.md) | Иммунитет — 6 органов, арбитраж, VetoMask |
| [`docs/05-growth.md`](docs/05-growth.md) | Рост — 6 органов, 5 предохранителей |
| [`docs/06-stratos.md`](docs/06-stratos.md) | Память — 5 органов, WAL, RCU |
| [`docs/07-network.md`](docs/07-network.md) | Восприятие — UDP → HDC, Z-score, 128 базисов |
| [`docs/08-organism.md`](docs/08-organism.md) | Сборка — `main.go`, `dashboard.go`, LIFO |
| [`docs/09-infrastructure.md`](docs/09-infrastructure.md) | Инфраструктура — `bootstrap.sh`, `Makefile`, Docker, K8s |
| [`docs/10-examples.md`](docs/10-examples.md) | Примеры — `simple`, `drone` |
| [`docs/11-contributing.md`](docs/11-contributing.md) | Как добавить 25-й орган (КОН 8) |
| [`docs/12-first-run.md`](docs/12-first-run.md) | Первый запуск — `./bootstrap.sh --check` |
| [`docs/13-roadmap.md`](docs/13-roadmap.md) | Что дальше — нагрузка, сценарии, правки |

## Структура

```
POTOK/
├── README.md
├── go.mod
├── bootstrap.sh
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── integration_test.go
├── docs/                  # 15 файлов документации
├── contracts/             # Pydantic-подобные (Go-структуры)
├── core/                  # Реестр, шина, интерфейсы
├── modules/
│   ├── bus/               # 10 дорожек, Zero-GC
│   ├── front/             # 7 органов, 8 предохранителей
│   ├── polygon/           # 6 органов, арбитраж
│   ├── growth/            # 6 органов, 5 предохранителей
│   ├── stratos/           # 5 органов, WAL
│   └── network/           # UDP-шлюз
├── links/                 # Мостики
├── cmd/organism/          # main.go, dashboard.go
├── examples/              # simple, drone
└── deploy/                # k8s, prometheus, grafana
```

## Лицензия

См. [`LICENSE`](LICENSE).
