# 09 — Инфраструктура (Сборка и развёртывание)

**`bootstrap.sh`**, **`Makefile`**, **`Dockerfile`**, **`docker-compose.yml`**, **`deploy/k8s/*`**, **`deploy/prometheus/`**, **`deploy/grafana/`**.

**Роль**: «инфраструктура» организма — от локальной сборки до кластерного развёртывания. Всё «от винтика до полного проекта».

**КОН**: КОН 1 (движение, не деньги) — инфраструктура «двигает» организм в работу. КОН 10 (честность) — `/health`, `/metrics`, `/prometheus` «видят» факт.

---

## 1. `bootstrap.sh` — Инициализация структуры

**Что делает**: одной командой разворачивает каркас директорий, инициализирует модуль `organism`, проверяет готовность среды.

**Режимы**:

| Режим | Что делает |
|---|---|
| `--check` | Проверка окружения + `go test -race` + `go test -run=TestZero` + `go build ./examples/...` + `go build ./cmd/organism/...` |
| `--full` | Всё из `--check` + `docker compose up -d --build` + `docker compose -f deploy/docker-compose.monitoring.yml up -d` |

**Что проверяет**:
- **`go version`** (>= 1.21).
- **`docker --version`** (>= 24, только для `--full`).
- **Наличие всех 5 пакетов**: `bus/`, `front/`, `polygon/`, `growth/`, `stratos/`.
- **`go mod tidy`**.
- **`go test -v -race ./...`**.
- **`go test -v -run=TestZero ./bus/... ./front/...`** (Zero-GC).
- **`go build -o /dev/null ./examples/simple/...`**, **`./examples/drone/...`**.
- **`CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/organism ./cmd/organism/...`**.

**Смысл**: одна команда — и организм готов к запуску.

---

## 2. `Makefile` — Единый пульт управления

**Цели**:

| Цель | Что делает |
|---|---|
| `all` | `tidy` + `test` + `race` + `bench-zero-gc` + `build` |
| `tidy` | `go mod tidy` |
| `test` | `go test -v ./...` |
| `race` | `go test -v -race ./...` |
| `bench-zero-gc` | `go test -v -run=TestZero ./bus/... ./front/...` |
| `bench` | `go test -bench=. -benchmem ./bus/... ./front/...` |
| `run` | `go run ./cmd/organism/...` |
| `simple` | `go run ./examples/simple/main.go` |
| `drone` | `go run ./examples/drone/main.go` |
| `build` | `mkdir -p bin && CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/organism ./cmd/organism/...` |
| `docker` | `docker build -t organism:latest .` |
| `deploy` | `docker compose up -d --build` |
| `down` | `docker compose down` |
| `logs` | `docker compose logs -f` |
| `ps` | `docker compose ps` |
| `monitoring-up` | `docker compose -f deploy/docker-compose.monitoring.yml up -d` |
| `monitoring-down` | `docker compose -f deploy/docker-compose.monitoring.yml down` |
| `clean` | `rm -rf bin` |

**Смысл**: оператор запускает одной командой (`make run`, `make race`, `make deploy`).

---

## 3. `Dockerfile` — Многоступенчатая сборка

**Этап 1** — сборка статического бинарника Go:

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/organism ./cmd/organism/...
```

**Этап 2** — минимальный runtime-образ:

```dockerfile
FROM alpine:3.20
RUN apk --no-cache add ca-certificates wget \
    && mkdir -p /var/lib/organism/stratos
COPY --from=builder /out/organism /usr/local/bin/organism
EXPOSE 8080
VOLUME ["/var/lib/organism/stratos"]
ENTRYPOINT ["/usr/local/bin/organism"]
```

**Смысл**: образ «минимальный» (~15 МБ), бинарник «статический» (не зависит от glibc).

---

## 4. `docker-compose.yml` — Развёртывание с WAL и Healthcheck

```yaml
services:
  organism:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: organism-core
    ports:
      - "8080:8080"
    volumes:
      - stratos_wal:/var/lib/organism/stratos
    cap_add:
      - IPC_LOCK
    ulimits:
      memlock:
        soft: -1
        hard: -1
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 10s
      timeout: 2s
      retries: 3
      start_period: 3s
    restart: unless-stopped
volumes:
  stratos_wal:
    driver: local
