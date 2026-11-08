package persona

import (
	"career-tool-backend/db"
	"career-tool-backend/domainevent"
)

// The kinds of change "career.persona.changed" announces (step 142).
const (
	ChangeCreated           = "created"
	ChangeDetails           = "details"
	ChangeSkillAdded        = "skill_added"
	ChangeSkillRemoved      = "skill_removed"
	ChangeExperienceAdded   = "experience_added"
	ChangeExperienceUpdated = "experience_updated"
	ChangeExperienceRemoved = "experience_removed"
)

// publishPersonaChanged announces "career.persona.changed" with { persona_id, profile_id, change } after a persona's
// scored data really changed (step 142): its details, a skill, an experience entry, or the persona itself coming into
// being. A change that changed nothing, a name or description edit (the matcher does not read them) and a delete send
// nothing; callers decide that and call this only after the write succeeded. Best effort like every domain event and
// synchronous: domainevent.Publish's own short timeout bounds it. A persona that cannot be found sends nothing.
func publishPersonaChanged(personaID, change string) {
	var profileID string
	if err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID); err != nil {
		return
	}
	domainevent.Publish("career.persona.changed", map[string]string{"persona_id": personaID, "profile_id": profileID, "change": change})
}

// PublishChanged is publishPersonaChanged for the profileitems package (plan 173): a change of the profile library or of what a persona
// uses changes what the persona shows, so the matcher must hear of it too. Same rules: after the write succeeded, only for a
// change that really changed something.
func PublishChanged(personaID, change string) { publishPersonaChanged(personaID, change) }
