#!/usr/bin/env python3
"""ЯНТАРНАЯ СФЕРА — витринная комната проекта (веб-окно).

Что это: самостоятельная комната-витрина в составе основного проекта POTOK.
Чат круга + дашборды (считаются по живым файлам репозитория) + отделы Янтарной Сферы.

Чем это НЕ является:
- это НЕ нервная система контура (живой OpenObserve — на машинах Создателя и смотрителя);
- это НЕ SurrealDB и не Drive: сюда ничего не пишется, кроме рантайм-чата;
- это НЕ хранилище секретов: пароли и ключи сюда не кладутся и отсюда не берутся.

Запуск:  python3 apps/amber-society/server.py   (порт 8090, переопределение: PORT=8091)
Только стандартная библиотека. Чат пишется в data/chat.jsonl (не коммитится).
"""
from __future__ import annotations

import json
import os
import re
import sqlite3
import threading
import time
import uuid
from collections import Counter
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlparse

ROOM = Path(__file__).resolve().parent
REPO = ROOM.parents[1]
STATIC = ROOM / "static"
DATA = ROOM / "data"
CHAT_FILE = DATA / "chat.jsonl"
LOCK = threading.Lock()

MAX_NAME = 48
MAX_TEXT = 2000
MAX_BODY = 8192
MAX_MESSAGES = 300

# Нано-сеть: прогон №3 по словам Создателя (2026-11-21). Независимо не проверялось.
NANO = {
    "label": "со слов Создателя, прогон №3 (2026-11-21); независимо не проверялось",
    "points": [
        {"iters": 0, "train": 0.3842, "val": None},
        {"iters": 4000, "train": None, "val": 0.2686},
        {"iters": 5999, "train": 0.1483, "val": 0.8422},
    ],
    "anomaly": "val@4000=0.2686: скачок вниз при нулевом обучении между подключениями; аномалия не закрыта",
}

# Меню «~400 МБ» — публичные карточки моделей, 2026-11.
AWAKENING = [
    {"name": "SmolLM2-360M (Q8)", "params": "360M", "size": "≈371 МБ", "note": "EN, быстрый; слабее RU"},
    {"name": "Qwen2.5-0.5B (Q8)", "params": "0.5B", "size": "≈510 МБ", "note": "RU + логика; больше «успокоения» ОЗУ"},
    {"name": "LiquidAI LFM2.5-350M (Q4_K_M)", "params": "350M", "size": "≈229 МБ", "note": "полевой дежурный; llama.cpp; tool calling"},
    {"name": "Gemma-3-270M (Q4)", "params": "270M", "size": "≈150 МБ", "note": "6 трлн токенов обучения; EN/многоязычный"},
    {"name": "potok-nano (наш)", "params": "~42 МБ", "size": "≈42 МБ", "note": "своя сеть; val растёт — метрика под вопросом"},
]

# Потоки OpenObserve контура (из паспорта, docs/amber-society/passport.md §2)
STREAMS = [
    {"name": "society_events", "what": "сырые события экспедиции"},
    {"name": "society_roster", "what": "реестр моделей (штат)"},
    {"name": "ai_consensus", "what": "протоколы советов и вердикты"},
    {"name": "system_docs", "what": "уставы и паспорта"},
]


def registry_stats() -> dict:
    try:
        lines = (REPO / "registry" / "models.yaml").read_text(encoding="utf-8").splitlines()
        legacy_idx = next(i for i, l in enumerate(lines) if l.rstrip() == "legacy_models:")
        active, legacy = "\n".join(lines[:legacy_idx]), "\n".join(lines[legacy_idx:])
        n_active = len(re.findall(r"(?m)^\s*-\s*id:\s*\d+", active))
        n_legacy = len(re.findall(r"(?m)^\s*-\s*id:\s*\d+", legacy))
        layers = dict(Counter(re.findall(r'(?m)^\s*layer:\s*"?([a-z]+)"?', active)))
        return {
            "available": True,
            "source": "registry/models.yaml (git)",
            "active": n_active,
            "legacy": n_legacy,
            "total": n_active + n_legacy,
            "layers": layers,
        }
    except Exception as exc:  # noqa: BLE001 — витрина обязана отвечать честно
        return {"available": False, "error": str(exc)[:160]}


def memory_stats() -> dict:
    db = REPO / "pritok" / "knowledge" / "memory" / "memory.db"
    if not db.exists():
        return {"available": False, "error": "memory.db не найдена"}
    try:
        con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
        cur = con.cursor()
        out = {
            "available": True,
            "source": "pritok/knowledge/memory/memory.db (копия Притока в git)",
            "notes": cur.execute("select count(*) from notes").fetchone()[0],
            "journal": cur.execute("select count(*) from journal").fetchone()[0],
            "runs": cur.execute("select count(*) from runs").fetchone()[0],
            "experiments": [r[0] for r in cur.execute("select slug from experiments").fetchall()],
        }
        con.close()
        return out
    except Exception as exc:  # noqa: BLE001
        return {"available": False, "error": str(exc)[:160]}


def departments() -> dict:
    try:
        return json.loads((DATA / "departments.json").read_text(encoding="utf-8"))
    except Exception as exc:  # noqa: BLE001
        return {"provenance": f"ошибка чтения departments.json: {exc}", "executive": [], "departments": []}


