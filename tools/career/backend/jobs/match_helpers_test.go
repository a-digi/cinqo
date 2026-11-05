package jobs

import (
	"testing"

	"career-tool-backend/db"
)

// Fixtures of the job-purge tests (orphans_test.go) that were written with the match tests; the match package keeps its own copies.

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func zombieJob(t *testing.T, id, profileID, personaID, kind, status string) {
	t.Helper()
	addMatchJob(t, id)
	var persona any
	if personaID != "" {
		persona = personaID
	}
	exec(t, `INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, kind) VALUES (?, ?, ?, 50, ?, ?)`, id, profileID, persona, status, kind)
	exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES (?, 'Go')`, id)
}

func personaRows(t *testing.T, jobID, personaID, profileID string) {
	t.Helper()
	exec(t, `INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, jobID, personaID, profileID)
}

func addMatchJob(t *testing.T, id string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		id, "https://x.example/"+id, matchJobDesc)
}

func str(s string) *string { return &s }

const matchJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
