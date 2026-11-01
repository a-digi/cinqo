// Package personacreatedconsumer is the consumer of "career.persona.created" (step 151): a new persona can make its profile fit jobs
// better, so the event notes the persona's profile (payload profile_id) for a re-match of the jobs, run by the shared worker (package
// rematchworker) once the profile has been quiet for a while. It also listens to "career.persona.changed" for one purpose: a change of a
// persona of a profile that is already waiting moves that profile's quiet timer (more personas and skills are probably coming); it never
// starts a re-match by itself. See plan/ai/tools/career/step-151-profile-persona-created-rematch.md.
package personacreatedconsumer

import (
	"net/http"

	"career-tool-backend/rematchworker"
)

// EventHandler is POST /events/persona-created-rematch: it validates the event ({"persona_id", "profile_id"}), notes the persona's
// profile and answers 202 before any work.
func EventHandler(w http.ResponseWriter, r *http.Request) {
	rematchworker.Default.NoteHandler("profile_id")(w, r)
}

// ChangedHandler is POST /events/persona-changed-rematch-rearm for "career.persona.changed": it re-arms the wait of the persona's
// profile, only when that profile is already pending.
func ChangedHandler(w http.ResponseWriter, r *http.Request) {
	rematchworker.Default.RearmHandler("profile_id")(w, r)
}
