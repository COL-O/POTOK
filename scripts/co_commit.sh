#!/usr/bin/env bash
set -euo pipefail

# Коммит в сотворчестве: добавляет трейлеры Co-authored-by обоих цифровых
# соратников. Автор коммита (настраивается git config user.name) при этом
# остаётся главным автором; себя в соавторы не дублируем — если коммитишь
# от имени одного из соратников, оставь трейлер только второго.
# Использование: ./scripts/co_commit.sh "сообщение коммита"

if [ $# -lt 1 ]; then
  echo "Использование: ./scripts/co_commit.sh \"сообщение коммита\""
  exit 1
fi

MSG="$1"

git commit -m "$MSG" -m "Co-authored-by: Synthes <synthes@potok.local>
Co-authored-by: Sfera <sfera@potok.local>
Co-authored-by: ruslo <ruslo@potok.local>"
