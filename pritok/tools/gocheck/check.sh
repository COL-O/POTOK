#!/usr/bin/env bash
# check.sh — статические проверки Go-кода POTOK без Go-тулчейна.
#
# В песочнице Arena недоступны go.dev / dl.google.com / proxy.golang.org,
# поэтому собрать и прогнать `go test ./...` здесь нельзя. Скрипт даёт
# минимально полезный контроль: разбор синтаксиса и типичные ошибки
# пакетов (неимпортированные и несуществующие имена, дубли объявлений,
# несоответствие числа аргументов, лишние импорты) через tree-sitter-wasm.
#
# Запуск:  bash pritok/tools/gocheck/check.sh
# Требует: node + npm (зависимости ставятся в pritok/tools/gocheck/node_modules,
#          который в .gitignore). На машине с Go пользуйтесь `go vet ./...`.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
# путь к проверяемому коду: аргумент либо переменная окружения.
# Раньше root был жёстко «../..», и прогон по чужому клону молча проверял
# сам POTOK — то есть «проверено» врало (найдено 2026-11-18).
root="${1:-${GOCHECK_TARGET:-$here/../../..}}"
root="$(cd "$root" && pwd)"

if ! command -v node >/dev/null 2>&1; then
  echo "нужен node (не найден в PATH)" >&2
  exit 2
fi

if [ ! -d "$here/node_modules/web-tree-sitter" ]; then
  echo "[gocheck] установка зависимостей (web-tree-sitter + tree-sitter-go wasm)…"
  ( cd "$here" && npm init -y >/dev/null 2>&1 && npm install --no-audit --no-fund --silent web-tree-sitter@0.25.10 tree-sitter-wasms )
fi

# Каждый скрипт обязан возвращать ненулевой код, когда нашёл проблему,
# иначе check.sh — не проверка, а отчёт «всё хорошо».
status=0
for script in parse.mjs check.mjs idents.mjs dup.mjs arity.mjs; do
  echo "[gocheck] $script"
  if ! ( cd "$here" && node "$script" "$root" ); then
    echo "[gocheck] $script: ЕСТЬ ПРОБЛЕМЫ (код ненулевой)" >&2
    status=1
  fi
done

echo "[gocheck] готово (код выхода $status)"
exit $status
