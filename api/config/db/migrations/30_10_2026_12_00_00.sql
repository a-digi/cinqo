/***Statement***/
ALTER TABLE tool_repositories ADD COLUMN is_native INTEGER NOT NULL DEFAULT 0;
/***Statement***/
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_repositories_one_native ON tool_repositories (is_native) WHERE is_native = 1;
