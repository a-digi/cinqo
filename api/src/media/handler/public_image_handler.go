package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// publicImageCacheControl gives a route with NO auth check a real
// cache lifetime — DownloadHandler (authenticated, ownership-checked,
// not meant to be hit repeatedly and cheaply) sets none today, but
// that reasoning doesn't apply here.
const publicImageCacheControl = "public, max-age=86400"

// PublicImageHandler handles GET /api/v1/media/images/{id}/public — a
// genuinely unauthenticated route (route-media.yaml's own
// `security: public`, same convention as ToolFrontendBundle), so a
// Tool can embed it in a plain <img src>, fetch it server-to-server
// with no browser session, or show it to a visitor who isn't logged in
// at all. See plan/ai/media/step-06-public-image-url.md.
//
// The id's own randomness (a UUID, never sequential — same reasoning
// as media_query.go's own Resolve doc comment) is the ONLY access
// control here, deliberately: this route NEVER serves a row that isn't
// already an image (Width/Height both zero, e.g. a PDF or a tool
// install zip) — that check is the hard boundary between "publicly
// servable" and everything else in Media, which stays exactly as
// ownership-locked as it always was via DownloadHandler.
func PublicImageHandler(reqCtx request.RequestContext) {
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

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	m, err := media_query.NewMediaQueryRepo(db).FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "media not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up media")
		return
	}

	if m.Width == 0 || m.Height == 0 {
		response.ErrorResponse(w, http.StatusNotFound, "media not found")
		return
	}

	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", m.OriginalFilename))
	w.Header().Set("Cache-Control", publicImageCacheControl)
	http.ServeFile(w, r, m.StoredPath)
}
