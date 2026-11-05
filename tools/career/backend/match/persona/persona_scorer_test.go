package personamatching

import (
	"career-tool-backend/match"
	"errors"
	"testing"

	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// seedPersonaIn adds another persona with the given skills to an existing profile.
func seedPersonaIn(t *testing.T, profileID, name string, skills []string) string {
	t.Helper()
	personaID, err := persona.CreatePersona(profileID, name, "")
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
	return personaID
}

// Every persona of every profile gets its own row with its own profile's id; the best persona's score of a profile is
// the score match.RunManualMatch gives that profile, and nothing is stored by the scorer.
func TestPersonaScorerScoresEveryPersonaOfEveryProfile(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	fit, fitPersona := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	weak := seedPersonaIn(t, fit, "Weak second persona", []string{"Cobol", "Fortran"})
	other, otherPersona := seedProfile(t, "Other", []string{"Cobol", "Fortran"})

	s, err := NewPersonaScorer()
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasPersonas() {
		t.Fatal("HasPersonas must be true")
	}
	rows, err := s.Rows("j1")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows = %v (%v), want 3", rows, err)
	}
	byPersona := map[string]int{}
	for _, r := range rows {
		byPersona[r.PersonaID] = r.Score
		wantProfile := map[string]string{fitPersona: fit, weak: fit, otherPersona: other}[r.PersonaID]
		if r.ProfileID != wantProfile || r.EngineVersion == "" {
			t.Errorf("row %+v: profile should be %q and the engine version stamped", r, wantProfile)
		}
	}
	if byPersona[fitPersona] <= byPersona[weak] || byPersona[fitPersona] < 60 || byPersona[weak] >= 60 {
		t.Errorf("scores %v: the fitting persona must score clearly above the weak one", byPersona)
	}
	for profileID, ids := range map[string][]string{fit: {fitPersona, weak}, other: {otherPersona}} {
		best := -1
		for _, id := range ids {
			best = max(best, byPersona[id])
		}
		res, err := match.RunManualMatch("j1", profileID)
		if err != nil || res.Score != best {
			t.Errorf("profile %s: best persona %d, match.RunManualMatch %d (%v)", profileID, best, res.Score, err)
		}
	}
}

func TestPersonaScorerStoresNothing(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	seedProfile(t, "Fit", []string{"Go", "Docker"})
	s, _ := NewPersonaScorer()
	if _, err := s.Rows("j1"); err != nil {
		t.Fatal(err)
	}
	if countRows(t, "job_persona_matches") != 0 || countRows(t, "job_matches") != 0 {
		t.Error("the scorer wrote something")
	}
}

func TestPersonaScorerLeavesOutAPersonaThatCannotJudge(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	empty, err := profile.CreateProfile("Empty", "Persona")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persona.CreatePersona(empty, "bare", ""); err != nil { // no skills, no titles
		t.Fatal(err)
	}
	s, _ := NewPersonaScorer()
	rows, err := s.Rows("j1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v (%v), want only the persona that can judge", rows, err)
	}
}

// With no persona at all there is nothing to score, the job is not even read, and the corpus is not loaded.
func TestPersonaScorerWithoutPersonasAnswersNothing(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	if _, err := profile.CreateProfile("No", "Personas"); err != nil {
		t.Fatal(err)
	}
	s, err := NewPersonaScorer()
	if err != nil {
		t.Fatal(err)
	}
	if s.HasPersonas() {
		t.Error("HasPersonas must be false")
	}
	for _, id := range []string{"j1", "does-not-exist"} {
		if rows, err := s.Rows(id); err != nil || len(rows) != 0 {
			t.Errorf("%s: %v %v", id, rows, err)
		}
	}
}

func TestPersonaScorerUnknownJob(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", []string{"Go"})
	s, _ := NewPersonaScorer()
	if _, err := s.Rows("nope"); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("err = %v", err)
	}
}
