#!/usr/bin/env bash
set -euo pipefail
MODE="${1:---check}"
echo "=== ПРОВЕРКА И СБОРКА ОРГАНИЗМА (РЕЖИМ: ${MODE}) ==="
command -v go >/dev/null 2>&1 || { echo "[ERR] Требуется Go 1.21+"; exit 1; }
echo "[ENV] $(go version)"
go mod tidy
echo "[TEST] Запуск всех тестов с детектором гонок (-race)..."
go test -v -race ./...
echo "[TEST] Проверка Zero-GC (0.0 аллокаций)..."
go test -v -run=TestZero ./bus/... ./front/...
echo "[CHECK] Проверка примеров..."
go build -o /dev/null ./examples/simple/...
go build -o /dev/null ./examples/drone/...
echo "[BUILD] Сборка bin/organism..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/organism ./cmd/organism/...
echo "=== УСПЕШНО ЗАВЕРШЕНО ==="
