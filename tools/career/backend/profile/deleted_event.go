package profile

import "career-tool-backend/domainevent"

// publishProfileDeleted announces "career.profile.deleted" with { profile_id } after a profile row was really deleted (step 152). The
// profile's personas went with it inside career.db and are not announced one by one: the consumer removes everything of the profile by
// its id. Best effort like every domain event and synchronous: domainevent.Publish's own short timeout bounds it.
func publishProfileDeleted(profileID string) {
	domainevent.Publish("career.profile.deleted", map[string]string{"profile_id": profileID})
}
