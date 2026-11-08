/***Statement***/
CREATE TABLE IF NOT EXISTS turn_runs (
    id               TEXT NOT NULL CONSTRAINT turn_runs_pk PRIMARY KEY,
    conversation_id  TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_content     TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','completed','failed','cancelled')),
    started_at       TEXT NOT NULL,
    finished_at      TEXT,
    log              TEXT NOT NULL DEFAULT ''
);
/***Statement***/
CREATE INDEX IF NOT EXISTS turn_runs_conversation_idx ON turn_runs(conversation_id);
/***Statement***/
CREATE UNIQUE INDEX IF NOT EXISTS turn_runs_one_running_idx ON turn_runs(conversation_id) WHERE status = 'running';
