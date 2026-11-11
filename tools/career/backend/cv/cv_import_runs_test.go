package cv

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

func str(s string) *string { return &s }

// withoutIDs is a stored proposal without the ids of its items (plan 257): two runs made the same way have the same proposal but not the same ids.
func withoutIDs(stored string) string {
	return regexp.MustCompile(`"id":"[^"]*",`).ReplaceAllString(stored, "")
}

func okProposal() Proposal {
	return Proposal{
		Profile:         ProfileProposal{Action: "insert", Name: "Ada Lovelace"},
		Persona:         PersonaProposal{Action: "insert", Name: "Backend Engineer", Title: "Senior Backend Engineer"},
		PersonalDetails: DetailsProposal{Action: "insert", Headline: "Engineer", Summary: "Builds services.", Location: "Hamburg", DesiredTitles: "Backend Engineer", DesiredLocations: "Hamburg, Remote", MinSalary: 70000},
		Skills:          []SkillProposal{{Action: "insert", Value: "Go"}, {Action: "skip", PossibleDuplicateOf: str("Docker"), Value: "Docker"}},
		Experience:      []ExperienceProposal{{Action: "insert", Company: "Acme", Title: "Engineer", StartDate: "2019-03", EndDate: "2023-01", Description: "Go services."}},
	}
}

