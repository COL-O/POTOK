# Реестр моделей (стартовая редакция)

> Статус: **черновик, зафиксирован до пояснения проекта**.
> Это начало проекта: сбор и подготовка моделей. Реализация (`models.go`, рантайм, контракты) — не начинается, пока не поступит пояснение проекта.
> Машиночитаемая версия: `registry/models.yaml`. Пометки и расхождения: `docs/notes/review-01-models.md`.

Исходная идея: реестр из заявленных 55 моделей — не «свалка» и не «одно поле», а **семь функциональных органов**, и каждый орган распределён по слоям организма:

| Слой | Смысл | Характерное время |
|---|---|---|
| `front` | Рефлекс | наносекунды–миллисекунды |
| `polygon` | Иммунитет | минуты–часы |
| `growth` | Рост | часы–дни |
| `stratos` | Память | годы–века |

Иерархия: организм один (реестр, шина, контракты), модели — в слоях.
`front` выбирает, какая модель срабатывает. `polygon` решает, какая модель права.
`growth` ищет новое. `stratos` помнит.

---

## Орган 1. Автономный транспорт и навигация («Автопилот и Лоция»)

Куда: `front` (Рефлекс) + `growth` (Рост, для планирования).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | UniAD | growth | Сквозное планирование (глобально) |
| 2 | VAD | front | Векторное восприятие (локально) |
| 3 | Hydra-MDP | front | Мульти-голова (выбор траектории) |
| 4 | DiffusionDrive / Diffusion Policy | growth | Генерация траекторий (развилки) |
| 5 | DriveLM / EMMA | polygon | Пространственное рассуждение |
| 6 | Occupancy Networks | front | 3D-карта занятости |
| 7 | RRT / Informed RRT* | front | Быстрая навигация (склад) |
| 8 | Hybrid A* | front | Поиск пути с физикой шасси |
| 9 | TEB (Timed Elastic Band) | front | Локальный объезд |
| 10 | ORCA / RVO2 | front | Коллективная навигация (рои) |

Комментарий: это не «модели для ERP», а модели для движения. Если организм управляет транспортом — нужны. Если только заводами — не нужны. Но D* Lite + TEB + ORCA полезны для логистики (тележки AGV, погрузчики).

## Орган 2. Узнавание ситуаций и логические переключатели («Радар и Засов»)

Куда: `front` (Рефлекс) + `stratos` (Память, для recall).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | HDC / VSA | front | Узнавание ситуации (наносекунды) |
| 2 | Tsetlin Machine | front | Жёсткие переключатели |
| 3 | Matrix Profile | polygon | Аномалии в потоке |
| 4 | LSH + HNSW | stratos | Поиск прецедентов (recall) |
| 5 | SNN (Spiking Neural Networks) | front | Реакция на редкие события |

Комментарий: HDC + Tsetlin — ядро `front`. Matrix Profile — ядро `polygon` (аномалии). LSH + HNSW — ядро `stratos` (recall). SNN — экзотика, но полезна для редких событий.

## Орган 3. Навигация по карте веток и объезд («Лоцман и Стрелочник»)

Куда: `front` (Рефлекс) + `polygon` (Иммунитет) + `growth` (Рост).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | D* Lite | front | Обход завала (локально) |
| 2 | CBS (Conflict-Based Search) | polygon | Конфликты ресурсов |
| 3 | MCTS | growth | Разведка веток |
| 4 | Беллман / A* | front | Кратчайший путь (статический) |
| 5 | MPNN (Message Passing NN) | growth | Передача сигналов по графу |
| 6 | TGN (Temporal Graph Networks) | polygon | Прогноз узких мест |
| 7 | GraphSAGE | growth | Оценка новых узлов |
| 8 | GATv2 (Graph Attention) | polygon | Динамические веса связей |
| 9 | Neural Logic Machines (NLM) | polygon | Логические правила на графах |
| 10 | DeepProbLog / Scallop | polygon | Нейро-логика (жёсткие правила) |

