// Package cvbuilder is the CV Builder feature's own HTTP surface — a
// deterministic (no AI turn), manual "pick a persona, pick a design,
// get a PDF" pipeline, distinct from jobs/cv_pdf.go's own AI-tailored
// per-job CV flow. See plan/ai/career/cv-builder/step-01-overview-and-data-model.md.
package cvbuilder

import (
	"database/sql"
	"strings"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

// AssembleCvData reads personaID's own current data (its owning
// profile's name and external links, plus the persona's own headline/
// summary/location/skills/experience) into the DEFAULT CvData a new
// CV starts from. This is only ever a starting point — per step 1's
// own addendum, the builder's own sidebar lets the user edit this
// freely before generating, and none of that editing ever calls back
// into this package's own read functions or writes to Persona's own
// tables. No exported single-row getter exists for a Profile-by-id or
// a Persona-by-id anywhere in this codebase (confirmed by reading
// profile.go/persona.go directly — both only expose list functions),
// so the two small lookups below are plain, direct queries, not a
// re-derivation of something already available.
func AssembleCvData(personaID string) (templates.CvData, error) {
	var data templates.CvData

	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return data, persona.ErrUnknownPersona
	case err != nil:
		return data, err
	}

	var firstName, lastName string
	if err := db.CareerDB.QueryRow(`SELECT first_name, last_name FROM profiles WHERE id = ?`, profileID).Scan(&firstName, &lastName); err != nil {
		return data, err
	}
	data.FullName = strings.TrimSpace(firstName + " " + lastName)

	linkRows, err := db.CareerDB.Query(`SELECT platform, url FROM profile_external_links WHERE profile_id = ? ORDER BY platform ASC`, profileID)
	if err != nil {
		return data, err
	}
	defer linkRows.Close()
	data.ExternalLinks = []templates.ExternalLink{}
	for linkRows.Next() {
		var link templates.ExternalLink
		if err := linkRows.Scan(&link.Platform, &link.URL); err != nil {
			return data, err
		}
		data.ExternalLinks = append(data.ExternalLinks, link)
	}
	if err := linkRows.Err(); err != nil {
		return data, err
	}

	// FetchPersonaDetails already reads persona_details/career_skills/
	// career_experience in one call — reused as-is rather than
	// re-querying those three tables from scratch. Its own result type
	// is unexported, but its fields are exported, so accessing them
	// here (without ever naming the type) is legal Go.
	details, err := persona.FetchPersonaDetails(personaID)
	if err != nil {
		return data, err
	}
	if details.PersonaDetails != nil {
		data.Headline = details.PersonaDetails.Headline
		data.Summary = details.PersonaDetails.Summary
		data.Location = details.PersonaDetails.Location
	}
	data.Skills = details.Skills
	data.Experience = make([]templates.ExperienceEntry, 0, len(details.Experience))
	for _, e := range details.Experience {
		data.Experience = append(data.Experience, templates.ExperienceEntry{
			Company:     e.Company,
			Title:       e.Title,
			StartDate:   e.StartDate,
			EndDate:     e.EndDate,
			Description: e.Description,
		})
	}

	return data, nil
}
