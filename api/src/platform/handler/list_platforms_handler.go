package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/platform"
)

// ListPlatformsHandler handles GET /api/v1/platforms — the registered
// {id, name} pairs, nothing more (step 1's List). See
// plan/ai/platform/step-05-platform-and-key-api.md.
func ListPlatformsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	response.SuccessResponse(w, http.StatusOK, platform.List())
}
