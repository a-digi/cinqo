// Package companydeletedconsumers holds the consumers of "career.company.deleted" (step 139). Each is a plain HTTP
// handler reached only by core's loopback dispatcher and declared under manifest.json's event_listeners[].
package companydeletedconsumers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"career-tool-backend/comments"
	"career-tool-backend/db"
)

// companyDeletedPayload is what "career.company.deleted" carries (see companies.PublishCompanyDeleted); the consumers
// here use only the id.
type companyDeletedPayload struct {
	CompanyID string `json:"company_id"`
}

// CompanyCommentsDeleteHandler is POST /events/company-deleted-comments: CompanyCommentsDelete removes every comment of
// the company that was deleted. It answers 200 when the comments are gone (or there were none), 400 for an unreadable
// event or an empty company id, 405 for a method other than POST, 200 too when core refuses with a 4xx (asking again
// gets the same answer; it is logged), and 500 when core could not purge them (unreachable, not configured, or a 5xx),
// so core's own retries deliver the event again. Purging twice is harmless, so a redelivery needs no memory.
func CompanyCommentsDeleteHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := comments.PurgeEntityStrict(comments.CompanyType, payload.CompanyID); err != nil {
		var refused *comments.RefusedError
		if errors.As(err, &refused) && refused.Final() {
			log.Printf("company comments delete: company %s: %v (not retried)", payload.CompanyID, err)
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Printf("company comments delete: company %s: %v", payload.CompanyID, err)
		http.Error(w, "failed to delete the company's comments", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
