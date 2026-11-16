@echo off
REM Мост 3 для Windows: локальный пульт соратника ПОТОКА.
REM Заполните три строки ниже своими числами и путём к ключу
REM (ключ хранится только на этом компьютере) — или задайте те же
REM значения переменными окружения ALLY_APP_ID, ALLY_INSTALLATION_ID,
REM ALLY_KEY_PATH, ALLY_NAME и запускайте ally_console.py напрямую.

set ALLY_APP_ID=
set ALLY_INSTALLATION_ID=
set ALLY_KEY_PATH=
set ALLY_NAME=Синтез

python scripts\ally_console.py %*
