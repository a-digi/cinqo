package handler

import (
	"database/sql"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"
)

// Dependent is an enabled tool that needs another one.
type Dependent struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type dependentsAnswer struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Dependents []Dependent `json:"dependents"`
}

// enabledDependents lists the enabled tools that declare they need slug, by name. A disabled tool does not count: it is not running, and enabling it
// again re-checks its requirements.
func enabledDependents(db *sql.DB, slug string) ([]Dependent, error) {
	rows, err := db.Query(`SELECT t.slug, t.name FROM tools t JOIN tool_required_tools r ON r.tool_id = t.id WHERE r.required_slug = ? AND t.enabled = 1 AND t.slug <> ? ORDER BY t.name, t.slug`, slug, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dependent{}
	for rows.Next() {
		var d Dependent
		if err := rows.Scan(&d.Slug, &d.Name); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// refuseWhenNeeded answers 409 with the list of enabled tools that need slug, unless the caller passed force=true (the person confirmed after seeing the
// list). It reports whether the request was answered. Plan dependency-install, step 9: the same rule as an install, seen from the other side.
func refuseWhenNeeded(reqCtx request.RequestContext, db *sql.DB, slug, doing string) bool {
	if reqCtx.GetRequest().URL.Query().Get("force") == "true" {
		return false
	}
	w := reqCtx.GetWriter()
	deps, err := enabledDependents(db, slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to check which tools need this one")
		return true
	}
	if len(deps) == 0 {
		return false
	}
	response.SuccessResponse(w, http.StatusConflict, dependentsAnswer{
		Code:       "has_dependents",
		Message:    "enabled tools need this one: " + doing + " it would stop them from working",
		Dependents: deps,
	})
	return true
}
