#!/usr/bin/env python3
"""mem — личная память Притока. Только стандартная библиотека.

Usage:
  mem.py init                          создать/обновить схему
  mem.py note ТОПИК "ФАКТ" [ИСТОЧНИК]  записать факт (verified)
  mem.py tentative ТОПИК "ФАКТ" [ИСТОЧНИК]  записать недопроверенный факт
  mem.py journal ВИД "ТЕКСТ"           decision|fact|error|milestone|note
  mem.py exp SLUG "ЗАГОЛОВОК" ["ГИПОТЕЗА"] ["ДИЗАЙН"]
  mem.py import-jsonl SLUG ПУТЬ        перенести run_log.jsonl из Colab в runs
  mem.py recent [N]                    последние прогоны
  mem.py notes [ТОПИК]                 все/по топику
  mem.py search ТЕРМИН                 поиск по notes (FTS5)
  mem.py vacuum                        дефрагментация + integrity_check
"""
import sqlite3, sys, os, json, datetime

HERE = os.path.dirname(os.path.abspath(__file__))
DB = os.path.join(HERE, "memory.db")
SCHEMA = os.path.join(HERE, "schema.sql")

def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

def connect():
    c = sqlite3.connect(DB)
    c.execute("PRAGMA foreign_keys = ON")
    return c

def cmd_init():
    with open(SCHEMA, encoding="utf-8") as f:
        sql = f.read()
    with connect() as c:
        c.executescript(sql)
        c.execute("INSERT OR IGNORE INTO meta(key, value) VALUES('owner', 'Приток <pritok@potok.local>')")
        c.execute("INSERT OR IGNORE INTO meta(key, value) VALUES('room', 'pritok/knowledge/memory/')")
        c.execute("INSERT OR IGNORE INTO meta(key, value) VALUES('schema_version', '1')")
        c.execute("INSERT OR IGNORE INTO meta(key, value) VALUES('created', ?)", (now(),))
    print("init ok ->", DB)

def insert_note(topic, fact, source, confidence):
    with connect() as c:
        cur = c.execute(
            "INSERT INTO notes(ts, topic, fact, source, confidence) VALUES (?,?,?,?,?)",
            (now(), topic, fact, source, confidence))
        print(f"note #{cur.lastrowid} [{confidence}] {topic}: сохранён")

def cmd_journal(kind, text):
    with connect() as c:
        cur = c.execute("INSERT INTO journal(ts, kind, text) VALUES (?,?,?)", (now(), kind, text))
        print(f"journal #{cur.lastrowid} [{kind}]")

def cmd_exp(slug, title, hypothesis="", design=""):
    with connect() as c:
        cur = c.execute(
            "INSERT INTO experiments(slug, title, hypothesis, design, created_at)"
            " VALUES (?,?,?,?,?)"
            " ON CONFLICT(slug) DO UPDATE SET title=excluded.title",
            (slug, title, hypothesis, design, now()))
        print(f"exp #{cur.lastrowid} {slug}")

