package jobmatchconsumer

import (
	profilematch "career-tool-backend/match/profile"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func send(h func(http.ResponseWriter, *http.Request), method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/job-found", strings.NewReader(body)))
	return rec.Code
}

func event(jobID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":"career.job.found","source_kind":"tool","source_id":"career","occurred_at":"2026-06-30T12:00:00Z","payload":{"job_id":%q,"title":"Backend Engineer","company":"","source":"manual"}}`, jobID)
}

// watchBatches replaces the matcher with one that records every batch it is given.
type batches struct {
	mu   sync.Mutex
	list [][]string
}

func (b *batches) sizes() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []int
	for _, l := range b.list {
		out = append(out, len(l))
	}
	return out
}

func watch(t *testing.T, fn func(ids []string) ([]profilematch.AutoMatchOutcome, error)) *batches {
	t.Helper()
	b := &batches{}
	old := autoMatch
	autoMatch = func(ids []string) ([]profilematch.AutoMatchOutcome, error) {
		b.mu.Lock()
		b.list = append(b.list, append([]string(nil), ids...))
		b.mu.Unlock()
		if fn != nil {
			return fn(ids)
		}
		return nil, nil
	}
	t.Cleanup(func() { autoMatch = old })
	return b
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTheRouteQueuesAValidEventAndAnswersAtOnce(t *testing.T) {
	c := New(10)
	if code := send(c.Handler, http.MethodPost, event("job-1")); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if len(c.queue) != 1 || <-c.queue != "job-1" {
		t.Error("the job id was not queued")
	}
	// without a worker nothing is scored: the answer came before any work (the queue is only a channel)
}

func TestTheRouteRefusesWhatItCannotUse(t *testing.T) {
	c := New(10)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":             {http.MethodGet, "", http.StatusMethodNotAllowed},
		"PUT":             {http.MethodPut, event("a"), http.StatusMethodNotAllowed},
		"not json":        {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":      {http.MethodPost, "", http.StatusBadRequest},
		"no payload":      {http.MethodPost, `{"topic":"career.job.found"}`, http.StatusBadRequest},
		"payload not obj": {http.MethodPost, `{"payload":"x"}`, http.StatusBadRequest},
		"empty job id":    {http.MethodPost, `{"payload":{"job_id":""}}`, http.StatusBadRequest},
		"missing job id":  {http.MethodPost, `{"payload":{"title":"t"}}`, http.StatusBadRequest},
		"job id number":   {http.MethodPost, `{"payload":{"job_id":5}}`, http.StatusBadRequest},
	} {
		if code := send(c.Handler, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}
	if len(c.queue) != 0 {
		t.Errorf("%d ids were queued by refused events", len(c.queue))
	}
}

// Only job_id of the payload is used: extra fields, a company, another source are all fine.
func TestOnlyTheJobIDIsUsed(t *testing.T) {
	c := New(10)
	for _, body := range []string{
		`{"payload":{"job_id":"a"}}`,
		`{"payload":{"job_id":"b","title":"T","company":"Acme","source":"crawled","extra":[1,2]}}`,
	} {
		if code := send(c.Handler, http.MethodPost, body); code != http.StatusAccepted {
			t.Errorf("%s: %d", body, code)
		}
	}
	if len(c.queue) != 2 {
		t.Errorf("queued %d", len(c.queue))
	}
}

func TestAFullQueueAnswers503AndKeepsWhatItHas(t *testing.T) {
	c := New(2)
	for _, id := range []string{"a", "b"} {
		if code := send(c.Handler, http.MethodPost, event(id)); code != http.StatusAccepted {
			t.Fatalf("%s: %d", id, code)
		}
	}
	if code := send(c.Handler, http.MethodPost, event("c")); code != http.StatusServiceUnavailable {
		t.Errorf("full queue: %d", code)
	}
	if len(c.queue) != 2 || <-c.queue != "a" || <-c.queue != "b" {
		t.Error("the queue lost or reordered ids")
	}
}

// Waiting ids are taken in batches of 50: 120 ids make batches of 50, 50 and 20, in the order they arrived.
func TestTheWorkerMatchesWaitingIDsInBatches(t *testing.T) {
	b := watch(t, nil)
	c := New(200)
	for i := 0; i < 120; i++ {
		c.queue <- fmt.Sprintf("j%03d", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "all batches", func() bool { return len(b.sizes()) == 3 })
	cancel()
	<-done
	if got := b.sizes(); got[0] != 50 || got[1] != 50 || got[2] != 20 {
		t.Errorf("batch sizes %v, want [50 50 20]", got)
	}
	if b.list[0][0] != "j000" || b.list[0][49] != "j049" || b.list[2][19] != "j119" {
		t.Errorf("order: %v ... %v", b.list[0][:2], b.list[2][18:])
	}
}

// A lone id is matched on its own, without waiting for a batch to fill.
func TestALoneIDIsMatchedAtOnce(t *testing.T) {
	b := watch(t, nil)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	c.queue <- "only"
	waitFor(t, "the lone id", func() bool { return len(b.sizes()) == 1 })
	cancel()
	<-done
	if got := b.sizes(); got[0] != 1 {
		t.Errorf("%v", got)
	}
}

// A batch that panics or fails is logged and the worker goes on with the next one.
func TestTheWorkerSurvivesAPanicAndAnError(t *testing.T) {
	calls := 0
	b := watch(t, func(ids []string) ([]profilematch.AutoMatchOutcome, error) {
		calls++
		switch calls {
		case 1:
			panic("boom")
		case 2:
			return nil, errors.New("profiles unreadable")
		case 3:
			return []profilematch.AutoMatchOutcome{{JobID: ids[0], Status: profilematch.AutoFailed, Err: errors.New("one job failed")}}, nil
		}
		return nil, nil
	})
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	for _, id := range []string{"a", "b", "c", "d"} {
		c.queue <- id
		want := len(b.sizes()) + 1
		waitFor(t, "batch "+id, func() bool { return len(b.sizes()) >= want })
	}
	cancel()
	<-done
	if len(b.sizes()) != 4 {
		t.Errorf("batches %v: the worker stopped after a failure", b.sizes())
	}
}

func TestTheWorkerStopsWhenTheContextIsCancelled(t *testing.T) {
	b := watch(t, nil)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker did not stop")
	}
	c.queue <- "late"
	time.Sleep(50 * time.Millisecond)
	if len(b.sizes()) != 0 {
		t.Error("a stopped worker matched an id")
	}
}

// The whole way with the real matcher and real tables: the event is posted, the worker matches, the job gets its
// best manual match; the same event twice does not change it; an unknown job id and a job nobody can judge leave nothing.
func TestAnEventEndsInAMatchWithTheRealMatcher(t *testing.T) {
	setup(t)
	mk := func(name string, skills ...string) (string, string) {
		pid, err := profile.CreateProfile(name, "Test")
		if err != nil {
			t.Fatal(err)
		}
		per, err := persona.CreatePersona(pid, name+" persona", "")
		if err != nil {
			t.Fatal(err)
		}
		h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
		if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
			t.Fatal(err)
		}
		for _, s := range skills {
			if err := persona.AddCareerSkill(per, s); err != nil {
				t.Fatal(err)
			}
		}
		return pid, per
	}
	mk("Other", "Cobol", "Fortran")
	fit, fitPersona := mk("Fit", "Go", "Docker", "Kubernetes", "PostgreSQL")
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES
		('j1', 'u1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', 'Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung.')`); err != nil {
		t.Fatal(err)
	}
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	defer func() { cancel(); <-done }()

	for _, id := range []string{"j1", "j1", "ghost"} {
		if code := send(c.Handler, http.MethodPost, event(id)); code != http.StatusAccepted {
			t.Fatalf("%s: %d", id, code)
		}
	}
	var profileID, personaID, kind, status string
	var score int
	waitFor(t, "the match", func() bool {
		return db.JobsDB.QueryRow(`SELECT profile_id, persona_id, kind, status, score FROM job_matches WHERE job_id = 'j1'`).
			Scan(&profileID, &personaID, &kind, &status, &score) == nil
	})
	if profileID != fit || personaID != fitPersona || kind != "manual" || status != "completed" || score < 60 {
		t.Errorf("stored: profile %s persona %s kind %s status %s score %d", profileID, personaID, kind, status, score)
	}
	time.Sleep(100 * time.Millisecond) // the duplicate and the ghost have been handled by now
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 1 {
		t.Errorf("%d match rows, want 1", n)
	}
}

// Enqueue (step 152) queues what fits in order, skips blank ids, never blocks on a full queue and says how many it queued.
func TestEnqueueQueuesInOrderSkipsBlanksAndNeverBlocks(t *testing.T) {
	c := New(3)
	if n := c.Enqueue([]string{"a", "", "b"}); n != 2 {
		t.Fatalf("queued %d, want 2 (the blank is skipped)", n)
	}
	done := make(chan int, 1)
	go func() { done <- c.Enqueue([]string{"c", "d", "e"}) }() // one fits, the rest would block
	select {
	case n := <-done:
		if n != 1 {
			t.Errorf("queued %d into the one free place", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
	if len(c.queue) != 3 || <-c.queue != "a" || <-c.queue != "b" || <-c.queue != "c" {
		t.Error("the queue lost or reordered ids")
	}
	if n := c.Enqueue(nil); n != 0 {
		t.Errorf("nothing to queue: %d", n)
	}
}

// What Enqueue queued is matched by the worker like an event's job.
func TestWhatIsEnqueuedIsMatchedByTheWorker(t *testing.T) {
	b := watch(t, nil)
	c := New(10)
	c.Enqueue([]string{"j1", "j2", "j3"})
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "the batch", func() bool { return len(b.sizes()) == 1 })
	cancel()
	<-done
	if got := b.list[0]; len(got) != 3 || got[0] != "j1" || got[2] != "j3" {
		t.Errorf("batch %v", got)
	}
}

func TestThePackageEnqueueUsesTheDefaultConsumer(t *testing.T) {
	before := len(Default.queue)
	if n := Enqueue([]string{"default-queue-test"}); n != 1 || len(Default.queue) != before+1 {
		t.Errorf("queued %d, queue %d -> %d", n, before, len(Default.queue))
	}
	<-Default.queue // no other test uses the default queue, so what is in it is ours: take it out again
}

// ---- a job whose text changed (step 164) ----

func changedEvent(jobID, source string) string {
	return fmt.Sprintf(`{"id":"e2","topic":"career.job.description_changed","payload":{"job_id":%q,"source":%q}}`, jobID, source)
}

// watchChanged replaces the re-matcher with one that records every batch of changed ids it is given.
func watchChanged(t *testing.T, fn func(ids []string) ([]profilematch.ChangedJobOutcome, error)) *batches {
	t.Helper()
	b := &batches{}
	old := rematch
	rematch = func(ids []string) ([]profilematch.ChangedJobOutcome, error) {
		b.mu.Lock()
		b.list = append(b.list, append([]string(nil), ids...))
		b.mu.Unlock()
		if fn != nil {
			return fn(ids)
		}
		return nil, nil
	}
	t.Cleanup(func() { rematch = old })
	return b
}

func TestTheChangedRouteQueuesOnTheSecondQueueOnly(t *testing.T) {
	c := New(10)
	for _, source := range []string{"detail_crawl", "listing_crawl", ""} {
		if code := send(c.ChangedHandler, http.MethodPost, changedEvent("job-"+source, source)); code != http.StatusAccepted {
			t.Fatalf("source %q: %d", source, code)
		}
	}
	if len(c.changed) != 3 || len(c.queue) != 0 {
		t.Errorf("changed %d, new %d", len(c.changed), len(c.queue))
	}
	if code := send(c.Handler, http.MethodPost, event("job-new")); code != http.StatusAccepted || len(c.queue) != 1 || len(c.changed) != 3 {
		t.Errorf("a found event goes on the first queue: %d %d %d", code, len(c.queue), len(c.changed))
	}
}

func TestTheChangedRouteRefusesWhatItCannotUse(t *testing.T) {
	c := New(2)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":          {http.MethodGet, "", http.StatusMethodNotAllowed},
		"not JSON":     {http.MethodPost, "nope", http.StatusBadRequest},
		"no job id":    {http.MethodPost, `{"id":"e","topic":"career.job.description_changed","payload":{"source":"detail_crawl"}}`, http.StatusBadRequest},
		"empty job id": {http.MethodPost, changedEvent("", "detail_crawl"), http.StatusBadRequest},
	} {
		if code := send(c.ChangedHandler, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
	if len(c.changed) != 0 {
		t.Errorf("%d ids queued by refused events", len(c.changed))
	}
	// a full second queue answers 503 and keeps what it has, whatever the first queue holds
	send(c.ChangedHandler, http.MethodPost, changedEvent("a", "x"))
	send(c.ChangedHandler, http.MethodPost, changedEvent("b", "x"))
	if code := send(c.ChangedHandler, http.MethodPost, changedEvent("c", "x")); code != http.StatusServiceUnavailable {
		t.Errorf("full queue: %d", code)
	}
	if len(c.changed) != 2 || len(c.queue) != 0 {
		t.Errorf("%d %d", len(c.changed), len(c.queue))
	}
}

// New jobs go to the matcher, changed jobs to the re-matcher, in one pass when both are waiting; a job that is both is only
// re-matched.
func TestTheWorkerSplitsABatchBetweenNewAndChangedJobs(t *testing.T) {
	found := watch(t, nil)
	changed := watchChanged(t, nil)
	c := New(20)
	c.queue <- "new-1"
	c.queue <- "both"
	c.queue <- "new-2"
	c.changed <- "both"
	c.changed <- "changed-1"
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "both handlers", func() bool { return len(found.sizes()) >= 1 && len(changed.sizes()) >= 1 })
	cancel()
	<-done
	got := func(b *batches) []string {
		b.mu.Lock()
		defer b.mu.Unlock()
		var all []string
		for _, l := range b.list {
			all = append(all, l...)
		}
		slices.Sort(all)
		return all
	}
	if f := got(found); !slices.Equal(f, []string{"new-1", "new-2"}) {
		t.Errorf("new jobs matched: %v", f)
	}
	if c := got(changed); !slices.Equal(c, []string{"both", "changed-1"}) {
		t.Errorf("changed jobs re-matched: %v", c)
	}
}

func TestAChangedIDAloneIsRematchedAtOnceAndNeverMatchedAsNew(t *testing.T) {
	found := watch(t, nil)
	changed := watchChanged(t, nil)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	c.changed <- "only"
	waitFor(t, "the re-match", func() bool { return len(changed.sizes()) == 1 })
	cancel()
	<-done
	if len(found.sizes()) != 0 {
		t.Errorf("the matcher for new jobs must not be called: %v", found.sizes())
	}
	if got := changed.sizes(); got[0] != 1 {
		t.Errorf("%v", got)
	}
}

// A batch is fifty ids in all, across both queues.
func TestABatchIsFiftyAcrossBothQueues(t *testing.T) {
	found := watch(t, nil)
	changed := watchChanged(t, nil)
	c := New(200)
	for i := 0; i < 40; i++ {
		c.queue <- fmt.Sprintf("n%03d", i)
	}
	for i := 0; i < 40; i++ {
		c.changed <- fmt.Sprintf("c%03d", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "all 80", func() bool {
		n := 0
		for _, s := range append(found.sizes(), changed.sizes()...) {
			n += s
		}
		return n == 80
	})
	cancel()
	<-done
	// no single call may hold more than 50 ids, and the two kinds are never mixed in one call
	for _, s := range append(found.sizes(), changed.sizes()...) {
		if s > 50 {
			t.Errorf("a call held %d ids", s)
		}
	}
}

func TestAFailingOrPanickingRematchDoesNotStopTheWorker(t *testing.T) {
	calls := 0
	changed := watchChanged(t, func(ids []string) ([]profilematch.ChangedJobOutcome, error) {
		calls++
		switch calls {
		case 1:
			panic("boom")
		case 2:
			return nil, errors.New("profiles unreadable")
		case 3:
			return []profilematch.ChangedJobOutcome{{JobID: ids[0], Status: profilematch.ChangedJobFailed, Err: errors.New("one job failed")}}, nil
		}
		return nil, nil
	})
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	for i := 1; i <= 4; i++ {
		c.changed <- fmt.Sprintf("j%d", i)
		waitFor(t, fmt.Sprintf("call %d", i), func() bool { return len(changed.sizes()) == i })
	}
	cancel()
	<-done
	if len(changed.sizes()) != 4 {
		t.Errorf("%v", changed.sizes())
	}
}

// The consumer is declared for the second topic, routed, and its first listener is untouched.
func TestTheChangedListenerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../../manifest.json")
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
	count := func(topic, path string) int {
		n := 0
		for _, l := range m.Listeners {
			if l.Topic == topic && l.PathSuffix == path {
				n++
			}
		}
		return n
	}
	if count("career.job.description_changed", "events/job-description-changed-job-match") != 1 {
		t.Error("manifest.json must declare career.job.description_changed -> events/job-description-changed-job-match exactly once")
	}
	for _, keep := range [][2]string{
		{"career.job.found", "events/job-found"},
		{"career.job.description_changed", "events/job-description-changed-persona-matches"},
		{"career.job.description_changed", "events/job-description-changed-profile-rules"},
	} {
		if count(keep[0], keep[1]) != 1 {
			t.Errorf("the listener %v must stay", keep)
		}
	}
	mainSrc, err := os.ReadFile("../../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/events/job-description-changed-job-match", jobmatchconsumer.ChangedEventHandler`) {
		t.Error("main.go does not route /events/job-description-changed-job-match")
	}
}

// One pass takes fifty ids in all, counted across both queues: the first one and 49 more, whichever queue they wait on.
func TestADrainTakesFiftyInAllAcrossBothQueues(t *testing.T) {
	c := New(200)
	for i := 0; i < 60; i++ {
		c.queue <- fmt.Sprintf("n%03d", i)
		c.changed <- fmt.Sprintf("c%03d", i)
	}
	b := c.drain(batch{found: []string{"first"}})
	if b.size() != 50 {
		t.Errorf("a pass must hold exactly 50 ids, it held %d (%d new, %d changed)", b.size(), len(b.found), len(b.changed))
	}
	if left := len(c.queue) + len(c.changed); left != 121-50 {
		t.Errorf("%d ids are left waiting, want %d", left, 121-50)
	}
	if b.found[0] != "first" {
		t.Errorf("the first id leads: %v", b.found[:1])
	}
}
