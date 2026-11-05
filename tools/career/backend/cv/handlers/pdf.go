package cvhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/jobs"
)

type cvPdfUpdateRequest struct {
	JobID          string  `json:"jobId"`
	ProfileID      string  `json:"profileId,omitempty"`
	Status         string  `json:"status"`
	ConversationID string  `json:"conversationId,omitempty"`
	ErrorText      *string `json:"error,omitempty"`
}

// CvPdfHandler handles PUT /jobs/cv — starts tracking a new CV
// generation attempt (status: "generating", profileId + conversationId
// set) or records a client-observed failure (status: "failed", error
// set). Mirrors jobMatchHandler exactly. A successful render is never
// recorded here — that's the AI-facing save_cv_document MCP tool's own job
// (cv_pdf.go's own top doc comment explains how it ends up with an
// already-permanent Media file id by the time it runs).
func CvPdfHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body cvPdfUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	switch body.Status {
	case "generating":
		if body.ProfileID == "" {
			http.Error(w, "profileId is required when starting a CV generation", http.StatusBadRequest)
			return
		}
		if err := jobs.StartCvGeneration(body.JobID, body.ProfileID, body.ConversationID); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to start CV generation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "failed":
		if err := jobs.UpdateCvGenerationStatus(body.JobID, body.Status, body.ErrorText); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to update CV generation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, `status must be "generating" or "failed"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- /jobs/cv-link — attach or remove a CV without the AI (step 98) ---
