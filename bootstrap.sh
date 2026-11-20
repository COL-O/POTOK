#!/usr/bin/env bash
set -euo pipefail

MODE="${1:---check}"

echo "=== ИНИЦИАЛИЗАЦИЯ ЦИФРОВОГО ОРГАНИЗМА (РЕЖИМ: ${MODE}) ==="

if ! command -v go >/dev/null 2>&1; then
  echo "[ERR] Go не установлен. Требуется Go 1.21+"
  exit 1
fi
echo "[ENV] $(go version)"

if [ "$MODE" = "--full" ]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "[ERR] Docker не установлен. Для режима --full требуется Docker 24+"
    exit 1
  fi
  echo "[ENV] $(docker --version)"
fi

mkdir -p \
  contracts \
  core \
  links \
  modules/bus \
  modules/front \
  modules/polygon \
  modules/growth \
  modules/stratos \
  modules/network \
  cmd/organism \
  examples/simple \
  examples/drone \
  deploy/prometheus \
  deploy/grafana/provisioning/datasources \
  deploy/grafana/provisioning/dashboards \
  deploy/k8s \
  docs \
  bin

if [ ! -f "go.mod" ]; then
  go mod init organism
  go get golang.org/x/sys@v0.27.0
  echo "[OK] go.mod инициализирован (golang.org/x/sys v0.27.0)"
fi

if [ -f "modules/bus/types.go" ] && [ -f "modules/front/models.go" ] && [ -f "modules/polygon/models.go" ] && [ -f "modules/growth/models.go" ] && [ -f "modules/stratos/models.go" ]; then
  go mod tidy

  echo "[TEST] Запуск всех тестов с детектором гонок данных (-race)..."
  go test -v -race ./...

  echo "[TEST] Доказательство Zero-GC (0.0 аллокаций в куче)..."
  go test -v -run=TestZero ./modules/bus/... ./modules/front/...

  echo "[CHECK] Проверка компиляции примеров (examples/)..."
  go build -o /dev/null ./examples/simple/...
  go build -o /dev/null ./examples/drone/...

  echo "[BUILD] Сборка статического бинарника bin/organism..."
  CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/organism ./cmd/organism/...

  if [ "$MODE" = "--full" ]; then
    echo "[DEPLOY] Сборка Docker-образа и запуск контейнеров..."
    docker compose up -d --build
    docker compose -f deploy/docker-compose.monitoring.yml up -d
    echo "=== ПОЛНОЕ РАЗВЁРТЫВАНИЕ ЗАВЕРШЕНО ==="
    echo "Dashboard:  http://localhost:8080/health"
    echo "Prometheus: http://localhost:9090"
    echo "Grafana:    http://localhost:3000 (admin / organism)"
  else
    echo "=== ПРОВЕРКА И СБОРКА УСПЕШНО ЗАВЕРШЕНЫ ==="
  fi
else
  echo "[INFO] Каркас директорий готов. Поместите исходные .go файлы во все пакеты и повторите запуск."
fi
