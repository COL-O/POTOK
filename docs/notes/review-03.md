# Review 03 — Открытые вопросы (сборка)

**Что** **уже** **есть**: **`docs/08-organism.md`** (`main.go`, `dashboard.go`), **`docs/09-infrastructure.md`** (`bootstrap.sh`, `Makefile`, Docker), **`docs/10-examples.md`** (`simple`, `drone`).

**Открытые** **вопросы**:

1. **`main.go`** **`LIFO defer`** — **что** если `gateway.Stop()` зависнет на `wg.Wait()`? **Нужен** ли таймаут 5 с (уже есть в `Stop()`)?
2. **`dashboard.Start()`** **`net.Listen`** — **что** если порт 8080 занят? **`log.Fatalf`** — **правильно** ли?
3. **`bootstrap.sh --check`** — **что** если `go test -race` долго? **Нужен** ли таймаут?
4. **`docker-compose.yml`** **`IPC_LOCK`** — **что** если Docker не даёт `CAP_IPC_LOCK`?
5. **`deploy/k8s`** **`storageClassName: nvme`** — **что** если кластер не имеет NVMe?
