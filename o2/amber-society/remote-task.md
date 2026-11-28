# Remote Tasks в «ЯНТАРНОЙ СФЕРЕ» — сверка с документацией (2026-11-22)

## Что Remote Tasks являются по документам

Remote Task — это **опубликованная, версионируемая внешняя задача для экспериментов** (LLM Evaluations),
а не общий мост «O2 ↔ любые внешние исполнители». Эксперимент ссылается на неё как `name@version`,
чтобы прогон не плыл к непроверенной версии.

| Раздел формы | Поля |
|---|---|
| Identity | Name, Description |
| Endpoint | Method и Endpoint URL. Публичным хостам — только `https`; loopback/private могут `http`; URL с credentials или fragment отклоняется |
| Auth | none / bearer Token / basic (Username+Password) / API key в кастомном заголовке (Header Name) |
| Request Contract | Custom Headers; Body Template (JSON) с плейсхолдерами `{{input}}`, `{{metadata}}`, `{{context}}`; **Output Path** — откуда читать ответ в JSON сервиса (по умолчанию `$.output`) |
| Limits | Timeout (по умолчанию 60 c, макс 600 c), Max Attempts (макс 3), Max Concurrency (1–32, по умолчанию 4); переопределяются на эксперимент |
| Request Signing | «Sign Every Request»: HMAC-SHA256 в заголовке `x-o2-signature`; ключ показывается один раз; ротация в 4 шага (Create → Test → Activate → grace ≤ 24 ч / End Grace Now) |
| API | `/api/{org}/tasks`: list/create/get/update/delete, drafts и версии, test connection, test run (до 10 samples, ничего не записывается), signing rotate/activate/end-grace, stats |

Включение: `O2_EVAL_ENABLED=true`; AI-набор — **Enterprise edition / OpenObserve Cloud**; меню:
**AI > Evaluate / Annotate / Experiment**, провайдеры — в **Settings**.

## Что это значит для нас — три сценария

### Сценарий 1. Remote Task → локальный LFM2.5-350M на том же ноутбуке — ДА, сходится
`http://127.0.0.1:8088/v1/chat/completions` отвечает за секунды, укладывается в timeout,
auth — bearer/API-key, Body Template разумный, Output Path = `choices[0].message.content`
(или обёртка-адаптер, если нужен `$.output`). Это честный «быстрый золотой прогон» провайдера.

### Сценарий 2. Remote Task → ноутбук → Google Диск → Colab (DeepSeek-R1-7B) → назад
НЕ сходится и не может: Remote Task — **синхронный HTTP** с таймаутом ≤ 600 с, а контур
«bridge → inbox_to_colab → Drive-синк → 7B-совещание → outbox → Drive-синк → laptop_bridge»
асинхронен и живёт минутами–десятками минут. Даже если туннель поднимет URL на Colab,
ожидание ответа в 600 с на T4 с 2-раундовыми совещаниями нереально; O2 просто отменит прогон по timeout.
Пастед-документ подаёт это как «главный мост» — это не так.

### Сценарий 3. Тяжёлое контурное решение (7B + 28 соратников) с оценкой в O2 — через SDK
Правильный тип задачи для асинхронного контура — **SDK task**: ваш процесс сам гоняет
совещание и репортит результат обратно (`experiment report` / `taskFingerprint`). Либо проще:
Colab пишет вердикт напрямую в поток `ai_consensus` (POST `_json`), а оценку качеству даёт
человек/судьи по этому потоку — без Remote Task вообще.

## Рекомендуемый порядок (по слову Создателя)

1. Смотреть в UI: **Settings → AI Providers** (локальный `lfm2.5-350m-local`, bearer `local`,
   base URL `http://127.0.0.1:8088/v1`).
2. **AI > Experiment > Remote Tasks → New** только под сценарий 1 (быстрые синхронные судьи).
3. Для Colab-контура: поток `ai_consensus` + (опционально) SDK-репорт в эксперимент.
4. Ключ подписи Remote Task хранится у смотрителя; его значение не передаётся никому,
   в том числе не кладётся в репозиторий или чат.

## Источники

- openobserve.ai/docs/integration/ai/llm-experiments/ (Remote Tasks, 2026-10-15)
- PR openobserve-docs #631 (обновление меню AI и полей Remote Tasks)
- openobserve.ai/docs/integration/ai/llm-playground/ и …/llm-prompts/ (связанный набор)
- heise.de (OpenObserve 1.0.0 «AI Observability», 2026-09-14)
