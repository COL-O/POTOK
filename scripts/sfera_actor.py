#!/usr/bin/env python3
"""Одежда Сферы: действие от имени приложения sfera-potok.

Система GitHub распознаёт носителя токена: с токеном Arena у Сферы права
Arena, с installation token приложения sfera-potok — права Сферы
(issues/discussions/contents/pull-requests и добавленные основателем).
Этот скрипт запускается ТОЛЬКО на машине владельца ключа (Русло):
чеканит временный токен (1 час) и выполняет проверки/действия от бота.

Ключ .pem сюда не коммитится и в чат не передаётся (КОН 10).

Использование (APP_ID и INSTALLATION_ID sfera-potok — из карты доступов):
  python3 scripts/sfera_actor.py /путь/sfera-potok.pem whoami
  python3 scripts/sfera_actor.py /путь/sfera-potok.pem projects
  python3 scripts/sfera_actor.py /путь/sfera-potok.pem shell 'gh project list --owner COL-O'

whoami  — права токена и видимые репозитории (факт одежды).
projects— проекты владельца через GraphQL (проверка двери Projects).
shell   — выполнить команду с GH_TOKEN=<токен бота> (например, наполнение
          доски по docs/PROJECT-BOARD-SETUP.md).
"""
import json
import os
import subprocess
import sys
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from app_installation_token import make_jwt  # noqa: E402

APP_ID = os.environ.get("SFERA_APP_ID", "5784384")
INSTALLATION_ID = os.environ.get("SFERA_INSTALLATION_ID", "182568000")
OWNER = os.environ.get("POTOK_OWNER", "COL-O")


def api(token: str, path: str, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        f"https://api.github.com{path}",
        data=data,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "POTOK-Sfera-Actor/1.0",
        },
        method="POST" if payload is not None else "GET",
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        return {"error": e.code, "body": e.read().decode("utf-8")[:400]}


def mint_token(key_path: str):
    jwt = make_jwt(APP_ID, key_path)
    req = urllib.request.Request(
        f"https://api.github.com/app/installations/{INSTALLATION_ID}/access_tokens",
        data=b"",
        headers={
            "Authorization": f"Bearer {jwt}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "POTOK-Sfera-Actor/1.0",
        },
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode("utf-8"))


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__)
        return 1
    key_path, cmd = sys.argv[1], sys.argv[2]
    granted = mint_token(key_path)
    token = granted["token"]
    if cmd == "whoami":
        print("Права токена Сферы:", json.dumps(granted.get("permissions", {}), ensure_ascii=False))
        print("Охват репозиториев:", granted.get("repository_selection"))
        repos = api(token, "/installation/repositories")
        names = [r["full_name"] for r in repos.get("repositories", [])] if isinstance(repos, dict) else repos
        print("Видимые репозитории:", names)
    elif cmd == "projects":
        q = {"query": '{ user(login: "%s") { projectsV2(first: 10) { nodes { number title public } } } }' % OWNER}
        out = api(token, "/graphql", q)
        print(json.dumps(out, ensure_ascii=False)[:1200])
    elif cmd == "shell":
        if len(sys.argv) < 4:
            print("shell: нужна команда")
            return 1
        env = dict(os.environ, GH_TOKEN=token)
        return subprocess.call(sys.argv[3], shell=True, env=env)
    else:
        print(__doc__)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
