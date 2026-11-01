// Package profilecreatedconsumer is the consumer of "career.profile.created" (step 151): it notes the new profile for a re-match of
// the jobs (jobs.RematchProfile), which the shared worker (package rematchworker) runs once the profile has been quiet for a while, so
// the personas and skills that follow the creation are in place. See plan/ai/tools/career/step-151-profile-persona-created-rematch.md.
package profilecreatedconsumer

import (
	"net/http"

	"career-tool-backend/rematchworker"
)

// EventHandler is POST /events/profile-created-rematch, reached only by core's loopback dispatcher: it validates the event
// ({"profile_id": "…"}), notes the profile and answers 202 before any work.
func EventHandler(w http.ResponseWriter, r *http.Request) {
	rematchworker.Default.NoteHandler("profile_id")(w, r)
}
