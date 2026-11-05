package match

import (
	"errors"
	"slices"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The scorer gives each profile's best score, the same number RunManualMatch stores for that profile, and writes
// nothing itself.
func TestProfileScorerMatchesTheManualRunAndStoresNothing(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	fit, _ := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other, _ := seedProfile(t, "Other", []string{"Cobol", "Fortran"})

	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	scores, err := s.Scores("j1")
	if err != nil || len(scores) != 2 {
		t.Fatalf("scores %v, %v", scores, err)
	}
	if countRows(t, "job_matches") != 0 {
		t.Fatal("scoring stored a match")
	}
	// the order of two profiles created in the same second follows their ids, so compare as sets
	var manual []int
	for _, id := range []string{fit, other} {
		res, err := RunManualMatch("j1", id)
		if err != nil {
			t.Fatal(err)
		}
		manual = append(manual, res.Score)
	}
	slices.Sort(manual)
	slices.Sort(scores)
	if !slices.Equal(manual, scores) {
		t.Errorf("scorer %v, manual runs %v", scores, manual)
	}
	if scores[1] <= scores[0] || scores[1] < 60 || scores[0] >= 60 {
		t.Errorf("scores %v: expected one profile clearly above 60 and one below", scores)
	}
}

// A profile without personas is left out; a persona with nothing to judge makes its profile "unable", also left out.
func TestProfileScorerLeavesOutProfilesThatCannotJudge(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	if _, err := profile.CreateProfile("No", "Personas"); err != nil {
		t.Fatal(err)
	}
	empty, err := profile.CreateProfile("Empty", "Persona")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persona.CreatePersona(empty, "bare", ""); err != nil { // no skills, no titles
		t.Fatal(err)
	}
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	scores, err := s.Scores("j1")
	if err != nil || len(scores) != 1 {
		t.Fatalf("scores %v, %v (want one: only the first profile can judge)", scores, err)
	}
}

func TestProfileScorerWithoutProfilesAnswersNothing(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	if scores, err := s.Scores("j1"); err != nil || len(scores) != 0 {
		t.Errorf("%v, %v", scores, err)
	}
}

func TestProfileScorerUnknownJob(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", []string{"Go"})
	s, _ := NewProfileScorer()
	if _, err := s.Scores("nope"); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("err = %v", err)
	}
}

// ScoreByProfile gives each profile's own score by id, the same numbers Scores gives.
func TestScoreByProfileKeysEachScoreByItsProfile(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	fit, _ := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other, _ := seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	by, err := s.ScoreByProfile("j1")
	if err != nil || len(by) != 2 {
		t.Fatalf("%v, %v", by, err)
	}
	if by[fit] < 60 || by[other] >= 60 || by[fit] <= by[other] {
		t.Errorf("fit=%d other=%d: the fitting profile must score clearly higher", by[fit], by[other])
	}
	for _, id := range []string{fit, other} {
		res, err := RunManualMatch("j1", id)
		if err != nil || res.Score != by[id] {
			t.Errorf("profile %s: manual run %d (%v), ScoreByProfile %d", id, res.Score, err, by[id])
		}
	}
	if _, err := s.ScoreByProfile("nope"); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("unknown job = %v", err)
	}
}

// NewProfileScorerFor reads only the listed profiles.
func TestNewProfileScorerForScoresOnlyTheListedProfiles(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	fit, _ := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other, _ := seedProfile(t, "Other", []string{"Cobol", "Fortran"})

	s, err := NewProfileScorerFor(map[string]bool{fit: true, "ghost": true})
	if err != nil {
		t.Fatal(err)
	}
	by, err := s.ScoreByProfile("j1")
	if err != nil || len(by) != 1 {
		t.Fatalf("%v, %v", by, err)
	}
	if _, ok := by[fit]; !ok {
		t.Error("the listed profile must be scored")
	}
	if _, ok := by[other]; ok {
		t.Error("an unlisted profile must not be scored")
	}
	all, _ := NewProfileScorerFor(nil)
	if byAll, _ := all.ScoreByProfile("j1"); len(byAll) != 2 {
		t.Errorf("nil means every profile, got %d", len(byAll))
	}
	empty, _ := NewProfileScorerFor(map[string]bool{})
	if byNone, _ := empty.ScoreByProfile("j1"); len(byNone) != 0 {
		t.Errorf("an empty set scores nobody, got %d", len(byNone))
	}
}
