package query

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolBackgroundQueryRepo struct {
	db *sql.DB
}

func NewToolBackgroundQueryRepo(db *sql.DB) *ToolBackgroundQueryRepo {
	return &ToolBackgroundQueryRepo{db: db}
}

// Requests lists what a tool asks to use without a signed-in person, in a fixed order.
func (r *ToolBackgroundQueryRepo) Requests(toolID string) ([]tool_entity.ToolBackgroundRequest, error) {
	rows, err := r.db.Query(`SELECT tool_id, target_slug, method, path_suffix FROM tool_background_requests WHERE tool_id = ? ORDER BY target_slug, path_suffix, method`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tool_entity.ToolBackgroundRequest{}
	for rows.Next() {
		var q tool_entity.ToolBackgroundRequest
		if err := rows.Scan(&q.ToolID, &q.TargetSlug, &q.Method, &q.PathSuffix); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Grants lists what an administrator approved for a tool, in a fixed order.
func (r *ToolBackgroundQueryRepo) Grants(toolID string) ([]tool_entity.ToolBackgroundGrant, error) {
	rows, err := r.db.Query(`SELECT tool_id, target_slug, method, path_suffix, approved_by, approved_at FROM tool_background_grants WHERE tool_id = ? ORDER BY target_slug, path_suffix, method`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tool_entity.ToolBackgroundGrant{}
	for rows.Next() {
		var g tool_entity.ToolBackgroundGrant
		if err := rows.Scan(&g.ToolID, &g.TargetSlug, &g.Method, &g.PathSuffix, &g.ApprovedBy, &g.ApprovedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// IsGranted reports whether the tool holds an approval for exactly this route of the target.
func (r *ToolBackgroundQueryRepo) IsGranted(toolID, targetSlug, method, pathSuffix string) (bool, error) {
	var one int
	err := r.db.QueryRow(`SELECT 1 FROM tool_background_grants WHERE tool_id = ? AND target_slug = ? AND method = ? AND path_suffix = ?`,
		toolID, targetSlug, method, pathSuffix).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// BackgroundSummary is how many unattended routes a tool asks for and how many of them an administrator approved.
type BackgroundSummary struct {
	Requested int `json:"requested"`
	Approved  int `json:"approved"`
}

// Summaries counts, per tool id, what each tool asks for and what is approved (a tool that asks for nothing is absent).
func (r *ToolBackgroundQueryRepo) Summaries() (map[string]BackgroundSummary, error) {
	rows, err := r.db.Query(`SELECT q.tool_id, COUNT(*),
		SUM(CASE WHEN EXISTS (SELECT 1 FROM tool_background_grants g WHERE g.tool_id = q.tool_id AND g.target_slug = q.target_slug AND g.method = q.method AND g.path_suffix = q.path_suffix) THEN 1 ELSE 0 END)
		FROM tool_background_requests q GROUP BY q.tool_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]BackgroundSummary{}
	for rows.Next() {
		var id string
		var s BackgroundSummary
		if err := rows.Scan(&id, &s.Requested, &s.Approved); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

// BackgroundUsage is one day's unattended calls of a tool for an organisation.
type BackgroundUsage struct {
	ToolID string `json:"tool_id"`
	OrgID  string `json:"org_id"`
	Day    string `json:"day"`
	Calls  int    `json:"calls"`
	Denied int    `json:"denied"`
}

// Usage lists the daily counters of a tool, newest day first.
func (r *ToolBackgroundQueryRepo) Usage(toolID string) ([]BackgroundUsage, error) {
	rows, err := r.db.Query(`SELECT tool_id, org_id, day, calls, denied FROM background_usage WHERE tool_id = ? ORDER BY day DESC, org_id`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackgroundUsage{}
	for rows.Next() {
		var u BackgroundUsage
		if err := rows.Scan(&u.ToolID, &u.OrgID, &u.Day, &u.Calls, &u.Denied); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// BackgroundUsageRow is one day's unattended calls of a tool for an organisation, with the tool named (a tool that was removed since is shown by its id).
type BackgroundUsageRow struct {
	ToolSlug string `json:"tool_slug"`
	ToolName string `json:"tool_name"`
	OrgID    string `json:"org_id"`
	Day      string `json:"day"`
	Calls    int    `json:"calls"`
	Denied   int    `json:"denied"`
}

// UsageSince lists every tool's daily counters from the given day (inclusive, "YYYY-MM-DD") on, newest day first, then by tool and organisation.
func (r *ToolBackgroundQueryRepo) UsageSince(day string) ([]BackgroundUsageRow, error) {
	rows, err := r.db.Query(`SELECT COALESCE(t.slug, u.tool_id), COALESCE(t.name, u.tool_id), u.org_id, u.day, u.calls, u.denied
		FROM background_usage u LEFT JOIN tools t ON t.id = u.tool_id WHERE u.day >= ? ORDER BY u.day DESC, 1, u.org_id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackgroundUsageRow{}
	for rows.Next() {
		var u BackgroundUsageRow
		if err := rows.Scan(&u.ToolSlug, &u.ToolName, &u.OrgID, &u.Day, &u.Calls, &u.Denied); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
