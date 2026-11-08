/***Statement***/
CREATE TABLE IF NOT EXISTS media_files (
    id TEXT NOT NULL CONSTRAINT media_files_pk PRIMARY KEY,
    tool_slug TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    extension TEXT NOT NULL,
    stored_path TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    uploaded_by_user_id TEXT NOT NULL,
    conversation_id TEXT,
    expires_at TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

/***Statement***/
CREATE INDEX IF NOT EXISTS media_files_tool_slug_idx ON media_files(tool_slug);

/***Statement***/
CREATE INDEX IF NOT EXISTS media_files_expires_at_idx ON media_files(expires_at);
