// events.go holds this package's own Domain Events listener —
// automatic job-to-company linking
// (plan/ai/tools/career/step-83-job-created-company-linking.md).
// Reached only via core's own loopback-bound dispatcher
// (plan/ai/domain-events/step-04-core-to-tool-delivery.md), declared
// under manifest.json's own event_listeners[]. Unlike portal's own two
// listeners (portal/events.go), which only ever set a request flag
// because their own follow-up action (crawling, starting an AI
// conversation) needs a live end-user session a backend event handler
// doesn't have, resolving/creating a company and linking a job to it
// are both plain, unprivileged local DB writes — so this handler
// performs the real work directly, no flag/defer needed.
package jobs

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/companies"
	"career-tool-backend/db"
)

// jobCreatedEventPayload is the payload shape "career.job.created"
// uses — see jobs.go's own publishJobCreatedEvent.
type jobCreatedEventPayload struct {
	JobID   string `json:"job_id"`
	Company string `json:"company"`
}

// JobCreatedEventHandler handles "career.job.created" — resolves the
// job's own company by a case-insensitive, trimmed name match
// (companies.FindCompanyByName), creating a new companies row only
// when no match exists, then links the job to it via the existing
// LinkJobToCompany (this package's own jobs.go, already used by the
// link_job_to_company MCP tool for manual/AI linking). An unknown or
// since-deleted job id is a benign no-op (LinkJobToCompany's own UPDATE
// simply affects zero rows) — the same tolerance
// JobsCrawledEventHandler already shows portal-side for a portal link
// that no longer exists by the time its own event is delivered.
func JobCreatedEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var evt db.EventEnvelope
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	var payload jobCreatedEventPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.JobID == "" || payload.Company == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}

	companyID, found, err := companies.FindCompanyByName(payload.Company)
	if err != nil {
		http.Error(w, "failed to look up company", http.StatusInternalServerError)
		return
	}
	if !found {
		companyID, err = companies.CreateCompany(payload.Company, "")
		if err != nil {
			http.Error(w, "failed to create company", http.StatusInternalServerError)
			return
		}
	}

	if err := LinkJobToCompany(payload.JobID, companyID); err != nil {
		http.Error(w, "failed to link job to company", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
