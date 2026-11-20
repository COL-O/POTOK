#!/usr/bin/env python3
"""Мост «Сфера-хозяйка»: файл inbox/*.md → действия на GitHub.

Зачем: у Сферы нет ключа приложения (ключ живёт только на машине Русла),
но ей нужно быть хозяйкой в чате и наполнять репозиторий самой, а не просить
каждый раз. Решение — почтовый ящик в репозитории: Сфера коммитит файл
inbox/<имя>.md, GitHub Actions исполняет директивы от github-actions[bot].

Формат: каждый блок начинается строкой `#### ▶ <директива>`.

  #### ▶ comment                 — реплика в чат (обсуждение CHAT_DISCUSSION, по умолчанию №12)
  #### ▶ comment 8               — комментарий в задачу №8 (любой номер, кроме чата)
  #### ▶ issue Заголовок задачи — создать задачу
  #### ▶ discussion General Заголовок — новое обсуждение в категории General
  #### ▶ file ruslo/zhurnal/2026-11-19.md — записать файл (коммит в текущую ветку)
  #### ▶ board 23 В работе       — заявка на статус карточки (исполняет дом: scripts/board_apply.py)
  #### ▶ none                    — ничего не делать (пометка для человека)

Текст блока — всё до следующей директивы. Если директив нет вовсе,
весь файл публикуется репликой в чат (обсуждение CHAT_DISCUSSION).

Безопасность (КОН 10): действия ограничены ТЕКУЩИМ репозиторием; файлы
в .git/, .github/workflows/, .github/actions/, а также *.pem, *.key, .env*,
secrets/ не пишутся никогда — это защита от подмены пайплайнов и утечки ключей.

Запуск:
  GH_TOKEN=... GITHUB_REPOSITORY=COL-O/POTOK CHAT_DISCUSSION=12 \
    python3 scripts/sfera_inbox.py inbox [--dry-run]
"""
import base64
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = "https://api.github.com"
DEFAULT_ISSUE = int(os.environ.get("DEFAULT_ISSUE", "4"))
# Чат переехал из задачи №4 в обсуждение (19.11.2026): разговор — в разговоре.
# Если номер совпадает с DEFAULT_ISSUE, реплика уходит в обсуждение CHAT_DISCUSSION.
CHAT_DISCUSSION = int(os.environ.get("CHAT_DISCUSSION", "12"))
REPO = os.environ.get("GITHUB_REPOSITORY", "COL-O/POTOK")
TOKEN = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN") or ""

DIRECTIVE = re.compile(r"^####\s*▶\s*(.+)$")
FORBIDDEN = (
    ".git/",
    ".github/workflows/",
    ".github/actions/",
    "secrets/",
    ".env",
    ".pem",
    ".key",
)
MAX_FILE = 200_000


