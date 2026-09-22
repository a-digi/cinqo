package query

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolQueryRepo struct {
	db *sql.DB
}

func NewToolQueryRepo(db *sql.DB) *ToolQueryRepo {
	return &ToolQueryRepo{db: db}
}

const toolColumns = `id, slug, name, version, kind, enabled, status, install_path, ` +
	`backend_executable_relpath, frontend_bundle_relpath, min_app_version, max_app_version, ` +
	`pid, created_at, updated_at, service_token`

func scanTool(scan func(dest ...any) error) (*tool_entity.Tool, error) {
	var t tool_entity.Tool
	var backendRel, frontendRel, minV, maxV, updatedAt sql.NullString
	var pid sql.NullInt64

	if err := scan(&t.ID, &t.Slug, &t.Name, &t.Version, &t.Kind, &t.Enabled, &t.Status,
		&t.InstallPath, &backendRel, &frontendRel, &minV, &maxV, &pid, &t.CreatedAt, &updatedAt, &t.ServiceToken); err != nil {
		return nil, err
	}

	t.BackendExecutableRelpath = backendRel.String
	t.FrontendBundleRelpath = frontendRel.String
	t.MinAppVersion = minV.String
	t.MaxAppVersion = maxV.String
	t.PID = int(pid.Int64)
	t.UpdatedAt = updatedAt.String

	return &t, nil
}

// FindBySlug returns sql.ErrNoRows (unwrapped, matching database/sql's
// own convention) when no tool has this slug — callers distinguish
// "not found" from a real error via errors.Is.
func (r *ToolQueryRepo) FindBySlug(slug string) (*tool_entity.Tool, error) {
	row := r.db.QueryRow(`SELECT `+toolColumns+` FROM tools WHERE slug = ? LIMIT 1`, slug)
	return scanTool(row.Scan)
}

func (r *ToolQueryRepo) FindByID(id string) (*tool_entity.Tool, error) {
	row := r.db.QueryRow(`SELECT `+toolColumns+` FROM tools WHERE id = ? LIMIT 1`, id)
	return scanTool(row.Scan)
}

// FindByServiceToken authenticates an inbound tool→core call (currently
// only POST /api/v1/events/publish) by its own service_token — the
// tool's slug (on the returned row) becomes that call's established
// identity. Returns sql.ErrNoRows (unwrapped) for an empty or unknown
// token — an empty string must never match, since a not-yet-backfilled
// tool row (see install_handler.go's own backfill rule) legitimately
// has service_token = "" and must not be treated as "no token
// required." See plan/ai/domain-events/step-05-tool-publish-endpoint.md.
func (r *ToolQueryRepo) FindByServiceToken(token string) (*tool_entity.Tool, error) {
	if token == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(`SELECT `+toolColumns+` FROM tools WHERE service_token = ? LIMIT 1`, token)
	return scanTool(row.Scan)
}

// List returns every installed tool, alphabetically by name.
func (r *ToolQueryRepo) List() ([]*tool_entity.Tool, error) {
	rows, err := r.db.Query(`SELECT ` + toolColumns + ` FROM tools ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tools := make([]*tool_entity.Tool, 0)
	for rows.Next() {
		t, err := scanTool(rows.Scan)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

// ExistingScopesExcludingTool returns every scope string already
// declared by a tool OTHER than excludeToolID — the pre-write half of
// the global-uniqueness guarantee the migration's unique index
// backstops (see plan/ai/tools/step-01-data-model-and-migration.md).
// Excluding the tool's own current scopes lets a tool re-declare a
// scope it already owns across a version update without that reading
// as a collision with itself. Pass "" for a fresh install (no existing
// tool to exclude).
func (r *ToolQueryRepo) ExistingScopesExcludingTool(excludeToolID string) (map[string]struct{}, error) {
	rows, err := r.db.Query(`SELECT scope FROM tool_scopes WHERE tool_id != ?`, excludeToolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = struct{}{}
	}
	return out, rows.Err()
}

// FindRoute looks up a single declared route by (tool, method, path
// suffix) — the allow-list the reverse proxy checks against. Returns
// sql.ErrNoRows (unwrapped) when the tool never declared this exact
// method+path in its manifest. See
// plan/ai/tools/step-05-reverse-proxy-and-request-enforcement.md.
func (r *ToolQueryRepo) FindRoute(toolID, method, pathSuffix string) (*tool_entity.ToolRoute, error) {
	row := r.db.QueryRow(
		`SELECT tool_id, method, path_suffix, required_scope, allow_capability_token FROM tool_routes WHERE tool_id = ? AND method = ? AND path_suffix = ? LIMIT 1`,
		toolID, method, pathSuffix,
	)
	var rt tool_entity.ToolRoute
	if err := row.Scan(&rt.ToolID, &rt.Method, &rt.PathSuffix, &rt.RequiredScope, &rt.AllowCapabilityToken); err != nil {
		return nil, err
	}
	return &rt, nil
}

// AllScopesGroupedByTool returns every installed tool's declared
// scopes, grouped by tool ID — regardless of the tool's enabled state
// (a disabled tool's declared scopes are still "registered," just
// currently unusable). Used by the security-scopes admin page to merge
// tool-declared scope groups into its registry view. See
// plan/ai/tools/step-06-scope-registry-integration.md.
func (r *ToolQueryRepo) AllScopesGroupedByTool() (map[string][]tool_entity.ToolScope, error) {
	rows, err := r.db.Query(`SELECT tool_id, scope, description FROM tool_scopes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]tool_entity.ToolScope{}
	for rows.Next() {
		var s tool_entity.ToolScope
		if err := rows.Scan(&s.ToolID, &s.Scope, &s.Description); err != nil {
			return nil, err
		}
		out[s.ToolID] = append(out[s.ToolID], s)
	}
	return out, rows.Err()
}

// ScopesForTool returns the bare scope strings toolID itself declared
// in its own manifest — used by the media package's own upload
// authorization check (a caller may upload into a tool's media
// namespace only if it holds one of that tool's own declared scopes),
// so it deliberately returns just the scope names, not the full
// ToolScope rows AllScopesGroupedByTool already serves the security
// admin page.
func (r *ToolQueryRepo) ScopesForTool(toolID string) ([]string, error) {
	rows, err := r.db.Query(`SELECT scope FROM tool_scopes WHERE tool_id = ?`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EnabledRouteScopes returns every required_scope declared across
// every ENABLED tool's own routes — tool_routes are never part of any
// route-*.yaml, so config.AllEnforcedScopes() can't see them on its
// own; this is folded into the security-scopes admin page's "enforced"
// set alongside that function's own result. A disabled tool's routes
// are unreachable (step 5), so they're deliberately excluded here too.
func (r *ToolQueryRepo) EnabledRouteScopes() ([]string, error) {
	rows, err := r.db.Query(
		`SELECT tr.required_scope FROM tool_routes tr JOIN tools t ON t.id = tr.tool_id WHERE t.enabled = 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
