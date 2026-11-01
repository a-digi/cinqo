package persona

import (
	"database/sql"
	"errors"
	"log"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
)

// profileOfPersona reads the profile a persona belongs to, before the persona is deleted (the row is gone afterwards). ok is false for
// a persona that does not exist (nothing will be deleted, so nothing will be announced). A failed read is logged and also gives ok
// false: announcing the delete is best effort and must never fail it.
func profileOfPersona(personaID string) (profileID string, ok bool) {
	err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		log.Printf("persona deleted event: could not read persona %s: %v", personaID, err)
		return "", false
	}
	return profileID, true
}

// publishPersonaDeleted announces "career.persona.deleted" with { persona_id, profile_id } after a persona row was really deleted
// (step 152). Best effort like every domain event and synchronous.
func publishPersonaDeleted(personaID, profileID string) {
	domainevent.Publish("career.persona.deleted", map[string]string{"persona_id": personaID, "profile_id": profileID})
}
