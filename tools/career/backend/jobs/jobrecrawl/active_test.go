package jobrecrawl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActiveIsEmptyListsNotNullWhenNothingRuns(t *testing.T) {
	setup(t)
	rec := httptest.NewRecorder()
	ActiveHandler(rec, httptest.NewRequest(http.MethodGet, "/jobs/recrawl/active", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"batches":[]`) || !strings.Contains(rec.Body.String(), `"workers":[]`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	// finished batches are not "active"
	addJob(t, "a")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome) VALUES ('a', 'old', '2026-11-06T10:00:00Z', 'done', 'refreshed')`)
	got, err := FindActive()
	if err != nil || len(got.Batches) != 0 || len(got.Workers) != 0 {
		t.Errorf("%+v %v", got, err)
	}
}

func TestActiveNamesTheBatchesAndTheJobsBeingCrawled(t *testing.T) {
	setup(t) // p1 / l1
	exec(t, `UPDATE portals SET name = 'Stepstone' WHERE id = 'p1'`)
	exec(t, `UPDATE portal_links SET title = 'Software jobs' WHERE id = 'l1'`)
	for _, id := range []string{"w0", "w1", "w2", "q1", "q2", "d1", "c1"} {
		addJob(t, id)
	}
	// batch b1: 3 running (slots 0 to 2, started in this order), 2 queued, 1 done, 1 cancelled
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, started_at, worker_slot, phase, phase_message) VALUES
		('w1', 'b1', '2026-11-06T10:00:00Z', 'running', '2026-11-06T10:00:07Z', 1, 'navigating', 'navigating to https://portal.example/jobs/w1'),
		('w0', 'b1', '2026-11-06T10:00:00Z', 'running', '2026-11-06T10:00:05Z', 0, NULL, NULL),
		('w2', 'b1', '2026-11-06T10:00:00Z', 'running', '2026-11-06T10:00:09Z', 2, 'extracting', 'extracting page 1')`)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('q1', 'b1', '2026-11-06T10:00:00Z', 'queued'), ('q2', 'b1', '2026-11-06T10:00:00Z', 'queued'), ('c1', 'b1', '2026-11-06T10:00:00Z', 'cancelled')`)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome) VALUES ('d1', 'b1', '2026-11-06T10:00:00Z', 'done', 'refreshed')`)

	got, err := FindActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Batches) != 1 {
		t.Fatalf("batches: %+v", got.Batches)
	}
	b := got.Batches[0]
	if b.BatchID != "b1" || b.Total != 7 || b.Done != 1 || b.Queued != 2 || b.Running != 3 || b.Cancelled != 1 || b.Stopping || b.StartedAt != "2026-11-06T10:00:00Z" {
		t.Errorf("batch: %+v", b)
	}
	if len(got.Workers) != 3 || got.Workers[0].JobID != "w0" || got.Workers[1].JobID != "w1" || got.Workers[2].JobID != "w2" {
		t.Fatalf("workers (oldest started first): %+v", got.Workers)
	}
	w := got.Workers[1]
	if w.JobTitle != "Job w1" || w.PortalName != "Stepstone" || w.PortalID != "p1" || w.PortalLinkID != "l1" || w.PortalLinkTitle != "Software jobs" ||
		w.URL != "https://portal.example/jobs/w1" || w.Slot != 1 || w.StartedAt != "2026-11-06T10:00:07Z" || w.Phase != "navigating" || w.Message != "navigating to https://portal.example/jobs/w1" {
		t.Errorf("worker: %+v", w)
	}
	// a worker that waits for the platform's turn has said nothing yet
	if g := got.Workers[0]; g.Phase != "" || g.Message != "" || g.Slot != 0 {
		t.Errorf("silent worker: %+v", g)
	}
}

func TestActiveSaysWhenABatchIsBeingStopped(t *testing.T) {
	setup(t)
	for _, id := range []string{"w", "c1", "c2"} {
		addJob(t, id)
	}
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, started_at, worker_slot) VALUES ('w', 'b1', '2026-11-06T10:00:00Z', 'running', '2026-11-06T10:00:05Z', 0)`)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('c1', 'b1', '2026-11-06T10:00:00Z', 'cancelled'), ('c2', 'b1', '2026-11-06T10:00:00Z', 'cancelled')`)
	got, err := FindActive()
	if err != nil || len(got.Batches) != 1 || !got.Batches[0].Stopping {
		t.Errorf("%+v %v", got, err)
	}
	// once the last crawl has finished the batch is no longer active
	exec(t, `UPDATE job_recrawl_requests SET state = 'done', outcome = 'refreshed' WHERE job_id = 'w'`)
	if got, _ := FindActive(); len(got.Batches) != 0 || len(got.Workers) != 0 {
		t.Errorf("a finished batch is still active: %+v", got)
	}
}

func TestActiveOrdersTheBatchesAndSurvivesOddRows(t *testing.T) {
	setup(t)
	for _, id := range []string{"a", "b", "gone"} {
		addJob(t, id)
	}
	addManual(t, "nolink", "", "https://by.hand/1") // a job with no platform
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('a', 'second', '2026-11-06T11:00:00Z', 'queued'), ('b', 'first', '2026-11-06T10:00:00Z', 'queued')`)
	// an old row: running with no slot or start time (written before the columns existed)
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('nolink', 'first', '2026-11-06T10:00:00Z', 'running')`)
	got, err := FindActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Batches) != 2 || got.Batches[0].BatchID != "first" || got.Batches[1].BatchID != "second" {
		t.Errorf("batches: %+v", got.Batches)
	}
	if len(got.Workers) != 1 {
		t.Fatalf("workers: %+v", got.Workers)
	}
	w := got.Workers[0]
	if w.JobID != "nolink" || w.Slot != -1 || w.StartedAt != "2026-11-06T10:00:00Z" || w.PortalName != "" || w.PortalLinkID != "" {
		t.Errorf("odd worker: %+v", w)
	}
	// deleting a job takes its request, so it leaves the list
	exec(t, `DELETE FROM jobs WHERE id = 'nolink'`)
	if got, _ := FindActive(); len(got.Workers) != 0 {
		t.Errorf("a deleted job is still listed: %+v", got.Workers)
	}
}

func TestActiveHandlerAnswersJSONAndRefusesOtherMethods(t *testing.T) {
	setup(t)
	addJob(t, "a")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, started_at, worker_slot) VALUES ('a', 'b1', '2026-11-06T10:00:00Z', 'running', '2026-11-06T10:00:05Z', 4)`)
	rec := httptest.NewRecorder()
	ActiveHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var got Active
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || len(got.Workers) != 1 || got.Workers[0].Slot != 4 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("the answer must not be cached")
	}
	rec = httptest.NewRecorder()
	ActiveHandler(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}
