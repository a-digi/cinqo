package cvbuilder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/cv/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

// Attaching a CV that already exists to a run (plan 253, step 2): what a run driven by code needs after it made the CV through the wizard's route.

func cvFor(t *testing.T, personaID, media string) CvDocument {
	t.Helper()
	doc, err := InsertCvDocument(personaID, "classic", "Ada - CV", templates.CvData{FullName: "Ada"}, media, "")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAttachCvToRun_TheRunTakesTheCvAndCanThenBeFinishedReady(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	doc := cvFor(t, pid, "m1")
	if err := FinishCvAiRun(run, cvAiReady, ""); !errors.Is(err, ErrCvAiNothingSaved) {
		t.Fatalf("before the attach the run has no CV: %v", err)
	}
	if err := AttachCvToRun(run, doc.ID); err != nil {
		t.Fatal(err)
	}
	if status, cv, _ := runRow(t, run); status != "generating" || cv != doc.ID {
		t.Fatalf("run = %q %q, want generating with the CV", status, cv)
	}
	if err := FinishCvAiRun(run, cvAiReady, ""); err != nil {
		t.Fatalf("finishing ready after the attach: %v", err)
	}
	if status, cv, _ := runRow(t, run); status != "ready" || cv != doc.ID {
		t.Fatalf("run = %q %q", status, cv)
	}
	// the CV itself is untouched
	got, err := GetCvDocument(doc.ID)
	if err != nil || got.Title != "Ada - CV" || got.MediaFileID != "m1" {
		t.Fatalf("CV = %+v %v", got, err)
	}
}

func TestAttachCvToRun_Refusals(t *testing.T) {
	pid := setupCvDB(t)
	profileID := profileOfPersona(t, pid)
	other, _ := persona.CreatePersona(profileID, "Other", "")

	run := newCvRun(t, pid)
	if err := AttachCvToRun("nope", cvFor(t, pid, "m0").ID); !errors.Is(err, ErrUnknownCvAiRun) {
		t.Errorf("unknown run: %v", err)
	}
	if err := AttachCvToRun(run, "nope"); !errors.Is(err, ErrUnknownCvDocument) {
		t.Errorf("unknown CV: %v", err)
	}
	// a CV built from another target role, even one that is also attached to this run's role
	foreign := cvFor(t, other, "m2")
	if err := AttachCvToRun(run, foreign.ID); !errors.Is(err, ErrCvNotOfRun) {
		t.Errorf("another role's CV: %v", err)
	}
	if err := AttachCvToPersona(foreign.ID, pid); err != nil {
		t.Fatal(err)
	}
	if err := AttachCvToRun(run, foreign.ID); !errors.Is(err, ErrCvNotOfRun) {
		t.Errorf("a CV only attached to the role (not built from it): %v", err)
	}
	if _, cv, _ := runRow(t, run); cv != "" {
		t.Errorf("a refused attach left the run with CV %q", cv)
	}

	// a CV that belongs to another run
	mine := cvFor(t, pid, "m3")
	if err := AttachCvToRun(run, mine.ID); err != nil {
		t.Fatal(err)
	}
	if err := FinishCvAiRun(run, cvAiReady, ""); err != nil {
		t.Fatal(err)
	}
	run2 := newCvRun(t, pid)
	if err := AttachCvToRun(run2, mine.ID); !errors.Is(err, ErrCvOfAnotherRun) {
		t.Errorf("a CV of another run: %v", err)
	}
	// the run already has a CV
	run3 := newCvRun(t, other)
	own := cvFor(t, other, "m4")
	if err := AttachCvToRun(run3, own.ID); err != nil {
		t.Fatal(err)
	}
	if err := AttachCvToRun(run3, cvFor(t, other, "m5").ID); !errors.Is(err, ErrCvAiAlreadySaved) {
		t.Errorf("a second CV: %v", err)
	}
	if _, cv, _ := runRow(t, run3); cv != own.ID {
		t.Errorf("the first CV must stay: %q", cv)
	}
	// a finished run (failed)
	if err := FinishCvAiRun(run2, cvAiFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	if err := AttachCvToRun(run2, cvFor(t, pid, "m6").ID); !errors.Is(err, ErrCvAiRunFinished) {
		t.Errorf("a finished run: %v", err)
	}
	if _, cv, _ := runRow(t, run2); cv != "" {
		t.Errorf("a finished run took CV %q", cv)
	}
}

func TestAttachCvToRun_AttachingTheSameCvAgainToTheSameRunIsRefused(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	doc := cvFor(t, pid, "m1")
	if err := AttachCvToRun(run, doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := AttachCvToRun(run, doc.ID); !errors.Is(err, ErrCvAiAlreadySaved) {
		t.Errorf("twice: %v", err)
	}
}

func TestAttachCvToRun_OfTwoCallsAtOnceOnlyOneWins(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	a, b := cvFor(t, pid, "ma"), cvFor(t, pid, "mb")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{a.ID, b.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = AttachCvToRun(run, id)
		}()
	}
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, ErrCvAiAlreadySaved) {
			t.Errorf("the loser got %v", e)
		}
	}
	if wins != 1 {
		t.Fatalf("%d calls won, want 1 (%v)", wins, errs)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_ai_runs WHERE cv_document_id IS NOT NULL`).Scan(&n); err != nil || n != 1 {
		t.Errorf("runs with a CV = %d (%v)", n, err)
	}
}

func TestCvAiRunAttachHandler(t *testing.T) {
	pid := setupCvDB(t)
	other, _ := persona.CreatePersona(profileOfPersona(t, pid), "Other", "")
	run := newCvRun(t, pid)
	mine, foreign := cvFor(t, pid, "m1"), cvFor(t, other, "m2")
	do := func(method, body string) (int, string) {
		rec := httptest.NewRecorder()
		CvAiRunAttachHandler(rec, httptest.NewRequest(method, "/cv-ai-runs/attach", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	for _, c := range []struct {
		name, method, body string
		want               int
	}{
		{"a GET", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"a DELETE", http.MethodDelete, "", http.StatusMethodNotAllowed},
		{"not json", http.MethodPost, "x", http.StatusBadRequest},
		{"no body fields", http.MethodPost, `{}`, http.StatusBadRequest},
		{"no run id", http.MethodPost, `{"cvDocumentId":"` + mine.ID + `"}`, http.StatusBadRequest},
		{"no cv id", http.MethodPost, `{"runId":"` + run + `"}`, http.StatusBadRequest},
		{"unknown run", http.MethodPost, `{"runId":"nope","cvDocumentId":"` + mine.ID + `"}`, http.StatusNotFound},
		{"unknown cv", http.MethodPost, `{"runId":"` + run + `","cvDocumentId":"nope"}`, http.StatusNotFound},
		{"another role's cv", http.MethodPost, `{"runId":"` + run + `","cvDocumentId":"` + foreign.ID + `"}`, http.StatusConflict},
		{"success", http.MethodPost, `{"runId":"` + run + `","cvDocumentId":"` + mine.ID + `"}`, http.StatusNoContent},
		{"again", http.MethodPost, `{"runId":"` + run + `","cvDocumentId":"` + mine.ID + `"}`, http.StatusConflict},
	} {
		if code, text := do(c.method, c.body); code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, code, c.want, text)
		}
	}
	if _, cv, _ := runRow(t, run); cv != mine.ID {
		t.Errorf("run's CV = %q", cv)
	}
	// finishing a run that took its CV by attach works over the existing route too
	rec := httptest.NewRecorder()
	CvAiRunsHandler(rec, httptest.NewRequest(http.MethodPut, "/cv-ai-runs", strings.NewReader(`{"id":"`+run+`","status":"ready"}`)))
	if rec.Code != http.StatusNoContent {
		t.Errorf("finish ready: %d %s", rec.Code, rec.Body.String())
	}
	// a CV that already belongs to another run is a conflict, not a server error
	run2 := newCvRun(t, pid)
	if code, text := do(http.MethodPost, `{"runId":"`+run2+`","cvDocumentId":"`+mine.ID+`"}`); code != http.StatusConflict {
		t.Errorf("a CV of another run: status %d (%s)", code, text)
	}
}
