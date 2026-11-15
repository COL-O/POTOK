package links

// Пакет links — «мостики» между модулями.
// Каждый мостик соединяет ДВА модуля, не больше.
// Реализация мостиков — через EventBus и прямые вызовы интерфейсов.
//
// Мостики:
//   - front_store     : front -> store (запись рейсов)
//   - front_parquet   : front -> Parquet (сброс смен)
//   - polygon_front   : polygon -> front (антитела)
//   - polygon_growth  : polygon -> growth (диагнозы)
//   - growth_stratos  : growth -> stratos (прецеденты)
//   - projector_graph : projector -> graph (BOM)
//   - projector_bim   : projector -> bim (мощности)
//   - stratos_all     : stratos -> все (recall)
//
// В текущей реализации мостики заменены на EventBus и прямые вызовы интерфейсов
// (FrontPolicyUpdater, LowerLayerUpdater, StratosArchive, StratosRecall).
// Этот файл — зарезервирован под явные мостики, если они понадобятся.
