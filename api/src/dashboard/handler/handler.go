// Package handler serves the Home dashboard's read endpoints. Both routes are
// authenticated with no static scope: which widgets a caller may see is decided
// per widget by the engine (a widget's own required scope), not by the route.
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/dashboard"
	layout_persistent "github.com/a-digi/cinqo/src/dashboard/repository/persistent"
	layout_query "github.com/a-digi/cinqo/src/dashboard/repository/query"
)

// layoutReader and layoutWriter are the two halves of the layout store the
// handlers need (reads and writes live in separate repositories).
type layoutReader interface {
	Find(userID string) ([]dashboard.LayoutEntry, error)
}

type layoutWriter interface {
	ReplaceAll(userID string, entries []dashboard.LayoutEntry) error
}

// maxLayoutBody bounds a layout save request.
const maxLayoutBody = 64 * 1024

type widgetResponse struct {
	ID             string         `json:"id"`
	Provider       string         `json:"provider"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Kind           string         `json:"kind"`
	Size           dashboard.Size `json:"size"`
	RefreshSeconds int            `json:"refreshSeconds"`
	// Hidden is this user's own choice; the widget is still listed so the
	// Customize view can show it again.
	Hidden bool `json:"hidden"`
}

// WidgetsHandler handles GET /api/v1/dashboard/widgets.
func WidgetsHandler(reqCtx request.RequestContext) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	engine := dashboard.Default()
	if engine == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "dashboard is not available")
		return
	}
	caller, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	var saved []dashboard.LayoutEntry
	if entries, err := layout_query.NewLayoutQueryRepo(db).Find(caller.UserID); err != nil {
		// A broken layout must not take the dashboard down: fall back to the
		// default arrangement and say so in the log.
		reqCtx.GetDI().GetLogger().Warning("dashboard: reading layout for the caller failed: %v", err)
	} else {
		saved = entries
	}
	serveWidgets(w, engine, caller, saved)
}

// LayoutHandler handles PUT /api/v1/dashboard/layout — saves the caller's own
// order and hidden flags. Keyed by the token's user id, never a caller-supplied one.
func LayoutHandler(reqCtx request.RequestContext) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != http.MethodPut {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	engine := dashboard.Default()
	if engine == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "dashboard is not available")
		return
	}
	caller, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	r.Body = http.MaxBytesReader(w, r.Body, maxLayoutBody)
	if err := saveLayout(w, r, engine, caller, layout_persistent.NewLayoutPersistentRepo(db)); err != nil {
		reqCtx.GetDI().GetLogger().Warning("dashboard: saving layout failed: %v", err)
	}
}

type layoutRequest struct {
	Widgets []struct {
		ID     string `json:"id"`
		Hidden bool   `json:"hidden"`
	} `json:"widgets"`
}

// saveLayout validates the body, drops anything the caller may not see, and
// stores the rest. It returns an error only for a storage failure (already
// answered with a 500), so the caller can log it.
func saveLayout(w http.ResponseWriter, r *http.Request, engine *dashboard.Engine, caller dashboard.Caller, store layoutWriter) error {
	var body layoutRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Widgets == nil {
		response.ErrorResponse(w, http.StatusBadRequest, "body must be {\"widgets\":[{\"id\":string,\"hidden\":bool}]}")
		return nil
	}
	if len(body.Widgets) > dashboard.MaxLayoutEntries {
		response.ErrorResponse(w, http.StatusBadRequest, "too many widgets")
		return nil
	}
	submitted := make([]dashboard.LayoutEntry, 0, len(body.Widgets))
	for _, e := range body.Widgets {
		submitted = append(submitted, dashboard.LayoutEntry{WidgetID: e.ID, Hidden: e.Hidden})
	}
	entries := dashboard.SanitizeLayout(engine.Widgets(caller), submitted)
	if err := store.ReplaceAll(caller.UserID, entries); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to save layout")
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// WidgetDataHandler handles GET /api/v1/dashboard/widgets/{id}/data.
func WidgetDataHandler(reqCtx request.RequestContext) {
	w, r := reqCtx.GetWriter(), reqCtx.GetRequest()
	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	engine := dashboard.Default()
	if engine == nil {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "dashboard is not available")
		return
	}
	caller, err := callerFrom(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	serveData(r.Context(), w, engine, caller, reqCtx.GetURI().GetPathVariable("id"))
}

func serveWidgets(w http.ResponseWriter, engine *dashboard.Engine, caller dashboard.Caller, saved []dashboard.LayoutEntry) {
	placed := dashboard.Arrange(engine.Widgets(caller), saved)
	out := make([]widgetResponse, 0, len(placed))
	for _, p := range placed {
		d := p.Definition
		out = append(out, widgetResponse{
			ID: d.ID(), Provider: d.Provider, Title: d.Title, Description: d.Description,
			Kind: string(d.Kind), Size: d.Size, RefreshSeconds: d.RefreshSeconds, Hidden: p.Hidden,
		})
	}
	response.SuccessResponse(w, http.StatusOK, out)
}

// serveData answers 404 for an id that does not exist or that the caller may not
// see (the same answer for both, so ids cannot be probed). A failing provider is
// not an HTTP error: it is a 200 whose state says so.
func serveData(ctx context.Context, w http.ResponseWriter, engine *dashboard.Engine, caller dashboard.Caller, id string) {
	data, ok := engine.Data(ctx, caller, id)
	if !ok {
		response.ErrorResponse(w, http.StatusNotFound, "widget not found")
		return
	}
	response.SuccessResponse(w, http.StatusOK, data)
}
