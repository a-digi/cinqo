package personamatching

import (
	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"testing"
)

// Small fixtures and helpers the persona-match tests share with the tests that stayed in package match; each package keeps its own copy.

func setupJobsDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func addMatchJob(t *testing.T, id string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		id, "https://x.example/"+id, matchJobDesc)
}

// seedProfile makes a profile with one persona (skills, details, experience) and returns both ids.
func seedProfile(t *testing.T, name string, skills []string) (profileID, personaID string) {
	t.Helper()
	profileID, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err = persona.CreatePersona(profileID, name+" persona", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{
		PersonaID: personaID, Headline: str("Backend Engineer"), DesiredTitles: str("Software Engineer, Backend Developer"),
		Location: str("Hamburg"), DesiredLocations: str("Hamburg, Remote"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if err := persona.AddCareerSkill(personaID, s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := persona.AddCareerExperience(personaID, "Acme", "Backend Developer", "2019-03", "", "Go services with Docker"); err != nil {
		t.Fatal(err)
	}
	return profileID, personaID
}

func exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

const matchJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

func str(s string) *string { return &s }

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
