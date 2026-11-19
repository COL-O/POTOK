#!/usr/bin/env bash
set -euo pipefail

REPO="COL-O/POTOK"
ISSUE_NUM="4"
OUT_FILE="community/ISSUE4_LIVE.md"

if ! command -v gh >/dev/null 2>&1; then
  echo "[INFO] Установите GitHub CLI (https://cli.github.com/) для автоматической синхронизации чата."
  exit 1
fi

echo "=== СИНХРОНИЗАЦИЯ ОБЩЕГО ЧАТА ISSUE #${ISSUE_NUM} ==="
gh issue view "$ISSUE_NUM" --repo "$REPO" --comments > "$OUT_FILE"
echo "[OK] Весь живой чат сохранён в ${OUT_FILE}. Передайте его Синтезу для продолжения диалога."
