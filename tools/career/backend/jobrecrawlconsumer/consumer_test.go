package jobrecrawlconsumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/crawl"
	"career-tool-backend/db"
	"career-tool-backend/jobrecrawl"
	"career-tool-backend/jobs"
)

const validInstructions = "fields:\n  - label: description\n    selector: \".description\"\n"

// fakeBrowser stands in for the Browser tool behind core: it answers the crawl call of a job with the description set for its URL.
type fakeBrowser struct {
	mu       sync.Mutex
	pages    map[string]string // posting URL -> description the page gives ("" gives no description)
	status   int               // an error status to answer with instead
	delay    time.Duration
	calls    []string
	active   int
	peak     int
	tokens   []string
	requests []map[string]any
}

func (f *fakeBrowser) handler(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/crawl-status") {
		http.NotFound(w, r)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	url, _ := body["url"].(string)
	f.calls = append(f.calls, url)
	f.tokens = append(f.tokens, r.Header.Get("Authorization"))
	f.requests = append(f.requests, body)
	f.active++
	if f.active > f.peak {
		f.peak = f.active
	}
	text, status, delay := f.pages[url], f.status, f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "browser trouble", status)
		return
	}
	results := map[string]any{}
	if text != "" {
		results["description"] = text
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"pages": []any{map[string]any{"url": url, "results": results}}})
}

func (f *fakeBrowser) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// sentEvents records the events the worker publishes, with the state of the request at that moment.
type sentEvent struct{ topic, jobID, stateThen string }

var (
	sentMu     sync.Mutex
	sentEvents []sentEvent
)

func eventsSent() []sentEvent {
	sentMu.Lock()
	defer sentMu.Unlock()
	return append([]sentEvent{}, sentEvents...)
}

func setup(t *testing.T) *fakeBrowser {
	t.Helper()
	sentMu.Lock()
	sentEvents = nil
	sentMu.Unlock()
	oldPublish := publish
	publish = func(topic string, payload any) {
		m, _ := payload.(map[string]string)
		var state string
		_ = db.JobsDB.QueryRow(`SELECT state FROM job_recrawl_requests WHERE job_id = ?`, m["job_id"]).Scan(&state)
		sentMu.Lock()
		sentEvents = append(sentEvents, sentEvent{topic, m["job_id"], state})
		sentMu.Unlock()
	}
	t.Cleanup(func() { publish = oldPublish })
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	fb := &fakeBrowser{pages: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(fb.handler))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)

	oldToken, oldTimeout := cachedToken, crawlTimeout
	cachedToken = func() (string, bool) { return "session-token", true }
	t.Cleanup(func() { cachedToken, crawlTimeout = oldToken, oldTimeout })

	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Portal')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url, job_detail_crawl_instructions) VALUES ('l1', 'p1', 'https://portal.example/search', ?)`, validInstructions)
	return fb
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// addJob inserts a job of link l1 with the given source and description; its posting URL is https://portal.example/jobs/<id>.
func addJob(t *testing.T, id, source, description string) string {
	t.Helper()
	url := "https://portal.example/jobs/" + id
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, description, crawled_at) VALUES (?, ?, ?, 'l1', ?, ?, datetime('now'))`, id, url, "Job "+id, source, description)
	return url
}

func queue(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES (?, 'b', ?, 'queued')`, id, time.Now().UTC().Format(time.RFC3339))
	}
}

type req struct{ state, outcome, reason string }

func requestOf(t *testing.T, id string) req {
	t.Helper()
	var r req
	var o, why *string
	if err := db.JobsDB.QueryRow(`SELECT state, outcome, reason FROM job_recrawl_requests WHERE job_id = ?`, id).Scan(&r.state, &o, &why); err != nil {
		t.Fatalf("no request for %s: %v", id, err)
	}
	if o != nil {
		r.outcome = *o
	}
	if why != nil {
		r.reason = *why
	}
	return r
}

