package personamatch_test

import (
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/persona/personamatch"
	"career-tool-backend/profile"
)

func setup(t *testing.T) {
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

func rowsOf(t *testing.T, where string, arg string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_persona_matches WHERE `+where+` = ?`, arg).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Deleting a persona removes that persona's scores, and only that persona's.
func TestDeletingAPersonaRemovesItsJobScores(t *testing.T) {
	setup(t)
	prof, err := profile.CreateProfile("Ana", "Test")
	if err != nil {
		t.Fatal(err)
	}
	p1, _ := persona.CreatePersona(prof, "One", "")
	p2, _ := persona.CreatePersona(prof, "Two", "")
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://x.example/1', 'Job')`); err != nil {
		t.Fatal(err)
	}
	if err := personamatch.Replace("j1", []personamatch.Row{{PersonaID: p1, ProfileID: prof, Score: 70}, {PersonaID: p2, ProfileID: prof, Score: 40}}); err != nil {
		t.Fatal(err)
	}
	if err := persona.DeletePersona(p1); err != nil {
		t.Fatal(err)
	}
	if rowsOf(t, "persona_id", p1) != 0 || rowsOf(t, "persona_id", p2) != 1 {
		t.Errorf("rows of the deleted persona %d, of the other %d, want 0 and 1", rowsOf(t, "persona_id", p1), rowsOf(t, "persona_id", p2))
	}
}

// Deleting an unknown persona touches nothing.
func TestDeletingAnUnknownPersonaRemovesNothing(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://x.example/1', 'Job')`); err != nil {
		t.Fatal(err)
	}
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: "p-real", ProfileID: "A", Score: 70}})
	if err := persona.DeletePersona("nope"); err != nil {
		t.Fatal(err)
	}
	if rowsOf(t, "persona_id", "p-real") != 1 {
		t.Error("another persona's row was removed")
	}
}

// Deleting a profile removes the scores of all its personas (the personas go with the profile), and not another profile's.
func TestDeletingAProfileRemovesTheJobScoresOfItsPersonas(t *testing.T) {
	setup(t)
	a, _ := profile.CreateProfile("Ana", "Test")
	b, _ := profile.CreateProfile("Ben", "Test")
	pa1, _ := persona.CreatePersona(a, "A1", "")
	pa2, _ := persona.CreatePersona(a, "A2", "")
	pb, _ := persona.CreatePersona(b, "B1", "")
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://x.example/1', 'Job')`); err != nil {
		t.Fatal(err)
	}
	if err := personamatch.Replace("j1", []personamatch.Row{
		{PersonaID: pa1, ProfileID: a, Score: 70}, {PersonaID: pa2, ProfileID: a, Score: 60}, {PersonaID: pb, ProfileID: b, Score: 50},
	}); err != nil {
		t.Fatal(err)
	}
	if err := profile.DeleteProfile(a); err != nil {
		t.Fatal(err)
	}
	if rowsOf(t, "profile_id", a) != 0 {
		t.Errorf("rows left for the deleted profile: %d", rowsOf(t, "profile_id", a))
	}
	if rowsOf(t, "profile_id", b) != 1 {
		t.Errorf("the other profile's rows = %d, want 1", rowsOf(t, "profile_id", b))
	}
}

// Deleting a persona or a profile through the real functions also removes the score state.
func TestDeletingAPersonaOrProfileRemovesTheScoreState(t *testing.T) {
	setup(t)
	a, _ := profile.CreateProfile("Ana", "Test")
	b, _ := profile.CreateProfile("Ben", "Test")
	pa1, _ := persona.CreatePersona(a, "A1", "")
	pa2, _ := persona.CreatePersona(a, "A2", "")
	pb, _ := persona.CreatePersona(b, "B1", "")
	for id, prof := range map[string]string{pa1: a, pa2: a, pb: b} {
		if err := personamatch.SetState(id, prof, "fp"); err != nil {
			t.Fatal(err)
		}
	}
	if err := persona.DeletePersona(pa1); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := personamatch.GetState(pa1); ok {
		t.Error("the deleted persona's state is still there")
	}
	if _, ok, _ := personamatch.GetState(pa2); !ok {
		t.Error("another persona's state was removed")
	}
	if err := profile.DeleteProfile(a); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := personamatch.GetState(pa2); ok {
		t.Error("the deleted profile's persona state is still there")
	}
	if _, ok, _ := personamatch.GetState(pb); !ok {
		t.Error("the other profile's state was removed")
	}
}
