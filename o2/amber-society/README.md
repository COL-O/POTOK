# OpenObserve для «ЯНТАРНОЙ СФЕРЫ» — слой проекта

Собрано цифровым соратником **Приток** (окно Arena), 2026-11-22, по слову Создателя
(«поставь в проект OpenObserve, потом осмотри»).

> **Статус: черновики, не применялось.** Живой OpenObserve живёт на машинах Создателя и смотрителя
> (на дом смотрителя известен O2 v1.1.2, см. `o2/README.md`). Из этой песочницы до него не дотянуться;
> применение любого файла отсюда — только руками смотрителя на его машине. Ни один секрет сюда не кладётся.

## Почему живой O2 не установлен здесь

Песочница сборки не может поставить OpenObserve и это честно фиксируется:

- `docker` / `podman` отсутствуют;
- в релизах GitHub (v1.2.0-rc2, `gh api repos/openobserve/openobserve/releases/latest`, 2026-11-22)
  **нет серверного бинарника** — только `openapi.json`; распространение идёт через Docker-образы / Cloud / Enterprise;
- сборка из исходников (`codeload.github.com/openobserve/openobserve`) потребовала бы Rust-тулчейн
  и часы компиляции — и дала бы пустышку без данных, рядом с вашим настоящим O2, а не «ваш» O2.

Поэтому «поставить в проект» означает: **описать контур как код в репозитории** — применяет смотритель.

## Что сверено по официальным источникам 2026-11-22

| Утверждение из разбора | Факт по openobserve.ai / GitHub | Где |
|---|---|---|
| AI-набор: Providers, Playground, Prompts, Datasets, Experiments | Существуют; входят в **Enterprise edition / OpenObserve Cloud**, включаются `O2_EVAL_ENABLED=true`; меню: **AI > Evaluate / Annotate / Experiment**, провайдеры — **в Settings** (до 7 типов) | docs `llm-experiments/`, `llm-playground/`, `llm-prompts/`, PR openobserve-docs #631 |
| Remote Tasks | Существует, `/api/{org}/tasks`; **часть Remote Task/Experiments** (оценочные задачи, не общий мост). Поля: Endpoint (http для loopback/private), Auth (none/bearer/basic/API-key), Body Template c `{{input}}`,`{{metadata}}`,`{{context}}`, Output Path (по умолч. `$.output`), Timeout (по умолч. 60, макс 600 c), Max Attempts ≤ 3, Max Concurrency 1–32 (по умолч. 4), подпись **HMAC-SHA256** в `x-o2-signature` (ключ показывается один раз) | docs `llm-experiments/` (Remote Tasks), PR openobserve-docs #631 |
| OTLP ingest | `POST {base}/api/{org}/v1/traces|metrics|logs`, trailing slash запрещён; без коллектора не обязателен | docs `opentelemetry/` |
| Потоки создаются при первом событии | Подтверждено документацией crate `openobserve_client` («OpenObserve creates streams on first write») | docs.rs |
| Enrichment Tables | Существуют: **Pipelines > Enrichment Tables**, CSV, функция `get_enrichment_table_record("name", {…})` в VRL (query-time и ingestion-time); SQL-джойн — с префиксом `enrich.` | docs `data-processing/enrichment-tables/` |
| JSON ingest `POST /api/default/<stream>/_json` | Существует, стандартный вход | o2/part*.sh и документация O2 |
| VRL Pipelines (real-time + scheduled) | Существуют; real-time = per-event трансформация на приёме; **агрегации в VRL нет** — сводки делаются SQL/reports/scheduled pipeline | docs O2 `user-guide/data-processing/pipelines/` |
| «Remote Task → ноутбук → Google Диск → Colab» как синхронный вызов | **Не сходится**: Remote Task — синхронный HTTP с таймаутом ≤ 600 с; контур через Drive асинхронен с минутами ожидания. Детали: `remote-task.md` | вывод из сверки |

**Что НЕ проверено (сознательно):** доступность AI-набора и Remote Tasks на вашем self-hosted O2 v1.1.2:
версия, редакция (OSS vs Enterprise) и наличие `O2_EVAL_ENABLED` известны только смотрителю. Просьба к Создателю: спросить смотрителя, **не присылая токены** — нужны только факты: «версия, редакция, включён ли AI suite и какие пункты видны в меню».

## Состав папки

| Файл | Что это | Статус |
|---|---|---|
| `streams.md` | Конверт событий и 4 потока контура (с `wake_needed`) | черновик |
| `enrichment/roster_map.csv` | Справочник «тип задачи → исполнитель → комната в SurrealDB» (собран из `data/departments.json` и паспорта) | черновик, источник — слова Создателя |
| `vrl/society_events_enrich.vrl` | Пример real-time пайплайна: нормализация + `wake_needed` при критических событиях | черновик, не прогонялся |
| `vrl/wake_filter_to_alerts.vrl` | Пример фильтра «только будить на аномалию» | черновик, не прогонялся |
| `remote-task.md` | Что Remote Tasks умеют по документам и как с ними сшивать (и не сшивать) AMBER SOCIETY | анализ |
| `dashboard-amber-society.json` | Черновик дашборда «Янтарная Сфера — контур» (line/bar/table) | черновик, применить: `python3 o2/apply.py --file o2/amber-society/dashboard-amber-society.json --dry-run` |

## Чего здесь намеренно нет

- Паролей, токенов, адресов с credentials — никогда.
- Выдуманного синтаксиса: всё, что не сверено по документам, помечено «черновик / не прогонялось».
- Записи в живые системы: применение — только по слову Создателя и руками смотрителя.
