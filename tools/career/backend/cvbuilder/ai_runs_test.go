package cvbuilder

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

func newCvRun(t *testing.T, personaID string) string {
	t.Helper()
	id, err := CreateCvAiRun(personaID, "conv-1", "for data roles")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func goodData() templates.CvData {
	return templates.CvData{FullName: "Ada Lovelace", Skills: []string{"Go"}}
}

func runRow(t *testing.T, id string) (status, cv, errText string) {
	t.Helper()
	var c, e *string
	if err := db.CareerDB.QueryRow(`SELECT status, cv_document_id, error FROM cv_ai_runs WHERE id = ?`, id).Scan(&status, &c, &e); err != nil {
		t.Fatal(err)
	}
	if c != nil {
		cv = *c
	}
	if e != nil {
		errText = *e
	}
	return
}

func TestCreateCvAiRun_Rules(t *testing.T) {
	pid := setupCvDB(t)
	if _, err := CreateCvAiRun("nope", "c", ""); !errors.Is(err, persona.ErrUnknownPersona) {
		t.Fatalf("unknown persona: %v", err)
	}
	if _, err := CreateCvAiRun(pid, "  ", ""); !errors.Is(err, errInvalidCvAiRun) {
		t.Fatalf("no conversation: %v", err)
	}
	if _, err := CreateCvAiRun(pid, "c", strings.Repeat("x", maxCvAiFocus+1)); !errors.Is(err, errInvalidCvAiRun) {
		t.Fatalf("long focus: %v", err)
	}
	first := newCvRun(t, pid)
	if _, err := CreateCvAiRun(pid, "c2", ""); !errors.Is(err, ErrCvAiRunInProgress) {
		t.Fatalf("second running run: %v", err)
	}
	// Another persona is not blocked.
	other, _ := persona.CreatePersona(profileOfPersona(t, pid), "Other", "")
	if _, err := CreateCvAiRun(other, "c3", ""); err != nil {
		t.Fatalf("other persona: %v", err)
	}
	// A finished run does not block, and a stale one is ended instead of blocking.
	if err := FinishCvAiRun(first, cvAiFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	again := newCvRun(t, pid)
	exec(t, `UPDATE cv_ai_runs SET created_at = datetime('now', '-31 minutes') WHERE id = ?`, again)
	if _, err := CreateCvAiRun(pid, "c4", ""); err != nil {
		t.Fatalf("after a stale run: %v", err)
	}
	if st, _, msg := runRow(t, again); st != "failed" || msg != staleCvAiText {
		t.Fatalf("stale run: %s %q", st, msg)
	}
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestSaveCvAiDocument_SavesOnTheRunsPersonaOnce(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	doc, err := SaveCvAiDocument(run, "classic", "  Ada - Backend  ", goodData(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.PersonaID != pid || doc.Title != "Ada - Backend" || doc.MediaFileID != "media-1" || doc.JobID != "" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.Personas) != 1 || doc.Personas[0].ID != pid || !doc.Personas[0].BuiltFrom {
		t.Fatalf("not linked to the run's persona: %+v", doc.Personas)
	}
	if _, cv, _ := runRow(t, run); cv != doc.ID {
		t.Fatalf("run's cv = %q, want %q", cv, doc.ID)
	}
	if _, err := SaveCvAiDocument(run, "classic", "Second", goodData(), "media-2"); !errors.Is(err, ErrCvAiAlreadySaved) {
		t.Fatalf("second save: %v", err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cv rows = %d (%v), want 1", n, err)
	}
}

func TestSaveCvAiDocument_Refusals(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	cases := []struct {
		name  string
		run   string
		tpl   string
		title string
		data  templates.CvData
		media string
		want  error
	}{
		{"unknown run", "nope", "classic", "T", goodData(), "m", ErrUnknownCvAiRun},
		{"unknown template", run, "no-such-template", "T", goodData(), "m", nil},
		{"blank title", run, "classic", "  ", goodData(), "m", nil},
		{"long title", run, "classic", strings.Repeat("x", maxCvAiTitle+1), goodData(), "m", nil},
		{"no full name", run, "classic", "T", templates.CvData{}, "m", nil},
		{"markup in data", run, "classic", "T", templates.CvData{FullName: "Ada", Summary: "<strong>hi</strong>"}, "m", nil},
		{"raw resource link", run, "classic", "T", goodData(), "file:///x/y.pdf", nil},
		{"leading slash", run, "classic", "T", goodData(), "/x.pdf", nil},
		{"no pdf", run, "classic", "T", goodData(), "", nil},
	}
	for _, c := range cases {
		_, err := SaveCvAiDocument(c.run, c.tpl, c.title, c.data, c.media)
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused save left %d CVs (%v)", n, err)
	}
	if _, cv, _ := runRow(t, run); cv != "" {
		t.Fatalf("a refused save tied a CV to the run: %q", cv)
	}

	// A run that is no longer generating cannot save.
	if err := FinishCvAiRun(run, cvAiFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveCvAiDocument(run, "classic", "T", goodData(), "m"); !errors.Is(err, ErrCvAiRunNotGenerating) {
		t.Fatalf("finished run: %v", err)
	}
}

func TestFinishCvAiRun_Rules(t *testing.T) {
	pid := setupCvDB(t)
	run := newCvRun(t, pid)
	if err := FinishCvAiRun(run, cvAiReady, ""); !errors.Is(err, ErrCvAiNothingSaved) {
		t.Fatalf("ready without a CV: %v", err)
	}
	if st, _, _ := runRow(t, run); st != "generating" {
		t.Fatalf("a refused finish changed the run to %s", st)
	}
	if err := FinishCvAiRun(run, cvAiFailed, "  "); !errors.Is(err, errInvalidCvAiRun) {
		t.Fatalf("failed without an error: %v", err)
	}
	if err := FinishCvAiRun(run, "weird", ""); !errors.Is(err, errInvalidCvAiRun) {
		t.Fatalf("bad status: %v", err)
	}
	if _, err := SaveCvAiDocument(run, "classic", "T", goodData(), "m"); err != nil {
		t.Fatal(err)
	}
	if err := FinishCvAiRun(run, cvAiReady, "ignored"); err != nil {
		t.Fatal(err)
	}
	if st, _, msg := runRow(t, run); st != "ready" || msg != "" {
		t.Fatalf("ready run: %s %q", st, msg)
	}
	if err := FinishCvAiRun(run, cvAiFailed, "late"); !errors.Is(err, ErrCvAiRunFinished) {
		t.Fatalf("finish twice: %v", err)
	}
	if err := FinishCvAiRun("nope", cvAiFailed, "x"); !errors.Is(err, ErrUnknownCvAiRun) {
		t.Fatalf("unknown run: %v", err)
	}
}

func TestCvAiRuns_ListDeleteAndCascades(t *testing.T) {
	pid := setupCvDB(t)
	first := newCvRun(t, pid)
	doc, err := SaveCvAiDocument(first, "classic", "T", goodData(), "m")
	if err != nil {
		t.Fatal(err)
	}
	if err := FinishCvAiRun(first, cvAiReady, ""); err != nil {
		t.Fatal(err)
	}
	second := newCvRun(t, pid)
	runs, err := ListCvAiRuns(pid)
	if err != nil || len(runs) != 2 || runs[0].ID != second || runs[1].ID != first || runs[1].CvDocumentID != doc.ID {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	if other, _ := persona.CreatePersona(profileOfPersona(t, pid), "Other", ""); true {
		if l, _ := ListCvAiRuns(other); len(l) != 0 {
			t.Fatalf("another persona lists %d runs", len(l))
		}
	}

	// Deleting the CV keeps the run (without the CV); deleting a run keeps the CV.
	if _, err := DeleteCvDocument(doc.ID); err != nil {
		t.Fatal(err)
	}
	if st, cv, _ := runRow(t, first); st != "ready" || cv != "" {
		t.Fatalf("run after deleting its CV: %s %q", st, cv)
	}
	doc2, _ := InsertCvDocument(pid, "classic", "Keep", goodData(), "m2", "")
	for i := 0; i < 2; i++ {
		if err := DeleteCvAiRun(first); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
	}
	if _, err := GetCvDocument(doc2.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting the persona deletes its runs.
	if _, err := db.CareerDB.Exec(`DELETE FROM personas WHERE id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_ai_runs`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("runs left after deleting the persona: %d (%v)", n, err)
	}
}

func TestCvAiRunsHandler(t *testing.T) {
	pid := setupCvDB(t)
	do := func(method, target, body string) (int, string) {
		rec := httptest.NewRecorder()
		CvAiRunsHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	post := func(p, c string) (int, string) {
		return do(http.MethodPost, "/cv-ai-runs", fmt.Sprintf(`{"personaId":%q,"conversationId":%q,"focus":"x"}`, p, c))
	}
	if code, _ := post("", ""); code != 400 {
		t.Errorf("empty create: %d", code)
	}
	if code, _ := post("nope", "c"); code != 404 {
		t.Errorf("unknown persona: %d", code)
	}
	code, body := post(pid, "c")
	if code != 200 || !strings.Contains(body, `"id"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, _ := post(pid, "c2"); code != 409 {
		t.Errorf("second running: %d", code)
	}
	id := strings.Split(strings.Split(body, `"id":"`)[1], `"`)[0]
	if code, _ := do(http.MethodPut, "/cv-ai-runs", fmt.Sprintf(`{"id":%q,"status":"ready"}`, id)); code != 400 {
		t.Errorf("ready without a CV: %d", code)
	}
	if code, _ := do(http.MethodPut, "/cv-ai-runs", fmt.Sprintf(`{"id":%q,"status":"failed","error":"boom"}`, id)); code != 204 {
		t.Errorf("finish failed: %d", code)
	}
	if code, _ := do(http.MethodPut, "/cv-ai-runs", fmt.Sprintf(`{"id":%q,"status":"failed","error":"boom"}`, id)); code != 409 {
		t.Errorf("finish again: %d", code)
	}
	if code, _ := do(http.MethodPut, "/cv-ai-runs", `{"id":"nope","status":"failed","error":"x"}`); code != 404 {
		t.Errorf("finish unknown: %d", code)
	}
	if code, b := do(http.MethodGet, "/cv-ai-runs?personaId="+pid, ""); code != 200 || !strings.Contains(b, `"status":"failed"`) {
		t.Errorf("list: %d %s", code, b)
	}
	if code, _ := do(http.MethodGet, "/cv-ai-runs", ""); code != 400 {
		t.Errorf("list without persona: %d", code)
	}
	if code, _ := do(http.MethodGet, "/cv-ai-runs?personaId=nope", ""); code != 404 {
		t.Errorf("list unknown persona: %d", code)
	}
	if code, _ := do(http.MethodDelete, "/cv-ai-runs?id="+id, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ := do(http.MethodDelete, "/cv-ai-runs", ""); code != 400 {
		t.Errorf("delete without id: %d", code)
	}
	if code, _ := do(http.MethodPatch, "/cv-ai-runs", ""); code != 405 {
		t.Errorf("wrong method: %d", code)
	}
}
