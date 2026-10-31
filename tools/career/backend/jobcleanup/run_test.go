package jobcleanup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
)

// fixedNow pins the clock to 2026-06-30 12:00:00 UTC, so "30 days" means a cutoff of 2026-05-31 12:00:00.
func fixedNow(t *testing.T) {
	t.Helper()
	old := now
	now = func() time.Time { return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = old })
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func addJob(t *testing.T, id, crawledAt, source string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, crawled_at, source) VALUES (?, ?, ?, 'Acme', ?, ?)`,
		id, "https://x.example/"+id, "Job "+id, crawledAt, source)
}

func exists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func opts(days int, manual bool) Options {
	return Options{Rule: RuleOlder, OlderThanDays: days, MinMatchPercent: 60, IncludeManual: manual}
}

const (
	old30  = "2026-05-01 00:00:00" // 60 days old
	young  = "2026-06-20 00:00:00" // 10 days old
	edgeIn = "2026-05-31 11:59:59" // one second older than 30 days
	edgeOn = "2026-05-31 12:00:00" // exactly 30 days: kept, the rule is "more than N days"
)

func TestOnlyJobsOlderThanTheLimitAreDeleted(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	addJob(t, "young", young, "crawled")
	addJob(t, "edge-in", edgeIn, "crawled")
	addJob(t, "edge-on", edgeOn, "crawled")

	p, err := PreviewCleanup(opts(30, false))
	if err != nil || p.WouldDelete != 2 || p.ByRule.Older != 2 || p.ByRule.LowMatch != 0 {
		t.Fatalf("preview: %+v, %v", p, err)
	}
	if p.TotalJobs != 4 {
		t.Errorf("totalJobs = %d, want all 4 jobs, not only the ones that would go", p.TotalJobs)
	}
	if !exists(t, "old") || !exists(t, "edge-in") {
		t.Fatal("a preview deleted something")
	}
	res, err := RunCleanup(opts(30, false))
	if err != nil || res.Deleted != 2 || res.ByRule.Older != 2 {
		t.Fatalf("run: %+v, %v", res, err)
	}
	for id, want := range map[string]bool{"old": false, "edge-in": false, "young": true, "edge-on": true} {
		if exists(t, id) != want {
			t.Errorf("%s exists = %v, want %v", id, !want, want)
		}
	}
	// a different limit moves the line
	res, _ = RunCleanup(opts(5, false))
	if res.Deleted != 2 || exists(t, "young") || exists(t, "edge-on") {
		t.Errorf("limit 5: %+v", res)
	}
}

func TestEverySafetyRuleKeepsAnOldJob(t *testing.T) {
	setup(t)
	fixedNow(t)
	exec(t, `INSERT INTO job_statuses (id, profile_id, name, kind) VALUES ('s1', 'prof', 'Interested', 'custom')`)

	addJob(t, "free", old30, "crawled")
	addJob(t, "board", old30, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('prof', 'board', 'fixed-declined')`)
	addJob(t, "cv-pdf", old30, "crawled")
	exec(t, `INSERT INTO job_cv_pdfs (job_id, profile_id, status) VALUES ('cv-pdf', 'prof', 'completed')`)
	addJob(t, "cv-gen", old30, "crawled")
	exec(t, `INSERT INTO job_cv_generations (id, job_id, profile_id, conversation_id, status) VALUES ('g1', 'cv-gen', 'prof', 'c1', 'generating')`)
	addJob(t, "matching", old30, "crawled")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, status) VALUES ('matching', 'prof', 'matching')`)
	addJob(t, "matched", old30, "crawled") // a finished match is no reason to keep it
	exec(t, `INSERT INTO job_matches (job_id, profile_id, status, score) VALUES ('matched', 'prof', 'completed', 12)`)
	addJob(t, "manual", old30, "manual")

	p, err := PreviewCleanup(opts(30, false))
	if err != nil {
		t.Fatal(err)
	}
	if p.WouldDelete != 2 || p.Kept != (Kept{OnBoard: 1, HasCv: 2, InProgress: 1, Manual: 1}) {
		t.Fatalf("preview: %+v", p)
	}
	ids := map[string]bool{}
	for _, s := range p.Sample {
		ids[s.ID] = true
	}
	if !ids["free"] || !ids["matched"] || len(ids) != 2 {
		t.Errorf("sample: %+v", p.Sample)
	}
	res, err := RunCleanup(opts(30, false))
	if err != nil || res.Deleted != 2 {
		t.Fatalf("run: %+v, %v", res, err)
	}
	for _, id := range []string{"board", "cv-pdf", "cv-gen", "matching", "manual"} {
		if !exists(t, id) {
			t.Errorf("%s was deleted", id)
		}
	}
	for _, id := range []string{"free", "matched"} {
		if exists(t, id) {
			t.Errorf("%s was kept", id)
		}
	}
}

// A job on a board keeps its board entry and the board is the first reason, even when the job also has a CV.
func TestTheBoardReasonComesFirst(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "both", old30, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('prof', 'both', 'fixed-applied')`)
	exec(t, `INSERT INTO job_cv_pdfs (job_id, profile_id, status) VALUES ('both', 'prof', 'completed')`)
	p, _ := PreviewCleanup(opts(30, false))
	if p.Kept != (Kept{OnBoard: 1}) || p.WouldDelete != 0 {
		t.Errorf("%+v", p)
	}
}

