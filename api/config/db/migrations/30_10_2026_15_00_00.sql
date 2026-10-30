/***Statement***/
CREATE TABLE IF NOT EXISTS tool_comment_types (
    tool_id     TEXT NOT NULL REFERENCES tools(id),
    entity_type TEXT NOT NULL,
    read_scope  TEXT NOT NULL,
    write_scope TEXT NOT NULL,
    PRIMARY KEY (tool_id, entity_type)
);
