/***Statement***/
CREATE TABLE IF NOT EXISTS tool_event_listeners (
    tool_id     TEXT NOT NULL REFERENCES tools(id),
    topic       TEXT NOT NULL,
    path_suffix TEXT NOT NULL,
    PRIMARY KEY (tool_id, topic, path_suffix)
);