func TestManualJobsGoOnlyWhenIncluded(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "manual", old30, "manual")
	addJob(t, "crawled", old30, "crawled")
	if res, _ := RunCleanup(opts(30, false)); res.Deleted != 1 || !exists(t, "manual") {
		t.Fatalf("not included: %+v", res)
	}
	if res, _ := RunCleanup(opts(30, true)); res.Deleted != 1 || exists(t, "manual") {
		t.Fatalf("included: %+v", res)
	}
}

// The delete statement itself re-checks every rule: a job that gained a board entry, a CV, a running match, or a
// fresh crawled_at after the candidates were listed is not deleted.
func TestTheDeleteRechecksEverythingAfterThePreview(t *testing.T) {
	setup(t)
	fixedNow(t)
	cutoff := opts(30, false).cutoff()
	for name, change := range map[string]string{
		"board":       `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('prof', 'x', 'fixed-not-reviewed')`,
		"cv":          `INSERT INTO job_cv_pdfs (job_id, profile_id) VALUES ('x', 'prof')`,
		"cv-gen":      `INSERT INTO job_cv_generations (id, job_id, profile_id, conversation_id) VALUES ('g', 'x', 'prof', 'c')`,
		"matching":    `INSERT INTO job_matches (job_id, profile_id, status) VALUES ('x', 'prof', 'matching')`,
		"re-crawl":    `UPDATE jobs SET crawled_at = '2026-06-29 00:00:00' WHERE id = 'x'`,
		"made-manual": `UPDATE jobs SET source = 'manual' WHERE id = 'x'`,
	} {
		exec(t, `DELETE FROM jobs`)
		addJob(t, "x", old30, "crawled")
		exec(t, change)
		ids, err := deleteBatch([]Sample{{ID: "x", reason: RuleOlder}}, cutoff, false)
		if err != nil || len(ids) != 0 || !exists(t, "x") {
			t.Errorf("%s: deleted %v, err %v", name, ids, err)
		}
	}
	exec(t, `DELETE FROM jobs`)
	addJob(t, "x", old30, "crawled")
	if ids, err := deleteBatch([]Sample{{ID: "x", reason: RuleOlder}}, cutoff, false); err != nil || len(ids) != 1 {
		t.Errorf("nothing changed: %v, %v", ids, err)
	}
}

// Deleting a job takes its match, skills and requirements with it, and only that job's.
func TestDeletingCascadesOnlyTheDeletedJob(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "gone", old30, "crawled")
	addJob(t, "stays", young, "crawled")
	for _, id := range []string{"gone", "stays"} {
		exec(t, `INSERT INTO job_matches (job_id, profile_id, status, score) VALUES (?, 'prof', 'completed', 50)`, id)
		exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES (?, 'Go')`, id)
	}
	if res, _ := RunCleanup(opts(30, false)); res.Deleted != 1 {
		t.Fatalf("%+v", res)
	}
	var m, s int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches WHERE job_id = 'gone'`).Scan(&m)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_match_skills WHERE job_id = 'gone'`).Scan(&s)
	if m != 0 || s != 0 {
		t.Errorf("orphans: %d matches, %d skills", m, s)
	}
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches WHERE job_id = 'stays'`).Scan(&m)
	if m != 1 {
		t.Error("the other job's match was removed")
	}
}

