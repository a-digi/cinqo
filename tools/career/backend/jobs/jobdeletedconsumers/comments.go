// Package jobdeletedconsumers holds the consumers of "career.job.deleted" (step 139). Each is a plain HTTP handler
// reached only by core's loopback dispatcher and declared under manifest.json's event_listeners[].
package jobdeletedconsumers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"career-tool-backend/comments"
	"career-tool-backend/db"
)

// jobDeletedPayload is what "career.job.deleted" carries (see jobs.PublishJobDeleted); the consumers use only the ids.
type jobDeletedPayload struct {
	JobID     string `json:"job_id"`
	CompanyID string `json:"company_id"`
}

// readJobDeleted reads and checks the event of a POST. It answers the request itself and returns ok false when the
// method is wrong or the event is unreadable or has no job id.
func readJobDeleted(w http.ResponseWriter, r *http.Request) (jobDeletedPayload, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return jobDeletedPayload{}, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var evt db.EventEnvelope
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return jobDeletedPayload{}, false
	}
	var payload jobDeletedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.JobID == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return jobDeletedPayload{}, false
	}
	return payload, true
}

// JobCommentsDeleteHandler is POST /events/job-deleted-comments: JobCommentsDelete removes every comment of the job
// that was deleted. It answers 200 when the comments are gone (or there were none), 400 for an unreadable event or an
// empty job id, and 500 when core could not purge them (unreachable, not configured, or a 5xx), so core's own retries deliver the event again. Purging twice
// is harmless, so a redelivery needs no memory.
func JobCommentsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	payload, ok := readJobDeleted(w, r)
	if !ok {
		return
	}
	if err := comments.PurgeEntityStrict(comments.JobType, payload.JobID); err != nil {
		var refused *comments.RefusedError
		if errors.As(err, &refused) && refused.Final() {
			// Core will say the same again (an id it does not accept, a bad token): log it and let the event go.
			log.Printf("job comments delete: job %s: %v (not retried)", payload.JobID, err)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("job comments delete: job %s: %v", payload.JobID, err)
		http.Error(w, "failed to delete the job's comments", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
