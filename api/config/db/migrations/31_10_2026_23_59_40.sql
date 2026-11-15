/***Statement***/
ALTER TABLE tool_routes ADD COLUMN allow_background INTEGER NOT NULL DEFAULT 0;
/***Statement***/
-- What an installed tool asks to use without a signed-in person (its manifest background_access). The target is named by slug, the same way requires_tools names it.
CREATE TABLE IF NOT EXISTS tool_background_requests (
    tool_id     TEXT NOT NULL REFERENCES tools(id),
    target_slug TEXT NOT NULL,
    method      TEXT NOT NULL,
    path_suffix TEXT NOT NULL,
    PRIMARY KEY (tool_id, target_slug, method, path_suffix)
);
/***Statement***/
-- What an administrator approved: a grant is never created by an install or an update, only by a person. An update that no longer asks for a route drops its grant.
CREATE TABLE IF NOT EXISTS tool_background_grants (
    tool_id     TEXT NOT NULL REFERENCES tools(id),
    target_slug TEXT NOT NULL,
    method      TEXT NOT NULL,
    path_suffix TEXT NOT NULL,
    approved_by TEXT NOT NULL,
    approved_at TEXT NOT NULL,
    PRIMARY KEY (tool_id, target_slug, method, path_suffix)
);
/***Statement***/
-- The per-organisation switch for unattended calls, off unless a row says on.
CREATE TABLE IF NOT EXISTS organisation_background (
    org_id     TEXT PRIMARY KEY REFERENCES organisations(id),
    enabled    INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    enabled_by TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
