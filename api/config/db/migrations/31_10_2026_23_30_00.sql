/***Statement***/
CREATE TABLE IF NOT EXISTS local_ai_models (
    id             TEXT    NOT NULL PRIMARY KEY,
    repo           TEXT    NOT NULL,
    file           TEXT    NOT NULL,
    revision       TEXT    NOT NULL,
    url            TEXT    NOT NULL,
    sha256         TEXT    NOT NULL,
    size_bytes     INTEGER NOT NULL,
    quantization   TEXT    NOT NULL DEFAULT '',
    license        TEXT    NOT NULL DEFAULT '',
    name           TEXT    NOT NULL,
    publisher      TEXT    NOT NULL DEFAULT '',
    context_tokens INTEGER NOT NULL DEFAULT 4096,
    why            TEXT    NOT NULL DEFAULT '',
    suggested_by   TEXT    NOT NULL DEFAULT '',
    status         TEXT    NOT NULL DEFAULT 'suggested' CHECK (status IN ('suggested', 'added', 'dismissed')),
    created_at     TEXT    NOT NULL,
    updated_at     TEXT
);
/***Statement***/
CREATE UNIQUE INDEX IF NOT EXISTS local_ai_models_source ON local_ai_models (repo, file, revision);