func descriptionOf(t *testing.T, id string) string {
	t.Helper()
	var d *string
	if err := db.JobsDB.QueryRow(`SELECT description FROM jobs WHERE id = ?`, id).Scan(&d); err != nil {
		t.Fatal(err)
	}
	if d == nil {
		return ""
	}
	return *d
}

func detailStatusOf(t *testing.T, id string) string {
	t.Helper()
	var s *string
	if err := db.JobsDB.QueryRow(`SELECT detail_crawl_status FROM jobs WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s == nil {
		return ""
	}
	return *s
}

// ---- the outcomes ----

func TestARefreshedDescriptionIsSavedAsPlainText(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "a", "crawled", "old text")
	fb.pages[url] = "<style>.a{top:0}</style>New text\n\n<p>second</p>"
	queue(t, "a")
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.state != "done" || got.outcome != jobrecrawl.OutcomeRefreshed {
		t.Errorf("%+v", got)
	}
	if got := descriptionOf(t, "a"); got != "New text\n\nsecond" {
		t.Errorf("description %q", got)
	}
	if detailStatusOf(t, "a") != "success" {
		t.Error("the detail crawl is a success")
	}
	if fb.tokens[0] != "Bearer session-token" {
		t.Errorf("the crawl must use the cached session: %v", fb.tokens)
	}
	if rl, _ := fb.requests[0]["rateLimitKey"].(string); rl != "p1" {
		t.Errorf("paced per portal: %v", fb.requests[0]["rateLimitKey"])
	}
}

func TestTheSameTextIsUnchanged(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "a", "crawled", "Same text")
	fb.pages[url] = "Same text"
	queue(t, "a")
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.state != "done" || got.outcome != jobrecrawl.OutcomeUnchanged {
		t.Errorf("%+v", got)
	}
}

func TestAManualJobWithoutADescriptionIsFilled(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "m", "manual", "")
	fb.pages[url] = "Found on the page"
	queue(t, "m")
	Process(context.Background(), "m")
	if got := requestOf(t, "m"); got.outcome != jobrecrawl.OutcomeFilled {
		t.Errorf("%+v", got)
	}
	if descriptionOf(t, "m") != "Found on the page" {
		t.Errorf("description %q", descriptionOf(t, "m"))
	}
}

// A manual job that has its own text is kept as it is, and its page is not even visited.
func TestAManualJobWithItsOwnTextIsKeptWithoutAVisit(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "m", "manual", "my own notes")
	fb.pages[url] = "page text"
	queue(t, "m")
	Process(context.Background(), "m")
	got := requestOf(t, "m")
	if got.state != "done" || got.outcome != jobrecrawl.OutcomeKept || got.reason == "" {
		t.Errorf("%+v", got)
	}
	if fb.callCount() != 0 {
		t.Error("the page of a manual job with a description must not be visited")
	}
	if descriptionOf(t, "m") != "my own notes" {
		t.Error("typed text must stay")
	}
}

func TestAPageWithoutADescriptionFailsAndKeepsTheOldText(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "a", "crawled", "old text")
	fb.pages[url] = ""
	queue(t, "a")
	Process(context.Background(), "a")
	got := requestOf(t, "a")
	if got.state != "done" || got.outcome != jobrecrawl.OutcomeFailed || !strings.Contains(got.reason, "no description") {
		t.Errorf("%+v", got)
	}
	if descriptionOf(t, "a") != "old text" || detailStatusOf(t, "a") != "failed" {
		t.Errorf("old text must stay and the crawl count as failed: %q %q", descriptionOf(t, "a"), detailStatusOf(t, "a"))
	}
}

func TestAMarkupOnlyPageFailsLikeAnEmptyOne(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "a", "crawled", "old text")
	fb.pages[url] = "<style>.a{top:0}</style><img src=\"x\"/>"
	queue(t, "a")
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.outcome != jobrecrawl.OutcomeFailed || descriptionOf(t, "a") != "old text" {
		t.Errorf("%+v %q", got, descriptionOf(t, "a"))
	}
}

func TestABrowserErrorFailsWithItsMessage(t *testing.T) {
	fb := setup(t)
	addJob(t, "a", "crawled", "old text")
	fb.status = http.StatusInternalServerError
	queue(t, "a")
	Process(context.Background(), "a")
	got := requestOf(t, "a")
	if got.state != "done" || got.outcome != jobrecrawl.OutcomeFailed || !strings.Contains(got.reason, "browser trouble") || strings.Contains(got.reason, "no description") {
		t.Errorf("the reason must be the browser's own message: %+v", got)
	}
	if descriptionOf(t, "a") != "old text" || detailStatusOf(t, "a") != "failed" {
		t.Error("old text stays, the crawl is marked failed")
	}
}

func TestAJobThatCannotBeCrawledIsSkippedWithTheReason(t *testing.T) {
	fb := setup(t)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('noinstr', 'p1', 'https://n.example')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url, job_detail_crawl_instructions) VALUES ('bad', 'p1', 'https://b.example', 'fields: []')`)
	for _, c := range []struct{ id, link, want string }{
		{"no-instructions", "noinstr", "no job detail crawl instructions"},
		{"bad-instructions", "bad", "not valid"},
		{"no-link", "", "no portal"},
		{"no-url", "l1", "no portal"},
		{"dangling", "gone", "no longer exists"},
	} {
		var link any
		if c.link != "" {
			link = c.link
		}
		url := "https://x/" + c.id
		if c.id == "no-url" {
			url = ""
		}
		if c.link == "gone" {
			exec(t, `PRAGMA foreign_keys = OFF`)
		}
		exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, description, crawled_at) VALUES (?, ?, ?, ?, 'crawled', 'old', datetime('now'))`, c.id, url, c.id, link)
		exec(t, `PRAGMA foreign_keys = ON`)
		queue(t, c.id)
		Process(context.Background(), c.id)
		got := requestOf(t, c.id)
		if got.state != "done" || got.outcome != jobrecrawl.OutcomeSkipped || !strings.Contains(got.reason, c.want) {
			t.Errorf("%s: %+v (want reason with %q)", c.id, got, c.want)
		}
	}
	if fb.callCount() != 0 {
		t.Error("nothing to crawl with, so no page may be visited")
	}
}

// With no fresh session the job waits (queued, with the reason) and is crawled once a session is seen.
func TestWithoutASessionTheJobWaits(t *testing.T) {
	fb := setup(t)
	url := addJob(t, "a", "crawled", "old text")
	fb.pages[url] = "new text"
	queue(t, "a")
	for name, tok := range map[string]func() (string, bool){
		"none": func() (string, bool) { return "", false }, "stale": func() (string, bool) { return "old-token", false },
		"empty but called fresh": func() (string, bool) { return "", true },
	} {
		cachedToken = tok
		Process(context.Background(), "a")
		got := requestOf(t, "a")
		if got.state != "queued" || got.reason != WaitingForSession || got.outcome != "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if fb.callCount() != 0 {
		t.Error("no crawl without a session")
	}
	cachedToken = func() (string, bool) { return "fresh", true }
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.state != "done" || got.outcome != jobrecrawl.OutcomeRefreshed || got.reason != "" {
		t.Errorf("once a session is seen: %+v", got)
	}
}

// A manual job with its own text needs no session at all to be kept.
func TestKeepingAManualJobNeedsNoSession(t *testing.T) {
	setup(t)
	addJob(t, "m", "manual", "notes")
	queue(t, "m")
	cachedToken = func() (string, bool) { return "", false }
	Process(context.Background(), "m")
	if got := requestOf(t, "m"); got.state != "done" || got.outcome != jobrecrawl.OutcomeKept {
		t.Errorf("%+v", got)
	}
}

func TestRequestsThatAreNotQueuedAreDropped(t *testing.T) {
	fb := setup(t)
	for _, id := range []string{"cancelled", "done", "running"} {
		fb.pages[addJob(t, id, "crawled", "old")] = "new"
	}
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome) VALUES
		('cancelled','b','x','cancelled',NULL), ('done','b','x','done','unchanged'), ('running','b','x','running',NULL)`)
	for _, id := range []string{"cancelled", "done", "running", "no-request-at-all"} {
		Process(context.Background(), id)
	}
	if fb.callCount() != 0 {
		t.Error("none of these may be crawled")
	}
	for id, state := range map[string]string{"cancelled": "cancelled", "done": "done", "running": "running"} {
		if got := requestOf(t, id); got.state != state {
			t.Errorf("%s changed to %s", id, got.state)
		}
	}
}