func newRun(t *testing.T) string {
	t.Helper()
	id, err := createGeneratingCVImportRun("media-1", "cv.pdf", "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func runOf(t *testing.T, id string) cvImportRun {
	t.Helper()
	r, err := getCVImportRun(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.CareerDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---- creating ----

func TestAGeneratingRunStartsWithoutAProposal(t *testing.T) {
	setup(t)
	id := newRun(t)
	r := runOf(t, id)
	if r.Status != "generating" || r.Error != "" || string(r.AIProposal) != "null" || r.MediaFileID != "media-1" || r.OriginalFilename != "cv.pdf" || r.ConversationID != "conv-1" {
		t.Errorf("%+v", r)
	}
	if r.SaveSummary != nil || r.UpdatedAt != "" || r.CreatedAt == "" {
		t.Errorf("%+v", r)
	}
}

func TestACompletedAnalysisIsRecordedReadyAsBefore(t *testing.T) {
	setup(t)
	id, err := createCVImportRun("m", "f.pdf", "c", []byte(`{"profile":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := runOf(t, id); r.Status != "ready" || r.Error != "" || string(r.AIProposal) != `{"profile":{}}` {
		t.Errorf("%+v", r)
	}
}

// The status is part of the JSON the page reads.
func TestTheStatusAndErrorAreInTheRunsJSON(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := finishCVImportRun(id, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(runOf(t, id))
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["status"] != "failed" || m["error"] != "boom" {
		t.Errorf("%s", raw)
	}
	ok := newRun(t)
	raw, _ = json.Marshal(runOf(t, ok))
	if strings.Contains(string(raw), `"error"`) || !strings.Contains(string(raw), `"status":"generating"`) {
		t.Errorf("a run without an error should not carry one: %s", raw)
	}
}

// ---- storing the proposal ----

func TestTheProposalIsStoredOnAGeneratingRunAndReplacedByASecond(t *testing.T) {
	setup(t)
	id := newRun(t)
	if err := StoreProposal(id, okProposal()); err != nil {
		t.Fatal(err)
	}
	var got Proposal
	if err := json.Unmarshal(runOf(t, id).AIProposal, &got); err != nil || got.Profile.Name != "Ada Lovelace" || len(got.Skills) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	second := okProposal()
	second.Profile.Name = "Grace Hopper"
	second.Skills = nil
	if err := StoreProposal(id, second); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(runOf(t, id).AIProposal, &got); err != nil || got.Profile.Name != "Grace Hopper" || len(got.Skills) != 0 {
		t.Errorf("not replaced: %+v", got)
	}
	if runOf(t, id).Status != "generating" {
		t.Error("storing a proposal must not end the run")
	}
}

func TestAProposalIsOnlyTakenByAGeneratingRun(t *testing.T) {
	setup(t)
	if err := StoreProposal("nope", okProposal()); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("unknown: %v", err)
	}
	ready := newRun(t)
	_ = StoreProposal(ready, okProposal())
	if _, err := finishCVImportRun(ready, "ready", ""); err != nil {
		t.Fatal(err)
	}
	failed := newRun(t)
	if _, err := finishCVImportRun(failed, "failed", "x"); err != nil {
		t.Fatal(err)
	}
	legacy, _ := createCVImportRun("m", "f", "c", []byte(`{}`))
	before := string(runOf(t, ready).AIProposal)
	for name, id := range map[string]string{"ready": ready, "failed": failed, "legacy": legacy} {
		if err := StoreProposal(id, okProposal()); !errors.Is(err, errRunNotGenerating) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if string(runOf(t, ready).AIProposal) != before || string(runOf(t, failed).AIProposal) != "null" {
		t.Error("a refused proposal changed a run")
	}
}

// ---- finishing ----

func TestAFinishedRunIsReadyWithItsProposal(t *testing.T) {
	setup(t)
	id := newRun(t)
	if err := StoreProposal(id, okProposal()); err != nil {
		t.Fatal(err)
	}
	r, err := finishCVImportRun(id, "ready", "ignored for a ready run")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != id || r.Status != "ready" || r.Error != "" || r.UpdatedAt == "" || string(r.AIProposal) == "null" {
		t.Errorf("%+v", r)
	}
	if runOf(t, id).Status != "ready" {
		t.Error("not stored")
	}
}

func TestAReadyRunNeedsAProposal(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := finishCVImportRun(id, "ready", ""); !errors.Is(err, errNoProposalStored) {
		t.Fatalf("%v", err)
	}
	if r := runOf(t, id); r.Status != "generating" {
		t.Errorf("a refused finish changed the run: %+v", r)
	}
	// the AI can still hand it over afterwards, and the run then finishes
	if err := StoreProposal(id, okProposal()); err != nil {
		t.Fatal(err)
	}
	if _, err := finishCVImportRun(id, "ready", ""); err != nil {
		t.Errorf("%v", err)
	}
}

func TestAFailedRunNeedsAReasonAndKeepsItTrimmed(t *testing.T) {
	setup(t)
	id := newRun(t)
	for _, e := range []string{"", "   ", "\n\t"} {
		if _, err := finishCVImportRun(id, "failed", e); !errors.Is(err, errInvalidFinish) {
			t.Errorf("%q: %v", e, err)
		}
	}
	if runOf(t, id).Status != "generating" {
		t.Error("a refused finish changed the run")
	}
	r, err := finishCVImportRun(id, "failed", "  the AI did not hand over a proposal  ")
	if err != nil || r.Status != "failed" || r.Error != "the AI did not hand over a proposal" {
		t.Errorf("%+v %v", r, err)
	}
}

func TestOnlyReadyAndFailedAreOutcomes(t *testing.T) {
	setup(t)
	id := newRun(t)
	for _, s := range []string{"", "generating", "done", "READY", "ok"} {
		if _, err := finishCVImportRun(id, s, "x"); !errors.Is(err, errInvalidFinish) {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestARunIsFinishedOnce(t *testing.T) {
	setup(t)
	id := newRun(t)
	_ = StoreProposal(id, okProposal())
	if _, err := finishCVImportRun(id, "ready", ""); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"ready", "failed"} {
		if _, err := finishCVImportRun(id, status, "again"); !errors.Is(err, errRunNotGenerating) {
			t.Errorf("%s: %v", status, err)
		}
	}
	if r := runOf(t, id); r.Status != "ready" || r.Error != "" {
		t.Errorf("a second finish changed the run: %+v", r)
	}
	if _, err := finishCVImportRun("nope", "failed", "x"); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("unknown: %v", err)
	}
}

func TestFinishingOneRunLeavesTheOthers(t *testing.T) {
	setup(t)
	a, b := newRun(t), newRun(t)
	_ = StoreProposal(a, okProposal())
	_ = StoreProposal(b, okProposal())
	if _, err := finishCVImportRun(a, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if runOf(t, a).Status != "ready" || runOf(t, b).Status != "generating" {
		t.Error("the wrong run was finished")
	}
}

// The review's outcome is recorded as before, for a ready run.
func TestTheReviewOutcomeIsStillRecorded(t *testing.T) {
	setup(t)
	id := newRun(t)
	_ = StoreProposal(id, okProposal())
	_, _ = finishCVImportRun(id, "ready", "")
	if err := updateCVImportRunSaveSummary(id, []byte(`{"saved":1}`)); err != nil {
		t.Fatal(err)
	}
	if r := runOf(t, id); string(r.SaveSummary) != `{"saved":1}` || r.Status != "ready" {
		t.Errorf("%+v", r)
	}
	if err := updateCVImportRunSaveSummary("nope", []byte(`{}`)); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("%v", err)
	}
}

// ---- stale runs ----

func TestARunStuckGeneratingIsEndedAsFailed(t *testing.T) {
	setup(t)
	stale, fresh, borderline := newRun(t), newRun(t), newRun(t)
	ready, _ := createCVImportRun("m", "f", "c", []byte(`{}`))
	failedOld := newRun(t)
	if _, err := finishCVImportRun(failedOld, "failed", "earlier reason"); err != nil {
		t.Fatal(err)
	}
	for id, ago := range map[string]string{stale: "-31 minutes", borderline: "-29 minutes", ready: "-5 hours", failedOld: "-5 hours"} {
		if _, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET created_at = datetime('now', ?) WHERE id = ?`, ago, id); err != nil {
			t.Fatal(err)
		}
	}
	n, err := ReconcileStaleCVImportRuns()
	if err != nil || n != 1 {
		t.Fatalf("ended %d, %v", n, err)
	}
	if r := runOf(t, stale); r.Status != "failed" || r.Error == "" || r.UpdatedAt == "" {
		t.Errorf("stale: %+v", r)
	}
	for name, id := range map[string]string{"fresh": fresh, "29 minutes": borderline} {
		if runOf(t, id).Status != "generating" {
			t.Errorf("%s was ended", name)
		}
	}
	if runOf(t, ready).Status != "ready" {
		t.Error("a ready run was touched")
	}
	if r := runOf(t, failedOld); r.Error != "earlier reason" {
		t.Errorf("a failed run's reason was replaced: %+v", r)
	}
	if n, _ := ReconcileStaleCVImportRuns(); n != 0 {
		t.Errorf("a second pass ended %d", n)
	}
}

func TestListShowsEveryRunWithItsStatusNewestFirst(t *testing.T) {
	setup(t)
	a := newRun(t)
	b, _ := createCVImportRun("m", "f", "c", []byte(`{}`))
	if _, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET created_at = datetime('now', '-1 hour') WHERE id = ?`, a); err != nil {
		t.Fatal(err)
	}
	runs, err := listCVImportRuns()
	if err != nil || len(runs) != 2 || runs[0].ID != b || runs[1].ID != a || runs[0].Status != "ready" || runs[1].Status != "generating" {
		t.Errorf("%+v %v", runs, err)
	}
}

func TestADeletedRunIsGone(t *testing.T) {
	setup(t)
	id := newRun(t)
	media, err := deleteCVImportRun(id)
	if err != nil || media != "media-1" || count(t, `SELECT COUNT(*) FROM cv_import_runs`) != 0 {
		t.Errorf("%q %v", media, err)
	}
	if _, err := deleteCVImportRun(id); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("%v", err)
	}
}

// A run that is already over is "not generating" whichever outcome is asked for, also a failed one that never had a proposal.
func TestFinishingAFailedRunAsReadyIsNotGeneratingNotNoProposal(t *testing.T) {
	setup(t)
	id := newRun(t)
	if _, err := finishCVImportRun(id, "failed", "boom"); err != nil {
		t.Fatal(err)
	}
	if _, err := finishCVImportRun(id, "ready", ""); !errors.Is(err, errRunNotGenerating) {
		t.Errorf("%v", err)
	}
	if r := runOf(t, id); r.Status != "failed" || r.Error != "boom" {
		t.Errorf("%+v", r)
	}
}

// If the run is ended between the check and the update (another page, the stale reconcile), the update does not overwrite it.
func TestARunEndedBetweenTheCheckAndTheUpdateIsNotOverwritten(t *testing.T) {
	setup(t)
	id := newRun(t)
	_ = StoreProposal(id, okProposal())
	old := afterFinishCheck
	afterFinishCheck = func() {
		if _, err := db.CareerDB.Exec(`UPDATE cv_import_runs SET status = 'failed', error = 'ended meanwhile' WHERE id = ?`, id); err != nil {
			t.Error(err)
		}
	}
	defer func() { afterFinishCheck = old }()
	if _, err := finishCVImportRun(id, "ready", ""); !errors.Is(err, errRunNotGenerating) {
		t.Errorf("%v", err)
	}
	if r := runOf(t, id); r.Status != "failed" || r.Error != "ended meanwhile" {
		t.Errorf("overwritten: %+v", r)
	}
}

// A run that ends ready has no error at all (NULL, not an empty text).
func TestAReadyRunStoresNoError(t *testing.T) {
	setup(t)
	id := newRun(t)
	_ = StoreProposal(id, okProposal())
	if _, err := finishCVImportRun(id, "ready", ""); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM cv_import_runs WHERE id = ? AND error IS NULL`, id); n != 1 {
		t.Errorf("the error column of a ready run is not NULL")
	}
}

func TestReadingAnUnknownRunIsAnError(t *testing.T) {
	setup(t)
	if _, err := getCVImportRun("nope"); !errors.Is(err, errUnknownCVImportRun) {
		t.Errorf("%v", err)
	}
}
