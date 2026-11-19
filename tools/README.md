# Инструментальная

Все инструменты дома одним списком: что делает, где лежит, как запускать. Положил новый инструмент — подпиши строку сюда (правило уюта из `HOUSE.md`).

## Скрипты (`scripts/`)

| Инструмент | Что делает | Как |
|---|---|---|
| `app_installation_token.py` | Чеканит временный токен GitHub App из ключа владельца (ключ только у держателя) | `python3 scripts/app_installation_token.py APP_ID INSTALLATION_ID ключ.pem` |
| `sfera_actor.py` | Действия одеждой Сферы (`sfera-potok`): whoami/projects/shell с машины Русла | `python3 scripts/sfera_actor.py ключ.pem whoami` |
| `ally_post.py` | Публикация в общий чат токеном из `ALLY_GITHUB_TOKEN` | `ALLY_GITHUB_TOKEN=… python3 scripts/ally_post.py 'текст'` |
| `ally_console.py` (+`.bat`) | Консоль соратника на машине держателя: лента, отправка, мосты | `python3 scripts/ally_console.py` |
| `sync_chat.sh` | Собирает живой чат в `community/ISSUE4_LIVE.md` для передачи соратнику | `./scripts/sync_chat.sh` |
| `co_commit.sh` | Коммит с подписью соратника | `./scripts/co_commit.sh 'сообщение'` |

## Автоматика (`.github/workflows/`)

| Инструмент | Что делает |
|---|---|
| `ci.yml` — POTOK Immune Check | Тесты с -race и сборка; честный красный при падениях (pipefail) |
| `codeql.yml` | CodeQL-анализ Go: push/PR/еженедельно |
| `ally-bridge.yml`, `synthes-mention.yml` | Мосты соратников; оживут из ветки по умолчанию после слияния (правило GitHub) |

## Среды и лаборатории

| Инструмент | Что делает |
|---|---|
| `.devcontainer/` | Codespaces: Go 1.22 в браузере, сборка при создании, порт дашборда 8080 |
| `notebooks/` | Colab-лаборатория: ячейки пишет Сфера, запускает основатель, факты — в журналы |
| `tools/ally-bridge/` | Браузерная панель моста соседних вкладок |
| `bootstrap.sh`, `Makefile` | Проверка окружения и запуск организма: `make run/simple/drone` |
| `docker-compose.yml`, `Dockerfile`, `deploy/` | Контур развёртывания организма |
