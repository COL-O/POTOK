# POTOK
Пробный проект

## Статус

Проектное описание поступает в обратном порядке — от окончания к началу; скелет «окончания» уже воплощён в коде.
Подробности: [docs/PROJECT-STATUS.md](docs/PROJECT-STATUS.md)

## Запуск скелета

```bash
go run .        # генерация мира (100 заводов), старт слоёв, тестовые рейсы
go test ./...   # unit + сквозной тест цепочки front→polygon→growth→stratos
```

Код — только стандартная библиотека Go (внешние зависимости скелета заменены локальными реализациями за теми же интерфейсами, см. разбор №3).

- Смысл проекта (организм на Go-сервере, Сфера Вернадского, система управления 100 заводами): [docs/concept/project-meaning.md](docs/concept/project-meaning.md)
- Реестр моделей (7 органов, слои front / polygon / growth / stratos): [docs/registry/models-55.md](docs/registry/models-55.md)
- Машиночитаемый реестр: [registry/models.yaml](registry/models.yaml)
- Слои polygon / growth / stratos — наборы моделей и как они решают: [docs/layers/polygon-growth-stratos.md](docs/layers/polygon-growth-stratos.md)
- Разборы и открытые вопросы: [docs/notes/review-01-models.md](docs/notes/review-01-models.md), [docs/notes/review-02-meaning-and-layers.md](docs/notes/review-02-meaning-and-layers.md)
