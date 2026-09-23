package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// toolServiceAuthScheme mirrors
// api/src/domainevent/handler/publish_handler.go's own constant of the
// same name exactly (deliberately not "Bearer" — a tool's own
// persistent service_token is a machine identity, never an end user's
// session).
const toolServiceAuthScheme = "ToolService "

type updateMediaTitleServiceRequest struct {
	Title string `json:"title"`
}

// UpdateTitleServiceHandler handles PATCH /api/v1/media/{id}/title — a
// SEPARATE route/handler from update_title_handler.go's own
// PATCH /api/v1/media/{id} (that one is end-user-JWT-ownership-checked;
// this one is Tool-service-token-checked — two different identity
// concepts, kept as two routes rather than branching inside one
// handler). route-media.yaml declares this "security: public" for the
// exact same reason publish_handler.go's own route does: a
// service-token-only request carries no end-user JWT/cookie at all, so
// the normal ScopeSecurityLayer would 401 it before this handler ever
// ran.
//
// A Tool may only retitle a row that already belongs to ITS OWN
// tool_slug — the same "own domain only" boundary this codebase
// already applies everywhere else in Media, just keyed by Tool
// identity (via FindByServiceToken) instead of a human's own scope
// set. Exists so a Tool's own backend (which never has an end-user
// session to authenticate a PATCH with — see
// tools/career/backend/media/settitle.go's own doc comment) can set a
// title it alone can compute deterministically (e.g. Career's own
// {job_title}_{portal_name}.pdf, only known once its own job/portal
// lookup runs). See plan/ai/media/step-10-obligatory-title.md.
func UpdateTitleServiceHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodPatch {
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

	var body updateMediaTitleServiceRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "title is required")
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
		response.ErrorResponse(w, http.StatusForbidden, "not authorized to update this media file")
		return
	}

	if err := media_persistent.NewMediaPersistentRepo(db).UpdateTitle(id, body.Title); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to update media")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
