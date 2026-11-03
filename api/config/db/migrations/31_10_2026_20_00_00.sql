-- Local AI: the one global settings row of the local language model feature.
-- The row always exists (id is fixed to 1 by the CHECK) and is only ever updated.
-- This file must not contain a semicolon inside a comment, because the migration runner cuts a file at every one.
CREATE TABLE IF NOT EXISTS local_ai_settings (
    id                INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
    enabled           INTEGER NOT NULL DEFAULT 0,
    mode              TEXT    NOT NULL DEFAULT 'managed' CHECK (mode IN ('managed', 'external')),
    external_base_url TEXT    NOT NULL DEFAULT '',
    model_id          TEXT    NOT NULL DEFAULT '',
    context_tokens    INTEGER NOT NULL DEFAULT 4096,
    idle_minutes      INTEGER NOT NULL DEFAULT 10,
    updated_at        TEXT
);

INSERT OR IGNORE INTO local_ai_settings (id) VALUES (1);
