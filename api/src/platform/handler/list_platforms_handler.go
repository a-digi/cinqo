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
	// RequiresKey is true for a platform reached with a stored API key
	// (the cloud platforms): the frontend shows "add a key" only then.
	RequiresKey bool `json:"requiresKey"`
	// OffersTools is false for a platform whose conversations are offered no tools and run in direct mode only: the
	// frontend then does not offer composer mode for it.
	OffersTools bool `json:"offersTools"`
	// NeedsShortSteps says the platform's models do better with work split into short, one-action messages than with one long
	// prompt (a caller that can split its work should).
	NeedsShortSteps bool `json:"needsShortSteps"`
}

// ListPlatformsHandler handles GET /api/v1/platforms. See
// plan/ai/platform/step-05-platform-and-key-api.md.
func ListPlatformsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	response.SuccessResponse(w, http.StatusOK, platformResponses())
}

// platformResponses is the list the endpoint returns: every registered platform that is available right now.
func platformResponses() []platformResponse {
	entries := platform.List()
	out := make([]platformResponse, 0, len(entries))
	for _, info := range entries {
		entry, _ := platform.Lookup(info.ID)
		if !entry.IsAvailable() {
			continue
		}
		out = append(out, platformResponse{ID: info.ID, Name: info.Name, Models: entry.SelectableModelList(), RequiresKey: entry.RequiresKey(), OffersTools: entry.OffersAllTools(), NeedsShortSteps: entry.ShortSteps})
	}
	return out
}
