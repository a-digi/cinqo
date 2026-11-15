package persistent

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

// ErrNotRequested is returned when someone tries to approve a route the tool does not ask for (an approval can only answer a request).
var ErrNotRequested = errors.New("the tool does not ask to use that route")

type ToolBackgroundPersistentRepo struct {
	db  *sql.DB
	now func() time.Time
}

func NewToolBackgroundPersistentRepo(db *sql.DB) *ToolBackgroundPersistentRepo {
	return &ToolBackgroundPersistentRepo{db: db, now: time.Now}
}

// SetClock replaces the clock (tests).
func (r *ToolBackgroundPersistentRepo) SetClock(now func() time.Time) { r.now = now }

// ReplaceRequests stores exactly what the tool's manifest asks for now (an empty list when it asks for nothing) and drops every approval that no longer answers a
// request: an update that stopped asking for a route loses that approval, and one that asks for more gets nothing until a person approves it.
func (r *ToolBackgroundPersistentRepo) ReplaceRequests(toolID string, requests []tool_entity.ToolBackgroundRequest) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tool_background_requests WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, q := range requests {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tool_background_requests (tool_id, target_slug, method, path_suffix) VALUES (?, ?, ?, ?)`,
			toolID, q.TargetSlug, q.Method, q.PathSuffix); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM tool_background_grants WHERE tool_id = ? AND NOT EXISTS (
		SELECT 1 FROM tool_background_requests q WHERE q.tool_id = tool_background_grants.tool_id AND q.target_slug = tool_background_grants.target_slug
		AND q.method = tool_background_grants.method AND q.path_suffix = tool_background_grants.path_suffix)`, toolID); err != nil {
		return err
	}
	return tx.Commit()
}

// Approve records that a person approved one request of a tool. It refuses a route the tool does not ask for, and approving twice changes nothing.
func (r *ToolBackgroundPersistentRepo) Approve(toolID string, q tool_entity.ToolBackgroundRequest, by string) error {
	if strings.TrimSpace(by) == "" {
		return errors.New("an approval needs the person who made it")
	}
	var one int
	if err := r.db.QueryRow(`SELECT 1 FROM tool_background_requests WHERE tool_id = ? AND target_slug = ? AND method = ? AND path_suffix = ?`,
		toolID, q.TargetSlug, q.Method, q.PathSuffix).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotRequested
		}
		return err
	}
	_, err := r.db.Exec(`INSERT OR IGNORE INTO tool_background_grants (tool_id, target_slug, method, path_suffix, approved_by, approved_at) VALUES (?, ?, ?, ?, ?, ?)`,
		toolID, q.TargetSlug, q.Method, q.PathSuffix, by, r.now().UTC().Format(time.RFC3339))
	return err
}

// Revoke takes one approval back (taking back what was never approved is not an error). It reports whether there was one.
func (r *ToolBackgroundPersistentRepo) Revoke(toolID string, q tool_entity.ToolBackgroundRequest) (bool, error) {
	res, err := r.db.Exec(`DELETE FROM tool_background_grants WHERE tool_id = ? AND target_slug = ? AND method = ? AND path_suffix = ?`,
		toolID, q.TargetSlug, q.Method, q.PathSuffix)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountCall adds one call (or one refusal) to the day's row of a tool and an organisation. The organisation is empty when the tool did not name a usable one.
func (r *ToolBackgroundPersistentRepo) CountCall(toolID, orgID string, denied bool) error {
	calls, refused := 1, 0
	if denied {
		calls, refused = 0, 1
	}
	_, err := r.db.Exec(`INSERT INTO background_usage (tool_id, org_id, day, calls, denied) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(tool_id, org_id, day) DO UPDATE SET calls = calls + excluded.calls, denied = denied + excluded.denied`,
		toolID, orgID, r.now().UTC().Format("2006-01-02"), calls, refused)
	return err
}
