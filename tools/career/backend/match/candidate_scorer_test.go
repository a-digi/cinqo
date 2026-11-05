package match

import (
	"slices"
	"testing"

	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

const salesDesc = "Wir suchen einen Vertriebsleiter für den Außendienst. Du führst Kundengespräche, betreust Key Accounts und verkaufst " +
	"Lizenzen an Einkäufer. Verkaufserfahrung von mindestens 5 Jahren ist erforderlich."

// For the same text, scoring a candidate gives exactly the numbers the saved job gets (the Jobs page's numbers), profile for profile.
func TestScoreCandidateIsTheNumberTheSavedJobGets(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Scores("j1")
	if err != nil || len(saved) != 2 {
		t.Fatalf("saved %v %v", saved, err)
	}
	candidate, err := s.ScoreCandidate(jobs.Candidate{Title: "Backend Engineer (m/w/d)", Company: "Acme", Location: "Hamburg", Description: matchJobDesc})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved, candidate) {
		t.Errorf("candidate %v, saved job %v", candidate, saved)
	}
}

func TestScoreCandidateTellsAFittingJobFromAnUnfittingOne(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	fit, err := s.ScoreCandidate(jobs.Candidate{Title: "Backend Engineer (m/w/d)", Company: "Acme", Location: "Hamburg", Description: matchJobDesc})
	if err != nil || len(fit) != 1 {
		t.Fatalf("%v %v", fit, err)
	}
	unfit, err := s.ScoreCandidate(jobs.Candidate{Title: "Sales Manager", Company: "Acme", Location: "Munich", Description: salesDesc})
	if err != nil || len(unfit) != 1 {
		t.Fatalf("%v %v", unfit, err)
	}
	if fit[0] < 60 || unfit[0] >= 30 || unfit[0] >= fit[0] {
		t.Errorf("fit %d, unfit %d: expected the fitting job well above 60 and the unfitting one under 30", fit[0], unfit[0])
	}
}

// Like the saved job's scoring: a profile without personas, and one whose persona has nothing to judge, are left out; with none
// that can judge the answer is empty; nothing is stored.
func TestScoreCandidateLeavesOutProfilesThatCannotJudgeAndStoresNothing(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	if _, err := profile.CreateProfile("No", "Personas"); err != nil {
		t.Fatal(err)
	}
	empty, err := profile.CreateProfile("Empty", "Persona")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persona.CreatePersona(empty, "bare", ""); err != nil {
		t.Fatal(err)
	}
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	scores, err := s.ScoreCandidate(jobs.Candidate{Title: "Backend Engineer", Description: matchJobDesc})
	if err != nil || len(scores) != 1 {
		t.Fatalf("%v %v (want one: only the first profile can judge)", scores, err)
	}
	if countRows(t, "jobs") != 0 || countRows(t, "job_matches") != 0 || countRows(t, "job_persona_matches") != 0 {
		t.Error("scoring a candidate must store nothing")
	}
}

func TestScoreCandidateWithoutProfilesAnswersNothing(t *testing.T) {
	setupJobsDB(t)
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	if scores, err := s.ScoreCandidate(jobs.Candidate{Title: "Backend Engineer", Description: matchJobDesc}); err != nil || len(scores) != 0 {
		t.Errorf("%v %v", scores, err)
	}
}

// A candidate with almost nothing in it still scores or is left out, never an error.
func TestScoreCandidateWithAnEmptyCandidateIsNotAnError(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", []string{"Go"})
	s, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScoreCandidate(jobs.Candidate{}); err != nil {
		t.Errorf("err %v", err)
	}
	if _, err := s.ScoreCandidate(jobs.Candidate{Title: "x"}); err != nil {
		t.Errorf("err %v", err)
	}
}