// More than one batch: 450 old jobs are all deleted (batches of 200), and the sample of a preview stops at 50.
func TestBatchesAndTheSampleLimit(t *testing.T) {
	setup(t)
	fixedNow(t)
	tx, _ := db.JobsDB.Begin()
	for i := 0; i < 450; i++ {
		if _, err := tx.Exec(`INSERT INTO jobs (id, source_url, title, crawled_at) VALUES (?, ?, 't', ?)`, fmt.Sprintf("j%03d", i), fmt.Sprintf("u%d", i), old30); err != nil {
			t.Fatal(err)
		}
	}
	_ = tx.Commit()
	p, _ := PreviewCleanup(opts(30, false))
	if p.WouldDelete != 450 || len(p.Sample) != 50 {
		t.Fatalf("preview: %d would go, %d in the sample", p.WouldDelete, len(p.Sample))
	}
	res, err := RunCleanup(opts(30, false))
	if err != nil || res.Deleted != 450 {
		t.Fatalf("%+v, %v", res, err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n)
	if n != 0 {
		t.Errorf("%d left", n)
	}
}

func TestARunSavesItsSummary(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", old30, "crawled")
	if _, err := RunCleanup(opts(30, false)); err != nil {
		t.Fatal(err)
	}
	s, _ := GetSettings()
	if s.LastRunAt != "2026-06-30 12:00:00" || s.LastRunDeleted != 1 || s.LastRunSummary != "older than 30 days: 1 deleted" {
		t.Errorf("%+v", s)
	}
}

func TestARunWhileAnotherRunsIsRefused(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", old30, "crawled")
	runMu.Lock()
	defer runMu.Unlock()
	if _, err := RunCleanup(opts(30, false)); err != ErrBusy {
		t.Errorf("err = %v", err)
	}
	if !exists(t, "a") {
		t.Error("the refused run deleted")
	}
}

func TestOptionsAreValidated(t *testing.T) {
	setup(t)
	for _, o := range []Options{
		{Rule: "", OlderThanDays: 30, MinMatchPercent: 60}, {Rule: "newer", OlderThanDays: 30, MinMatchPercent: 60},
		{Rule: RuleOlder, OlderThanDays: 0, MinMatchPercent: 60}, {Rule: RuleOlder, OlderThanDays: 3651, MinMatchPercent: 60},
		{Rule: RuleLowMatch, OlderThanDays: 30, MinMatchPercent: 0}, {Rule: RuleBoth, OlderThanDays: 30, MinMatchPercent: 101},
	} {
		if _, err := PreviewCleanup(o); err == nil {
			t.Errorf("preview accepted %+v", o)
		}
		if _, err := RunCleanup(o); err == nil {
			t.Errorf("run accepted %+v", o)
		}
	}
}

func post(h http.HandlerFunc, body string) (int, string) {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
	return rec.Code, rec.Body.String()
}

func TestHandlers(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	addJob(t, "young", young, "crawled")

	// the saved threshold (90 days) keeps both; a body value overrides it for this call
	code, body := post(PreviewHandler, `{"rule":"older"}`)
	var p Preview
	_ = json.Unmarshal([]byte(body), &p)
	if code != 200 || p.WouldDelete != 0 {
		t.Fatalf("saved threshold: %d %s", code, body)
	}
	code, body = post(PreviewHandler, `{"rule":"older","olderThanDays":30}`)
	_ = json.Unmarshal([]byte(body), &p)
	if code != 200 || p.WouldDelete != 1 || len(p.Sample) != 1 || p.Sample[0].ID != "old" {
		t.Fatalf("override: %d %s", code, body)
	}
	if !strings.Contains(body, `"totalJobs":2`) || !strings.Contains(body, `"sample":[`) || !strings.Contains(body, `"kept":{"onBoard":0,"hasCv":0,"inProgress":0,"manual":0,"unjudged":0}`) {
		t.Errorf("json shape: %s", body)
	}
	// an empty preview still has a list, not null
	if _, body = post(PreviewHandler, `{"rule":"older","olderThanDays":3650}`); !strings.Contains(body, `"sample":[]`) {
		t.Errorf("empty sample: %s", body)
	}

	// run needs confirm
	if code, _ = post(RunHandler, `{"rule":"older","olderThanDays":30}`); code != 400 || !exists(t, "old") {
		t.Errorf("without confirm: %d", code)
	}
	if code, _ = post(RunHandler, `{"rule":"older","olderThanDays":30,"confirm":false}`); code != 400 || !exists(t, "old") {
		t.Errorf("confirm false: %d", code)
	}
	for _, bad := range []string{`{"rule":"newer","confirm":true}`, `{"rule":"low_match","minMatchPercent":0,"confirm":true}`, `{"rule":"older","olderThanDays":0,"confirm":true}`, `nope`} {
		if code, _ = post(RunHandler, bad); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}
	code, body = post(RunHandler, `{"rule":"older","olderThanDays":30,"confirm":true}`)
	if code != 200 || !strings.Contains(body, `"deleted":1`) || exists(t, "old") || !exists(t, "young") {
		t.Fatalf("run: %d %s", code, body)
	}
	// busy is a 409
	runMu.Lock()
	code, _ = post(RunHandler, `{"rule":"older","olderThanDays":1,"confirm":true}`)
	runMu.Unlock()
	if code != http.StatusConflict || !exists(t, "young") {
		t.Errorf("busy: %d", code)
	}
	// only POST
	for _, h := range []http.HandlerFunc{PreviewHandler, RunHandler} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET: %d", rec.Code)
		}
	}
}

