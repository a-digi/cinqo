package crawlhandlers

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// maxInstructionAttemptBodyBytes bounds the start/finish request
// bodies; the longest legitimate field (a capped error message) is far
// below this.
const maxInstructionAttemptBodyBytes = 64 << 10

type instructionAttemptStartRequest struct {
	PortalLinkID   string `json:"portalLinkId"`
	Kind           string `json:"kind"`
	TriggeredBy    string `json:"triggeredBy"`
	ConversationID string `json:"conversationId"`
}

type instructionAttemptFinishRequest struct {
	PortalLinkID string `json:"portalLinkId"`
	Kind         string `json:"kind"`
	Error        string `json:"error"`
}

// InstructionAttemptStartHandler handles POST /portal-links/
// instruction-attempts/start (step 86) — the frontend's "a Generate
// with AI run is starting" signal. Idempotent: started is false when
// one was already running for this link and kind.
func InstructionAttemptStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body instructionAttemptStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInstructionAttemptBodyBytes)).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	if body.TriggeredBy == "" {
		body.TriggeredBy = portal.InstructionTriggerManual
	}
	attemptID, started, err := portal.StartInstructionAttempt(body.PortalLinkID, body.Kind, body.TriggeredBy, body.ConversationID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "start instruction attempt", err)
		return
	}
	db.WriteJSON(w, map[string]any{"attemptId": attemptID, "started": started})
}

// InstructionAttemptFinishHandler handles POST /portal-links/
// instruction-attempts/finish (step 86). The outcome is decided
// server-side (succeeded only if an AI save tool saved during the
// run); error is just the reason to show if it failed. status is
// "none" when no attempt was running — not an error, so the fresh-run
// and resumed-after-reload paths can both call this.
func InstructionAttemptFinishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body instructionAttemptFinishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxInstructionAttemptBodyBytes)).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	status, err := portal.FinishInstructionAttempt(body.PortalLinkID, body.Kind, body.Error)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "finish instruction attempt", err)
		return
	}
	db.WriteJSON(w, map[string]any{"status": status})
}
