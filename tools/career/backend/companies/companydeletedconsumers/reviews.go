package companydeletedconsumers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/reviews"
)

// CompanyReviewsDeleteHandler is POST /events/company-deleted-reviews: it removes every review (star rating) of the company that was
// deleted, the same way CompanyCommentsDeleteHandler removes its comments. It answers 200 when the reviews are gone (or there were
// none), 400 for an unreadable event or an empty company id, 405 for a method other than POST, 200 too when core refuses with a 4xx
// (asking again gets the same answer; it is logged), and 500 when core could not purge them (unreachable, not configured, or a 5xx), so
// core's own retries deliver the event again. Purging twice is harmless, so a redelivery needs no memory.
func CompanyReviewsDeleteHandler(w http.ResponseWriter, r *http.Request) {
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
	var payload companyDeletedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.CompanyID == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	if err := reviews.PurgeEntityStrict(reviews.CompanyType, payload.CompanyID); err != nil {
		var refused *reviews.RefusedError
		if errors.As(err, &refused) && refused.Final() {
			log.Printf("company reviews delete: company %s: %v (not retried)", payload.CompanyID, err)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("company reviews delete: company %s: %v", payload.CompanyID, err)
		http.Error(w, "failed to delete the company's reviews", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
