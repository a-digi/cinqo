/***Statement***/
CREATE TABLE IF NOT EXISTS tool_review_types (
    tool_id     TEXT NOT NULL REFERENCES tools(id),
    entity_type TEXT NOT NULL,
    scale       INTEGER NOT NULL CHECK (scale IN (3, 5, 10)),
    half_stars  INTEGER NOT NULL DEFAULT 0,
    read_scope  TEXT NOT NULL,
    write_scope TEXT NOT NULL,
    PRIMARY KEY (tool_id, entity_type)
);