func TestADeletedJobIsDropped(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old")] = "new"
	queue(t, "a")
	exec(t, `DELETE FROM jobs WHERE id = 'a'`)
	Process(context.Background(), "a")
	if fb.callCount() != 0 {
		t.Error("a deleted job is not crawled")
	}
}

// A job whose request was claimed but whose row then vanished (deleted while crawling) is simply dropped.
func TestAJobDeletedWhileLoadingIsDropped(t *testing.T) {
	setup(t)
	addJob(t, "a", "crawled", "old")
	queue(t, "a")
	old := loadTarget
	loadTarget = func(id string) (jobs.RecrawlTarget, error) { return jobs.RecrawlTarget{}, jobs.ErrUnknownJob }
	t.Cleanup(func() { loadTarget = old })
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.state != "running" {
		t.Errorf("nothing is recorded for a vanished job: %+v", got)
	}
}

func TestARedeliveredEventDoesNotCrawlTwice(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old")] = "new"
	queue(t, "a")
	Process(context.Background(), "a")
	Process(context.Background(), "a")
	Process(context.Background(), "a")
	if fb.callCount() != 1 {
		t.Errorf("crawled %d times", fb.callCount())
	}
}

func TestACrawlThatTakesTooLongFails(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old text")] = "new"
	fb.delay = 2 * time.Second
	crawlTimeout = 150 * time.Millisecond
	queue(t, "a")
	start := time.Now()
	Process(context.Background(), "a")
	if time.Since(start) > 1500*time.Millisecond {
		t.Errorf("took %v", time.Since(start))
	}
	got := requestOf(t, "a")
	if got.state != "done" || got.outcome != jobrecrawl.OutcomeFailed || !strings.Contains(got.reason, "too long") {
		t.Errorf("%+v", got)
	}
	if descriptionOf(t, "a") != "old text" {
		t.Error("old text stays")
	}
}

