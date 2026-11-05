package jobshandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/companies"
	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

type linkJobToCompanyRequest struct {
	ID        string `json:"id"`
	CompanyID string `json:"companyId"`
}

// JobsHandler's own GET always calls jobs.SearchJobs — deliberately not
// a separate list-vs-search branch: SearchJobs already degrades to
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
			j, err := jobs.GetJobByID(id)
			if err != nil {
				if errors.Is(err, jobs.ErrUnknownJob) {
					http.Error(w, "unknown job id", http.StatusNotFound)
					return
				}
				http.Error(w, "failed to load job: "+err.Error(), http.StatusInternalServerError)
				return
			}
			db.WriteJSON(w, map[string]any{"job": j})
			return
		}
		limit := atoiOrZero(q.Get("limit"))
		offset := atoiOrZero(q.Get("offset"))
		// companyId may be repeated (?companyId=A&companyId=B) since the
		// Jobs page's own company filter is multi-select — q["companyId"]
		// (unlike q.Get, which only ever returns the first) is every
		// value for that key, filtered of any accidental empty entries.
		var companyIds []string
		for _, id := range q["companyId"] {
			if id != "" {
				companyIds = append(companyIds, id)
			}
		}
		result, err := jobs.SearchJobsSorted(q.Get("query"), q.Get("location"), companyIds, q.Get("portalLinkId"), q.Get("portalId"), jobs.ClampLimit(limit), offset, q.Get("sort"))
		if errors.Is(err, jobs.ErrInvalidSort) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, result)

	// Step 97: a person adds a job by hand.
	case http.MethodPost:
		var body struct {
			Title       string `json:"title"`
			Company     string `json:"company"`
			Location    string `json:"location"`
			Description string `json:"description"`
			PostedAt    string `json:"postedAt"`
			SourceURL   string `json:"sourceUrl"`
		}
		// A description is at most 100,000 characters; leave room for the rest.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		j, err := jobs.AddManualJob(jobs.ManualJobInput{
			Title: body.Title, Company: body.Company, Location: body.Location,
			Description: body.Description, PostedAt: body.PostedAt, SourceURL: body.SourceURL,
		})
		if v, ok := jobs.IsValidationError(err); ok {
			http.Error(w, v.Error(), http.StatusBadRequest)
			return
		}
		if d, ok := jobs.IsDuplicate(err); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "duplicate", "reason": d.Reason, "message": d.Error(), "existingJobId": d.ExistingID})
			return
		}
		if err != nil {
			http.Error(w, "failed to add job: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"job": j})

	case http.MethodPut:
		var body linkJobToCompanyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := jobs.LinkJobToCompany(body.ID, body.CompanyID); err != nil {
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
		if err := jobs.DeleteJob(id); err != nil {
			http.Error(w, "failed to delete job: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
