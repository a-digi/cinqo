package profile

import "career-tool-backend/domainevent"

// publishProfileCreated announces "career.profile.created" with { profile_id } after a profile was really inserted (step 151). A new
// profile has no personas yet, so on its own it changes no match; the consumer waits for what follows. Best effort like every domain
// event and synchronous: domainevent.Publish's own short timeout bounds it.
func publishProfileCreated(profileID string) {
	domainevent.Publish("career.profile.created", map[string]string{"profile_id": profileID})
}