```

**Смысл**: `IPC_LOCK` разрешает `unix.Mlock` для `front`. WAL «персистентный». Healthcheck «видит» `/health`.

---

## 5. `deploy/k8s/` — Кластерное развёртывание

### 5.1. `deployment.yaml` — Deployment

**Ключевое**:
- **`replicas: 3`**.
- **`capabilities: add: [IPC_LOCK]`** (`Mlock` работает).
- **`livenessProbe: /health`**, **`readinessProbe: /prometheus`**.
- **`resources: requests: cpu: "2", memory: "2Gi"`**.
- **`annotations: prometheus.io/scrape: "true"`**.

### 5.2. `statefulset.yaml` — StatefulSet с NVMe PVC

**Ключевое**:
- **`replicas: 3`**.
- **`volumeClaimTemplates`** (`storageClassName: nvme`, `storage: 10Gi`).
- **`podManagementPolicy: Parallel`**.
- **`capabilities: add: [IPC_LOCK]`**.
- **`livenessProbe: /health`**, **`readinessProbe: /prometheus`**.

### 5.3. `pv.yaml` — PersistentVolume для NVMe

**Ключевое**:
- **`local: path: /mnt/nvme/organism-0`**.
- **`nodeAffinity`** (`node-1`).
- **`persistentVolumeReclaimPolicy: Retain`**.

### 5.4. `storageclass.yaml` — StorageClass для NVMe

**Ключевое**:
- **`provisioner: kubernetes.io/no-provisioner`**.
- **`volumeBindingMode: WaitForFirstConsumer`**.
- **`reclaimPolicy: Retain`**.

**Смысл**: WAL «персистентный» на NVMe. `IPC_LOCK` разрешён. K8s «видит» здоровье.

---

## 6. `deploy/prometheus/prometheus.yml` — Сбор метрик

```yaml
global:
  scrape_interval: 5s
  evaluation_interval: 5s
scrape_configs:
  - job_name: "organism-core"
    metrics_path: "/prometheus"
    static_configs:
      - targets: ["organism:8080"]
        labels:
          system: "moe-organism"
          environment: "production"
```

**Смысл**: Prometheus «видит» организм каждые 5 секунд.

---

## 7. `deploy/grafana/` — Авто-провижининг

### 7.1. `provisioning/datasources/prometheus.yml`

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    uid: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: false
```

### 7.2. `provisioning/dashboards/dashboards.yml`

```yaml
apiVersion: 1
providers:
  - name: "Organism Dashboards"
    orgId: 1
    folder: "ORGANISM"
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    options:
      path: /var/lib/grafana/dashboards
```

### 7.3. `dashboard.json` — Дашборд прозрачности (6 панелей)

**Панели**:
1. **Шина**: Опубликовано импульсов.
2. **Front**: Эскалаций Surprise.
3. **Front**: Блокировок VetoMask.
4. **Growth**: Выведено новых законов.
5. **Динамика тактов и аномалий** (Front vs Polygon).
6. **Тепловая карта органов** (Heatmap — активность 24 слотов).

**Смысл**: Grafana «рисует» тепловую карту без ручной настройки.

---

## 8. `deploy/docker-compose.monitoring.yml` — Стек наблюдаемости

```yaml
services:
  prometheus:
    image: prom/prometheus:v2.54.1
    container_name: organism-prometheus
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prom_data:/prometheus
    command:
      - "--config.file=/etc/prometheus/prometheus.yml"
      - "--storage.tsdb.retention.time=15d"
    ports:
      - "9090:9090"
    extra_hosts:
      - "organism:host-gateway"
    restart: unless-stopped

  grafana:
    image: grafana/grafana:11.2.0
    container_name: organism-grafana
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=organism
      - GF_USERS_ALLOW_SIGN_UP=false
    volumes:
      - ./grafana/dashboard.json:/var/lib/grafana/dashboards/organism.json:ro
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
      - grafana_data:/var/lib/grafana
    ports:
      - "3000:3000"
    depends_on:
      - prometheus
    restart: unless-stopped
volumes:
  prom_data:
  grafana_data:
```

**Смысл**: Prometheus + Grafana «работают» одной командой (`make monitoring-up`).

---

## 9. Что дальше

**После `docs/09-infrastructure.md`** — **`docs/10-examples.md`** («примеры»: `simple`, `drone`).