// Two runs started together never both delete: one wins, the other is busy or finds nothing.
func TestTwoRunsAtOnceDeleteEachJobOnce(t *testing.T) {
	setup(t)
	fixedNow(t)
	for i := 0; i < 30; i++ {
		addJob(t, fmt.Sprintf("j%02d", i), old30, "crawled")
	}
	var wg sync.WaitGroup
	total := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := RunCleanup(opts(30, false)); err == nil {
				mu.Lock()
				total += res.Deleted
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if total != 30 {
		t.Errorf("deleted %d in total, want 30", total)
	}
}

// The cutoff is computed in UTC whatever zone the clock is in: 12:00 in UTC+14 is 22:00 the day before in UTC.
func TestTheCutoffIsUTC(t *testing.T) {
	setup(t)
	old := now
	defer func() { now = old }()
	now = func() time.Time { return time.Date(2026, 7, 1, 8, 0, 0, 0, time.FixedZone("UTC+14", 14*3600)) }
	if got := opts(1, false).cutoff(); got != "2026-06-29 18:00:00" {
		t.Errorf("cutoff = %s", got)
	}
}

// The comments of each deleted job (and only of those) are purged in core.
func TestDeletedJobsHaveTheirCommentsPurged(t *testing.T) {
	setup(t)
	fixedNow(t)
	var mu sync.Mutex
	var purged []string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b struct{ EntityType, EntityID string }
		_ = json.Unmarshal(raw, &b)
		mu.Lock()
		purged = append(purged, b.EntityType+":"+b.EntityID)
		mu.Unlock()
	}))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	addJob(t, "gone", old30, "crawled")
	addJob(t, "kept", young, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'kept', 'fixed-applied')`)
	addJob(t, "kept-board", old30, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'kept-board', 'fixed-applied')`)
	if _, err := RunCleanup(opts(30, false)); err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0] != "job:gone" {
		t.Errorf("purged %v", purged)
	}
}

func TestIncludeManualCanBeOverriddenInTheBody(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "manual", old30, "manual")
	if _, body := post(PreviewHandler, `{"rule":"older","olderThanDays":30}`); !strings.Contains(body, `"wouldDelete":0`) {
		t.Errorf("saved (off): %s", body)
	}
	if _, body := post(PreviewHandler, `{"rule":"older","olderThanDays":30,"includeManual":true}`); !strings.Contains(body, `"wouldDelete":1`) {
		t.Errorf("override on: %s", body)
	}
	exec(t, `UPDATE job_cleanup_settings SET include_manual = 1`)
	if _, body := post(PreviewHandler, `{"rule":"older","olderThanDays":30}`); !strings.Contains(body, `"wouldDelete":1`) {
		t.Errorf("saved (on): %s", body)
	}
	if _, body := post(PreviewHandler, `{"rule":"older","olderThanDays":30,"includeManual":false}`); !strings.Contains(body, `"wouldDelete":0`) {
		t.Errorf("override off: %s", body)
	}
}
