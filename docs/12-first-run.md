# 12 — Первый запуск

**Шаг 1** — `./bootstrap.sh --check`.
**Шаг 2** — `make run`.
**Шаг 3** — `curl /health`, `/metrics`, `/prometheus`.
**Шаг 4** — `go run ./examples/simple/...`, `go run ./examples/drone/...`.
**Шаг 5** — UDP-пакет на `:9000`.

**КОН**: КОН 10 (честность) — каждый шаг «показывает» факт. КОН 1 (движение, не деньги) — организм «двигается» от сборки к работе.

---

## 1. Шаг 1 — `./bootstrap.sh --check`

**Команда**:

```bash
cd /path/to/POTOK
chmod +x bootstrap.sh
./bootstrap.sh --check 2>&1 | tee bootstrap_check.log
```

**Что должно произойти**:

```
=== ИНИЦИАЛИЗАЦИЯ ЦИФРОВОГО ОРГАНИЗМА (РЕЖИМ: --check) ===
[ENV] go version go1.21.x linux/amd64
[OK] go.mod инициализирован (golang.org/x/sys v0.27.0)
[TEST] Запуск всех тестов с детектором гонок данных (-race)...
ok      organism/bus     0.5s
ok      organism/front   0.3s
ok      organism/polygon 0.4s
ok      organism/growth  0.6s
ok      organism/stratos 0.2s
ok      organism/network 0.3s
ok      organism/cmd/organism 0.1s
[TEST] Доказательство Zero-GC (0.0 аллокаций в куче)...
ok      organism/bus     (TestZero...)
ok      organism/front   (TestZero...)
[CHECK] Проверка компиляции примеров (examples/)...
[BUILD] Сборка статического бинарника bin/organism...
=== ПРОВЕРКА И СБОРКА УСПЕШНО ЗАВЕРШЕНЫ ===
```

**Если всё так** — Шаг 1 пройден. Переходим к Шагу 2.

**Если «упало»** — пришли лог (`bootstrap_check.log`). Разберём каждый этап по факту:

| Этап | Что может пойти не так | Что делать |
|---|---|---|
| `[ENV]` | `go` не установлен | Установить Go 1.21+ |
| `go mod init` | `go.mod` не создан | `go mod init organism` |
| `go get` | `golang.org/x/sys` не скачался | `go get golang.org/x/sys@v0.27.0` |
| `go test -race` | Ошибка компиляции (сигнатуры) | Прислать `file:line:column` + сообщение |
| `go test -race` | `FAIL` (логика) | Прислать имя теста + `t.Fatalf` |
| `go test -run=TestZero` | `allocs > 0` | Прислать имя теста + `allocs` |
| `go build ./examples/...` | `undefined` (сигнатуры) | Прислать ошибку `go build` |
| `go build ./cmd/organism/...` | `undefined` (сигнатуры) | Прислать ошибку `go build` |

---

## 2. Шаг 2 — `make run`

**Команда**:

```bash
make run
```

**Что должно произойти**:

```
=== ИНИЦИАЛИЗАЦИЯ ЦИФРОВОГО ОРГАНИЗМА (4 КОНТУРА ВРЕМЕНИ) ===
[Dashboard] HTTP-сервер прозрачности готов на :8080 (/health, /metrics, /prometheus)
[Gateway]   UDP-рецептор телеметрии и графов запущен на :9000
[Такт 1 | front]   Штатный рейс: Status=1, Confidence=0.99 | UDP-пакетов=0
[Такт 2 | front]   Слепое пятно HDC: Status=3 -> взведён Surprise!
[Такт 2 | polygon] Диагноз поставлен: Cause=77, Antibody=888, Confidence=0.91 (отправлено в Карантин front)
[Такт 3 | polygon] Выход за паспорт KAN (25.0 > 10.0) -> Эскалация в growth!
[Такт 3 | growth]  Выведен новый закон Gen#2: Guard=[0.0..25.0], BlackSwans=1/1, Alpha=0.1
[Такт 4 | front]   Повтор сложного сбоя: отработан как рефлекс! Confidence=0.95

=== ИТОГОВЫЙ ТЕПЛОВОЙ ОТЧЁТ ОРГАНИЗМА (КОН 10) ===
Шина (Bus):     Опубликовано=4 | Ошибок CRC32=0 | Пропусков эпохи=0
Слой front:     Surprise=1 | Заблокировано Veto=0 | Эскалаций в growth=1
Слой polygon:   Диагнозов=1 | Антител в Карантин=1 | Выдано Veto=0
Слой growth:    Новых законов=1 | Забраковано=0 | Реабилитаций Veto=0
Слой stratos:   Записей законов в WAL=1 | Токсичных меток=0
```

