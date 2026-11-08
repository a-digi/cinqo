/***Statement***/
CREATE TABLE IF NOT EXISTS tool_mcp_tools (
    tool_id       TEXT NOT NULL REFERENCES tools(id),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    input_schema  TEXT NOT NULL,
    PRIMARY KEY (tool_id, name)
);
