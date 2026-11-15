package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/manifest"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// The administrator's side of unattended use (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 2): what an installed tool asks to use without a
// signed-in person, and approving or taking back each route. Routes are declared with the tool:manage scope in route-tool.yaml. An install or an update never
// approves anything: only these calls do.

// backgroundRoute names one route of a target tool.
type backgroundRoute struct {
	TargetSlug string `json:"target_slug"`
	Method     string `json:"method"`
	PathSuffix string `json:"path_suffix"`
}

type backgroundBody struct {
	Routes []backgroundRoute `json:"routes"`
}

// backgroundItem is one asked-for route as the page shows it.
type backgroundItem struct {
	backgroundRoute
	TargetName string `json:"target_name"`
	// AllowedNow: the target tool is installed and still marks the route allow_background. An approval needs it, and a call needs it every time.
	AllowedNow bool   `json:"allowed_now"`
	Approved   bool   `json:"approved"`
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
}

// maxBackgroundCall bounds how many routes one call may name.
const maxBackgroundCall = manifest.MaxBackgroundRoutesPerTool * 4

var errUnknownTool = errors.New("tool not found")

// backgroundItems lists what the tool asks for, each with its approval and whether the target still allows it, in a fixed order.
func backgroundItems(db *sql.DB, slug string) ([]backgroundItem, error) {
	t, err := tool_query.NewToolQueryRepo(db).FindBySlug(slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errUnknownTool
		}
		return nil, err
	}
	bq := tool_query.NewToolBackgroundQueryRepo(db)
	reqs, err := bq.Requests(t.ID)
	if err != nil {
		return nil, err
	}
	grants, err := bq.Grants(t.ID)
	if err != nil {
		return nil, err
	}
	granted := map[string]tool_entity.ToolBackgroundGrant{}
	for _, g := range grants {
		granted[g.TargetSlug+"\x00"+g.Method+"\x00"+g.PathSuffix] = g
	}
	allowed, err := tool_query.NewToolQueryRepo(db).BackgroundRoutesBySlug()
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	out := make([]backgroundItem, 0, len(reqs))
	for _, q := range reqs {
		name, seen := names[q.TargetSlug]
		if !seen {
			name = q.TargetSlug
			if target, err := tool_query.NewToolQueryRepo(db).FindBySlug(q.TargetSlug); err == nil {
				name = target.Name
			}
			names[q.TargetSlug] = name
		}
		it := backgroundItem{backgroundRoute: backgroundRoute{TargetSlug: q.TargetSlug, Method: q.Method, PathSuffix: q.PathSuffix}, TargetName: name}
		_, it.AllowedNow = allowed[q.TargetSlug][q.Method+" "+q.PathSuffix]
		if g, ok := granted[q.TargetSlug+"\x00"+q.Method+"\x00"+q.PathSuffix]; ok {
			it.Approved, it.ApprovedBy, it.ApprovedAt = true, g.ApprovedBy, g.ApprovedAt
		}
		out = append(out, it)
	}
	return out, nil
}

func backgroundFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnknownTool):
		response.ErrorResponse(w, http.StatusNotFound, "tool not found")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the unattended access of the tool")
	}
}

// BackgroundListHandler handles GET /api/v1/tools/{slug}/background.
func BackgroundListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	items, err := backgroundItems(db, reqCtx.GetURI().GetPathVariable("slug"))
	if err != nil {
		backgroundFail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"items": items})
}

// BackgroundApproveHandler handles POST /api/v1/tools/{slug}/background/approve.
func BackgroundApproveHandler(reqCtx request.RequestContext) {
	sub, _, err := callerIdentity(reqCtx)
	if err != nil || sub == "" {
		response.ErrorResponse(reqCtx.GetWriter(), http.StatusUnauthorized, "unauthorized")
		return
	}
	changeBackground(reqCtx, sub, true)
}

// BackgroundRevokeHandler handles POST /api/v1/tools/{slug}/background/revoke.
func BackgroundRevokeHandler(reqCtx request.RequestContext) {
	sub, _, err := callerIdentity(reqCtx)
	if err != nil || sub == "" {
		response.ErrorResponse(reqCtx.GetWriter(), http.StatusUnauthorized, "unauthorized")
		return
	}
	changeBackground(reqCtx, sub, false)
}

func changeBackground(reqCtx request.RequestContext, caller string, approve bool) {
	w := reqCtx.GetWriter()
	var body backgroundBody
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	slug := reqCtx.GetURI().GetPathVariable("slug")
	if code, msg := applyBackground(db, slug, caller, approve, body.Routes); code != 0 {
		response.ErrorResponse(w, code, msg)
		return
	}
	items, err := backgroundItems(db, slug)
	if err != nil {
		backgroundFail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"items": items})
}

// applyBackground approves or takes back the named routes of a tool and says why it did not: a zero code means it did. Everything is checked before anything
// is written, so a call either does all it names or nothing. An approval needs the route to be one the tool asks for and the target to still allow it.
func applyBackground(db *sql.DB, slug, caller string, approve bool, routes []backgroundRoute) (int, string) {
	if len(routes) == 0 {
		return http.StatusBadRequest, "name at least one route"
	}
	if len(routes) > maxBackgroundCall {
		return http.StatusBadRequest, "too many routes in one call"
	}
	t, err := tool_query.NewToolQueryRepo(db).FindBySlug(slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return http.StatusNotFound, "tool not found"
		}
		return http.StatusInternalServerError, "failed to read the tool"
	}
	items, err := backgroundItems(db, slug)
	if err != nil {
		return http.StatusInternalServerError, "failed to read the unattended access of the tool"
	}
	asked := map[string]backgroundItem{}
	for _, it := range items {
		asked[it.TargetSlug+"\x00"+it.Method+"\x00"+it.PathSuffix] = it
	}
	var chosen []tool_entity.ToolBackgroundRequest
	for _, r := range routes {
		it, ok := asked[r.TargetSlug+"\x00"+r.Method+"\x00"+r.PathSuffix]
		if !ok {
			return http.StatusBadRequest, "the tool does not ask to use " + strings.TrimSpace(r.Method+" "+r.TargetSlug+"/"+r.PathSuffix)
		}
		if approve && !it.AllowedNow {
			return http.StatusConflict, it.TargetName + " does not allow background use of " + it.Method + " " + it.PathSuffix + " (any more)"
		}
		chosen = append(chosen, tool_entity.ToolBackgroundRequest{ToolID: t.ID, TargetSlug: r.TargetSlug, Method: r.Method, PathSuffix: r.PathSuffix})
	}
	repo := tool_persistent.NewToolBackgroundPersistentRepo(db)
	for _, c := range chosen {
		if approve {
			err = repo.Approve(t.ID, c, caller)
		} else {
			_, err = repo.Revoke(t.ID, c)
		}
		if err != nil {
			return http.StatusInternalServerError, "failed to save"
		}
	}
	return 0, ""
}
