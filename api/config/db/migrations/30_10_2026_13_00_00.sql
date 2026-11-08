/***Statement***/
CREATE TABLE IF NOT EXISTS tool_dashboard_widgets (
    tool_id         TEXT NOT NULL REFERENCES tools(id),
    key             TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL,
    schema_version  INTEGER NOT NULL DEFAULT 1,
    data_path       TEXT NOT NULL,
    width           INTEGER NOT NULL DEFAULT 1,
    height          INTEGER NOT NULL DEFAULT 1,
    refresh_seconds INTEGER NOT NULL DEFAULT 60,
    PRIMARY KEY (tool_id, key)
);
