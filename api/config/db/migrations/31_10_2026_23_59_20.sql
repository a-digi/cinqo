/***Statement***/
CREATE TABLE IF NOT EXISTS mcp_settings (
    id         INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    enabled    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT
);
INSERT OR IGNORE INTO mcp_settings (id, enabled) VALUES (1, 0);
CREATE TABLE IF NOT EXISTS mcp_tokens (
    id           TEXT NOT NULL PRIMARY KEY,
    user_id      TEXT NOT NULL,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    scopes       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    expires_at   TEXT,
    last_used_at TEXT,
    revoked_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_mcp_tokens_user ON mcp_tokens (user_id);