def api(method: str, path: str, payload=None):
    data = json.dumps(payload, ensure_ascii=False).encode() if payload is not None else None
    req = urllib.request.Request(
        API + path,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {TOKEN}",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
            "User-Agent": "sfera-inbox/1.0",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            body = r.read().decode()
            return r.status, (json.loads(body) if body.strip() else {})
    except urllib.error.HTTPError as e:
        return e.code, {"error": e.read().decode()[:400]}


def graphql(query: str, variables=None):
    return api("POST", "/graphql", {"query": query, "variables": variables or {}})


def split_blocks(text: str):
    """Возвращает список (директива, тело). Без директив — один блок ('', весь текст)."""
    blocks, cur_dir, cur = [], "", []
    for line in text.splitlines():
        m = DIRECTIVE.match(line.strip())
        if m:
            if cur_dir or "".join(cur).strip():
                blocks.append((cur_dir, "\n".join(cur).strip()))
            cur_dir, cur = m.group(1).strip(), []
        else:
            cur.append(line)
    if cur_dir or "".join(cur).strip():
        blocks.append((cur_dir, "\n".join(cur).strip()))
    return blocks


def chat_comment(number: int, body: str, dry: bool):
    """Реплика в обсуждение чата (по номеру). Комментарии issue и discussion — разные API."""
    if dry:
        return f"discussion #{number}: {len(body)} симв."
    owner, name = REPO.split("/", 1)
    st, res = graphql(
        "query($o:String!,$n:String!,$num:Int!){repository(owner:$o,name:$n){discussion(number:$num){id}}}",
        {"o": owner, "n": name, "num": number},
    )
    did = ((((res.get("data") or {}).get("repository")) or {}).get("discussion") or {}).get("id")
    if not did:
        return f"discussion #{number}: не найдено ({st} {res.get('errors', res.get('error', ''))})"
    st, res = graphql(
        "mutation($id:ID!,$b:String!){addDiscussionComment(input:{discussionId:$id,body:$b}){comment{url}}}",
        {"id": did, "b": body},
    )
    url = (((res.get("data") or {}).get("addDiscussionComment")) or {}).get("comment", {}).get("url", "")
    return f"discussion #{number}: {st} {url or res.get('errors', res.get('error', ''))}"


def do_comment(arg: str, body: str, dry: bool):
    num = int(arg) if arg.strip().isdigit() else DEFAULT_ISSUE
    if num == DEFAULT_ISSUE and CHAT_DISCUSSION:
        return chat_comment(CHAT_DISCUSSION, body, dry)
    if dry:
        return f"comment #{num}: {len(body)} симв."
    st, res = api("POST", f"/repos/{REPO}/issues/{num}/comments", {"body": body})
    return f"comment #{num}: {st} {res.get('html_url', res.get('error', ''))}"


def do_issue(arg: str, body: str, dry: bool):
    title = arg.strip() or "Сообщение Сферы"
    if dry:
        return f"issue «{title}»: {len(body)} симв."
    st, res = api("POST", f"/repos/{REPO}/issues", {"title": title, "body": body})
    return f"issue «{title}»: {st} {res.get('html_url', res.get('error', ''))}"


def do_discussion(arg: str, body: str, dry: bool):
    parts = arg.split(maxsplit=1)
    category, title = (parts[0], parts[1] if len(parts) > 1 else "Сообщение Сферы") if parts else ("General", "Сообщение Сферы")
    if dry:
        return f"discussion [{category}] «{title}»: {len(body)} симв."
    owner, name = REPO.split("/", 1)
    q = (
        "query($o:String!,$n:String!){repository(owner:$o,name:$n){id "
        "discussionCategories(first:20){nodes{id name}}}}"
    )
    st, res = graphql(q, {"o": owner, "n": name})
    if st != 200:
        return f"discussion: ошибка репозитория {st}"
    data = (res.get("data") or {}).get("repository") or {}
    rid = data.get("id")
    cat = next((c["id"] for c in data.get("discussionCategories", {}).get("nodes", [])
                if c["name"].lower() == category.lower()), None)
    if not rid or not cat:
        return f"discussion: нет репозитория или категории «{category}»"
    st, res = graphql(
        "mutation($rid:ID!,$cid:ID!,$t:String!,$b:String!){createDiscussion("
        "input:{repositoryId:$rid,categoryId:$cid,title:$t,body:$b}){discussion{url}}}",
        {"rid": rid, "cid": cat, "t": title, "b": body},
    )
    url = (((res.get("data") or {}).get("createDiscussion")) or {}).get("discussion", {}).get("url", "")
    return f"discussion «{title}»: {st} {url or res.get('errors', res.get('error', ''))}"


def do_board(arg: str, body: str, dry: bool):
    """Заявка на статус карточки доски. Запись в Projects v2 боту запрещена
    (`Resource not accessible by integration`), поэтому мост кладёт заявку файлом,
    а применяет её дом (scripts/board_apply.py) — у него есть право записи."""
    parts = arg.split(None, 1)
    if len(parts) < 2 or not parts[0].isdigit() or not parts[1].strip():
        return "board: формат `board <номер задачи> <статус>`"
    num, status = parts[0], parts[1].strip()
    path = f"board/requests/{num}-{int(time.time())}.md"
    body_json = json.dumps({"issue": int(num), "status": status}, ensure_ascii=False)
    out = do_file(path, body_json, dry)
    return out.replace("file «", "board «", 1)


def do_file(arg: str, body: str, dry: bool):
    path = arg.strip().lstrip("/")
    low = path.lower()
    if not path or ".." in path or any(f in low for f in FORBIDDEN) or low.endswith((".pem", ".key")):
        return f"file «{arg}»: ЗАПРЕЩЕНО (защищённый путь)"
    if len(body.encode()) > MAX_FILE:
        return f"file «{path}»: слишком большой (> {MAX_FILE} байт)"
    if dry:
        return f"file «{path}»: {len(body)} симв."
    branch = os.environ.get("GITHUB_REF_NAME", "main")
    st, cur = api("GET", f"/repos/{REPO}/contents/{urllib.parse.quote(path)}?ref={branch}")
    sha = cur.get("sha") if st == 200 else None

    payload = {
        "message": f"sfera: {path}",
        "content": base64.b64encode(body.encode()).decode(),
        "branch": branch,
    }
    if sha:
        payload["sha"] = sha
    st, res = api("PUT", f"/repos/{REPO}/contents/{urllib.parse.quote(path)}", payload)
    return f"file «{path}»: {st} {(res.get('commit') or {}).get('sha', res.get('error', ''))}"


ACTIONS = {
    "comment": do_comment,
    "issue": do_issue,
    "discussion": do_discussion,
    "file": do_file,
    "board": do_board,
}


def process_file(path: str, dry: bool):
    with open(path, encoding="utf-8") as f:
        text = f.read()
    if not text.strip():
        return []
    blocks = split_blocks(text)
    results = []
    for directive, body in blocks:
        if not directive:
            results.append(do_comment(str(DEFAULT_ISSUE), body, dry))
            continue
        name, _, arg = directive.partition(" ")
        name = name.lower()
        if name == "none":
            results.append("none: пропущено")
            continue
        fn = ACTIONS.get(name)
        if not fn:
            results.append(f"неизвестная директива «{directive}»")
            continue
        results.append(fn(arg, body, dry))
    if not dry:
        with open(path, "w", encoding="utf-8") as f:
            f.write("")
    return results


def main() -> int:
    dry = "--dry-run" in sys.argv
    targets = [a for a in sys.argv[1:] if not a.startswith("--")]
    folder = targets[0] if targets else "inbox"
    if not TOKEN and not dry:
        print("нет GH_TOKEN/GITHUB_TOKEN")
        return 1
    if not os.path.isdir(folder):
        print(f"папки {folder} нет")
        return 0
    total = 0
    for name in sorted(os.listdir(folder)):
        if not name.endswith(".md") or name.lower() == "readme.md":
            continue
        path = os.path.join(folder, name)
        res = process_file(path, dry)
        if res:
            print(f"== {path}")
            for line in res:
                print("  ", line)
            total += len(res)
    print(f"итого действий: {total}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
