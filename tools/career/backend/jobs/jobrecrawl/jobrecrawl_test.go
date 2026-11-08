package jobrecrawl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Portal')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url, job_detail_crawl_instructions) VALUES ('l1', 'p1', 'https://portal.example/search', 'fields: []')`)
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// addJob inserts a crawled job of link l1 with a posting URL.
func addJob(t *testing.T, id string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES (?, ?, ?, 'l1', 'crawled', datetime('now'))`, id, "https://portal.example/jobs/"+id, "Job "+id)
}

func addManual(t *testing.T, id, linkID, url string) {
	t.Helper()
	var link any
	if linkID != "" {
		link = linkID
	}
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES (?, ?, ?, ?, 'manual', datetime('now'))`, id, url, "Manual "+id, link)
}

func rowOf(t *testing.T, id string) (state, batch, outcome string) {
	t.Helper()
	var o, b *string
	if err := db.JobsDB.QueryRow(`SELECT state, batch_id, outcome FROM job_recrawl_requests WHERE job_id = ?`, id).Scan(&state, &b, &o); err != nil {
		t.Fatalf("no request row for %s: %v", id, err)
	}
	if b != nil {
		batch = *b
	}
	if o != nil {
		outcome = *o
	}
	return
}

func countRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_recrawl_requests`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func skipMap(s []Skip) map[string]string {
	m := map[string]string{}
	for _, k := range s {
		m[k.JobID] = k.Reason
	}
	return m
}

func TestRequestQueuesTheJobsThatCanBeRecrawled(t *testing.T) {
	setup(t)
	addJob(t, "a")
	addJob(t, "b")
	res, err := Request([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if res.BatchID == "" || len(res.Accepted) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("%+v", res)
	}
	for _, id := range []string{"a", "b"} {
		if state, batch, _ := rowOf(t, id); state != StateQueued || batch != res.BatchID {
			t.Errorf("%s: %s %s", id, state, batch)
		}
	}
}

func TestEverySkipReason(t *testing.T) {
	setup(t)
	addJob(t, "ok")
	addJob(t, "busy-queued")
	addJob(t, "busy-running")
	addManual(t, "no-link", "", "https://x/1")
	addManual(t, "no-url", "l1", "")
	addManual(t, "manual-with-link", "l1", "https://x/2")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('busy-queued', 'old', 'x', 'queued'), ('busy-running', 'old', 'x', 'running')`)
	// A job whose link was deleted has no portal either.
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('gone', 'p1', 'https://gone.example')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES ('dangling', 'https://x/3', 'Dangling', 'gone', 'crawled', datetime('now'))`)
	exec(t, `PRAGMA foreign_keys = OFF`)
	exec(t, `DELETE FROM portal_links WHERE id = 'gone'`)
	exec(t, `PRAGMA foreign_keys = ON`)

	res, err := Request([]string{"ok", "missing", "no-link", "no-url", "busy-queued", "busy-running", "manual-with-link", "dangling"})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(res.Accepted)
	if strings.Join(res.Accepted, ",") != "manual-with-link,ok" {
		t.Errorf("accepted %v", res.Accepted)
	}
	want := map[string]string{
		"missing": SkipNoSuchJob, "no-link": SkipNoPortal, "no-url": SkipNoPortal, "dangling": SkipNoPortal,
		"busy-queued": SkipAlreadyBusy, "busy-running": SkipAlreadyBusy,
	}
	got := skipMap(res.Skipped)
	if len(got) != len(want) {
		t.Errorf("skipped %v", got)
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Errorf("%s: %q, want %q", id, got[id], reason)
		}
	}
	// A skipped job gets no row and the busy ones keep their old batch.
	if countRows(t) != 4 { // the two busy rows plus the two accepted
		t.Errorf("rows: %d", countRows(t))
	}
	if _, batch, _ := rowOf(t, "busy-queued"); batch != "old" {
		t.Errorf("a busy request must keep its batch: %s", batch)
	}
}

// A failure half way queues nothing, and the (single) database connection is free again afterwards.
func TestAFailureHalfWayQueuesNothing(t *testing.T) {
	setup(t)
	addJob(t, "a")
	addJob(t, "b")
	addJob(t, "c")
	exec(t, `CREATE TRIGGER stop_b BEFORE INSERT ON job_recrawl_requests WHEN NEW.job_id = 'b' BEGIN SELECT RAISE(ABORT, 'stop'); END`)
	if _, err := Request([]string{"a", "b", "c"}); err == nil {
		t.Fatal("expected the failed insert")
	}
	if countRows(t) != 0 {
		t.Errorf("a failed request left %d rows", countRows(t))
	}
	exec(t, `DROP TRIGGER stop_b`)
	res, err := Request([]string{"a", "b", "c"})
	if err != nil || len(res.Accepted) != 3 {
		t.Errorf("after the fix: %+v %v", res, err)
	}
}

func TestDuplicatesCountOnce(t *testing.T) {
	setup(t)
	addJob(t, "a")
	res, err := Request([]string{"a", "a", "a", "nope", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Accepted) != 1 || len(res.Skipped) != 1 || countRows(t) != 1 {
		t.Errorf("%+v rows=%d", res, countRows(t))
	}
}

func TestAFinishedOrCancelledRequestIsReplaced(t *testing.T) {
	setup(t)
	addJob(t, "done")
	addJob(t, "cancelled")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome, reason, finished_at) VALUES
		('done', 'old', 'x', 'done', 'failed', 'boom', 'y'), ('cancelled', 'old', 'x', 'cancelled', NULL, NULL, 'y')`)
	res, err := Request([]string{"done", "cancelled"})
	if err != nil || len(res.Accepted) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	for _, id := range []string{"done", "cancelled"} {
		state, batch, outcome := rowOf(t, id)
		if state != StateQueued || batch != res.BatchID || outcome != "" {
			t.Errorf("%s: %s %s %q", id, state, batch, outcome)
		}
	}
	var reason, finished *string
	if err := db.JobsDB.QueryRow(`SELECT reason, finished_at FROM job_recrawl_requests WHERE job_id = 'done'`).Scan(&reason, &finished); err != nil || reason != nil || finished != nil {
		t.Errorf("the old reason and finish time must be cleared: %v %v %v", reason, finished, err)
	}
}

