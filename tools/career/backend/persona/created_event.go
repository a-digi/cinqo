package persona

import "career-tool-backend/domainevent"

// publishPersonaCreated announces "career.persona.created" with { persona_id, profile_id } after a persona was really inserted (step
// 151): by CreatePersona (the HTTP route and the AI tool) and by the AI's persona proposals (ApplyDecisions, after its transaction
// committed). It is a separate, explicit fact next to "career.persona.changed" with change "created", which stays as it is (the
// persona-score consumer of step 142 depends on it). Best effort like every domain event and synchronous.
func publishPersonaCreated(personaID, profileID string) {
	domainevent.Publish("career.persona.created", map[string]string{"persona_id": personaID, "profile_id": profileID})
}
