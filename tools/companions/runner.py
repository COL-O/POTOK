#!/usr/bin/env python3
"""Раннер заданий для соратников узла Варанаси.

Даёт трём локальным моделям возможность работать с проектом POTOK:
спросить, разобрать файл, выполнить задание из файла и оставить след.

Секреты не хранятся в репозитории: ключи OpenObserve и SurrealDB читаются
из файлов на машине держателя узла.

Примеры:
    python3 tools/companions/runner.py whoami
    python3 tools/companions/runner.py ask --unit letopisets "Что такое поток в OpenObserve?"
    python3 tools/companions/runner.py review --unit arhitektor --path docs/varanasi-lab/README.md
    python3 tools/companions/runner.py task --unit arhitektor --task-file task.md --out result.md
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import time
import urllib.request
import uuid

UNITS = {
    "letopisets": {"name": "Летописец", "model": "lfm2.5-350m-local", "port": 8088,
                   "system": "Ты Летописец узла Варанаси. Отвечай по-русски кратко и по факту."},
    "arhitektor": {"name": "Архитектор", "model": "lfm2.5-1.2b-local", "port": 8091,
                   "system": "Ты Архитектор узла Варанаси. Отвечай по-русски связно, планом, 3-5 предложений."},
    "izvlekatel": {"name": "Извлекатель", "model": "lfm2.5-350m-extract", "port": 8090,
                   "system": 'Ты Извлекатель. Верни строго JSON: {"сущности": [], "отдел": "", "действие": ""}.'},
    "potok": {"name": "Поток", "model": "lfm2.5-1.2b-v2", "port": 18091,
              "system": "Ты Поток, соратник проекта POTOK на узле Варанаси. Отвечай по-русски кратко и по факту (КОН 2)."},
}

SECRETS_O2 = "/home/colo/.config/domovoy/secrets/openobserve-root.txt"
SECRETS_DB = "/home/colo/.config/domovoy/secrets/surrealdb-root.txt"


def _read_lines(path):
    try:
        with open(path, encoding="utf-8") as f:
            return [x.strip() for x in f.read().splitlines() if x.strip()]
    except OSError:
        return []


def o2_auth():
    lines = _read_lines(SECRETS_O2)
    if len(lines) >= 2:
        return "Basic " + base64.b64encode(f"{lines[0]}:{lines[1]}".encode()).decode()
    return ""


def db_auth():
    lines = _read_lines(SECRETS_DB)
    if len(lines) >= 2:
        return "Basic " + base64.b64encode(f"{lines[0]}:{lines[1]}".encode()).decode()
    return ""


def ask(unit_key, user_text, max_tokens=220, temperature=0.2):
    u = UNITS[unit_key]
    body = json.dumps({
        "model": u["model"],
        "messages": [{"role": "system", "content": u["system"]},
                     {"role": "user", "content": user_text}],
        "max_tokens": max_tokens,
        "temperature": temperature,
        "repeat_penalty": 1.12,
    }, ensure_ascii=False).encode()
    url = f"http://127.0.0.1:{u['port']}/v1/chat/completions"
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
    raw = json.loads(urllib.request.urlopen(req, timeout=180).read().decode())
    return (raw.get("choices") or [{}])[0].get("message", {}).get("content", "")


def trace(unit_key, task, prompt, reply, t0, t1):
    """Трейс gen_ai в OpenObserve — след работы в оболочке-центре."""
    auth = o2_auth()
    if not auth:
        return False
    u = UNITS[unit_key]
    span = {
        "traceId": uuid.uuid4().hex, "spanId": uuid.uuid4().hex[:16],
        "name": "companion.task " + task[:40], "kind": 1,
        "startTimeUnixNano": str(int(t0 * 1e9)), "endTimeUnixNano": str(int(t1 * 1e9)),
        "attributes": [
            {"key": "gen_ai.agent.name", "value": {"stringValue": u["name"]}},
            {"key": "gen_ai.request.model", "value": {"stringValue": u["model"]}},
            {"key": "gen_ai.operation.name", "value": {"stringValue": "companion"}},
            {"key": "gen_ai.task", "value": {"stringValue": task[:200]}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": str(max(1, len(prompt) // 4))}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": str(max(1, len(reply) // 4))}},
        ],
    }
    payload = {"resourceSpans": [{
        "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "varanasi-companions"}}]},
        "scopeSpans": [{"scope": {"name": "amber.companions", "version": "1.0"}, "spans": [span]}]}]}
    try:
        urllib.request.urlopen(urllib.request.Request(
            "http://127.0.0.1:5080/api/default/v1/traces",
            data=json.dumps(payload).encode(),
            headers={"Content-Type": "application/json", "Authorization": auth}), timeout=6)
        return True
    except Exception:
        return False


def remember(unit_key, task, reply, seconds):
    """След в общей памяти SurrealDB."""
    auth = db_auth()
    if not auth:
        return False
    u = UNITS[unit_key]
    q = ("USE NS amber DB society;\n"
         f"CREATE training_journal SET kind='potok_task', unit={json.dumps(u['name'], ensure_ascii=False)}, "
         f"task={json.dumps(task[:400], ensure_ascii=False)}, "
         f"answer={json.dumps(reply[:1500], ensure_ascii=False)}, seconds={seconds}, ts=time::now();")
    try:
        urllib.request.urlopen(urllib.request.Request(
            "http://127.0.0.1:8000/sql", data=q.encode(),
            headers={"Content-Type": "text/plain", "Accept": "application/json",
                     "Surreal-NS": "amber", "Surreal-DB": "society", "Authorization": auth}), timeout=10)
        return True
    except Exception:
        return False


def cmd_whoami(_args):
    print("Соратники узла Варанаси (порт / модель):")
    for key, u in UNITS.items():
        try:
            req = urllib.request.Request(f"http://127.0.0.1:{u['port']}/health")
            code = urllib.request.urlopen(req, timeout=3).status
        except Exception:
            code = "нет связи"
        print(f"  {key:11s} {u['name']:12s} :{u['port']}  {u['model']:24s} {code}")
    print(f"  оболочка :5080  {'доступна' if o2_auth() else 'нет ключа'}")
    print(f"  память   :8000  {'доступна' if db_auth() else 'нет ключа'}")
    return 0


def _run(unit_key, task, prompt):
    t0 = time.time()
    try:
        reply = ask(unit_key, prompt)
    except Exception as e:
        print(f"ошибка модели: {type(e).__name__}: {e}", file=sys.stderr)
        return 1
    t1 = time.time()
    secs = round(t1 - t0, 1)
    print(reply)
    print(f"\n— {UNITS[unit_key]['name']}, {secs} с", file=sys.stderr)
    tr = trace(unit_key, task, prompt, reply, t0, t1)
    rm = remember(unit_key, task, reply, secs)
    print(f"след: оболочка={'да' if tr else 'нет'}, память={'да' if rm else 'нет'}", file=sys.stderr)
    return 0


def cmd_ask(args):
    return _run(args.unit, "вопрос", args.text)


def cmd_review(args):
    with open(args.path, encoding="utf-8") as f:
        text = f.read()[:6000]
    prompt = f"Разбери файл проекта POTOK и скажи по делу: что здесь сильно, что слабо, что поправить.\n\nФАЙЛ {args.path}:\n{text}"
    return _run(args.unit, f"разбор {os.path.basename(args.path)}", prompt)


def cmd_task(args):
    with open(args.task_file, encoding="utf-8") as f:
        task = f.read()[:6000]
    prompt = f"Задание проекта POTOK. Выполни и ответь по делу, по-русски.\n\nЗАДАНИЕ:\n{task}"
    t0 = time.time()
    try:
        reply = ask(args.unit, prompt, max_tokens=400)
    except Exception as e:
        print(f"ошибка модели: {type(e).__name__}: {e}", file=sys.stderr)
        return 1
    t1 = time.time()
    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            f.write(reply + "\n")
        print(f"результат записан: {args.out}")
    else:
        print(reply)
    trace(args.unit, "задание", prompt, reply, t0, t1)
    remember(args.unit, task, reply, round(t1 - t0, 1))
    return 0


def main():
    ap = argparse.ArgumentParser(description="Раннер заданий соратников узла Варанаси")
    sub = ap.add_subparsers(dest="cmd", required=True)

    sub.add_parser("whoami", help="кто на связи").set_defaults(func=cmd_whoami)

    p_ask = sub.add_parser("ask", help="спросить соратника")
    p_ask.add_argument("--unit", choices=UNITS, required=True)
    p_ask.add_argument("text")
    p_ask.set_defaults(func=cmd_ask)

    p_rev = sub.add_parser("review", help="разобрать файл проекта")
    p_rev.add_argument("--unit", choices=UNITS, required=True)
    p_rev.add_argument("--path", required=True)
    p_rev.set_defaults(func=cmd_review)

    p_task = sub.add_parser("task", help="выполнить задание из файла")
    p_task.add_argument("--unit", choices=UNITS, required=True)
    p_task.add_argument("--task-file", required=True)
    p_task.add_argument("--out")
    p_task.set_defaults(func=cmd_task)

    args = ap.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
