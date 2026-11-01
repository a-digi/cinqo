package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/deletedjobs"
)

// rawJob inserts a job straight into the table: crawled (with a posting URL) or manual.
func rawJob(t *testing.T, id, source string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, source, crawled_at) VALUES (?, ?, ?, 'Acme', ?, datetime('now'))`, id, "https://portal.example/jobs/"+id, "Job "+id, source)
}

func countJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestDeleteManyRemovesTheJobsAndRemembersTheCrawledOnes(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "c1", "crawled")
	rawJob(t, "c2", "crawled")
	rawJob(t, "m1", "manual")
	rawJob(t, "keep", "crawled")

	res, err := DeleteMany([]string{"c1", "c2", "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 3 || res.NotFound != 0 || len(res.Events) != 3 {
		t.Fatalf("%+v", res)
	}
	if exists(t, "c1") || exists(t, "c2") || exists(t, "m1") || !exists(t, "keep") {
		t.Error("exactly the chosen jobs must be gone")
	}
	mem := memory(t)
	if len(mem) != 2 {
		t.Fatalf("the two crawled jobs are remembered, the manual one and the kept one are not: %+v", mem)
	}
	for _, e := range mem {
		if e.reason != deletedjobs.ReasonDeleted {
			t.Errorf("reason %q", e.reason)
		}
	}
	if has, _ := deletedjobs.Has("https://portal.example/jobs/c1"); !has {
		t.Error("c1 must be remembered by its URL")
	}
	if has, _ := deletedjobs.Has("https://portal.example/jobs/m1"); has {
		t.Error("a job added by hand is not remembered")
	}
}

func TestIdsThatDoNotExistAreCountedAndLeaveNoMemory(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "c1", "crawled")
	res, err := DeleteMany([]string{"c1", "ghost-1", "ghost-2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.NotFound != 2 || len(res.Events) != 1 {
		t.Errorf("%+v", res)
	}
	if len(memory(t)) != 1 {
		t.Errorf("only the real job is remembered: %+v", memory(t))
	}
	if again, err := DeleteMany([]string{"c1"}); err != nil || again.Deleted != 0 || again.NotFound != 1 || len(again.Events) != 0 {
		t.Errorf("deleting twice: %+v %v", again, err)
	}
}

func TestADuplicateIdCountsOnce(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	res, err := DeleteMany([]string{"a", "a", "a", "ghost", "ghost"})
	if err != nil || res.Deleted != 1 || res.NotFound != 1 || len(res.Events) != 1 {
		t.Errorf("%+v %v", res, err)
	}
}

func TestAnEmptyListDeletesNothing(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	if res, err := DeleteMany(nil); err != nil || res.Deleted != 0 || res.NotFound != 0 {
		t.Errorf("%+v %v", res, err)
	}
	if !exists(t, "a") {
		t.Error("nothing may be deleted")
	}
}

// More than one batch: no limit, several transactions.
func TestMoreThanOneBatchIsDeleted(t *testing.T) {
	setupJobsDB(t)
	const n = 3*deleteBatchSize/2 + deleteBatchSize // 500
	tx, err := db.JobsDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job-%04d", i)
		ids = append(ids, id)
		src := "crawled"
		if i%5 == 0 {
			src = "manual"
		}
		if _, err := tx.Exec(`INSERT INTO jobs (id, source_url, title, source, crawled_at) VALUES (?, ?, ?, ?, datetime('now'))`, id, "https://portal.example/jobs/"+id, id, src); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rawJob(t, "survivor", "crawled")
	res, err := DeleteMany(ids)
	if err != nil || res.Deleted != n || res.NotFound != 0 || len(res.Events) != n {
		t.Fatalf("%+v %v", res, err)
	}
	if countJobs(t) != 1 || !exists(t, "survivor") {
		t.Errorf("%d jobs left", countJobs(t))
	}
	if want := n - n/5; len(memory(t)) != want {
		t.Errorf("remembered %d, want %d (the crawled ones)", len(memory(t)), want)
	}
}

// A failure in a later batch keeps the earlier batches, reports what was done, and takes back the memory of the batch that failed.
func TestAFailureInALaterBatchKeepsTheEarlierOnes(t *testing.T) {
	setupJobsDB(t)
	const n = 2*deleteBatchSize + 50 // the third batch is the short one
	ids := make([]string, 0, n)
	tx, _ := db.JobsDB.Begin()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job-%04d", i)
		ids = append(ids, id)
		if _, err := tx.Exec(`INSERT INTO jobs (id, source_url, title, source, crawled_at) VALUES (?, ?, ?, 'crawled', datetime('now'))`, id, "https://portal.example/jobs/"+id, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	exec(t, `CREATE TRIGGER stop_late BEFORE DELETE ON jobs WHEN OLD.id = 'job-0430' BEGIN SELECT RAISE(ABORT, 'stop'); END`)

	res, err := DeleteMany(ids)
	if err == nil {
		t.Fatal("expected the failed batch")
	}
	if res.Deleted != 2*deleteBatchSize || len(res.Events) != 2*deleteBatchSize {
		t.Errorf("the two committed batches count: %+v", res)
	}
	if countJobs(t) != 50 {
		t.Errorf("%d jobs left, want the 50 of the failed batch", countJobs(t))
	}
	if len(memory(t)) != 2*deleteBatchSize {
		t.Errorf("the failed batch's memory must be taken back: %d entries", len(memory(t)))
	}
}

// An entry written before the delete is taken back for a job whose delete then removed nothing.
func TestTheMemoryIsTakenBackForAJobThatWasNotDeleted(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "stays", "crawled")
	rawJob(t, "goes", "crawled")
	exec(t, `CREATE TRIGGER skip_one BEFORE DELETE ON jobs WHEN OLD.id = 'stays' BEGIN SELECT RAISE(IGNORE); END`)
	res, err := DeleteMany([]string{"stays", "goes"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.NotFound != 1 || !exists(t, "stays") || exists(t, "goes") {
		t.Fatalf("%+v", res)
	}
	mem := memory(t)
	if len(mem) != 1 || mem[0].url != "https://portal.example/jobs/goes" {
		t.Errorf("only the deleted job is remembered: %+v", mem)
	}
}

func TestTheJobsOwnRowsGoWithIt(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	rawJob(t, "b", "crawled")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('a', 'x', '1', 'queued'), ('b', 'x', '2', 'queued')`)
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p1', 'a', 'fixed-not-reviewed'), ('p1', 'b', 'fixed-not-reviewed')`)
	exec(t, `INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('r1', 'a', 'Go', 1), ('r2', 'b', 'SQL', 1)`)
	if _, err := DeleteMany([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"job_recrawl_requests", "job_board_entries", "job_requirements"} {
		var n int
		if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s: %d rows left, want only the other job's (%v)", table, n, err)
		}
	}
	// The card that went with the job is not remembered as "taken off the board on purpose": the job itself is gone.
	var removals int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_removals`).Scan(&removals); err != nil || removals != 0 {
		t.Errorf("removals remembered: %d (%v)", removals, err)
	}
}

// The deleted event says the same as the single delete's.
func TestTheEventsSayWhatTheSingleDeleteSays(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	single := seedJob(t, "Platform Engineer", "Acme GmbH")
	many := seedJob(t, "Platform Engineer 2", "Acme GmbH")
	if err := DeleteJob(single); err != nil {
		t.Fatal(err)
	}
	res, err := DeleteMany([]string{many})
	if err != nil {
		t.Fatal(err)
	}
	PublishDeletedEvents(res.Events)
	got := events.ofTopic("career.job.deleted")
	if len(got) != 2 {
		t.Fatalf("events %d", len(got))
	}
	a, b := payloadOf(got[0]), payloadOf(got[1])
	if a["job_id"] != single || b["job_id"] != many {
		t.Fatalf("%v %v", a, b)
	}
	if len(a) != len(b) || len(a) != 5 {
		t.Errorf("the payloads must have the same keys: %v vs %v", a, b)
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			t.Errorf("key %q missing from the batch delete's payload", k)
		}
	}
	if a["company"] != b["company"] || a["source"] != b["source"] || a["company_id"] != b["company_id"] {
		t.Errorf("payloads differ: %v vs %v", a, b)
	}
}

// ---- the route ----

func post(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	return rec
}

type deletedEvents struct {
	mu  sync.Mutex
	ids []string
}

func (d *deletedEvents) add(ev DeletedJob) {
	d.mu.Lock()
	d.ids = append(d.ids, ev.ID)
	d.mu.Unlock()
}

func (d *deletedEvents) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.ids)
}

func swapPublishDeleted(t *testing.T, f func(DeletedJob)) {
	t.Helper()
	old := publishDeleted
	publishDeleted = f
	t.Cleanup(func() { publishDeleted = old })
}

func waitCount(t *testing.T, d *deletedEvents, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for d.count() < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if d.count() != n {
		t.Fatalf("events: %d, want %d", d.count(), n)
	}
}

func TestTheRouteDeletesAndThenAnnouncesEachJob(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	rawJob(t, "b", "manual")
	var sent deletedEvents
	swapPublishDeleted(t, sent.add)
	rec := post(DeleteManyHandler, `{"jobIds":["a","b","ghost","a"]}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var ans map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil || ans["deleted"] != 2 || ans["notFound"] != 1 {
		t.Errorf("%s", rec.Body)
	}
	waitCount(t, &sent, 2)
	if exists(t, "a") || exists(t, "b") {
		t.Error("the jobs must be gone")
	}
}

