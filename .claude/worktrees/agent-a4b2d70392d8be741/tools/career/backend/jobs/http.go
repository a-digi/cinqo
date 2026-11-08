// http.go is the plain HTTP mirror of this package's own MCP tools
// (jobs.go/job_match.go) — a human's own browser session and the AI's
// own MCP tool-calling loop are two different callers needing two
// different transports to reach the same underlying data. Every
// handler here calls the exact same functions those files already
// built — never independently reimplemented. Deliberately not
// MCP-exposed themselves; scope enforcement happens at the host's own
// reverse proxy via manifest.json's routes[], same as every other
// tool's own HTTP routes.
//
// Originally these handlers shared one flat http.go with every other
// domain's own; each now lives beside the business logic it calls, in
// that domain's own package.
package jobs

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/companies"
	"career-tool-backend/shared"
)

// --- /jobs ---

type linkJobToCompanyRequest struct {
	ID        string `json:"id"`
	CompanyID string `json:"companyId"`
}

// JobsHandler's own GET always calls searchJobs — deliberately not a
// separate list-vs-search branch: searchJobs already degrades to
// "return everything" when query/location/companyId are all empty,
// the same overlap this step's own design doc calls out on the MCP
// side (list_jobs/search_jobs). PUT is narrow — it only ever sets or
// clears company_id (step 14's own link_job_to_company), never any
// other job field: jobs are still populated only by the AI's own
// crawling workflow (step 4), never hand-entered through this UI. See
// plan/ai/tools/career/step-14-companies.md.
func JobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		// id (Jobs page's own Eye icon -> job details page) short-circuits
		// to a single-job read instead of the paged list below — same
		// "?id= means one item" convention this handler's own DELETE
		// branch already uses.
		if id := q.Get("id"); id != "" {
			j, err := getJobByID(id)
			if err != nil {
				if errors.Is(err, errUnknownJob) {
					http.Error(w, "unknown job id", http.StatusNotFound)
					return
				}
				http.Error(w, "failed to load job: "+err.Error(), http.StatusInternalServerError)
				return
			}
			shared.WriteJSON(w, map[string]any{"job": j})
			return
		}
		limit := atoiOrZero(q.Get("limit"))
		offset := atoiOrZero(q.Get("offset"))
		result, err := searchJobs(q.Get("query"), q.Get("location"), q.Get("companyId"), q.Get("portalLinkId"), q.Get("portalId"), clampLimit(limit), offset)
		if err != nil {
			http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, result)

	case http.MethodPut:
		var body linkJobToCompanyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := linkJobToCompany(body.ID, body.CompanyID); err != nil {
			companies.WriteCompanyAwareError(w, "link job to company", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deleteJob(id); err != nil {
			http.Error(w, "failed to delete job: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
		// PUT /jobs/match is always the AI-conversation-tracking path —
		// the deterministic mechanism has its own separate endpoint
		// (POST /jobs/match/now, job_match_now.go) that calls
		// startJobMatch directly with kind="deterministic".
		if err := startJobMatch(body.JobID, body.ProfileID, body.ConversationID); err != nil {
			if errors.Is(err, errUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to start job match: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "failed":
		if err := updateJobMatchStatus(body.JobID, body.Status, body.ErrorText); err != nil {
			if errors.Is(err, errUnknownJob) {
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

// JobLocationsHandler handles GET /job-locations — the Jobs page's own
// Location filter dropdown's data source (step — see
// listDistinctJobLocations's own doc comment). Read-only, no
// query params: every distinct location is always returned, the same
// "fetch everything, no pagination" shape fetchCompanies/fetchPortals
// already use for their own filter dropdowns.
func JobLocationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	locations, err := listDistinctJobLocations()
	if err != nil {
		http.Error(w, "failed to list job locations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	shared.WriteJSON(w, map[string]any{"locations": locations})
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
