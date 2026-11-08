package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_query "github.com/a-digi/cinqo/src/media/repository/query"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// ExistsServiceHandler handles GET /api/v1/media/{id}/exists — a
// read-only sibling of update_title_service_handler.go's own
// PATCH .../title, same ToolService-bearer auth and same "own tool_slug
// only" boundary, but with no side effect: it exists so a Tool's own
// backend can cheaply confirm a Media file id it's holding onto (e.g.
// Career's own job_cv_pdfs.media_file_id) still refers to a real row
// before showing it as available, rather than discovering it's gone
// only when a human clicks a dead download link. Returns 204 if the
// row exists and belongs to the calling tool, 404 otherwise (never
// distinguishing "doesn't exist" from "exists but belongs to another
// tool" in the response body — same "don't expose why" convention as
// every other Media handler's 404). See
// plan/ai/media/step-11-media-exists-check.md.
func ExistsServiceHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), toolServiceAuthScheme)
	if !ok {
		response.ErrorResponse(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	tool, err := tool_query.NewToolQueryRepo(db).FindByServiceToken(token)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to authenticate")
			return
		}
		response.ErrorResponse(w, http.StatusUnauthorized, "invalid service token")
		return
	}

	m, err := media_query.NewMediaQueryRepo(db).FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "media not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up media")
		return
	}

	if m.ToolSlug != tool.Slug {
		response.ErrorResponse(w, http.StatusNotFound, "media not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
