/***Statement***/
CREATE TABLE IF NOT EXISTS tools (
    id TEXT NOT NULL CONSTRAINT tools_pk PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('frontend_only','backend_only','frontend_and_backend')),
    enabled INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'installed' CHECK (status IN ('installed','starting','running','stopped','error')),
    install_path TEXT NOT NULL,
    backend_executable_relpath TEXT,
    frontend_bundle_relpath TEXT,
    min_app_version TEXT,
    max_app_version TEXT,
    pid INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);

/***Statement***/
CREATE TABLE IF NOT EXISTS tool_scopes (
    tool_id TEXT NOT NULL REFERENCES tools(id),
    scope TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (tool_id, scope)
);

/***Statement***/
CREATE UNIQUE INDEX IF NOT EXISTS tool_scopes_scope_unique ON tool_scopes(scope);

/***Statement***/
CREATE TABLE IF NOT EXISTS tool_routes (
    tool_id TEXT NOT NULL REFERENCES tools(id),
    method TEXT NOT NULL,
    path_suffix TEXT NOT NULL,
    required_scope TEXT NOT NULL,
    PRIMARY KEY (tool_id, method, path_suffix)
);

/***Statement***/
CREATE TABLE IF NOT EXISTS tool_required_scopes (
    tool_id TEXT NOT NULL REFERENCES tools(id),
    scope TEXT NOT NULL,
    PRIMARY KEY (tool_id, scope)
);
