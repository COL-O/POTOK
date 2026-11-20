#!/usr/bin/env python3
"""Применяет дашборд зрения (OpenObserve) из файла репозитория в живую O2.

Зрение — тоже часть дома, и оно должно быть видно и поправимо соратникам:
панели лежат в `o2/dashboard-organism.json`, правка идёт через PR, а в живой
OpenObserve файл попадает этим скриптом (у соратников нет доступа к живой O2,
и это правильно — пароль остаётся на машине).

Запуск (на машине, где живёт O2):
    python3 o2/apply.py                 # применить dashboard-organism.json
    python3 o2/apply.py --dry-run       # показать, что изменится
    python3 o2/apply.py --file X.json   # другой файл

Адрес O2 и логин берутся из среды или из файла секрета:
    OKNO_O2 / OKNO_SECRET      (те же переменные, что у окна дома)

Важно про API v1.1.2: обновление дашборда требует `hash` — без него O2
отвечает «500 ... missing or invalid hash value». Поэтому скрипт сначала
читает текущий hash и только потом отправляет файл.
"""
import argparse
import json
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request

O2 = os.environ.get("OKNO_O2", "http://127.0.0.1:5080").rstrip("/")
ORG = os.environ.get("OKNO_O2_ORG", "default")
SECRET = pathlib.Path(os.environ.get(
    "OKNO_SECRET",
    str(pathlib.Path.home() / ".config/domovoy/secrets/openobserve-root.txt")))


def creds() -> tuple[str, str]:
    if SECRET.exists():
        lines = [x.strip() for x in SECRET.read_text(encoding="utf-8").splitlines() if x.strip()]
        if len(lines) >= 2:
            return lines[0], lines[1]
    user = os.environ.get("OKNO_O2_USER", "")
    password = os.environ.get("OKNO_O2_PASSWORD", "")
    if user and password:
        return user, password
    sys.exit(f"нет доступа к O2: заполни {SECRET} (две строки: логин, пароль)")


def call(method: str, path: str, data: bytes | None, user: str, password: str):
    req = urllib.request.Request(O2 + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    import base64
    token = base64.b64encode(f"{user}:{password}".encode()).decode()
    req.add_header("Authorization", "Basic " + token)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, {"message": body[:300]}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--file", default=str(pathlib.Path(__file__).with_name("dashboard-organism.json")))
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    path = pathlib.Path(args.file)
    if not path.exists():
        sys.exit(f"нет файла {path}")
    body = json.loads(path.read_text(encoding="utf-8"))
    dash_id = str(body.get("dashboardId") or "").strip()
    if not dash_id:
        sys.exit("в файле нет dashboardId — это должен быть экспорт дашборда O2 (v8)")

    user, password = creds()
    folder = urllib.parse.quote(os.environ.get("OKNO_O2_FOLDER", "default"))

    st, cur = call("GET", f"/api/{ORG}/dashboards/{dash_id}?folder={folder}", None, user, password)
    if st != 200:
        sys.exit(f"дашборд {dash_id} не читается: {st} {cur.get('message', cur)}")

    old = cur.get("v8") or {}
    new_panels = sum(len(t.get("panels", [])) for t in body.get("tabs", []))
    old_panels = sum(len(t.get("panels", [])) for t in old.get("tabs", []))
    print(f"дашборд «{body.get('title', '')}»: сейчас {old_panels} панелей, в файле {new_panels}")

    if json.dumps(old, sort_keys=True, ensure_ascii=False) == json.dumps(body, sort_keys=True, ensure_ascii=False):
        print("файл и живой дашборд совпадают — применять нечего")
        return 0

    if args.dry_run:
        print("--dry-run: изменения есть, отправка пропущена")
        return 0

    hsh = urllib.parse.quote(str(cur.get("hash") or ""))
    st, res = call("PUT", f"/api/{ORG}/dashboards/{dash_id}?folder={folder}&hash={hsh}",
                   json.dumps(body, ensure_ascii=False).encode(), user, password)
    if st != 200:
        print(f"не применено: {st} {res.get('message', res)}")
        return 1
    print(f"применено: {new_panels} панелей в живом дашборде {dash_id}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