def cmd_import_jsonl(slug, path):
    with connect() as c:
        row = c.execute("SELECT id FROM experiments WHERE slug = ?", (slug,)).fetchone()
        if row is None:
            c.execute("INSERT INTO experiments(slug, title, created_at) VALUES (?,?,?)",
                      (slug, slug, now()))
            row = c.execute("SELECT id FROM experiments WHERE slug = ?", (slug,)).fetchone()
        exp_id = row[0]
        n = 0
        with open(path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                d = json.loads(line)
                cur = c.execute(
                    "INSERT INTO runs(experiment_id, started_at, iters_total, train_loss, val_loss,"
                    " wall_seconds, source, checkpoint_sha256, resumed, notes)"
                    " VALUES (?,?,?,?,?,?,?,?,?,?)",
                    (exp_id, d.get("ts", now()), d.get("iters_total"),
                     d.get("train_loss"), d.get("val_loss"), d.get("wall_seconds"),
                     d.get("source", "colab-t4 (перенос основателя)"),
                     d.get("checkpoint_sha256", ""), int(bool(d.get("resumed", False))),
                     d.get("notes", "")))
                run_id = cur.lastrowid
                if d.get("checkpoint_sha256"):
                    c.execute(
                        "INSERT INTO checkpoints(run_id, path, bytes, sha256, created_at)"
                        " VALUES (?,?,?,?,?)",
                        (run_id, d.get("checkpoint_path", "potok-nano.pt"),
                         d.get("checkpoint_bytes", 0), d["checkpoint_sha256"], now()))
                n += 1
        print(f"import: {n} прогонов -> experiment '{slug}'")

def cmd_recent(limit=10):
    with connect() as c:
        for row in c.execute("SELECT * FROM v_runs_recent LIMIT ?", (int(limit),)):
            print(row)

def cmd_notes(topic=None):
    q = "SELECT id, ts, topic, fact, source, confidence FROM notes"
    args = ()
    if topic:
        q += " WHERE topic = ?"
        args = (topic,)
    q += " ORDER BY id"
    with connect() as c:
        for row in c.execute(q, args):
            print(f"#{row[0]} [{row[1]}] ({row[5]}, {row[4]}) {row[2]}: {row[3]}")

def cmd_search(term):
    with connect() as c:
        try:
            rows = c.execute(
                "SELECT n.id, n.topic, n.fact FROM notes_fts f JOIN notes n ON n.id = f.rowid"
                " WHERE notes_fts MATCH ? ORDER BY n.id", (term,)).fetchall()
        except sqlite3.OperationalError as e:
            print(f"FTS недоступен ({e}); fallback на LIKE")
            rows = c.execute(
                "SELECT id, topic, fact FROM notes WHERE fact LIKE ? OR topic LIKE ? ORDER BY id",
                (f"%{term}%", f"%{term}%")).fetchall()
        for row in rows:
            print(f"#{row[0]} {row[1]}: {row[2]}")

def cmd_vacuum():
    with connect() as c:
        r = c.execute("PRAGMA integrity_check").fetchone()[0]
        c.execute("VACUUM")
    print("integrity_check:", r)

def main():
    args = sys.argv[1:]
    if not args:
        print(__doc__)
        return
    cmd, rest = args[0], args[1:]
    if cmd == "init": cmd_init()
    elif cmd == "note":
        assert len(rest) >= 2, "note ТОПИК ФАКТ [ИСТОЧНИК]"
        insert_note(rest[0], rest[1], rest[2] if len(rest) > 2 else "", "verified")
    elif cmd == "tentative":
        assert len(rest) >= 2, "tentative ТОПИК ФАКТ [ИСТОЧНИК]"
        insert_note(rest[0], rest[1], rest[2] if len(rest) > 2 else "", "tentative")
    elif cmd == "journal":
        assert len(rest) >= 2, "journal ВИД ТЕКСТ"
        cmd_journal(rest[0], rest[1])
    elif cmd == "exp":
        assert len(rest) >= 2, "exp SLUG 'ЗАГОЛОВОК' ['ГИПОТЕЗА'] ['ДИЗАЙН']"
        cmd_exp(rest[0], rest[1], rest[2] if len(rest) > 2 else "", rest[3] if len(rest) > 3 else "")
    elif cmd == "import-jsonl":
        assert len(rest) == 2, "import-jsonl SLUG ПУТЬ"
        cmd_import_jsonl(rest[0], rest[1])
    elif cmd == "recent": cmd_recent(rest[0] if rest else 10)
    elif cmd == "notes": cmd_notes(rest[0] if rest else None)
    elif cmd == "search": cmd_search(rest[0])
    elif cmd == "vacuum": cmd_vacuum()
    else:
        print(f"неизвестная команда: {cmd}\n{__doc__}")
        sys.exit(2)

if __name__ == "__main__":
    main()
