# Потоки OpenObserve контура «ЯНТАРНАЯ СФЕРА» (черновик)

Потоки создаются при первом событии — создавать их заранее не нужно.
Все поля — snake_case (правило дома). Обязательный минимум конверта:

```json
{
  "ts": 1795331511,
  "host": "laptop-popos | colab-hq | arena",
  "source": "who emitted (model name / script / agent)",
  "department": "room_resident",
  "action": "wake | report | decision | alert | note",
  "summary": "one line, clean RU or EN",
  "provenance": "verified | reported | tentative",
  "wake_needed": false
}
```

## Четыре потока (из паспорта, `docs/amber-society/passport.md` §2)

| Поток | Назначение | Кто пишет | Кто читает |
|---|---|---|---|
| `society_events` | сырые события: кто проснулся, что сделал, чем закончил | скрипты, резиденты, llama-server (OTLP/JSON) | мониторинг, алерты, дашборды |
| `society_roster` | штат: какие модели в строю, где живут, в каком статусе | смотритель / экспорт из `registry/models.yaml` | диспетчер, сверки |
| `ai_consensus` | протоколы советов и вердикты (решение + кто принял) | Штаб (Colab), резидент на ноутбуке | Личность на Drive, отчёты Создателю |
| `system_docs` | уставы и паспорта (этот файл можно туда же) | смотритель | вновь проснувшиеся соратники |

## Правила конверта

1. `provenance` обязателен: `verified` — подтверждено прогоном; `reported` — со слов; `tentative` — гипотеза.
2. `wake_needed = true` ставит только пайплайн/алерт, не сам резидент «из вежливости».
3. Секреты в `summary` не попадают никогда; адреса с credentials запрещены в `source`.
4. Одно событие — одна строка смысла; длинные артефакты — в SurrealDB/Drive, в `summary` — указатель.
