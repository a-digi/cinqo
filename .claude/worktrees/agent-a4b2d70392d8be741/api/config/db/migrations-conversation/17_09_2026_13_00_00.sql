/***Statement***/
CREATE TABLE IF NOT EXISTS sub_agent_runs (
    id                  TEXT NOT NULL CONSTRAINT sub_agent_runs_pk PRIMARY KEY,
    parent_turn_run_id  TEXT NOT NULL REFERENCES turn_runs(id) ON DELETE CASCADE,
    conversation_id     TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    task                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','completed','failed','cancelled')),
    result              TEXT,
    log                 TEXT NOT NULL DEFAULT '',
    started_at          TEXT NOT NULL,
    finished_at         TEXT,
    cancel_requested    INTEGER NOT NULL DEFAULT 0,
    prompt_tokens       INTEGER NOT NULL DEFAULT 0,
    completion_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0
);
/***Statement***/
CREATE INDEX IF NOT EXISTS sub_agent_runs_parent_idx ON sub_agent_runs(parent_turn_run_id);
