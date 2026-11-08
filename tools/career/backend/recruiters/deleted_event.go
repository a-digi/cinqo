package recruiters

import (
	"career-tool-backend/domainevent"
)

// PublishRecruiterDeleted announces "career.recruiter.deleted" for a recruiter whose row was really removed (plan 168, step 7),
// with its id. Best effort like every domain event, and synchronous: domainevent.Publish's own short timeout bounds it. Callers
// send it only after the delete succeeded and affected a row.
func PublishRecruiterDeleted(id string) {
	domainevent.Publish("career.recruiter.deleted", map[string]string{"recruiter_id": id})
}
