package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
	"career-tool-backend/jobs"
	"career-tool-backend/profile"
)

// The manual (no AI) Job Match routes (step 129), all behind tool:career:jobs. See
// plan/ai/tools/career/step-129-manual-job-match.md. Nothing here calls an AI.

const maxJobMatchBodyBytes = 64 << 10

// writeJSONStatus writes a JSON body with a status other than 200.
func writeJSONStatus(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeJobMatchError maps a match or requirements error to its HTTP answer. Not enough data to match is a 422
// whose body carries the reasons a person can act on.
func writeJobMatchError(w http.ResponseWriter, action string, err error) {
	var ins *jobmatcher.InsufficientDataError
	if re, ok := jobs.IsRequirementsError(err); ok {
		http.Error(w, re.Message, http.StatusBadRequest)
		return
	}
	switch {
	case errors.Is(err, jobs.ErrUnknownJob):
		http.Error(w, "unknown job id", http.StatusNotFound)
	case errors.Is(err, profile.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusNotFound)
	case errors.Is(err, jobmatcher.ErrNoPersonas):
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "reasons": []string{err.Error()}})
	case errors.As(err, &ins):
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]any{"error": ins.Error(), "reasons": ins.Reasons})
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// jobMatchRunHandler handles POST /jobs/match/run: scores the job against the profile's personas with the manual
// engine and stores the best result as the job's match. The body names the job and the profile and nothing else:
// the score only ever comes from the engine, never from the request.
func jobMatchRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		JobID     string `json:"jobId"`
		ProfileID string `json:"profileId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJobMatchBodyBytes)).Decode(&body); err != nil || body.JobID == "" || body.ProfileID == "" {
		http.Error(w, "jobId and profileId are required", http.StatusBadRequest)
		return
	}
	result, err := jobs.RunManualMatch(body.JobID, body.ProfileID)
	if err != nil {
		writeJobMatchError(w, "match the job", err)
		return
	}
	db.WriteJSON(w, result)
}

// jobRequirementsHandler handles GET /job-requirements?jobId= (the job's requirements) and PUT (replace the whole
// list: { jobId, requirements: [{ term, tier }] }).
func jobRequirementsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobID := r.URL.Query().Get("jobId")
		if jobID == "" {
			http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
			return
		}
		list, err := jobs.GetJobRequirements(jobID)
		if err != nil {
			writeJobMatchError(w, "read the requirements", err)
			return
		}
		db.WriteJSON(w, map[string]any{"requirements": list})

	case http.MethodPut:
		var body struct {
			JobID        string                  `json:"jobId"`
			Requirements []jobs.RequirementInput `json:"requirements"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJobMatchBodyBytes)).Decode(&body); err != nil || body.JobID == "" {
			http.Error(w, "jobId and requirements are required", http.StatusBadRequest)
			return
		}
		list, err := jobs.SetJobRequirements(body.JobID, body.Requirements)
		if err != nil {
			writeJobMatchError(w, "save the requirements", err)
			return
		}
		db.WriteJSON(w, map[string]any{"requirements": list})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// jobRequirementsSuggestHandler handles GET /job-requirements/suggest?jobId=: the job's most distinctive words as
// candidate requirements (a word count against the saved jobs, never an AI call).
func jobRequirementsSuggestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
		return
	}
	suggestions, err := jobs.SuggestJobRequirements(jobID)
	if err != nil {
		writeJobMatchError(w, "suggest requirements", err)
		return
	}
	db.WriteJSON(w, map[string]any{"suggestions": suggestions})
}
