package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/platform"
)

// platformResponse is {id, name} plus the platform's own selectable
// model list — empty (never null) when no model choice applies, so the
// frontend's create-conversation flow knows whether to show a model
// picker at all. See
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
type platformResponse struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

// ListPlatformsHandler handles GET /api/v1/platforms. See
// plan/ai/platform/step-05-platform-and-key-api.md.
func ListPlatformsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	entries := platform.List()
	out := make([]platformResponse, 0, len(entries))
	for _, info := range entries {
		entry, _ := platform.Lookup(info.ID)
		models := entry.SelectableModels
		if models == nil {
			models = []string{}
		}
		out = append(out, platformResponse{ID: info.ID, Name: info.Name, Models: models})
	}

	response.SuccessResponse(w, http.StatusOK, out)
}