def clean_field(value, limit: int) -> str:
    text = str(value or "")
    text = "".join(ch for ch in text if ch == "\n" or ch == "\t" or ord(ch) >= 32)
    return text.strip()[:limit]


def read_messages(limit: int = MAX_MESSAGES) -> list:
    if not CHAT_FILE.exists():
        return []
    rows = []
    try:
        with CHAT_FILE.open("r", encoding="utf-8") as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                try:
                    rows.append(json.loads(line))
                except json.JSONDecodeError:
                    continue
    except OSError:
        return []
    return rows[-limit:]


def append_message(name: str, text: str) -> dict:
    msg = {
        "id": uuid.uuid4().hex[:12],
        "ts": int(time.time()),
        "name": name,
        "text": text,
    }
    with LOCK:
        DATA.mkdir(parents=True, exist_ok=True)
        with CHAT_FILE.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(msg, ensure_ascii=False) + "\n")
    return msg


class Handler(BaseHTTPRequestHandler):
    server_version = "AmberRoom/1.0"

    # ---------- helpers ----------
    def _send(self, code: int, body: bytes, content_type: str, extra: dict | None = None):
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Referrer-Policy", "no-referrer")
        for key, value in (extra or {}).items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(body)

    def _json(self, code: int, obj) -> None:
        self._send(code, json.dumps(obj, ensure_ascii=False).encode("utf-8"), "application/json; charset=utf-8")

    def _file(self, path: Path, content_type: str) -> None:
        try:
            body = path.read_bytes()
        except OSError:
            self._json(404, {"error": "not found"})
            return
        self._send(200, body, content_type)

    def log_message(self, fmt, *args):  # компактный лог в stdout процесса
        print(f"[room] {self.address_string()} {fmt % args}", flush=True)

    # ---------- routes ----------
    def do_GET(self):
        path = urlparse(self.path).path
        if path == "/":
            return self._file(STATIC / "index.html", "text/html; charset=utf-8")
        if path == "/api/health":
            return self._json(200, {"ok": True, "room": "amber-society", "ts": int(time.time())})
        if path == "/api/state":
            return self._json(
                200,
                {
                    "registry": registry_stats(),
                    "memory": memory_stats(),
                    "nano": NANO,
                    "awakening": AWAKENING,
                    "streams": STREAMS,
                    "departments": departments(),
                    "generated_at": int(time.time()),
                },
            )
        if path == "/api/messages":
            return self._json(200, {"messages": read_messages()})
        if path == "/api/stream":
            return self._sse()
        if path.startswith("/static/"):
            rel = Path(path[len("/static/"):])
            target = (STATIC / rel).resolve()
            if STATIC.resolve() not in target.parents or not target.is_file():
                return self._json(404, {"error": "not found"})
            ctype = {
                ".css": "text/css; charset=utf-8",
                ".js": "text/javascript; charset=utf-8",
                ".svg": "image/svg+xml",
                ".png": "image/png",
            }.get(target.suffix.lower(), "application/octet-stream")
            return self._file(target, ctype)
        return self._json(404, {"error": "not found"})

    def do_POST(self):
        path = urlparse(self.path).path
        if path != "/api/messages":
            return self._json(404, {"error": "not found"})
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            return self._json(400, {"error": "bad length"})
        if length <= 0 or length > MAX_BODY:
            return self._json(413, {"error": "body too big"})
        try:
            payload = json.loads(self.rfile.read(length))
        except (json.JSONDecodeError, UnicodeDecodeError):
            return self._json(400, {"error": "bad json"})
        name = clean_field(payload.get("name"), MAX_NAME) or "Аноним"
        text = clean_field(payload.get("text"), MAX_TEXT)
        if not text:
            return self._json(400, {"error": "empty text"})
        msg = append_message(name, text)
        return self._json(201, {"ok": True, "message": msg})

    def _sse(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream; charset=utf-8")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.send_header("X-Accel-Buffering", "no")
        self.end_headers()
        offset = CHAT_FILE.stat().st_size if CHAT_FILE.exists() else 0
        last_ping = time.time()
        try:
            while True:
                try:
                    size = CHAT_FILE.stat().st_size if CHAT_FILE.exists() else 0
                    if size < offset:  # файл пересоздан — начинаем сначала
                        offset = 0
                    if size > offset:
                        with CHAT_FILE.open("r", encoding="utf-8") as fh:
                            fh.seek(offset)
                            chunk = fh.read()
                            offset = fh.tell()
                        for line in chunk.splitlines():
                            line = line.strip()
                            if not line:
                                continue
                            try:
                                json.loads(line)
                            except json.JSONDecodeError:
                                continue
                            self.wfile.write(f"data: {line}\n\n".encode("utf-8"))
                        self.wfile.flush()
                    if time.time() - last_ping > 20:
                        self.wfile.write(b": ping\n\n")
                        self.wfile.flush()
                        last_ping = time.time()
                    time.sleep(1.0)
                except OSError:
                    time.sleep(1.5)
        except (BrokenPipeError, ConnectionResetError):
            return


def main():
    port = int(os.environ.get("PORT", "8090"))
    DATA.mkdir(parents=True, exist_ok=True)
    httpd = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"[room] ЯНТАРНАЯ СФЕРА слушает 0.0.0.0:{port}", flush=True)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[room] остановлена", flush=True)


if __name__ == "__main__":
    main()