**Если всё так** — организм «живёт». Переходим к Шагу 3.

**Если `gateway.StartUDP(":9000")` «упал»** — порт занят. Освободить порт (`kill` процесса) или изменить порт в `main.go`.

---

## 3. Шаг 3 — `curl /health`, `/metrics`, `/prometheus`

**Команда**:

```bash
curl -s localhost:8080/health | jq
curl -s localhost:8080/metrics | jq
curl -s localhost:8080/prometheus | head -20
```

**Что должно произойти**:

**`/health`**:

```json
{
  "status": "healthy",
  "published_ticks": 4,
  "checksum_errors": 0,
  "bounds_errors": 0
}
```

**`/metrics`**:

```json
{
  "timestamp_ns": 1731000000000000000,
  "bus": {"published": 4, "checksum_errors": 0, "backpressure": 0},
  "front": {"surprise_count": 1, "veto_blocked": 0, "escalated_growth": 1},
  "polygon": {"diagnosed_count": 1, "antibodies_sent": 1, "veto_issued_count": 0},
  "growth": {"evolved_laws_count": 1, "rejected_laws_count": 0, "rollback_count": 0},
  "stratos": {"laws_recorded": 1, "toxic_marked": 0},
  "heatmap": [...]
}
```

**`/prometheus`**:

```
# TYPE organism_bus_published_total counter
organism_bus_published_total 4
# TYPE organism_front_surprise_total counter
organism_front_surprise_total 1
...
```

**Смысл**: организм «видит» себя честно (КОН 10).

---

## 4. Шаг 4 — `examples/simple` и `examples/drone`

**Команды**:

```bash
go run ./examples/simple/main.go
go run ./examples/drone/main.go
```

**Что должно произойти**:

**`simple`**:

```
Знакомый импульс: Status=1, Action=42, Confidence=0.99
Неизвестный импульс: Status=3, Surprises=1
```

**`drone`**:

```
[Полёт 1] Штатный режим: Status=1, Confidence=0.92, Вызовов DroneAcoustic=1
[Полёт 2] Акустический удар: ActionCode=707 (Уклонение), Confidence=0.96, NextMask=0x30
```

**Смысл**: организм «делится» под задачу (КОН 8).

---

## 5. Шаг 5 — UDP-пакет на `:9000`

**Команда** (Linux / WSL2):

```bash
# Отправляем тестовый Tensor-пакет (ORG1) на UDP :9000
printf '\x31\x47\x52\x4f\x01\x00\x00\x00\x64\x00\x00\x00\x0a\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00' | nc -u -w1 localhost 9000
```

**Что должно произойти**: `/metrics` покажет `PacketsReceived = 1`, `ImpulsesPushed = 1`.

**Смысл**: UDP-шлюз «принимает» реальную телеметрию.

---

## 6. Что делать, если что-то «упало»

**Пришли**:

1. Лог `bootstrap_check.log` (если Шаг 1 «упал»).
2. Лог `make run` (если Шаг 2 «упал»).
3. Вывод `curl` (если Шаг 3 «упал»).
4. Вывод `go run` (если Шаг 4 «упал»).
5. `/metrics` (если Шаг 5 «упал»).

**Я разберу каждый этап по факту.**

---

## 7. Что дальше

**После `docs/12-first-run.md`** — **`docs/13-roadmap.md`** («что дальше»: нагрузка, сценарии, правки).
