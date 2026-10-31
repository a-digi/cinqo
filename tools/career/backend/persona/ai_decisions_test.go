package persona

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/db"
)

// runWith makes a ready run proposing the named personas.
func runWith(t *testing.T, profileID string, names ...string) string {
	t.Helper()
	id := mustRun(t, profileID)
	ps := make([]string, len(names))
	for i, n := range names {
		ps[i] = strings.Replace(okPersona, "Backend", n, 1)
	}
	if err := FinishAiRun(id, "ready", []byte(personasJSON(ps...)), ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func accept(i int, p ProposedPersona) Decision {
	return Decision{Index: i, Decision: "accepted", Persona: &p}
}
func decline(i int) Decision { return Decision{Index: i, Decision: "declined"} }

func runRow(t *testing.T, id string) (decidedAt *string, declined *string) {
	t.Helper()
	if err := db.CareerDB.QueryRow(`SELECT decided_at, declined_json FROM persona_ai_runs WHERE id = ?`, id).Scan(&decidedAt, &declined); err != nil {
		t.Fatal(err)
	}
	return
}

func TestApplyDecisions_CreatesTheAcceptedRecordsTheDeclinedAndClosesTheRun(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")

	// Out of order on purpose: the answer and the history follow the proposal, not the request.
	created, err := ApplyDecisions(run, []Decision{
		accept(2, pp("Gamma edited", []string{"Go"})),
		decline(1),
		accept(0, pp("Alpha edited", []string{"SQL"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || created[0].Name != "Alpha edited" || created[1].Name != "Gamma edited" {
		t.Fatalf("created = %+v, want the accepted ones in proposal order with the edited content", created)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE ai_run_id = ? AND profile_id = ?`, run, pid); n != 2 {
		t.Fatalf("%d personas linked to the run", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE name = 'Beta'`); n != 0 {
		t.Fatal("the declined persona was created")
	}
	decidedAt, declined := runRow(t, run)
	if decidedAt == nil || declined == nil || *declined != `[{"name":"Beta"}]` {
		t.Fatalf("decided_at=%v declined_json=%v", decidedAt, declined)
	}
	runs, _ := ListAiRuns(pid)
	if runs[0].DecidedAt == "" || len(runs[0].Declined) != 1 || runs[0].Declined[0].Name != "Beta" || len(runs[0].Personas) != 2 {
		t.Fatalf("list = %+v", runs[0])
	}
}

func TestApplyDecisions_DecliningEverythingCreatesNothingAndClosesTheRun(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta")
	created, err := ApplyDecisions(run, []Decision{decline(0), decline(1)})
	if err != nil || len(created) != 0 {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas`); n != 0 {
		t.Fatalf("%d personas created", n)
	}
	runs, _ := ListAiRuns(pid)
	if runs[0].DecidedAt == "" || len(runs[0].Declined) != 2 || runs[0].Declined[0].Name != "Alpha" || runs[0].Declined[1].Name != "Beta" {
		t.Fatalf("list = %+v", runs[0])
	}
	if _, err := ApplyDecisions(run, []Decision{decline(0), decline(1)}); !errors.Is(err, ErrAiRunDecided) {
		t.Fatalf("deciding again: %v", err)
	}
}

func TestApplyDecisions_AnUndecidedRunHasNoDecidedAtAndAnEmptyDeclinedList(t *testing.T) {
	pid := setupRunsDB(t)
	runWith(t, pid, "Alpha")
	runs, _ := ListAiRuns(pid)
	raw, _ := json.Marshal(runs[0])
	if strings.Contains(string(raw), "decidedAt") || !strings.Contains(string(raw), `"declined":[]`) {
		t.Fatalf("json = %s", raw)
	}
}

// A request that is incomplete or contradicts itself is refused, and nothing changes. Each case gets
// its own run: a request that is valid would decide a persona and change the next case.
func TestApplyDecisions_RefusesContradictoryRequests(t *testing.T) {
	pid := setupRunsDB(t)
	one := pp("Alpha", nil)
	cases := map[string][]Decision{
		"none":                  {},
		"a duplicate index":     {accept(0, one), decline(0)},
		"out of range":          {accept(0, one), decline(2)},
		"negative index":        {accept(0, one), decline(-1)},
		"unknown decision":      {accept(0, one), {Index: 1, Decision: "maybe"}},
		"no decision at all":    {accept(0, one), {Index: 1}},
		"accepted w/o content":  {accept(0, one), {Index: 1, Decision: "accepted"}},
		"declined with content": {accept(0, one), {Index: 1, Decision: "declined", Persona: &one}},
	}
	for name, ds := range cases {
		run := runWith(t, pid, "Alpha", "Beta")
		if _, err := ApplyDecisions(run, ds); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("%s: %v, want ErrInvalidProposal", name, err)
		}
		if decidedAt, _ := runRow(t, run); decidedAt != nil {
			t.Errorf("%s: a refused call decided the run", name)
		}
	}
	if n := count(t, `SELECT COUNT(*) FROM personas`); n != 0 {
		t.Fatalf("%d personas created by refused calls", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM persona_ai_runs WHERE decided_indexes IS NOT NULL`); n != 0 {
		t.Fatalf("%d runs have decisions recorded by refused calls", n)
	}
}

// The user decides one persona at a time: accepting one creates it at once and the others keep
// waiting; the run closes only when the last one is decided.
func TestApplyDecisions_OnePersonaAtATime(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")
	decided := func() []int {
		runs, _ := ListAiRuns(pid)
		return runs[0].Decided
	}
	same := reflect.DeepEqual

	// accept only the middle one: it exists at once, the run stays open, the others still wait
	created, err := ApplyDecisions(run, []Decision{accept(1, pp("Beta edited", []string{"Go"}))})
	if err != nil || len(created) != 1 || created[0].Name != "Beta edited" {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE ai_run_id = ?`, run); n != 1 {
		t.Fatalf("%d personas after accepting one", n)
	}
	if decidedAt, _ := runRow(t, run); decidedAt != nil {
		t.Fatal("the run was closed with two personas still waiting")
	}
	if !same(decided(), []int{1}) {
		t.Fatalf("decided = %v, want [1]", decided())
	}

	// decline the first: nothing is created, the run is still open
	if created, err := ApplyDecisions(run, []Decision{decline(0)}); err != nil || len(created) != 0 {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if decidedAt, _ := runRow(t, run); decidedAt != nil {
		t.Fatal("the run was closed with one persona still waiting")
	}
	if !same(decided(), []int{0, 1}) {
		t.Fatalf("decided = %v, want [0 1]", decided())
	}

	// a persona that was already decided cannot be decided again, whatever the answer
	for _, d := range []Decision{accept(1, pp("Again", nil)), decline(1), decline(0), accept(0, pp("Again", nil))} {
		if _, err := ApplyDecisions(run, []Decision{d}); !errors.Is(err, ErrPersonaDecided) {
			t.Errorf("deciding persona %d again: %v, want ErrPersonaDecided", d.Index, err)
		}
	}
	// a request that mixes a decided persona with a waiting one changes nothing
	if _, err := ApplyDecisions(run, []Decision{accept(2, pp("Gamma", nil)), decline(0)}); !errors.Is(err, ErrPersonaDecided) {
		t.Fatalf("mixed request: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE ai_run_id = ?`, run); n != 1 {
		t.Fatalf("%d personas after a refused mixed request, want 1", n)
	}

	// the last one closes the run, and the history says what happened to each
	if _, err := ApplyDecisions(run, []Decision{accept(2, pp("Gamma", nil))}); err != nil {
		t.Fatal(err)
	}
	decidedAt, declined := runRow(t, run)
	if decidedAt == nil || declined == nil || *declined != `[{"name":"Alpha"}]` {
		t.Fatalf("decided_at=%v declined_json=%v", decidedAt, declined)
	}
	runs, _ := ListAiRuns(pid)
	if runs[0].DecidedAt == "" || !same(runs[0].Decided, []int{0, 1, 2}) || len(runs[0].Personas) != 2 || len(runs[0].Declined) != 1 {
		t.Fatalf("list = %+v", runs[0])
	}
	if _, err := ApplyDecisions(run, []Decision{decline(0)}); !errors.Is(err, ErrAiRunDecided) {
		t.Fatalf("deciding on a closed run: %v", err)
	}
}

// Declines accumulate across requests, in the order they were made.
func TestApplyDecisions_DeclinesAccumulate(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")
	for _, i := range []int{2, 0, 1} {
		if _, err := ApplyDecisions(run, []Decision{decline(i)}); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := ListAiRuns(pid)
	names := []string{}
	for _, d := range runs[0].Declined {
		names = append(names, d.Name)
	}
	if strings.Join(names, ",") != "Gamma,Alpha,Beta" || runs[0].DecidedAt == "" {
		t.Fatalf("declined=%v decidedAt=%q", names, runs[0].DecidedAt)
	}
}

// A run nobody has decided on yet lists no decided persona, as an empty list.
func TestListAiRuns_DecidedIsAnEmptyListUntilSomethingIsDecided(t *testing.T) {
	pid := setupRunsDB(t)
	runWith(t, pid, "Alpha")
	runs, _ := ListAiRuns(pid)
	raw, _ := json.Marshal(runs[0])
	if !strings.Contains(string(raw), `"decided":[]`) {
		t.Fatalf("json = %s", raw)
	}
}

func TestApplyDecisions_TheAcceptedContentIsValidatedLikeAProposal(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta")
	for name, ds := range map[string][]Decision{
		"markup":          {accept(0, pp("<b>x</b>", nil)), decline(1)},
		"blank name":      {accept(0, pp("  ", nil)), decline(1)},
		"duplicate names": {accept(0, pp("Same", nil)), accept(1, pp("same", nil))},
		"empty skill":     {accept(0, pp("A", []string{" "})), decline(1)},
		"bad role":        {accept(0, pp("A", nil, ProposedRole{Company: "", Title: "T"})), decline(1)},
	} {
		if _, err := ApplyDecisions(run, ds); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Two accepted personas that edited to different names are fine.
	if _, err := ApplyDecisions(run, []Decision{accept(0, pp("One", nil)), accept(1, pp("Two", nil))}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDecisions_OnlyAReadyUndecidedRunWithAProposalCanBeDecided(t *testing.T) {
	pid := setupRunsDB(t)
	if _, err := ApplyDecisions("nope", []Decision{decline(0)}); !errors.Is(err, ErrUnknownAiRun) {
		t.Errorf("unknown run: %v", err)
	}
	if _, err := ApplyDecisions(mustRun(t, pid), []Decision{decline(0)}); !errors.Is(err, ErrAiRunNotReady) {
		t.Errorf("generating run: %v", err)
	}
	failed := mustRun(t, pid)
	_ = FinishAiRun(failed, "failed", nil, "boom")
	if _, err := ApplyDecisions(failed, []Decision{decline(0)}); !errors.Is(err, ErrAiRunNotReady) {
		t.Errorf("failed run: %v", err)
	}
	// A run whose proposal is empty (the AI said the data was too thin) has nothing to decide.
	empty := mustRun(t, pid)
	if err := FinishAiRun(empty, "ready", []byte(`{"personas":[],"note":"too thin"}`), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDecisions(empty, nil); !errors.Is(err, ErrAiRunNotReady) {
		t.Errorf("empty proposal: %v", err)
	}
	// Decided once; deleting what it created does not re-open it.
	run := runWith(t, pid, "Alpha")
	if _, err := ApplyDecisions(run, []Decision{accept(0, pp("Alpha", nil))}); err != nil {
		t.Fatal(err)
	}
	exec(t, `DELETE FROM personas WHERE ai_run_id = ?`, run)
	if _, err := ApplyDecisions(run, []Decision{decline(0)}); !errors.Is(err, ErrAiRunDecided) {
		t.Errorf("after deleting the personas: %v, want the run to stay closed", err)
	}
}

// A failure while writing leaves nothing behind and the run still undecided.
func TestApplyDecisions_IsAllOrNothing(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")
	exec(t, `CREATE TRIGGER boom BEFORE INSERT ON career_experience WHEN NEW.company = 'BOOM' BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	_, err := ApplyDecisions(run, []Decision{
		accept(0, pp("Alpha", nil, ProposedRole{Company: "Fine", Title: "T"})),
		decline(1),
		accept(2, pp("Gamma", nil, ProposedRole{Company: "BOOM", Title: "T"})),
	})
	if err == nil {
		t.Fatal("the failing insert was not reported")
	}
	if n := count(t, `SELECT COUNT(*) FROM personas`); n != 0 {
		t.Fatalf("%d personas after a failed decision", n)
	}
	if decidedAt, declined := runRow(t, run); decidedAt != nil || declined != nil {
		t.Fatalf("the run was changed by a failed decision: %v %v", decidedAt, declined)
	}
	exec(t, `DROP TRIGGER boom`)
	if _, err := ApplyDecisions(run, []Decision{accept(0, pp("Alpha", nil)), decline(1), decline(2)}); err != nil {
		t.Fatalf("a retry after the failure: %v", err)
	}
}

func decide(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return apply(t, body)
}

func TestAiRunApplyHandler_Decisions(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta")
	persona := `{"name":"Alpha","description":"","details":{"headline":"h","summary":"s","location":"","desiredTitles":"","desiredLocations":"","minSalary":0},"skills":["Go"],"experience":[]}`

	for name, body := range map[string]string{
		"bad decision":    `{"runId":"` + run + `","decisions":[{"index":0,"decision":"x"},{"index":1,"decision":"declined"}]}`,
		"duplicate index": `{"runId":"` + run + `","decisions":[{"index":0,"decision":"declined"},{"index":0,"decision":"declined"}]}`,
		"unknown run":     `{"runId":"nope","decisions":[{"index":0,"decision":"declined"}]}`,
		"neither shape":   `{"runId":"` + run + `"}`,
		"empty decisions": `{"runId":"` + run + `","decisions":[]}`,
	} {
		if w := decide(t, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	w := decide(t, `{"runId":"`+run+`","decisions":[{"index":0,"decision":"accepted","persona":`+persona+`},{"index":1,"decision":"declined"}]}`)
	var got struct {
		Personas []aiRunPersona `json:"personas"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Personas) != 1 || got.Personas[0].Name != "Alpha" {
		t.Fatalf("decide: %d %s", w.Code, w.Body)
	}
	if w := decide(t, `{"runId":"`+run+`","decisions":[{"index":0,"decision":"declined"},{"index":1,"decision":"declined"}]}`); w.Code != http.StatusConflict {
		t.Errorf("decide again: %d, want 409", w.Code)
	}
	gen := mustRun(t, pid)
	if w := decide(t, `{"runId":"`+gen+`","decisions":[{"index":0,"decision":"declined"}]}`); w.Code != http.StatusConflict {
		t.Errorf("a run that is still working: %d, want 409", w.Code)
	}
}
