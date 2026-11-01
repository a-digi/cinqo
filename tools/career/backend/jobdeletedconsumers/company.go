package jobdeletedconsumers

import (
	"log"
	"net/http"

	"career-tool-backend/companies"
)

// CompanyDeleteHandler is POST /events/job-deleted-company: CompanyDelete deletes the company of the job that was
// deleted when that company has no other job and no recruiters (companies.DeleteCompanyIfUnused, one guarded
// statement). A job that had no company (an empty company_id), a company that is gone already or one that is still in
// use is no work and no error: 200. 400 for an unreadable event or an empty job id, 405 for a method other than POST, 500
// when the database fails (core's own retries deliver the event again). Only the company the event names is ever
// touched, so a company nobody has just left (one made by hand with no jobs yet) is never deleted by this.
func CompanyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := readJobDeleted(w, r)
	if !ok {
		return
	}
	if payload.CompanyID != "" {
		if _, err := companies.DeleteCompanyIfUnused(payload.CompanyID); err != nil {
			log.Printf("company delete: company %s (job %s): %v", payload.CompanyID, payload.JobID, err)
			http.Error(w, "failed to check or delete the company", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
