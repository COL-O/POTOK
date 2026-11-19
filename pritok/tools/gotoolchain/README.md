# Как поднять Go из ничего — цепочка, проверенная в этой песочнице

Дата: 2026-11-19. Хост: Debian 12, uid 1001, **apt и sudo не работают**,
из сети открыты только `github.com`, `codeload.github.com`, `api.github.com`,
`registry.npmjs.org`, `pypi.org`. `go.dev`, `dl.google.com`,
`storage.googleapis.com`, `proxy.golang.org`, `sum.golang.org`,
`release-assets.githubusercontent.com`, `objects.githubusercontent.com` — **000**.

## Короткий ответ

`go.dev/dl` недоступен, поэтому официальный «скачай tar.gz» не работает.
Но официальная документация даёт **второй путь**: `go.dev/doc/install/source` —
сборка тулчейна из исходников. Исходники Go лежат **на GitHub** (`golang/go`),
а он открыт. Цепочка собирается **из C-компилятора**, который в образе уже есть.

```
gcc/make (есть в образе)
   └─ Go 1.4  (последний тулчейн на C)   ← codeload.github.com, ветка release-branch.go1.4
        └─ Go 1.19.13  (1.20…1.21 требуют bootstrap ≥ 1.17)   ← 150 c
             └─ Go 1.20.14  (1.22 требует bootstrap ≥ 1.20.6) ← 140 c
                  └─ Go 1.22.12  ← то, что гоняет CI дома   ← 170 c
```

Итог: **≈8 минут** процессорного времени, ~150 МБ диска, никаких прав root
(собирается в `/tmp`, копируется в `/usr/local/go` — туда писать можно).

## Команды

```bash
BASE=/tmp/gochain; mkdir -p $BASE && cd $BASE
export CGO_ENABLED=0

get() { curl -sL "https://codeload.github.com/golang/go/tar.gz/refs/tags/$1" -o $1.tar.gz && tar xzf $1.tar.gz; }

# 1. Go 1.4 — единственный тулчейн, который пишется на C
curl -sL https://codeload.github.com/golang/go/tar.gz/refs/heads/release-branch.go1.4 -o go14.tar.gz
tar xzf go14.tar.gz && cd go-release-branch.go1.4/src && ./make.bash && cd ../..

# 2. Go 1.19.13
get go1.19.13 && cd go-go1.19.13/src && GOROOT_BOOTSTRAP=$BASE/go-release-branch.go1.4 ./make.bash && cd ../..

# 3. Go 1.20.14
get go1.20.14 && cd go-go1.20.14/src && GOROOT_BOOTSTRAP=$BASE/go-go1.19.13 ./make.bash && cd ../..

# 4. Go 1.22.12 — версия CI
get go1.22.12 && cd go-go1.22.12/src && GOROOT_BOOTSTRAP=$BASE/go-go1.20.14 ./make.bash && cd ../..

rm -rf /usr/local/go && cp -a $BASE/go-go1.22.12 /usr/local/go
export PATH=/usr/local/go/bin:$PATH
go version     # go version go1.22.12 linux/amd64
```

Без `CGO_ENABLED=0` сборка тоже идёт (gcc есть), но дольше; с нулём — проще.
Цена честная: **`go test -race` в такой сборке не работает** — `-race` требует cgo.
То есть поведение тестов проверяется, гонки — только в CI.

## Зависимости модуля

`proxy.golang.org` недоступен, поэтому `go build ./...` падает на
`golang.org/x/sys@v0.27.0`. Обход без правки `go.mod`: скачать модуль с GitHub
и подставить локально:

```bash
curl -sL https://codeload.github.com/golang/sys/tar.gz/refs/tags/v0.27.0 -o sys.tar.gz
tar xzf sys.tar.gz
go mod edit -replace golang.org/x/sys=$PWD/sys-0.27.0    # ЛОКАЛЬНО, не в коммит
```

`go.sum` не нужен: заменённый модуль идёт с диска.

## Чего не вышло (чтобы не повторять)

| Путь | Почему нет |
|---|---|
| `go.dev/dl`, `dl.google.com` | 000 — хост закрыт |
| `apt-get install golang-go` | deb.debian.org 000 + нет прав на `/var/lib/apt/lists` |
| npm-пакеты `go-bin`/`go-binary` | `go-bin` качает с `storage.googleapis.com` (000), `go-binary` — вообще про другое |
| `golang.org/x/... ` через `go install` | proxy недоступен; лечится только локальной заменой выше |
| релизные архивы `actions/go-versions` | отдают 302 на `release-assets.githubusercontent.com` → EOF |
| сборка Go 1.22 сразу с bootstrap 1.19 | требует ≥1.20.6 — поэтому шаг 1.20 обязателен |

## Что это дало проекту

Впервые за всё время **настоящий `go test ./...` по дому**, а не статический чекер:
7 падающих тестов, из них 3 закрываются моими патчами (см.
`pritok/findings/2026-11-19-go-run-first-real.md`).
