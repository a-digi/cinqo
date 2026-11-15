/***Statement***/
-- Unattended calls per tool, organisation and UTC day: how many went through and how many were refused after the tool had identified itself. One row per day, not one per call.
CREATE TABLE IF NOT EXISTS background_usage (
    tool_id TEXT NOT NULL,
    org_id  TEXT NOT NULL,
    day     TEXT NOT NULL,
    calls   INTEGER NOT NULL DEFAULT 0,
    denied  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (tool_id, org_id, day)
);
