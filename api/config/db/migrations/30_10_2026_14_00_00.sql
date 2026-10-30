/***Statement***/
CREATE TABLE IF NOT EXISTS dashboard_layout (
    user_id   TEXT NOT NULL,
    widget_id TEXT NOT NULL,
    position  INTEGER NOT NULL,
    hidden    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, widget_id)
);
