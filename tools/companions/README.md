# Инструменты соратников узла Варанаси

Один инструмент — одна дверь: локальные модели узла выполняют задания проекта POTOK
и оставляют след в оболочке-центре и в общей памяти.

## Что это

`runner.py` — обёртка над тремя локальными моделями. Он не хранит ключей: пароли
OpenObserve и SurrealDB читаются из файлов на машине держателя узла
(`~/.config/domovoy/secrets/`). В репозиторий они не попадают.

## Команды

```bash
# кто на связи
python3 tools/companions/runner.py whoami

# спросить соратника
python3 tools/companions/runner.py ask --unit letopisets "Что такое поток в OpenObserve?"

# разобрать файл проекта
python3 tools/companions/runner.py review --unit arhitektor --path docs/varanasi-lab/README.md

# выполнить задание из файла и записать результат
python3 tools/companions/runner.py task --unit arhitektor --task-file task.md --out result.md
```

## Соратники

| Ключ | Имя | Модель | Порт | Зона |
|---|---|---|---|---|
| `letopisets` | Летописец | LFM2.5-350M | 8088 | короткие ответы по факту |
| `arhitektor` | Архитектор | LFM2.5-1.2B | 8091 | планы и связность |
| `izvlekatel` | Извлекатель | LFM2-350M-Extract | 8090 | JSON-извлечение (на доработке) |

## Что оставляет каждый вызов

- **Трейс** `gen_ai` в OpenObserve (`service.name = varanasi-companions`) — модель, задача,
  токены, время.
- **Строку** в SurrealDB `training_journal` (`kind = 'potok_task'`) — задание и ответ.

Так работает правило узла: **что сделано — то увидено и записано**.

## Границы

- Модели локальные: без машины держателя узла инструмент не работает.
- Извлекатель требует проверки человеком: его русский язык нестабилен.
- Секреты — только на машине, никогда в git.

— Инструктор узла Варанаси
