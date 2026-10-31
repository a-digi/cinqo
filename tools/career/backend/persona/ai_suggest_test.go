package persona

import (
	"database/sql"
	"encoding/json"

	"career-tool-backend/db"
	"errors"
	"testing"
)

func suggestion(name string) ProposedPersona {
	return ProposedPersona{Name: name, Skills: []string{"Go"}}
}

func TestAddSuggestion_StoresAndTheRunIsFinishedFromThem(t *testing.T) {
	run := mustRun(t, setupRunsDB(t))
	a, err := AddSuggestion(run, suggestion("Backend Engineer"))
	if err != nil || a == "" {
		t.Fatalf("first: %q %v", a, err)
	}
	b, err := AddSuggestion(run, suggestion("Tech Lead"))
	if err != nil || b == a {
		t.Fatalf("second: %q %v", b, err)
	}
	if err := FinishAiRun(run, runReady, nil, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := ListAiRuns(mustProfileOf(t, run))
	if err != nil || len(runs) != 1 || runs[0].Proposal == nil {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	var prop Proposal
	if err := json.Unmarshal(runs[0].Proposal, &prop); err != nil {
		t.Fatal(err)
	}
	got := prop.Personas
	if len(got) != 2 || got[0].Name != "Backend Engineer" || got[1].Name != "Tech Lead" {
		t.Fatalf("proposal personas: %+v", got)
	}
}

func TestAddSuggestion_NoSuggestionsMeansAnEmptyProposal(t *testing.T) {
	pid := setupRunsDB(t)
	run := mustRun(t, pid)
	if err := FinishAiRun(run, runReady, []byte("null"), ""); err != nil {
		t.Fatal(err)
	}
	runs, _ := ListAiRuns(pid)
	var prop Proposal
	if err := json.Unmarshal(runs[0].Proposal, &prop); err != nil || len(prop.Personas) != 0 {
		t.Fatalf("want an empty proposal, got %s (%v)", runs[0].Proposal, err)
	}
}

func TestAddSuggestion_Refusals(t *testing.T) {
	pid := setupRunsDB(t)
	run := mustRun(t, pid)
	if _, err := AddSuggestion("nope", suggestion("X")); !errors.Is(err, ErrUnknownAiRun) {
		t.Fatalf("unknown run: %v", err)
	}
	if _, err := AddSuggestion(run, ProposedPersona{}); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("no name: %v", err)
	}
	if _, err := AddSuggestion(run, suggestion("Dev")); err != nil {
		t.Fatal(err)
	}
	if _, err := AddSuggestion(run, suggestion(" dev ")); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("same name: %v", err)
	}
	for _, n := range []string{"A", "B"} {
		if _, err := AddSuggestion(run, suggestion(n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddSuggestion(run, suggestion("D")); !errors.Is(err, errTooManySuggested) {
		t.Fatalf("fourth: %v", err)
	}
	if err := FinishAiRun(run, runReady, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AddSuggestion(run, suggestion("E")); !errors.Is(err, ErrRunNotGenerating) {
		t.Fatalf("after finish: %v", err)
	}
}

func mustProfileOf(t *testing.T, run string) string {
	t.Helper()
	var id string
	if err := dbRow(`SELECT profile_id FROM persona_ai_runs WHERE id = ?`, run).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func dbRow(q string, args ...any) *sql.Row { return db.CareerDB.QueryRow(q, args...) }
