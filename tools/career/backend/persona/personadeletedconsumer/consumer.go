// Package personadeletedconsumer is the consumer of "career.persona.deleted" (step 152): when a persona was deleted it removes the
// matches that persona produced (jobs.PurgePersonaMatches: the job matches whose best persona it was, and its persona scores), and
// hands the jobs that lost their match to the job-match worker so the profile's other personas, or the other profiles, match them
// at once. See plan/ai/tools/career/step-152-profile-persona-deleted-events.md.
package personadeletedconsumer

import (
	"encoding/json"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobmatchconsumer"
	"career-tool-backend/jobs"
)

// maxIDLength bounds the ids of an event (real ids are UUIDs).
const maxIDLength = 200

// What the handler uses; vars so a test can watch the calls and make them fail.
var (
	purge    = jobs.PurgePersonaMatches
	handOver = jobmatchconsumer.Enqueue
)

// EventHandler is POST /events/persona-deleted-matches, reached only by core's loopback dispatcher: it removes the persona's matches
// and hands the freed jobs over. It answers 200 when the matches are gone (or there were none), 400 for an unreadable event or an
// empty or overlong persona_id or profile_id (both are needed: the profile scopes the removal), 405 for a method other than POST, and
// 500 when the removal fails, so core's own retries deliver the event again. Doing it twice is harmless.
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
		PersonaID string `json:"persona_id"`
		ProfileID string `json:"profile_id"`
	}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil ||
		payload.PersonaID == "" || len(payload.PersonaID) > maxIDLength || payload.ProfileID == "" || len(payload.ProfileID) > maxIDLength {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	freed, err := purge(payload.PersonaID, payload.ProfileID)
	if err != nil {
		log.Printf("persona deleted: persona %s: %v", payload.PersonaID, err)
		http.Error(w, "failed to remove the persona's matches", http.StatusInternalServerError)
		return
	}
	if len(freed) > 0 {
		queued := handOver(freed)
		log.Printf("persona deleted: persona %s: %d match(es) removed, %d job(s) queued for a new match", payload.PersonaID, len(freed), queued)
	}
	w.WriteHeader(http.StatusOK)
}