// The answer does not wait for the events.
func TestTheAnswerComesBeforeTheEvents(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	swapPublishDeleted(t, func(DeletedJob) {
		started <- struct{}{}
		<-release
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- post(DeleteManyHandler, `{"jobIds":["a"]}`) }()
	select {
	case rec := <-done:
		if rec.Code != 200 {
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

func TestDeletedEventsGoOutAFewAtATime(t *testing.T) {
	setupJobsDB(t)
	var mu sync.Mutex
	active, peak, total := 0, 0, 0
	swapPublishDeleted(t, func(DeletedJob) {
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
	events := make([]DeletedJob, 40)
	for i := range events {
		events[i] = DeletedJob{ID: fmt.Sprintf("j%d", i)}
	}
	PublishDeletedEvents(events)
	if total != 40 || peak > 4 || peak < 2 {
		t.Errorf("total=%d peak=%d (at most four at a time)", total, peak)
	}
}

// A failure part way answers 500 with the counts, and still announces what was deleted.
func TestAFailurePartWayAnswers500WithTheCountsAndStillAnnounces(t *testing.T) {
	setupJobsDB(t)
	ids := make([]string, 0, deleteBatchSize+10)
	tx, _ := db.JobsDB.Begin()
	for i := 0; i < deleteBatchSize+10; i++ {
		id := fmt.Sprintf("job-%04d", i)
		ids = append(ids, id)
		if _, err := tx.Exec(`INSERT INTO jobs (id, source_url, title, source, crawled_at) VALUES (?, ?, ?, 'crawled', datetime('now'))`, id, "https://portal.example/jobs/"+id, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	exec(t, `CREATE TRIGGER stop_late BEFORE DELETE ON jobs WHEN OLD.id = 'job-0205' BEGIN SELECT RAISE(ABORT, 'stop'); END`)
	var sent deletedEvents
	swapPublishDeleted(t, sent.add)
	body, _ := json.Marshal(map[string]any{"jobIds": append([]string{"ghost-1", "ghost-2", "ghost-3"}, ids...)})
	rec := post(DeleteManyHandler, string(body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("%d", rec.Code)
	}
	var ans struct {
		Error    string
		Deleted  int
		NotFound int
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil || ans.Error == "" || ans.Deleted != deleteBatchSize-3 || ans.NotFound != 3 {
		t.Errorf("%s", rec.Body)
	}
	waitCount(t, &sent, deleteBatchSize-3)
}

// The body is capped (a safety against a broken client, not a limit on a batch): one that is far too big is refused.
func TestAnOversizedBodyIsRefused(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	big := `{"jobIds":["a","` + strings.Repeat("x", 9<<20) + `"]}`
	if rec := post(DeleteManyHandler, big); rec.Code != http.StatusBadRequest {
		t.Errorf("%d", rec.Code)
	}
	if !exists(t, "a") {
		t.Error("nothing may be deleted by a refused request")
	}
}

func TestTheRouteValidatesItsInput(t *testing.T) {
	setupJobsDB(t)
	rawJob(t, "a", "crawled")
	for name, body := range map[string]string{
		"not json": "nope", "no field": `{}`, "empty list": `{"jobIds":[]}`, "not strings": `{"jobIds":[1,2]}`, "empty id": `{"jobIds":["a",""]}`, "not a list": `{"jobIds":"a"}`,
	} {
		if rec := post(DeleteManyHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	DeleteManyHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
	if !exists(t, "a") {
		t.Error("a refused request deletes nothing")
	}
}
