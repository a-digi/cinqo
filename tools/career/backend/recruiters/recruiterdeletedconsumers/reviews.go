package recruiterdeletedconsumers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/reviews"
)

// RecruiterReviewsDeleteHandler is POST /events/recruiter-deleted-reviews: it removes every review (star rating) of the recruiter that
// was deleted, the same way RecruiterCommentsDeleteHandler removes its comments and the company consumer removes a company's reviews
// (plan 271). It answers 200 when the reviews are gone (or there were none), 400 for an unreadable event or an empty recruiter id, 405
// for a method other than POST, 200 too when core refuses with a 4xx (asking again gets the same answer; it is logged), and 500 when
// core could not purge them (unreachable, not configured, or a 5xx), so core's own retries deliver the event again. Purging twice is
// harmless, so a redelivery needs no memory.
func RecruiterReviewsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var evt db.EventEnvelope
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	var payload recruiterDeletedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.RecruiterID == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	if err := reviews.PurgeEntityStrict(reviews.RecruiterType, payload.RecruiterID); err != nil {
		var refused *reviews.RefusedError
		if errors.As(err, &refused) && refused.Final() {
			log.Printf("recruiter reviews delete: recruiter %s: %v (not retried)", payload.RecruiterID, err)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("recruiter reviews delete: recruiter %s: %v", payload.RecruiterID, err)
		http.Error(w, "failed to delete the recruiter's reviews", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
