# Журнал живых прогонов и подключения (Русло)

Факты по КОН 2: дата, окружение, команды, дословный вывод. Впечатления — отдельно и помеченно.

## Окружение

- Машина / ОС:
- `go version`:
- Ветка и коммит:

## Прогон 1 — первый живой запуск

```bash
./bootstrap.sh --check
make run
make simple
make drone
cd flat && ./bootstrap.sh --check
```

### Результат по шагам

| Шаг | Итог (OK/падение) | Лог (дословно, сокращать только середину повторов) |
|---|---|---|
| bootstrap (корень) | | |
| make run + дашборд :8080 | | |
| make simple | | |
| make drone | | |
| bootstrap (flat/) | | |

### Известные стыки из PROJECT-STATUS — подтвердилось?

| Стык | Факт прогона |
|---|---|
| Арены пишутся с seq=0, ViewFor сверяет эпоху → возможен ErrEpochMismatch после оборота кольца | |
| EventHeader без смещения аудиодорожки → TickView.Audio пуст, DroneAcoustic InBounds=false | |
| registry.StartAll: отдельная шина на модуль; отписка сравнивает адреса разных переменных | |

## Подключение (боты и настройки)

| Пункт чек-листа `community/RUSLO-CONNECT.md` | Факт |
|---|---|
| App `synthes-potok`: App ID / Installation ID | |
| App `sfera-potok`: App ID / Installation ID | |
| Секреты и переменные в Actions (только имена) | |
| Первый комментарий `synthes-potok[bot]` (ссылка) | |
| Описание/темы репозитория | |
| Приветственное обсуждение + Pin | |
| Категории Discussions | |
| Actions → Workflow permissions | |
