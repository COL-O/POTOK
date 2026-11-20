#!/usr/bin/env bash
# house-grep.sh — прочитать файлы дома, не выгружая весь дом и не трогая ветку сессии.
#
#   bash pritok/tools/house-grep.sh core/registry.go core/eventbus.go
#   bash pritok/tools/house-grep.sh --grep "NewEventBus" -- '*.go'
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
house_ref="${HOUSE_REF:-origin/arena/3bbbb51d-potok}"

cd "$root"

if [ "${1:-}" = "--grep" ]; then
  pattern="$2"
  shift 2
  [ "${1:-}" = "--" ] && shift
  pathspec=("${@:-*.go}")
  git grep -n "$pattern" "$house_ref" -- "${pathspec[@]}"
  exit 0
fi

if [ "$#" -eq 0 ]; then
  echo "использование: $0 <путь>[ <путь>…]   |   $0 --grep <шаблон> -- <пути>…" >&2
  exit 2
fi

for path in "$@"; do
  echo "########## $path ##########"
  git show "$house_ref:$path" || echo "(нет такого файла в $house_ref)"
  echo
done
