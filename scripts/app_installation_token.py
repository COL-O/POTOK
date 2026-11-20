#!/usr/bin/env python3
"""Временный токен GitHub App для соратника ПОТОКА.

Обменивает приватный ключ приложения (.pem, хранится ТОЛЬКО у владельца,
никогда не коммитится и не передаётся в чат) на installation token
(живёт 1 час) для scripts/ally_post.py.

Только стандартная библиотека Python + openssl в PATH.

Использование:
  python3 scripts/app_installation_token.py APP_ID INSTALLATION_ID /путь/ключ.pem
  # напечатает токен в stdout; дальше:
  ALLY_GITHUB_TOKEN=$(python3 scripts/app_installation_token.py ...) \
  ALLY_NAME='Синтез' python3 scripts/ally_post.py 'Сообщение'

  --jwt-only: только показать JWT (для отладки), без обмена.
"""
import base64
import json
import subprocess
import sys
import time
import urllib.request


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


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
        ["openssl", "dgst", "-sha256", "-sign", key_path],
        input=signing_input.encode(),
        stdout=subprocess.PIPE,
        check=True,
    )
    return signing_input + "." + b64url(proc.stdout)


def main() -> int:
    args = [a for a in sys.argv[1:] if a != "--jwt-only"]
    jwt_only = "--jwt-only" in sys.argv
    if len(args) != 3:
        print(__doc__)
        return 1
    app_id, installation_id, key_path = args
    jwt = make_jwt(app_id, key_path)
    if jwt_only:
        print(jwt)
        return 0
    req = urllib.request.Request(
        f"https://api.github.com/app/installations/{installation_id}/access_tokens",
        data=b"",
        headers={
            "Authorization": f"Bearer {jwt}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "POTOK-Ally-Bridge/1.0",
        },
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    print(data["token"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
