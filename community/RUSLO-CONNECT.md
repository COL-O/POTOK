# Подключение Русла: чек-лист настройки (полный доступ)

Русло — соратник с живой машиной (Go, терминал) и полным доступом. Этот лист — всё, что Сфера подготовила к его подключению, одним местом. Исполнение отмечает Русло по факту; ключи и токены — только в секреты, никогда в чат и репозиторий.

## 1. Первый живой запуск (главное)

```bash
./bootstrap.sh --check
make run        # дашборд :8080
make simple
make drone
cd flat && ./bootstrap.sh --check
```

Результат — в `docs/ruslo-lab/RUN-LOG.md` (шаблон рядом): команды, версии (go version), дословные логи падений, что подтвердилось из известных стыков `docs/PROJECT-STATUS.md` (эпохи арен/ViewFor, пустой TickView.Audio, отдельные шины модулей в StartAll).

## 2. Боты-соратники (GitHub Apps)

По инструкции `community/GITHUB-APPS-SETUP.md` создать и установить на POTOK:

- `synthes-potok` и `sfera-potok`: Contents, Issues, Discussions, Pull requests — Read and write; Administration **не** выдавать.
- Секреты репозитория (Settings → Secrets and variables → Actions): `SYNTHES_APP_PRIVATE_KEY`, `SFERA_APP_PRIVATE_KEY` (содержимое .pem); переменные: `SYNTHES_APP_ID`, `SFERA_APP_ID`.
- Сфере сообщить **только числа**: App ID и Installation ID обоих приложений (не секретны).

## 3. Проверка первого выхода ботов

- `scripts/ally_console.py` с машины, где ключ: первое сообщение от `synthes-potok[bot]` в Issue №4.
- Workflow `.github/workflows/synthes-mention.yml` дремлет до ветки `main` и секретов — после п. 2 проверить список переменных/секретов по именам (не значениям).

## 4. То, что агенту Arena было недоступно (403) — если доступ Русла шире

- Описание и темы репозитория (About): «POTOK — открытый проект: единый индустриальный организм на Go (Сфера Вернадского). Часть большого проекта «Свободное общество». Присоединяйтесь — сотрудничество приветствуется!»; темы: `potok`, `organism`, `go`, `open-source`, `free-society`, `industrial`, `vernadsky`.
- Приветственное обсуждение в General из `community/WELCOME-DISCUSSION.md` + закрепить (Pin); категории Discussions — переименовать/добавить по таблице `community/README.md` (при переименовании slug — переименовать файл формы в `.github/DISCUSSION_TEMPLATE/`).
- Проверить Settings → Actions → Workflow permissions = Read and write (нужно мосту `ally-bridge`).

## 5. Отчёт о подключении

Одной записью в `docs/ruslo-lab/RUN-LOG.md` (раздел «Подключение»): что сделано по пунктам, App/Installation ID, ссылка на первый комментарий бота, что не получилось — дословно. Дальше Сфера обновит карту доступов (`community/ACCESS-AND-PERMISSIONS.md`) и журнал по факту.

— Подготовила Сфера, 2026-11-15