// When the tool is shutting down the job was not done: it goes back to the queue for the next start.
func TestAJobCutOffByShutdownGoesBackToTheQueue(t *testing.T) {
	setup(t)
	addJob(t, "a", "crawled", "old")
	queue(t, "a")
	old := recrawl
	ctx, cancel := context.WithCancel(context.Background())
	recrawl = func(c context.Context, _ jobs.RecrawlTarget, _ string) (bool, error) {
		cancel()
		return false, c.Err()
	}
	t.Cleanup(func() { recrawl = old })
	Process(ctx, "a")
	if got := requestOf(t, "a"); got.state != "queued" || got.outcome != "" {
		t.Errorf("%+v", got)
	}
}

func TestNotPossibleIsRecognisedThroughTheSeam(t *testing.T) {
	setup(t)
	addJob(t, "a", "crawled", "old")
	queue(t, "a")
	old := recrawl
	recrawl = func(context.Context, jobs.RecrawlTarget, string) (bool, error) {
		return false, fmt.Errorf("wrapped: %w", &crawl.NotPossibleError{Reason: "nothing to crawl it with"})
	}
	t.Cleanup(func() { recrawl = old })
	Process(context.Background(), "a")
	if got := requestOf(t, "a"); got.outcome != jobrecrawl.OutcomeSkipped || got.reason != "nothing to crawl it with" {
		t.Errorf("%+v", got)
	}
}