// No limit: thousands of ids go in one transaction.
func TestThousandsOfJobsAreQueuedInOneTransaction(t *testing.T) {
	setup(t)
	const n = 3000
	tx, err := db.JobsDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job-%05d", i)
		ids = append(ids, id)
		if _, err := tx.Exec(`INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES (?, ?, ?, 'l1', 'crawled', datetime('now'))`, id, "https://portal.example/"+id, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res, err := Request(ids)
	if err != nil || len(res.Accepted) != n {
		t.Fatalf("accepted %d: %v", len(res.Accepted), err)
	}
	if countRows(t) != n {
		t.Errorf("rows %d", countRows(t))
	}
	var batches int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(DISTINCT batch_id) FROM job_recrawl_requests`).Scan(&batches); err != nil || batches != 1 {
		t.Errorf("one batch id: %d (%v)", batches, err)
	}
	if time.Since(start) > 20*time.Second {
		t.Errorf("too slow: %v", time.Since(start))
	}
}

func TestDeletingAJobDeletesItsRequest(t *testing.T) {
	setup(t)
	addJob(t, "a")
	if _, err := Request([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	exec(t, `DELETE FROM jobs WHERE id = 'a'`)
	if countRows(t) != 0 {
		t.Error("the request must go with the job")
	}
}

func TestStatusThroughEveryState(t *testing.T) {
	setup(t)
	for _, id := range []string{"q", "r", "rf", "un", "fi", "ke", "fa", "sk", "ca"} {
		addJob(t, id)
	}
	addManual(t, "man", "l1", "https://x/m")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome, reason) VALUES
		('q','b1','1','queued',NULL,NULL), ('r','b1','2','running',NULL,NULL),
		('rf','b1','3','done','refreshed',NULL), ('un','b1','4','done','unchanged',NULL), ('fi','b1','5','done','filled',NULL),
		('ke','b1','6','done','kept','typed text'), ('fa','b1','7','done','failed','boom'), ('sk','b1','8','done','skipped','no instructions'),
		('ca','b1','9','cancelled',NULL,NULL), ('man','other','1','queued',NULL,NULL)`)
	st, err := Status("b1", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 9 || st.Queued != 1 || st.Running != 1 || st.Done != 6 || st.Cancelled != 1 {
		t.Errorf("%+v", st)
	}
	if st.Counts != (Counts{Refreshed: 1, Unchanged: 1, Filled: 1, Kept: 1, Failed: 1, Skipped: 1}) {
		t.Errorf("counts %+v", st.Counts)
	}
	if len(st.Jobs) != 9 || st.Jobs[0].JobID != "q" || st.Jobs[6].Reason != "boom" || st.Jobs[6].Outcome != "failed" {
		t.Errorf("jobs %+v", st.Jobs)
	}
	other, err := Status("other", 100, 0)
	if err != nil || other.Total != 1 || !other.Jobs[0].Manual || other.Jobs[0].Title != "Manual man" {
		t.Errorf("%+v %v", other, err)
	}
	if st.Jobs[0].Manual {
		t.Error("a crawled job is not manual")
	}
}

func TestStatusPagesTheJobsButCountsAll(t *testing.T) {
	setup(t)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("j%d", i)
		addJob(t, id)
		exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES (?, 'b', ?, 'queued')`, id, fmt.Sprintf("%d", i))
	}
	st, err := Status("b", 2, 2)
	if err != nil || st.Total != 5 || len(st.Jobs) != 2 || st.Jobs[0].JobID != "j2" || st.Jobs[1].JobID != "j3" {
		t.Errorf("%+v %v", st, err)
	}
	past, err := Status("b", 2, 10)
	if err != nil || past.Total != 5 || len(past.Jobs) != 0 {
		t.Errorf("%+v %v", past, err)
	}
}

func TestStatusOfAnUnknownBatch(t *testing.T) {
	setup(t)
	if _, err := Status("nope", 10, 0); err != ErrUnknownBatch {
		t.Errorf("err %v", err)
	}
}

func TestCancelOnlyTouchesQueuedRequestsOfThatBatch(t *testing.T) {
	setup(t)
	for _, id := range []string{"q1", "q2", "r", "d", "o"} {
		addJob(t, id)
	}
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome) VALUES
		('q1','b','1','queued',NULL), ('q2','b','2','queued',NULL), ('r','b','3','running',NULL), ('d','b','4','done','refreshed'), ('o','other','5','queued',NULL)`)
	n, err := Cancel("b")
	if err != nil || n != 2 {
		t.Fatalf("cancelled %d: %v", n, err)
	}
	want := map[string]string{"q1": StateCancelled, "q2": StateCancelled, "r": StateRunning, "d": StateDone, "o": StateQueued}
	for id, state := range want {
		if got, _, _ := rowOf(t, id); got != state {
			t.Errorf("%s: %s, want %s", id, got, state)
		}
	}
	if again, err := Cancel("b"); err != nil || again != 0 {
		t.Errorf("a second cancel changes nothing: %d %v", again, err)
	}
	if _, err := Cancel("nope"); err != ErrUnknownBatch {
		t.Errorf("unknown batch: %v", err)
	}
}

