package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// MineHandler handles GET /api/v1/media/mine?toolSlug=... — any
// authenticated caller, no static scope (route-media.yaml): unlike
// ListHandler (cinqo:super:admin, sees every user's media), this only
// ever returns the CALLING user's own uploads, so no extra privilege
// is needed to see your own data. userID comes only from
// callerIdentity's validated JWT subject, never from a query param or
// body field — there is no way to pass someone else's id in. Backs a
// tool's own "my uploads" list (e.g. Career's Import CV page, relayed
// through its own backend — see
// plan/ai/media/step-04-career-uploaded-list.md).
func MineHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, _, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	toolSlug := r.URL.Query().Get("toolSlug")

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	files, err := media_query.NewMediaQueryRepo(db).FindAllForUser(userID, toolSlug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list media")
		return
	}

	out := make([]mediaFileResponse, 0, len(files))
	for _, f := range files {
		out = append(out, toMediaFileResponse(f))
	}

	response.SuccessResponse(w, http.StatusOK, out)
}
