package jobdeletedconsumers

import (
	"log"
	"net/http"

	"career-tool-backend/match/personamatch"
)

// PersonaMatchJobDeletedHandler is POST /events/job-deleted-persona-matches: PersonaMatchJobDeleted deletes the mapping
// between the personas and the job that was deleted (step 141). The mapping lives in the same database as the job with
// ON DELETE CASCADE, so the job's own delete has normally removed it already; this is the explicit consumer that makes
// sure no row survives, and it does nothing when there is none. 200 when the mapping is gone (or was never there), 400 for
// an unreadable event or an empty job id, 405 for a method other than POST, 500 when the database fails (core's own
// retries deliver the event again). Deleting twice is harmless, so a redelivery needs no memory.
func PersonaMatchJobDeletedHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := readJobDeleted(w, r)
	if !ok {
		return
	}
	if err := personamatch.DeleteForJob(payload.JobID); err != nil {
		log.Printf("persona match job deleted: job %s: %v", payload.JobID, err)
		http.Error(w, "failed to delete the job's persona matches", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
