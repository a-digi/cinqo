/***Statement***/
CREATE TABLE IF NOT EXISTS system_runs (
    id             TEXT PRIMARY KEY,
    started_at     TEXT NOT NULL,
    last_seen_at   TEXT NOT NULL,
    ended_at       TEXT,
    clean          INTEGER NOT NULL DEFAULT 0,
    pid            INTEGER NOT NULL DEFAULT 0,
    process_start  TEXT NOT NULL DEFAULT '',
    last_heartbeat TEXT NOT NULL DEFAULT ''
);
/***Statement***/
CREATE INDEX IF NOT EXISTS idx_system_runs_open ON system_runs (ended_at);
/***Statement***/
CREATE TABLE IF NOT EXISTS system_events (
    id              TEXT PRIMARY KEY,
    at              TEXT NOT NULL,
    kind            TEXT NOT NULL,
    verdict         TEXT NOT NULL,
    evidence        TEXT NOT NULL DEFAULT '[]',
    acknowledged_at TEXT
);
/***Statement***/
CREATE INDEX IF NOT EXISTS idx_system_events_at ON system_events (at);
