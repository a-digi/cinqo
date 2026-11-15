/***Statement***/
CREATE TABLE IF NOT EXISTS organisations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TEXT NOT NULL,
    updated_at TEXT
);
/***Statement***/
-- user_id is the PRIMARY KEY, so a user belongs to exactly one organisation (decision 2026-11-15). It is the identity provider's sub, core has no users table.
CREATE TABLE IF NOT EXISTS user_organisations (
    user_id    TEXT PRIMARY KEY,
    org_id     TEXT NOT NULL REFERENCES organisations(id),
    created_at TEXT NOT NULL
);
/***Statement***/
CREATE INDEX IF NOT EXISTS idx_user_organisations_org ON user_organisations (org_id);
/***Statement***/
-- The single organisation every existing install already is. Users are attached to it lazily on their first request.
INSERT OR IGNORE INTO organisations (id, name, status, created_at) VALUES ('default', 'Default', 'active', '2026-11-15T00:00:00Z');
