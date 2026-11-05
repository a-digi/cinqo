package jobshandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/jobs"
	"career-tool-backend/match"
)

// jobMatchUpdateRequest is the human/frontend-facing shape PUT
// /jobs/match accepts — deliberately has NO score or personaId field
// at all: those are written exclusively by the AI's own save_job_match
// MCP tool (job_match.go), never by this endpoint, so a compromised or
// buggy frontend call can never fabricate a match result. Status is
// always required (the only two real calls are "start tracking a
// fresh attempt" [status: matching, profileId + conversationId set]
// and "record a client-observed failure" [status: failed, error set])
// — ErrorText a plain *string (not the elsewhere-established
// presence-gated-clear pattern) since this endpoint never needs to
// CLEAR a previously recorded error independently of also changing
// status; every call sets both together.
type jobMatchUpdateRequest struct {
	JobID          string  `json:"jobId"`
	ProfileID      string  `json:"profileId,omitempty"`
	Status         string  `json:"status"`
	ConversationID string  `json:"conversationId,omitempty"`
	ErrorText      *string `json:"error,omitempty"`
}

// JobMatchHandler handles PUT /jobs/match — see jobMatchUpdateRequest's
// own doc comment for the security reasoning behind its narrow shape.
func JobMatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body jobMatchUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	switch body.Status {
	case "matching":
		if body.ProfileID == "" {
			http.Error(w, "profileId is required when starting a match", http.StatusBadRequest)
			return
		}
		if err := match.StartJobMatch(body.JobID, body.ProfileID, body.ConversationID); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to start job match: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "failed":
		if err := match.UpdateJobMatchStatus(body.JobID, body.Status, body.ErrorText); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to update job match: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, `status must be "matching" or "failed"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- /jobs/cv — "Generate CV PDF" ---