// ---- the routes ----

type published struct {
	mu     sync.Mutex
	events []string
}

func (p *published) add(topic string, payload any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, _ := payload.(map[string]string)
	p.events = append(p.events, topic+":"+m["job_id"])
}

func (p *published) sorted() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string{}, p.events...)
	sort.Strings(out)
	return out
}

func swapPublish(t *testing.T, f func(string, any)) {
	t.Helper()
	old := publish
	publish = func(topic string, payload any) { f(topic, payload) }
	t.Cleanup(func() { publish = old })
}

func post(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	return rec
}

func TestTheRouteAnswers202ThenSendsOneEventPerJob(t *testing.T) {
	setup(t)
	addJob(t, "a")
	addJob(t, "b")
	addManual(t, "m", "", "")
	var p published
	swapPublish(t, p.add)

	rec := post(t, RequestHandler, `{"jobIds":["a","b","m","gone"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var ans struct {
		BatchID  string
		Accepted int
		Skipped  []Skip
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil || ans.BatchID == "" || ans.Accepted != 2 || len(ans.Skipped) != 2 {
		t.Fatalf("%+v %v", ans, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(p.sorted()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := strings.Join(p.sorted(), ","); got != Topic+":a,"+Topic+":b" {
		t.Errorf("events %s", got)
	}
}

// The answer does not wait for the events: a publisher that hangs does not hold the response.
func TestTheAnswerComesBeforeTheEvents(t *testing.T) {
	setup(t)
	addJob(t, "a")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	swapPublish(t, func(string, any) {
		started <- struct{}{}
		<-release
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- post(t, RequestHandler, `{"jobIds":["a"]}`) }()
	select {
	case rec := <-done:
		if rec.Code != http.StatusAccepted {
			t.Errorf("%d", rec.Code)
		}
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("the response waited for the event")
	}
	close(release)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Error("the event was never published")
	}
}

func TestEventsGoOutAFewAtATime(t *testing.T) {
	setup(t)
	var mu sync.Mutex
	active, peak, total := 0, 0, 0
	swapPublish(t, func(string, any) {
		mu.Lock()
		active++
		total++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
	})
	ids := make([]string, 40)
	for i := range ids {
		ids[i] = fmt.Sprintf("j%d", i)
	}
	publishAll(ids)
	if total != 40 || peak > publishParallelism || peak < 2 {
		t.Errorf("total=%d peak=%d (limit %d)", total, peak, publishParallelism)
	}
}

func TestNothingToRecrawlIs400WithTheReasons(t *testing.T) {
	setup(t)
	addManual(t, "m", "", "")
	var p published
	swapPublish(t, p.add)
	rec := post(t, RequestHandler, `{"jobIds":["m","gone"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%d", rec.Code)
	}
	var ans struct {
		Error   string
		Skipped []Skip
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil || len(ans.Skipped) != 2 || ans.Error == "" {
		t.Errorf("%s", rec.Body)
	}
	time.Sleep(50 * time.Millisecond)
	if len(p.sorted()) != 0 || countRows(t) != 0 {
		t.Error("nothing may be queued or sent")
	}
}

func TestRequestRouteValidation(t *testing.T) {
	setup(t)
	for name, body := range map[string]string{
		"not json": `nope`, "no field": `{}`, "empty list": `{"jobIds":[]}`, "not strings": `{"jobIds":[1,2]}`, "empty id": `{"jobIds":["a",""]}`, "not a list": `{"jobIds":"a"}`,
	} {
		if rec := post(t, RequestHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	RequestHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestStatusRoute(t *testing.T) {
	setup(t)
	addJob(t, "a")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('a','b1','1','queued')`)
	get := func(q string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		StatusHandler(rec, httptest.NewRequest(http.MethodGet, "/?"+q, nil))
		return rec
	}
	rec := get("batchId=b1")
	var st BatchStatus
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &st) != nil || st.Total != 1 || st.Queued != 1 || len(st.Jobs) != 1 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	for q, code := range map[string]int{"": 400, "batchId=nope": 404, "batchId=b1&limit=x": 400, "batchId=b1&offset=x": 400} {
		if got := get(q).Code; got != code {
			t.Errorf("%q: %d, want %d", q, got, code)
		}
	}
	if rec := get("batchId=b1&limit=0"); rec.Code != 200 {
		t.Errorf("a limit of 0 is clamped, not refused: %d", rec.Code)
	}
	if rec := get("batchId=b1&offset=-5"); rec.Code != 200 {
		t.Errorf("a negative offset is clamped: %d", rec.Code)
	}
	bad := httptest.NewRecorder()
	StatusHandler(bad, httptest.NewRequest(http.MethodPost, "/", nil))
	if bad.Code != 405 {
		t.Errorf("POST: %d", bad.Code)
	}
}

func TestCancelRoute(t *testing.T) {
	setup(t)
	addJob(t, "a")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('a','b1','1','queued')`)
	rec := post(t, CancelHandler, `{"batchId":"b1"}`)
	var ans map[string]int
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &ans) != nil || ans["cancelled"] != 1 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	for body, code := range map[string]int{`{"batchId":"nope"}`: 404, `{}`: 400, `nope`: 400} {
		if got := post(t, CancelHandler, body).Code; got != code {
			t.Errorf("%s: %d, want %d", body, got, code)
		}
	}
	bad := httptest.NewRecorder()
	CancelHandler(bad, httptest.NewRequest(http.MethodGet, "/", nil))
	if bad.Code != 405 {
		t.Errorf("GET: %d", bad.Code)
	}
}

// ---- what the worker uses ----

func reasonOf(t *testing.T, id string) (reason, finished string) {
	t.Helper()
	var r, f *string
	if err := db.JobsDB.QueryRow(`SELECT reason, finished_at FROM job_recrawl_requests WHERE job_id = ?`, id).Scan(&r, &f); err != nil {
		t.Fatal(err)
	}
	if r != nil {
		reason = *r
	}
	if f != nil {
		finished = *f
	}
	return
}

func TestClaimTakesOnlyAQueuedRequestOnce(t *testing.T) {
	setup(t)
	for _, id := range []string{"q", "r", "d", "c"} {
		addJob(t, id)
	}
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, reason) VALUES
		('q','b','1','queued','waiting for a signed-in session'), ('r','b','2','running',NULL), ('d','b','3','done',NULL), ('c','b','4','cancelled',NULL)`)
	if ok, err := Claim("q"); err != nil || !ok {
		t.Fatalf("claim q: %v %v", ok, err)
	}
	if state, _, _ := rowOf(t, "q"); state != StateRunning {
		t.Errorf("state %s", state)
	}
	if reason, _ := reasonOf(t, "q"); reason != "" {
		t.Errorf("the waiting reason must be cleared on a claim: %q", reason)
	}
	if ok, _ := Claim("q"); ok {
		t.Error("a second claim must fail")
	}
	for _, id := range []string{"r", "d", "c", "nothing"} {
		if ok, err := Claim(id); ok || err != nil {
			t.Errorf("%s: %v %v", id, ok, err)
		}
	}
}

func TestReleasePutsARunningRequestBackWithAReason(t *testing.T) {
	setup(t)
	addJob(t, "r")
	addJob(t, "d")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('r','b','1','running'), ('d','b','2','done')`)
	if err := Release("r", "waiting"); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := rowOf(t, "r"); state != StateQueued {
		t.Errorf("state %s", state)
	}
	if reason, _ := reasonOf(t, "r"); reason != "waiting" {
		t.Errorf("reason %q", reason)
	}
	if err := Release("r", ""); err != nil { // not running any more: nothing changes
		t.Fatal(err)
	}
	if reason, _ := reasonOf(t, "r"); reason != "waiting" {
		t.Errorf("a release of a queued request changes nothing: %q", reason)
	}
	if err := Release("d", "x"); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := rowOf(t, "d"); state != StateDone {
		t.Errorf("a done request stays done: %s", state)
	}
	// an empty reason is stored as no reason
	exec(t, `UPDATE job_recrawl_requests SET state = 'running', reason = 'old' WHERE job_id = 'r'`)
	if err := Release("r", ""); err != nil {
		t.Fatal(err)
	}
	if reason, _ := reasonOf(t, "r"); reason != "" {
		t.Errorf("reason %q", reason)
	}
}

func TestFinishRecordsTheOutcomeOfARunningRequestOnly(t *testing.T) {
	setup(t)
	for _, id := range []string{"r", "q"} {
		addJob(t, id)
	}
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('r','b','1','running'), ('q','b','2','queued')`)
	if err := Finish("r", OutcomeFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	state, _, outcome := rowOf(t, "r")
	reason, finished := reasonOf(t, "r")
	if state != StateDone || outcome != OutcomeFailed || reason != "boom" || finished == "" {
		t.Errorf("%s %s %q %q", state, outcome, reason, finished)
	}
	if err := Finish("q", OutcomeRefreshed, ""); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := rowOf(t, "q"); state != StateQueued {
		t.Errorf("a queued request is not finished by Finish: %s", state)
	}
	exec(t, `UPDATE job_recrawl_requests SET state = 'running' WHERE job_id = 'q'`)
	if err := Finish("q", OutcomeRefreshed, ""); err != nil {
		t.Fatal(err)
	}
	if reason, _ := reasonOf(t, "q"); reason != "" {
		t.Errorf("no reason is stored as none: %q", reason)
	}
}

func TestDueQueuedAndReviveRunning(t *testing.T) {
	setup(t)
	for _, id := range []string{"old", "fresh", "run", "done"} {
		addJob(t, id)
	}
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Format(time.RFC3339)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('old','b',?, 'queued'), ('fresh','b',?, 'queued'), ('run','b',?, 'running'), ('done','b',?, 'done')`, old, fresh, old, old)
	ids, err := DueQueued(5*time.Minute, 10)
	if err != nil || len(ids) != 1 || ids[0] != "old" {
		t.Errorf("due: %v %v", ids, err)
	}
	n, err := ReviveRunning()
	if err != nil || n != 1 {
		t.Fatalf("revived %d: %v", n, err)
	}
	if state, _, _ := rowOf(t, "run"); state != StateQueued {
		t.Errorf("state %s", state)
	}
	if state, _, _ := rowOf(t, "done"); state != StateDone {
		t.Error("a done request is not revived")
	}
	if again, _ := ReviveRunning(); again != 0 {
		t.Errorf("a second revive %d", again)
	}
}

// The two topics of this feature must be declared in the manifest, or core never delivers them to the consumers (a typo here would
// break the chain without any error).
func TestTheManifestListensToTheTopics(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Listeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	has := func(topic, path string) bool {
		for _, l := range m.Listeners {
			if l.Topic == topic && l.PathSuffix == path {
				return true
			}
		}
		return false
	}
	if Topic != "career.job.recrawl_requested" || DescriptionChangedTopic != "career.job.description_changed" {
		t.Errorf("topics changed: %q %q", Topic, DescriptionChangedTopic)
	}
	if !has("career.job.recrawl_requested", "events/job-recrawl") {
		t.Error("the recrawl consumer's listener is missing from the manifest")
	}
	if !has("career.job.description_changed", "events/job-description-changed-persona-matches") {
		t.Error("the persona consumer's description-changed listener is missing from the manifest")
	}
}

func workerOf(t *testing.T, id string) (started, phase, message string, slot *int) {
	t.Helper()
	var s, p, m *string
	if err := db.JobsDB.QueryRow(`SELECT started_at, worker_slot, phase, phase_message FROM job_recrawl_requests WHERE job_id = ?`, id).Scan(&s, &slot, &p, &m); err != nil {
		t.Fatal(err)
	}
	if s != nil {
		started = *s
	}
	if p != nil {
		phase = *p
	}
	if m != nil {
		message = *m
	}
	return
}

// Plan 194: what the Crawl Monitor shows of a job that is being crawled.
func TestClaimSlotRecordsWhenAndByWhomAJobWasTaken(t *testing.T) {
	setup(t)
	addJob(t, "a")
	addJob(t, "b")
	queueIDs(t, "a", "b")
	if ok, err := ClaimSlot("a", 3); err != nil || !ok {
		t.Fatal(ok, err)
	}
	started, _, _, slot := workerOf(t, "a")
	if started == "" || slot == nil || *slot != 3 {
		t.Errorf("slot %v started %q", slot, started)
	}
	if _, err := time.Parse(time.RFC3339, started); err != nil {
		t.Errorf("started_at is not a time: %q", started)
	}
	// the plain Claim is the main crawler
	if ok, _ := Claim("b"); !ok {
		t.Fatal("claim")
	}
	if _, _, _, slot := workerOf(t, "b"); slot == nil || *slot != 0 {
		t.Errorf("Claim must be slot 0: %v", slot)
	}
	// a job that is not queued is not touched by a second claim
	if ok, _ := ClaimSlot("a", 1); ok {
		t.Error("a running job was claimed again")
	}
	if _, _, _, slot := workerOf(t, "a"); *slot != 3 {
		t.Errorf("slot of a running job changed: %d", *slot)
	}
}

func TestThePhaseIsKeptOnlyWhileTheJobRuns(t *testing.T) {
	setup(t)
	addJob(t, "a")
	queueIDs(t, "a")
	if err := SetPhase("a", "navigating", "too early"); err != nil {
		t.Fatal(err)
	}
	if _, phase, _, _ := workerOf(t, "a"); phase != "" {
		t.Errorf("a queued job got a phase: %q", phase)
	}
	if ok, _ := ClaimSlot("a", 2); !ok {
		t.Fatal("claim")
	}
	if err := SetPhase("a", "navigating", "navigating to https://x"); err != nil {
		t.Fatal(err)
	}
	if _, phase, message, _ := workerOf(t, "a"); phase != "navigating" || message != "navigating to https://x" {
		t.Errorf("phase %q %q", phase, message)
	}
	// a finish keeps when it started, clears the phase, and a late update changes nothing
	startedBefore, _, _, _ := workerOf(t, "a")
	if err := Finish("a", OutcomeRefreshed, ""); err != nil {
		t.Fatal(err)
	}
	started, phase, _, slot := workerOf(t, "a")
	if started != startedBefore || started == "" || phase != "" || slot == nil || *slot != 2 {
		t.Errorf("after the finish: started %q (was %q) phase %q slot %v", started, startedBefore, phase, slot)
	}
	_ = SetPhase("a", "extracting", "late")
	if _, phase, _, _ := workerOf(t, "a"); phase != "" {
		t.Errorf("a late update marked a finished job: %q", phase)
	}
}

func TestAReleaseAndARestartClearWhatAWorkerRecorded(t *testing.T) {
	setup(t)
	addJob(t, "a")
	addJob(t, "b")
	queueIDs(t, "a", "b")
	for i, id := range []string{"a", "b"} {
		if ok, _ := ClaimSlot(id, i+1); !ok {
			t.Fatal("claim", id)
		}
		_ = SetPhase(id, "navigating", "x")
	}
	if err := Release("a", "waiting for a signed-in session"); err != nil {
		t.Fatal(err)
	}
	if n, err := ReviveRunning(); err != nil || n != 1 {
		t.Fatalf("revive: %d %v", n, err)
	}
	for _, id := range []string{"a", "b"} {
		started, phase, message, slot := workerOf(t, id)
		if started != "" || phase != "" || message != "" || slot != nil {
			t.Errorf("%s kept %q %q %q %v", id, started, phase, message, slot)
		}
		if state, _, _ := rowOf(t, id); state != StateQueued {
			t.Errorf("%s is %s", id, state)
		}
	}
}

// queueIDs queues the jobs in one batch.
func queueIDs(t *testing.T, ids ...string) {
	t.Helper()
	res, err := Request(ids)
	if err != nil || len(res.Accepted) != len(ids) {
		t.Fatalf("queue %v: %+v %v", ids, res, err)
	}
}
