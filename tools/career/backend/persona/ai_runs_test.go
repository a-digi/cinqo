package persona

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

func setupRunsDB(t *testing.T) (profileID string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	id, err := profile.CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustRun(t *testing.T, profileID string) string {
	t.Helper()
	id, err := CreateAiRun(profileID, "conv-1", "backend and management")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func statusOf(t *testing.T, id string) (status, errText string) {
	t.Helper()
	var e *string
	if err := db.CareerDB.QueryRow(`SELECT status, error FROM persona_ai_runs WHERE id = ?`, id).Scan(&status, &e); err != nil {
		t.Fatal(err)
	}
	if e != nil {
		errText = *e
	}
	return
}

func TestCreateAiRun_Rules(t *testing.T) {
	pid := setupRunsDB(t)
	if _, err := CreateAiRun("nope", "c", ""); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Fatalf("unknown profile: %v", err)
	}
	if _, err := CreateAiRun(pid, "  ", ""); err == nil {
		t.Fatal("a blank conversation id was accepted")
	}
	if _, err := CreateAiRun(pid, "c", strings.Repeat("x", maxFocus+1)); err == nil {
		t.Fatal("a too long focus was accepted")
	}
	id, err := CreateAiRun(pid, "conv-9", "  one for management  ")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := ListAiRuns(pid)
	if err != nil || len(runs) != 1 || runs[0].ID != id || runs[0].Focus != "one for management" ||
		runs[0].Status != "generating" || runs[0].ConversationID != "conv-9" || runs[0].Personas == nil || runs[0].Proposal != nil {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
}

func TestFinishAiRun_ReadyStoresTheNormalizedProposal(t *testing.T) {
	pid := setupRunsDB(t)
	id := mustRun(t, pid)
	if err := FinishAiRun(id, "ready", []byte(personasJSON(okPersona)), "ignored"); err != nil {
		t.Fatal(err)
	}
	runs, _ := ListAiRuns(pid)
	if runs[0].Status != "ready" || runs[0].Error != "" || runs[0].FinishedAt == "" {
		t.Fatalf("run = %+v", runs[0])
	}
	var p Proposal
	if err := json.Unmarshal(runs[0].Proposal, &p); err != nil || len(p.Personas) != 1 || p.Personas[0].Name != "Backend" {
		t.Fatalf("proposal = %s err=%v", runs[0].Proposal, err)
	}
}

func TestFinishAiRun_Rules(t *testing.T) {
	pid := setupRunsDB(t)

	// An invalid proposal is refused and the run stays generating (it can still be finished).
	id := mustRun(t, pid)
	if err := FinishAiRun(id, "ready", []byte(`{"personas":null}`), ""); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("invalid proposal: %v", err)
	}
	if s, _ := statusOf(t, id); s != "generating" {
		t.Fatalf("status = %s after a refused finish", s)
	}
	// Failed needs an error; a long one is cut.
	if err := FinishAiRun(id, "failed", nil, "  "); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("failed without error: %v", err)
	}
	if err := FinishAiRun(id, "failed", nil, strings.Repeat("e", maxRunError+50)); err != nil {
		t.Fatal(err)
	}
	if s, e := statusOf(t, id); s != "failed" || len([]rune(e)) != maxRunError {
		t.Fatalf("status=%s error length=%d", s, len([]rune(e)))
	}
	// Finished is final: both ways, and the proposal is not kept for a failed run.
	if err := FinishAiRun(id, "ready", []byte(personasJSON()), ""); !errors.Is(err, ErrAiRunFinished) {
		t.Fatalf("second finish: %v", err)
	}
	if err := FinishAiRun("nope", "failed", nil, "x"); !errors.Is(err, ErrUnknownAiRun) {
		t.Fatalf("unknown run: %v", err)
	}
	if err := FinishAiRun(id, "generating", nil, ""); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("bad status: %v", err)
	}
}

