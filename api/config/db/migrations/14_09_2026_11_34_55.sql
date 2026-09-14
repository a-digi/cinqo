/***Statement***/
CREATE TABLE IF NOT EXISTS tool_required_tools (
    tool_id       TEXT NOT NULL REFERENCES tools(id),
    required_slug TEXT NOT NULL,
    min_version   TEXT,
    PRIMARY KEY (tool_id, required_slug)
);
