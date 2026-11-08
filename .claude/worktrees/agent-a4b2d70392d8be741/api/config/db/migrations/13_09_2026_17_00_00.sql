/***Statement***/
CREATE TABLE IF NOT EXISTS platform_keys (
    id             TEXT NOT NULL CONSTRAINT platform_keys_pk PRIMARY KEY,
    label          TEXT NOT NULL,
    platform       TEXT NOT NULL,
    encrypted_key  TEXT NOT NULL,
    created_by     TEXT NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
