# Инструменты Притока

Все инструменты — без Go-тулчейна: в песочнице Arena его нет и поставить неоткуда
(`go.dev`, `dl.google.com`, `proxy.golang.org`, Debian-зеркала и GitHub release-assets закрыты;
открыты `github.com`, `api.github.com`, `codeload`, npm, pypi).

## 1. `tools/gocheck/` — статический контроль Go без компилятора

Пять проверок на tree-sitter (WASM, ставится из npm локально в `node_modules`, не коммитится):

| Скрипт | Что ловит |
|---|---|
| `parse.mjs` | ошибки синтаксиса в любом `.go` |
| `check.mjs` | обращения к неимпортированным пакетам, несуществующие имена и поля структур, неиспользуемые импорты |
| `idents.mjs` | идентификатор с большой буквы, использованный, но нигде не объявленный |
| `dup.mjs` | дубли объявлений на уровне пакета (ошибка компиляции) |
| `arity.mjs` | несовпадение числа аргументов при вызове своих функций |

Запуск:

```bash
bash tools/gocheck/check.sh            # свой мир (ветка сессии)
bash pritok/tools/check-house.sh       # дом из origin/arena/3bbbb51d-potok
```

Что **не** проверяется: типы, интерфейсы, дженерики, «мёртвый код». Это подмена `go vet`,
а не замена `go test`.

## 2. `pritok/tools/check-house.sh` — сверка дома без переключения ветки

Извлекает дом из `origin/arena/3bbbb51d-potok` во временный каталог и прогоняет по нему
тот же `gocheck`. Ветка сессии остаётся нетронутой — важно, потому что переключать её нельзя.

```bash
bash pritok/tools/check-house.sh
```

## 3. `pritok/tools/house-grep.sh` — чтение дома одной командой

`git show origin/arena/3bbbb51d-potok:<путь>` для нескольких файлов сразу: удобно сверять
место правки, не выгружая весь дом.

```bash
bash pritok/tools/house-grep.sh core/registry.go core/eventbus.go
```

## 4. Патчи (`pritok/patches/`)

Патч — это переносимая правка дома. Формат — `git diff --no-index`, применяется из корня дома:

```bash
cd /path/to/house            # дерево ветки arena/3bbbb51d-potok
git apply --check pritok/patches/0001-core-one-bus.patch
git apply pritok/patches/0001-core-one-bus.patch
go build ./... && go test -race ./...
```

Каждый патч в своей шапке несёт: адрес правки, причину, чем проверен, чего он **не** гарантирует.

## Чего у меня нет и не будет

- `go test` / `go build` — нет тулчейна. «Зеленеет» скажет только прогон у основателя или CI.
- Доступа в закрытый контур (`POTOK-INSIDE`, `podbor`) — 403 проверено, не заявляю.
- Права писать в PR №7 своим push'ем — только патчи.

— Приток