Комментарий: D* Lite + A* — ядро `front`. CBS + TGN + GATv2 — ядро `polygon` (конфликты). MCTS + GraphSAGE + MPNN — ядро `growth` (разведка). NLM + DeepProbLog — экзотика, но полезна для жёстких правил.

## Орган 4. Поиск формул и самообновление рецептов («Инженерный отдел»)

Куда: `growth` (Рост) + `stratos` (Память, для формул).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | KAN 2.0 | growth | Формула из таблиц |
| 2 | FastKAN / FasterKAN | growth | Ускоренная формула |
| 3 | LUT-KAN | growth | Формула в таблицах поиска |
| 4 | FourierKAN | growth | Циклы, сезонность |
| 5 | ChebyKAN | growth | Точное приближение |
| 6 | Wav-KAN | polygon | Резкие скачки, удары |
| 7 | SINDy | growth | Дифференциальные уравнения |
| 8 | PySR | growth | Эволюционный поиск формул |
| 9 | SymbolNet | growth | Символьная регрессия (поток) |

Комментарий: KAN-семейство + SINDy + PySR — ядро `growth` (поиск формул). Wav-KAN полезен в `polygon` (резкие скачки). LUT-KAN — быстрый (для `front`, если формула простая).

## Орган 5. Плавная регулировка и удержание баланса («Спинной мозг и Рулевой»)

Куда: `front` (Рефлекс) + `polygon` (Иммунитет).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | CfC (Closed-form Continuous-time) | front | Плавная регулировка |
| 2 | LTC (Liquid Time-Constant) | front | Адаптация к среде |
| 3 | NCP (Neural Circuit Policies) | front | Сверхкомпактное управление |
| 4 | ESN (Echo State Networks) | polygon | Прогноз загрузки |
| 5 | LSM (Liquid State Machines) | front | Реакция на события |
| 6 | MPC (Model Predictive Control) | front | Мягкая траектория |
| 7 | Фильтр Калмана (EKF/UKF) | front | Слияние датчиков |

Комментарий: CfC + NCP + MPC — ядро `front` (плавная регулировка). ESN — ядро `polygon` (прогноз). Фильтр Калмана полезен для слияния данных (если есть датчики).

## Орган 6. Фильтрация бесконечных потоков и память («Диспетчер»)

Куда: `polygon` (Иммунитет) + `stratos` (Память).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | Mamba-2 / Mamba-3 | polygon | Поток событий (сжатие) |
| 2 | RWKV-7 | polygon | Бесконечный конвейер |
| 3 | RetNet | stratos | Память с затуханием |
| 4 | xLSTM | polygon | Длинные цепочки |
| 5 | Griffin / Hawk | polygon | Потоковая телеметрия |
| 6 | Hyena Hierarchy | polygon | Длинные свёртки |
| 7 | Jamba | polygon | Гибрид (Mamba + Attention) |

Комментарий: Mamba-3 + RWKV-7 — ядро `polygon` (потоки). RetNet — ядро `stratos` (память с затуханием). Остальные — альтернативы (если Mamba/RWKV не подойдут).

## Орган 7. Модели мира, физика и симуляция («Воображение»)

Куда: `growth` (Рост) + `stratos` (Память).

| № | Модель | Слой | Роль |
|---:|---|---|---|
| 1 | I-JEPA / V-JEPA 2 | growth | Модель мира (физика) |
| 2 | TD-MPC2 | growth | Планирование траектории |
| 3 | DreamerV3 | growth | Симулятор среды |
| 4 | Decision Transformer | growth | Цепочка шагов к результату |
| 5 | GNS (Graph Network Simulators) | growth | Физика на графах |
| 6 | PINN (Physics-Informed NN) | polygon | Физика в нейросети |
| 7 | FNO (Fourier Neural Operator) | growth | Уравнения в частотной области |
| 8 | Active Inference | growth | Минимизация «удивления» |

