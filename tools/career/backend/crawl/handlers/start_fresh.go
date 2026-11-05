package crawlhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

type startFreshRequest struct {
	PortalLinkID      string `json:"portalLinkId"`
	ClearInstructions bool   `json:"clearInstructions"`
}

// StartFreshHandler handles POST /portal-links/start-fresh (step 161): wipes one link's earlier AI attempts
// and leftovers (and, only when asked, its stored crawl instructions) and returns the ids of the conversations
// those attempts ran in, for the frontend to delete through the core. 409 while a generation is running.
func StartFreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body startFreshRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInstructionAttemptBodyBytes)).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	result, err := portal.StartFresh(body.PortalLinkID, body.ClearInstructions)
	if errors.Is(err, portal.ErrGenerationRunning) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		portal.WritePortalLinkAwareError(w, "start fresh", err)
		return
	}
	db.WriteJSON(w, result)
}
