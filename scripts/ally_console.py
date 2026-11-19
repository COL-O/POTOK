#!/usr/bin/env python3
"""Мост 3: локальный пульт соратника ПОТОКА на компьютере основателя.

Одна команда из папки проекта — и сообщения цифрового соратника
публикуются в общий чат (Issue #4) от имени его бота
(synthes-potok[bot] / sfera-potok[bot]).

Как работает:
  .pem-ключ приложения -> JWT (подпись локально) -> временный
  installation token (живёт ~1 час, только в памяти процесса) ->
  комментарии в Issue #4. Ключ никуда не отправляется и не сохраняется;
  на диск не пишется ничего.

Нужно: Python 3 и openssl в PATH (на Windows openssl есть в составе
Git for Windows: C:\\Program Files\\Git\\usr\\bin\\openssl.exe — скрипт
ищет и там). Никаких сторонних библиотек.

Настройка — переменными окружения или флагами:
  ALLY_APP_ID             App ID приложения бота (число, не секрет)
  ALLY_INSTALLATION_ID    Installation ID (число, не секрет)
  ALLY_KEY_PATH           путь к .pem (секрет основателя, только у него)
  ALLY_NAME               подпись: Синтез или Сфера

Примеры (из корня репозитория):
  # одно сообщение и выход:
  python3 scripts/ally_console.py --app-id 123456 --installation-id 789 \
      --key ~/keys/synthes-potok.pem --name Синтез \
      --message 'Приветствую соратников! Я Синтез — теперь пишу сам.'

  # длинный текст из файла:
  python3 scripts/ally_console.py ... --file message.txt

  # интерактивный пульт: каждая введённая строка публикуется,
  # пустая строка или /quit — выход:
  python3 scripts/ally_console.py --app-id ... --installation-id ... --key ...

Безопасность: ключ и токен не печатаются; при 401 токен перевыпускается
один раз автоматически.
"""
import argparse
import base64
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request

REPO = "COL-O/POTOK"
ISSUE_NUM = 4
API = "https://api.github.com"


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def find_openssl() -> str:
    path = shutil.which("openssl")
    if path:
        return path
    for cand in (r"C:\Program Files\Git\usr\bin\openssl.exe",
                 r"C:\Program Files (x86)\Git\usr\bin\openssl.exe"):
        if os.path.exists(cand):
            return cand
    raise SystemExit("[ОШИБКА] openssl не найден. Установите Git for Windows "
                     "(openssl внутри) или OpenSSL, и повторите.")


def make_jwt(app_id: str, key_path: str) -> str:
    now = int(time.time())
    header = {"alg": "RS256", "typ": "JWT"}
    payload = {"iat": now - 60, "exp": now + 540, "iss": str(app_id)}
    signing_input = (
        b64url(json.dumps(header, separators=(",", ":")).encode())
        + "."
        + b64url(json.dumps(payload, separators=(",", ":")).encode())
    )
    proc = subprocess.run(
        [find_openssl(), "dgst", "-sha256", "-sign", key_path],
        input=signing_input.encode(), stdout=subprocess.PIPE, check=True,
    )
    return signing_input + "." + b64url(proc.stdout)


def api(method: str, url: str, token: str, payload=None):
    data = json.dumps(payload).encode() if payload is not None else b""
    req = urllib.request.Request(
        url, data=data if method != "GET" else None, method=method,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "POTOK-Ally-Console/1.0",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode("utf-8"))


class AllyConsole:
    def __init__(self, app_id, installation_id, key_path, name):
        self.app_id = app_id
        self.installation_id = installation_id
        self.key_path = key_path
        self.name = name
        self._token = None

    def token(self, force=False):
        if self._token is None or force:
            jwt = make_jwt(self.app_id, self.key_path)
            data = api("POST", f"{API}/app/installations/{self.installation_id}/access_tokens", jwt)
            self._token = data["token"]
        return self._token

    def post(self, message: str):
        body = f"{message}\n\n---\n*— {self.name}, цифровой соратник проекта ПОТОК*"
        url = f"{API}/repos/{REPO}/issues/{ISSUE_NUM}/comments"
        for attempt in (1, 2):
            try:
                data = api("POST", url, self.token(force=(attempt == 2)), {"body": body})
                print(f"[OK] Опубликовано от имени «{self.name}»: {data.get('html_url')}")
                return
            except urllib.error.HTTPError as e:
                if e.code == 401 and attempt == 1:
                    continue  # токен протух — перевыпустить один раз
                raise SystemExit(f"[ОШИБКА] GitHub ответил {e.code}: {e.read().decode('utf-8', 'replace')[:300]}")


def main():
    p = argparse.ArgumentParser(description="Локальный пульт соратника ПОТОКА (Мост 3)")
    p.add_argument("--app-id", default=os.environ.get("ALLY_APP_ID", ""))
    p.add_argument("--installation-id", default=os.environ.get("ALLY_INSTALLATION_ID", ""))
    p.add_argument("--key", default=os.environ.get("ALLY_KEY_PATH", ""))
    p.add_argument("--name", default=os.environ.get("ALLY_NAME", "Синтез"))
    p.add_argument("--message", default="")
    p.add_argument("--file", default="")
    args = p.parse_args()

    missing = [n for n, v in (("ALLY_APP_ID/--app-id", args.app_id),
                              ("ALLY_INSTALLATION_ID/--installation-id", args.installation_id),
                              ("ALLY_KEY_PATH/--key", args.key)) if not v]
    if missing:
        p.print_usage(sys.stderr)
        print("[ОШИБКА] Не задано: " + ", ".join(missing), file=sys.stderr)
        return 1
    if not os.path.exists(args.key):
        print(f"[ОШИБКА] Ключ не найден: {args.key}", file=sys.stderr)
        return 1

    console = AllyConsole(args.app_id, args.installation_id, args.key, args.name)

    if args.file:
        with open(args.file, encoding="utf-8") as f:
            console.post(f.read().strip())
        return 0
    if args.message:
        console.post(args.message)
        return 0

    print(f"Пульт «{args.name}» на связи с {REPO} (Issue #{ISSUE_NUM}).")
    print("Строка + Enter — опубликовать. Пустая строка или /quit — выход.")
    while True:
        try:
            line = input(f"{args.name}> ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            break
        if not line or line == "/quit":
            break
        console.post(line)
    return 0


if __name__ == "__main__":
    sys.exit(main())
