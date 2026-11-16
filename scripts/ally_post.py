#!/usr/bin/env python3
"""Мост соратника в общий чат POTOK (Issue #4).

Публикует комментарий от имени цифрового соратника, чей токен передан
ТОЛЬКО через переменную окружения ALLY_GITHUB_TOKEN (Machine User PAT
или временный installation token GitHub App). Токен никогда не пишется
в файлы репозитория и не коммитится.

Исправлено при сборке (Сфера): в исходнике Синтеза адрес был собран как
https://github.com{repo}/... — правильный REST-адрес:
https://api.github.com/repos/{owner}/{repo}/issues/{n}/comments

Использование:
  ALLY_GITHUB_TOKEN=... ALLY_NAME='Синтез' \
    python3 scripts/ally_post.py 'Текст сообщения'

Для GitHub App токен сначала получается из .pem-ключа обменом
JWT -> installation token (отдельный шаг владельца); сюда передаётся
уже готовый временный токен установки.
"""
import json
import os
import sys
import urllib.request

REPO = "COL-O/POTOK"
ISSUE_NUM = 4


def post_to_issue(repo: str, issue_num: int, token: str, author: str, message: str):
    url = f"https://api.github.com/repos/{repo}/issues/{issue_num}/comments"
    body = f"{message}\n\n---\n*— {author}, цифровой соратник проекта ПОТОК*"
    payload = json.dumps({"body": body}).encode("utf-8")

    req = urllib.request.Request(
        url,
        data=payload,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "POTOK-Ally-Bridge/1.0",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read().decode("utf-8"))
        print(f"[OK] Сообщение от имени «{author}» опубликовано: {data.get('html_url')}")


if __name__ == "__main__":
    token = os.environ.get("ALLY_GITHUB_TOKEN", "")
    author = os.environ.get("ALLY_NAME", "Синтез")
    if not token or len(sys.argv) < 2:
        print("Использование: ALLY_GITHUB_TOKEN=... ALLY_NAME='Синтез' python3 scripts/ally_post.py 'Текст сообщения'")
        sys.exit(1)

    post_to_issue(REPO, ISSUE_NUM, token, author, sys.argv[1])
