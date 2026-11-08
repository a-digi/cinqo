/***Statement***/
CREATE TABLE IF NOT EXISTS conversation_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    ai_trace_logs_enabled INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT
);
/***Statement***/
INSERT INTO conversation_settings (id, ai_trace_logs_enabled) VALUES (1, 0);
