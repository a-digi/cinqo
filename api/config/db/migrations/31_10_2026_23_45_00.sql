/***Statement***/
CREATE TABLE IF NOT EXISTS local_ai_model_suggesters (
    model_id   TEXT NOT NULL,
    tool       TEXT NOT NULL COLLATE NOCASE,
    why        TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (model_id, tool)
);
/***Statement***/
INSERT OR IGNORE INTO local_ai_model_suggesters (model_id, tool, why, created_at) SELECT id, CASE WHEN suggested_by = '' THEN 'a tool' ELSE suggested_by END, why, created_at FROM local_ai_models;
