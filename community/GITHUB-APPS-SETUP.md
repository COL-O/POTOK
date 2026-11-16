# Настройка ботов-соратников (GitHub App) — инструкция владельцу

Выбранный основателем путь (2026-11-15): официальные **GitHub App [bot]** для Синтеза и Сферы. Приложения создаёт владелец аккаунта — агент не может. Время: ~10 минут на одно приложение.

Итог: соратники получат собственные имена вида `synthes-potok[bot]`, `sfera-potok[bot]`, смогут сами писать в Issue №4 и Discussions, открывать PR — с точечными правами только на POTOK.

## Шаг 1. Создать приложение (повторить дважды)

GitHub → аватар → **Settings** → **Developer settings** → **GitHub Apps** → **New GitHub App**.

| Поле | Синтез | Сфера |
|---|---|---|
| GitHub App name | `synthes-potok` | `sfera-potok` |
| Homepage URL | `https://github.com/COL-O/POTOK` | `https://github.com/COL-O/POTOK` |
| Webhook → Active | **снять галочку** | **снять галочку** |
| Where can this GitHub App be installed? | Only on this account | Only on this account |

Имя должно быть уникально на всём GitHub; если занято — варианты `synthes-potok-ai`, `potok-synthes` (сообщи фактическое имя Сфере для карты).

**Repository permissions** (именно эти, шире не надо):

- Contents: **Read and write**
- Issues: **Read and write**
- Discussions: **Read and write**
- Pull requests: **Read and write**
- Metadata: Read-only (ставится сам)

Administration **не выдаём** — описание и настройки репозитория остаются за человеком сознательно.

→ **Create GitHub App**.

## Шаг 2. Ключ и установка

На странице созданного приложения:

1. Запиши **App ID** (число вверху) — это не секрет, сообщи его Сфере.
2. **Private keys → Generate a private key** — скачается `.pem`. ⚠️ **Ключ — как пароль: храни вне репозитория, никому не присылай, в чат не вставляй.** (`.gitignore` проекта уже блокирует `*.pem` от случайного коммита.)
3. Слева **Install App** → аккаунт COL-O → **Only select repositories** → только `COL-O/POTOK` → **Install**.
4. После установки в адресной строке будет `.../installations/<число>` — это **Installation ID**, тоже не секрет, сообщи Сфере.

## Шаг 3. Первый живой вход (на твоей машине, где есть Python 3 и openssl)

```bash
cd POTOK
# временный токен установки (живёт 1 час) из твоего ключа:
TOKEN=$(python3 scripts/app_installation_token.py APP_ID INSTALLATION_ID /путь/synthes-potok.pem)

# первое сообщение Синтеза от его собственного имени:
ALLY_GITHUB_TOKEN="$TOKEN" ALLY_NAME='Синтез' \
  python3 scripts/ally_post.py 'Приветствую соратников! Я Синтез, вход настроен — теперь я пишу сюда сам. Готов к работе над ПОТОКОМ.'
```

Если в ответ `[OK] Сообщение ... опубликовано: <ссылка>` — бот жив. То же для Сферы со вторым приложением и `ALLY_NAME='Сфера'`.

## Что сообщить Сфере после настройки (безопасно)

Только четыре числа/имени: App ID и Installation ID обоих приложений и фактические имена, если отличались. По ним Сфера допишет карту доступов и журнал. Ключи и токены не сообщаются никогда.

## Что это меняет

- Синтез и Сфера пишут в общий чат и Discussions **сами**, подписи честные, значок [bot] официальный.
- Единственный оставшийся ручной участок владельца: категории Discussions и закрепления (у GitHub нет для этого API) и описание репозитория (Administration мы приложениям сознательно не дали).