func TestListAiRuns_NewestFirst_WithTheirPersonas_OnlyThisProfile(t *testing.T) {
	pid := setupRunsDB(t)
	other, _ := profile.CreateProfile("Alan", "Turing")
	exec(t, `INSERT INTO persona_ai_runs (id, profile_id, conversation_id, created_at) VALUES
		('r1', ?, 'c1', '2026-01-01 10:00:00'), ('r2', ?, 'c2', '2026-01-02 10:00:00'), ('rx', ?, 'cx', '2026-01-03 10:00:00')`, pid, pid, other)
	exec(t, `INSERT INTO personas (id, profile_id, name, created_at, ai_run_id) VALUES
		('p1', ?, 'First', '2026-01-02 11:00:00', 'r2'), ('p2', ?, 'Second', '2026-01-02 12:00:00', 'r2'), ('p3', ?, 'Manual', '2026-01-02 13:00:00', NULL)`, pid, pid, pid)
	// A stale 'generating' run would be ended by the list; keep these fresh by finishing them.
	exec(t, `UPDATE persona_ai_runs SET status = 'failed', error = 'x'`)

	runs, err := ListAiRuns(pid)
	if err != nil || len(runs) != 2 || runs[0].ID != "r2" || runs[1].ID != "r1" {
		t.Fatalf("runs = %+v err=%v", runs, err)
	}
	if len(runs[0].Personas) != 2 || runs[0].Personas[0].Name != "First" || runs[0].Personas[1].Name != "Second" || len(runs[1].Personas) != 0 {
		t.Fatalf("personas = %+v / %+v", runs[0].Personas, runs[1].Personas)
	}
}

func TestDeleteAiRun_KeepsThePersonas_AndIsIdempotent(t *testing.T) {
	pid := setupRunsDB(t)
	id := mustRun(t, pid)
	exec(t, `INSERT INTO personas (id, profile_id, name, ai_run_id) VALUES ('p1', ?, 'Kept', ?)`, pid, id)
	for i := 0; i < 2; i++ {
		if err := DeleteAiRun(id); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	var n int
	var aiRun *string
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM persona_ai_runs`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("runs left: %d err=%v", n, err)
	}
	if err := db.CareerDB.QueryRow(`SELECT ai_run_id FROM personas WHERE id = 'p1'`).Scan(&aiRun); err != nil || aiRun != nil {
		t.Fatalf("persona ai_run_id = %v err=%v, want NULL (persona kept)", aiRun, err)
	}
}

func TestDeleteProfile_CascadesItsRuns(t *testing.T) {
	pid := setupRunsDB(t)
	mustRun(t, pid)
	if err := profile.DeleteProfile(pid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM persona_ai_runs`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("runs left: %d err=%v", n, err)
	}
}

func TestReconcileStaleAiRuns(t *testing.T) {
	pid := setupRunsDB(t)
	exec(t, `INSERT INTO persona_ai_runs (id, profile_id, conversation_id, status, created_at) VALUES
		('old', ?, 'c', 'generating', datetime('now', '-31 minutes')),
		('fresh', ?, 'c', 'generating', datetime('now', '-5 minutes')),
		('done', ?, 'c', 'ready', datetime('now', '-3 hours'))`, pid, pid, pid)
	exec(t, `UPDATE persona_ai_runs SET proposal_json = '{"personas":[],"note":""}' WHERE id = 'done'`)

	n, err := ReconcileStaleAiRuns()
	if err != nil || n != 1 {
		t.Fatalf("ended %d err=%v, want 1", n, err)
	}
	if s, e := statusOf(t, "old"); s != "failed" || e != staleAiRunErrorMessage {
		t.Fatalf("old: %s %q", s, e)
	}
	if s, _ := statusOf(t, "fresh"); s != "generating" {
		t.Fatalf("fresh: %s", s)
	}
	if s, _ := statusOf(t, "done"); s != "ready" {
		t.Fatalf("done: %s", s)
	}
	// The list heals a stuck run on its own.
	exec(t, `UPDATE persona_ai_runs SET created_at = datetime('now', '-2 hours') WHERE id = 'fresh'`)
	runs, _ := ListAiRuns(pid)
	for _, r := range runs {
		if r.ID == "fresh" && r.Status != "failed" {
			t.Fatalf("the list did not end the stale run: %s", r.Status)
		}
	}
}

func call(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	AiRunsHandler(w, httptest.NewRequest(method, target, strings.NewReader(body)))
	return w
}

