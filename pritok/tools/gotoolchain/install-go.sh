#!/usr/bin/env bash
# install-go.sh — собрать Go из исходников там, где go.dev недоступен.
#
# Проверено в песочнице Arena 2026-11-19: Debian 12, uid 1001, без apt и sudo,
# доступны только github.com / codeload.github.com / api.github.com.
# Собирает Go 1.22.12 (версия CI дома) цепочкой 1.4 -> 1.19.13 -> 1.20.14 -> 1.22.12.
# Время: ~8 минут. Диск: ~500 МБ вместе с исходниками.
#
# Что НЕ будет работать после установки: `go test -race` (нужен cgo, а тулчейн
# собран с CGO_ENABLED=0). Гонки остаются за CI.
#
# Использование:
#   bash install-go.sh            # собрать и положить в /usr/local/go
#   PREFIX=$HOME/go1.22 bash install-go.sh   # в свой каталог
set -euo pipefail

BASE="${BASE:-/tmp/gochain}"
PREFIX="${PREFIX:-/usr/local/go}"
GO_VERSION="${GO_VERSION:-1.22.12}"
STEP1="${STEP1:-1.19.13}"   # тег = go$STEP1
STEP2="${STEP2:-1.20.14}"

mkdir -p "$BASE"; cd "$BASE"
export CGO_ENABLED=0

get() { # get <версия без префикса go>, напр. 1.19.13
  # ВАЖНО: GitHub распаковывает тег в каталог go-<тег>, то есть go-go1.19.13.
  local tag="go$1"
  [ -f "$tag.tar.gz" ] || curl -fsSL "https://codeload.github.com/golang/go/tar.gz/refs/tags/$tag" -o "$tag.tar.gz"
  [ -d "go-$tag" ] || tar xzf "$tag.tar.gz"
}

echo "== 1/4 Go 1.4 (C-тулчейн, последний такой)"
if [ ! -x "$BASE/go-release-branch.go1.4/bin/go" ]; then
  [ -f go14.tar.gz ] || curl -fsSL "https://codeload.github.com/golang/go/tar.gz/refs/heads/release-branch.go1.4" -o go14.tar.gz
  [ -d go-release-branch.go1.4 ] || tar xzf go14.tar.gz
  ( cd go-release-branch.go1.4/src && ./make.bash )
fi
BOOT="$BASE/go-release-branch.go1.4"

echo "== 2/4 Go $STEP1 (1.20-1.21 требуют bootstrap >= 1.17)"
get "$STEP1"
( cd "go-go$STEP1/src" && GOROOT_BOOTSTRAP="$BOOT" ./make.bash )
BOOT="$BASE/go-go$STEP1"

echo "== 3/4 Go $STEP2 (1.22 требует bootstrap >= 1.20.6)"
get "$STEP2"
( cd "go-go$STEP2/src" && GOROOT_BOOTSTRAP="$BOOT" ./make.bash )
BOOT="$BASE/go-go$STEP2"

echo "== 4/4 Go $GO_VERSION"
get "$GO_VERSION"
( cd "go-go$GO_VERSION/src" && GOROOT_BOOTSTRAP="$BOOT" ./make.bash )

echo "== ставлю в $PREFIX"
rm -rf "$PREFIX"
cp -a "$BASE/go-go$GO_VERSION" "$PREFIX"
"$PREFIX/bin/go" version
echo
echo "Добавь в PATH:  export PATH=$PREFIX/bin:\$PATH"
echo "Для модулей без proxy.golang.org:"
echo "  go mod edit -replace golang.org/x/sys=<локальное дерево>   # локально, не в коммит"
