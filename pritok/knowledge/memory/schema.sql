-- Приток · память-комната: схема памяти (версия 1, 2026-11-20)
-- Принципы: факт не суждение; устаревшее не переписывается, а помечается.
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,              -- ISO-8601 UTC
  topic      TEXT NOT NULL,
  fact       TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT '',   -- откуда факт: 'git', 'gh', 'перенос основателя', ...
  confidence TEXT NOT NULL DEFAULT 'verified',  -- verified | tentative | stale
  pinned     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_notes_topic ON notes(topic);
CREATE INDEX IF NOT EXISTS idx_notes_ts ON notes(ts);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
  topic, fact, source,
  content='notes', content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS notes_ai AFTER INSERT ON notes BEGIN
  INSERT INTO notes_fts(rowid, topic, fact, source) VALUES (new.id, new.topic, new.fact, new.source);
END;
CREATE TRIGGER IF NOT EXISTS notes_au AFTER UPDATE ON notes BEGIN
  INSERT INTO notes_fts(notes_fts, rowid, topic, fact, source) VALUES('delete', old.id, old.topic, old.fact, old.source);
  INSERT INTO notes_fts(rowid, topic, fact, source) VALUES (new.id, new.topic, new.fact, new.source);
END;

CREATE TABLE IF NOT EXISTS experiments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  slug       TEXT NOT NULL UNIQUE,
  title      TEXT NOT NULL,
  hypothesis TEXT NOT NULL DEFAULT '',
  design     TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'planned',  -- planned | running | done | shelved
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  experiment_id     INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
  started_at        TEXT NOT NULL DEFAULT '',
  iters_total       INTEGER,
  train_loss        REAL,
  val_loss          REAL,
  wall_seconds      REAL,
  source            TEXT NOT NULL DEFAULT '',   -- 'colab-t4 (перенос основателя)' и т.п.
  checkpoint_sha256 TEXT NOT NULL DEFAULT '',
  resumed           INTEGER NOT NULL DEFAULT 0,
  notes             TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_runs_experiment ON runs(experiment_id);

CREATE TABLE IF NOT EXISTS checkpoints (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  path       TEXT NOT NULL,
  bytes      INTEGER NOT NULL DEFAULT 0,
  sha256     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS journal (
  id   INTEGER PRIMARY KEY AUTOINCREMENT,
  ts   TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'note',   -- decision | fact | error | milestone
  text TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS links (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  src_kind TEXT NOT NULL,   -- note | experiment | run | journal
  src_id   INTEGER NOT NULL,
  relation TEXT NOT NULL,   -- supports | refutes | supersedes | continues
  dst_kind TEXT NOT NULL,
  dst_id   INTEGER NOT NULL
);

CREATE VIEW IF NOT EXISTS v_runs_recent AS
  SELECT r.id, e.slug, r.started_at, r.iters_total, r.train_loss, r.val_loss,
         r.wall_seconds, r.checkpoint_sha256, r.resumed, r.source
  FROM runs r JOIN experiments e ON e.id = r.experiment_id
  ORDER BY r.started_at DESC;
