/***Statement***/
CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT NOT NULL CONSTRAINT conversations_pk PRIMARY KEY,
    user_id     TEXT NOT NULL,
    title       TEXT NOT NULL,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    file_path   TEXT NOT NULL
);