func TestAiRunsHandler(t *testing.T) {
	pid := setupRunsDB(t)

	// POST
	for name, body := range map[string]string{
		"not json":        `x`,
		"no profile":      `{"conversationId":"c"}`,
		"no conversation": `{"profileId":"` + pid + `"}`,
		"unknown profile": `{"profileId":"nope","conversationId":"c"}`,
		"focus too long":  `{"profileId":"` + pid + `","conversationId":"c","focus":"` + strings.Repeat("x", maxFocus+1) + `"}`,
	} {
		if w := call(t, http.MethodPost, "/persona-ai-runs", body); w.Code != http.StatusBadRequest {
			t.Errorf("POST %s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	w := call(t, http.MethodPost, "/persona-ai-runs", `{"profileId":"`+pid+`","conversationId":"c1","focus":"f"}`)
	var created struct{ ID string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil || created.ID == "" {
		t.Fatalf("POST: %d %s", w.Code, w.Body)
	}

	// GET
	if w := call(t, http.MethodGet, "/persona-ai-runs", ""); w.Code != http.StatusBadRequest {
		t.Errorf("GET without profileId: %d", w.Code)
	}
	if w := call(t, http.MethodGet, "/persona-ai-runs?profileId=nope", ""); w.Code != http.StatusBadRequest {
		t.Errorf("GET unknown profile: %d", w.Code)
	}
	w = call(t, http.MethodGet, "/persona-ai-runs?profileId="+pid, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"personas":[]`) || strings.Contains(w.Body.String(), `"proposal"`) || strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("GET: %d %s", w.Code, w.Body)
	}

	// PUT
	put := func(body string) int { return call(t, http.MethodPut, "/persona-ai-runs", body).Code }
	if c := put(`{}`); c != http.StatusBadRequest {
		t.Errorf("PUT without id: %d", c)
	}
	if c := put(`{"id":"nope","status":"failed","error":"x"}`); c != http.StatusBadRequest {
		t.Errorf("PUT unknown run: %d", c)
	}
	if c := put(`{"id":"` + created.ID + `","status":"ready","proposal":{"personas":null}}`); c != http.StatusBadRequest {
		t.Errorf("PUT invalid proposal: %d", c)
	}
	if c := put(`{"id":"` + created.ID + `","status":"ready","proposal":` + personasJSON(okPersona) + `}`); c != http.StatusNoContent {
		t.Fatalf("PUT ready: %d", c)
	}
	if c := put(`{"id":"` + created.ID + `","status":"failed","error":"late"}`); c != http.StatusConflict {
		t.Errorf("PUT after finished: %d, want 409", c)
	}
	w = call(t, http.MethodGet, "/persona-ai-runs?profileId="+pid, "")
	if !strings.Contains(w.Body.String(), `"status":"ready"`) || !strings.Contains(w.Body.String(), `"proposal":{"personas"`) {
		t.Fatalf("GET after finish: %s", w.Body)
	}

	// DELETE
	if c := call(t, http.MethodDelete, "/persona-ai-runs", "").Code; c != http.StatusBadRequest {
		t.Errorf("DELETE without id: %d", c)
	}
	for i := 0; i < 2; i++ {
		if c := call(t, http.MethodDelete, "/persona-ai-runs?id="+created.ID, "").Code; c != http.StatusNoContent {
			t.Errorf("DELETE run %d: %d", i, c)
		}
	}
	if c := call(t, http.MethodPatch, "/persona-ai-runs", "").Code; c != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: %d", c)
	}
}

// The persona list carries aiRunId only for a persona that came from a run.
func TestListPersonas_CarriesTheAiRunId(t *testing.T) {
	pid := setupRunsDB(t)
	id := mustRun(t, pid)
	exec(t, `INSERT INTO personas (id, profile_id, name, ai_run_id) VALUES ('ai', ?, 'FromAI', ?), ('man', ?, 'Manual', NULL)`, pid, id, pid)
	personas, err := ListPersonas(pid)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(personas)
	var out []map[string]any
	_ = json.Unmarshal(raw, &out)
	byName := map[string]map[string]any{}
	for _, p := range out {
		byName[p["name"].(string)] = p
	}
	if byName["FromAI"]["aiRunId"] != id {
		t.Fatalf("FromAI = %v", byName["FromAI"])
	}
	if _, has := byName["Manual"]["aiRunId"]; has {
		t.Fatalf("a manual persona must not carry the field: %v", byName["Manual"])
	}
}
