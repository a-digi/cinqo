/***Statement***/
CREATE TABLE IF NOT EXISTS tool_repositories (
    id              TEXT PRIMARY KEY,
    url             TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    catalog_json    TEXT,
    last_fetched_at TEXT,
    last_status     TEXT NOT NULL DEFAULT 'never' CHECK (last_status IN ('never','ok','error')),
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT
);
