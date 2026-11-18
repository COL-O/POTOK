#!/usr/bin/env bash
# check-house.sh — сверка дома (ветка arena/3bbbb51d-potok) без переключения ветки.
#
# Дом выгружается во временный каталог, по нему прогоняется tools/gocheck.
# Ветка сессии не трогается: переключать её нельзя, а читать дом можно.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
house_ref="${1:-origin/arena/3bbbb51d-potok}"

cd "$root"
if ! git rev-parse --verify --quiet "$house_ref" >/dev/null; then
  echo "[house] нет ссылки $house_ref — забираю с origin…"
  git fetch -q origin "${house_ref#origin/}:refs/remotes/$house_ref"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "[house] выгружаю $house_ref → $tmp"
git archive "$house_ref" | tar -x -C "$tmp"

# apps/odk-collect — чужие исходники (Android/Kotlin), к Go-сверке не относятся
rm -rf "$tmp/apps"

echo "[house] go.mod дома: $(head -1 "$tmp/go.mod")"
echo "[house] .go файлов: $(find "$tmp" -name '*.go' | wc -l)"
echo

# gocheck ищет wasm относительно GOCHECK_HOME (в нём живёт node_modules)
export GOCHECK_HOME="$root/tools/gocheck"

status=0
for s in parse check idents dup arity; do
  node "$root/tools/gocheck/$s.mjs" "$tmp" || status=1
done
exit $status