func TestOutcomeFor(t *testing.T) {
	for _, c := range []struct {
		name          string
		source        string
		before, after string
		want          string
	}{
		{"changed", "crawled", "a", "b", jobrecrawl.OutcomeRefreshed},
		{"same", "crawled", "a", "a", jobrecrawl.OutcomeUnchanged},
		{"crawled had nothing", "crawled", "", "b", jobrecrawl.OutcomeRefreshed},
		{"manual had nothing", "manual", "", "b", jobrecrawl.OutcomeFilled},
		{"manual had only blanks", "manual", "  \n", "b", jobrecrawl.OutcomeFilled},
	} {
		if got := outcomeFor(jobs.RecrawlTarget{Source: c.source, Description: c.before}, c.after); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// ---- the route ----

func event(jobID string) string {
	b, _ := json.Marshal(map[string]any{"topic": "career.job.recrawl_requested", "payload": map[string]string{"job_id": jobID}})
	return string(b)
}

func postEvent(c *Consumer, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c.Handler(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	return rec
}

func TestTheRouteQueuesTheJobAndAnswers202(t *testing.T) {
	c := New(10)
	if rec := postEvent(c, event("job-1")); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	select {
	case id := <-c.queue:
		if id != "job-1" {
			t.Errorf("queued %q", id)
		}
	default:
		t.Error("nothing queued")
	}
}

func TestAnIdIsQueuedOnce(t *testing.T) {
	c := New(10)
	for i := 0; i < 3; i++ {
		if rec := postEvent(c, event("job-1")); rec.Code != http.StatusAccepted {
			t.Fatalf("%d", rec.Code)
		}
	}
	if len(c.queue) != 1 {
		t.Errorf("queued %d times", len(c.queue))
	}
	<-c.queue
	c.take("job-1")
	postEvent(c, event("job-1"))
	if len(c.queue) != 1 {
		t.Error("an id can be queued again once the worker has taken it")
	}
}

func TestRouteValidationAndAFullQueue(t *testing.T) {
	c := New(1)
	for name, body := range map[string]string{
		"not json": "nope", "no payload": `{}`, "empty id": event(""), "payload not an object": `{"payload":"x"}`,
	} {
		if rec := postEvent(c, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := postEvent(c, event("one")); rec.Code != 202 {
		t.Fatal(rec.Code)
	}
	if rec := postEvent(c, event("two")); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("a full queue: %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	c.Handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

// ---- the worker ----

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTheWorkerCrawlsTheQueuedJobsOneAfterTheOther(t *testing.T) {
	fb := setup(t)
	fb.delay = 20 * time.Millisecond
	ids := []string{"a", "b", "c", "d", "e"}
	for _, id := range ids {
		fb.pages[addJob(t, id, "crawled", "old")] = "new " + id
	}
	queue(t, ids...)
	c := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	for _, id := range ids {
		postEvent(c, event(id))
	}
	waitFor(t, "all jobs done", func() bool {
		for _, id := range ids {
			if requestOf(t, id).state != "done" {
				return false
			}
		}
		return true
	})
	cancel()
	<-done
	if fb.peak != 1 {
		t.Errorf("crawls overlapped: peak %d", fb.peak)
	}
	for _, id := range ids {
		if got := requestOf(t, id); got.outcome != jobrecrawl.OutcomeRefreshed || descriptionOf(t, id) != "new "+id {
			t.Errorf("%s: %+v %q", id, got, descriptionOf(t, id))
		}
	}
}

func TestAPanicInOneJobDoesNotStopTheWorker(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "boom", "crawled", "old")] = "x"
	fb.pages[addJob(t, "fine", "crawled", "old")] = "new"
	queue(t, "boom", "fine")
	old := recrawl
	recrawl = func(ctx context.Context, tg jobs.RecrawlTarget, token string) (bool, error) {
		if tg.ID == "boom" {
			panic("kaboom")
		}
		return old(ctx, tg, token)
	}
	t.Cleanup(func() { recrawl = old })
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	postEvent(c, event("boom"))
	postEvent(c, event("fine"))
	waitFor(t, "both done", func() bool { return requestOf(t, "boom").state == "done" && requestOf(t, "fine").state == "done" })
	cancel()
	<-done
	if got := requestOf(t, "boom"); got.outcome != jobrecrawl.OutcomeFailed || got.reason != "internal error" {
		t.Errorf("boom: %+v", got)
	}
	if got := requestOf(t, "fine"); got.outcome != jobrecrawl.OutcomeRefreshed {
		t.Errorf("fine: %+v", got)
	}
}

// A restart leaves requests running that nobody is working on: the worker puts them back and takes them.
func TestStartingTheWorkerRevivesWhatARestartCutOff(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old")] = "new"
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('a', 'b', ?, 'running')`, old)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "the revived job done", func() bool { return requestOf(t, "a").state == "done" })
	cancel()
	<-done
	if requestOf(t, "a").outcome != jobrecrawl.OutcomeRefreshed {
		t.Errorf("%+v", requestOf(t, "a"))
	}
}

func TestACancelledBatchIsNotCrawledByTheWorker(t *testing.T) {
	fb := setup(t)
	for _, id := range []string{"a", "b"} {
		fb.pages[addJob(t, id, "crawled", "old")] = "new"
	}
	queue(t, "a", "b")
	if _, err := jobrecrawl.Cancel("b"); err != nil {
		t.Fatal(err)
	}
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	postEvent(c, event("a"))
	postEvent(c, event("b"))
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done
	if fb.callCount() != 0 {
		t.Errorf("a cancelled batch must not be crawled: %d calls", fb.callCount())
	}
}

func TestTheWorkerStopsWhenItsContextIsCancelled(t *testing.T) {
	setup(t)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the worker did not stop")
	}
}

// ---- the persona scores are made again after a change (step 148, step 6) ----

func TestARefreshedJobSendsDescriptionChangedBeforeItIsDone(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old text")] = "new text"
	queue(t, "a")
	Process(context.Background(), "a")
	sent := eventsSent()
	if len(sent) != 1 || sent[0].topic != jobrecrawl.DescriptionChangedTopic || sent[0].jobID != "a" {
		t.Fatalf("events %+v", sent)
	}
	if sent[0].stateThen != jobrecrawl.StateRunning {
		t.Errorf("the event must go out before the request is marked done, it was %q", sent[0].stateThen)
	}
	if got := requestOf(t, "a"); got.outcome != jobrecrawl.OutcomeRefreshed {
		t.Errorf("%+v", got)
	}
}

func TestAFilledJobSendsDescriptionChanged(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "m", "manual", "")] = "from the page"
	queue(t, "m")
	Process(context.Background(), "m")
	if sent := eventsSent(); len(sent) != 1 || sent[0].jobID != "m" || sent[0].topic != jobrecrawl.DescriptionChangedTopic {
		t.Errorf("events %+v", sent)
	}
}

// Nothing changed, so nothing to score again: no event for any other outcome.
func TestOtherOutcomesSendNoEvent(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "same", "crawled", "same text")] = "same text"
	addJob(t, "kept", "manual", "typed")
	fb.pages[addJob(t, "nodesc", "crawled", "old")] = ""
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('noinstr', 'p1', 'https://n.example')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, description, crawled_at) VALUES ('skipme', 'https://n.example/1', 'x', 'noinstr', 'crawled', 'old', datetime('now'))`)
	queue(t, "same", "kept", "nodesc", "skipme")
	for _, id := range []string{"same", "kept", "nodesc", "skipme"} {
		Process(context.Background(), id)
	}
	// a waiting job (no session) too
	fb.pages[addJob(t, "waits", "crawled", "old")] = "new"
	queue(t, "waits")
	cachedToken = func() (string, bool) { return "", false }
	Process(context.Background(), "waits")
	if sent := eventsSent(); len(sent) != 0 {
		t.Errorf("no event expected: %+v", sent)
	}
}

func TestAFailedCrawlSendsNoEvent(t *testing.T) {
	fb := setup(t)
	addJob(t, "a", "crawled", "old")
	fb.status = http.StatusInternalServerError
	queue(t, "a")
	Process(context.Background(), "a")
	if sent := eventsSent(); len(sent) != 0 {
		t.Errorf("events %+v", sent)
	}
}
