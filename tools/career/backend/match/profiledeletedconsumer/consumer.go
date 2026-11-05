// Package profiledeletedconsumer is the consumer of "career.profile.deleted" (step 152): when a profile was deleted it removes every
// match the profile left behind (jobs.PurgeProfileMatches), so no zombie match stays in the Match column, and hands the jobs that
// lost their match to the job-match worker so the profiles that remain match them at once. See
// plan/ai/tools/career/step-152-profile-persona-deleted-events.md.
package profiledeletedconsumer

import (
	"encoding/json"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/match"
	"career-tool-backend/match/jobmatchconsumer"
	"career-tool-backend/match/rematchworker"
)

// maxIDLength bounds the profile id of an event (real ids are UUIDs).
const maxIDLength = 200

// What the handler uses; vars so a test can watch the calls and make them fail.
var (
	purge    = match.PurgeProfileMatches
	forget   = rematchworker.Forget
	handOver = jobmatchconsumer.Enqueue
)

// EventHandler is POST /events/profile-deleted-matches, reached only by core's loopback dispatcher: it takes the profile out of the
// re-match worker's waiting set, removes the profile's matches and hands the freed jobs over. It answers 200 when the matches are gone
// (or there were none), 400 for an unreadable event or an empty or overlong profile id (only profile_id of the payload is used), 405
// for a method other than POST, and 500 when the removal fails, so core's own retries deliver the event again. Doing it twice is
// harmless, so a redelivery needs no memory.
func EventHandler(w http.ResponseWriter, r *http.Request) {
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
	var payload struct {
		ProfileID string `json:"profile_id"`
	}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.ProfileID == "" || len(payload.ProfileID) > maxIDLength {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	// Out of the waiting set first: a profile that is deleted must not be re-matched (a run already going is waited for by the purge).
	forget(payload.ProfileID)
	freed, err := purge(payload.ProfileID)
	if err != nil {
		log.Printf("profile deleted: profile %s: %v", payload.ProfileID, err)
		http.Error(w, "failed to remove the profile's matches", http.StatusInternalServerError)
		return
	}
	if len(freed) > 0 {
		queued := handOver(freed)
		log.Printf("profile deleted: profile %s: %d match(es) removed, %d job(s) queued for a new match", payload.ProfileID, len(freed), queued)
	}
	w.WriteHeader(http.StatusOK)
}