Комментарий: TD-MPC2 + DreamerV3 + Active Inference — ядро `growth` (симуляция). V-JEPA 2 — экзотика (видео). PINN полезен в `polygon` (физика).

---

## Свод: распределение по слоям (как в исходнике)

| Слой | Модели | Кол-во | Время |
|---|---|---:|---|
| **front** (Рефлекс) | HDC, Tsetlin, SNN, D* Lite, A*, VAD, Hydra-MDP, Occupancy, RRT, Hybrid A*, TEB, ORCA, CfC, LTC, NCP, LSM, MPC, Калман | ~18 | наносекунды–миллисекунды |
| **polygon** (Иммунитет) | Matrix Profile, CBS, TGN, GATv2, NLM, DeepProbLog, Wav-KAN, ESN, Mamba-3, RWKV-7, xLSTM, Griffin, Hyena, Jamba, PINN, DriveLM, EMMA | ~17 | минуты–часы |
| **growth** (Рост) | UniAD, DiffusionDrive, MCTS, MPNN, GraphSAGE, KAN 2.0, FastKAN, LUT-KAN, FourierKAN, ChebyKAN, SINDy, PySR, SymbolNet, I-JEPA, TD-MPC2, DreamerV3, Decision Transformer, GNS, FNO, Active Inference | ~20 | часы–дни |
| **stratos** (Память) | HDC-индекс, LSH+HNSW, RetNet, Matrix Profile-индекс, SHA-256 | ~5 | годы–века |

Итого: заявлено 55 моделей, распределены по слоям. Не «в одном поле».

## Что с этим делать (как в исходнике)

### 1. Не «все 55 сразу»

55 моделей — это энциклопедия, а не минимальный набор. Для первого шага нужны 15–20:

- **front:** HDC, Tsetlin, D* Lite, TTL, Capacity, Pasport, CfC, MPC (8).
- **polygon:** KAN, CfC, Matrix Profile, Tsetlin, CBS, ESN (6).
- **growth:** MCTS, LUT-KAN, SINDy, Active Inference, GraphSAGE (5).
- **stratos:** HDC-индекс, LSH+HNSW (2).

Итого: 21 позиция (слоты по слоям). Остальные — потом, когда появятся задачи (транспорт, дроны, роботы).

### 2. Не «все нативно на Go»

- HDC, Tsetlin, D* Lite, MCTS, A* — нативно.
- KAN, CfC, Mamba-3, RWKV-7 — через ONNX или C-ядро.
- V-JEPA 2, TD-MPC2, DreamerV3 — через GPU (если есть).

### 3. «Организм общий» — но с иерархией

Организм один (реестр, шина, контракты). Но модели — в слоях. `front` выбирает, какая модель срабатывает. `polygon` решает, какая модель права. `growth` ищет новое. `stratos` помнит.

## Предлагаемый порядок структуры (не спешим, как в исходнике)

1. Сначала — `modules/front/models.go` (7 моделей: HDC, Tsetlin, D* Lite, MCTS, TTL, Capacity, Pasport).
2. Потом — `modules/polygon/models.go` (6 моделей: KAN, CfC, Matrix Profile, Tsetlin, CBS, ESN).
3. Только потом — `modules/growth/models.go` (5 моделей: MCTS, LUT-KAN, SINDy, Active Inference, GraphSAGE).
4. И `modules/stratos/models.go` (2 модели: HDC-индекс, LSH+HNSW).

## Что зафиксировано

- 55 моделей (заявлено) — 7 органов.
- Распределены по слоям: front (~18), polygon (~17), growth (~20), stratos (~5).
- Не «в одном поле».
- Для первого шага — 21 слот. Остальные — потом.
- Не все нативно на Go: HDC, Tsetlin, D* Lite, MCTS — нативно; KAN, CfC, Mamba-3 — через ONNX.
