package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
)

// fakeRepos records refreshes and can block, fail or panic per repository.
type fakeRepos struct {
	mu        sync.Mutex
	repos     []*repo_entity.Repository
	refreshed []string
	errs      map[string]error  // Refresh returns this error
	rowErr    map[string]string // Refresh succeeds but the row says "error"
	block     chan struct{}     // when set, Refresh waits for it (or the context)
	panicOn   string
	listErr   error
	lockedFor map[string]int // Refresh fails with a lock error this many times first
}

func (f *fakeRepos) List() ([]*repo_entity.Repository, error) { return f.repos, f.listErr }

func (f *fakeRepos) Refresh(ctx context.Context, id string) (*repo_entity.Repository, error) {
	f.mu.Lock()
	f.refreshed = append(f.refreshed, id)
	block, err, rowErr, pan := f.block, f.errs[id], f.rowErr[id], f.panicOn == id
	if f.lockedFor[id] > 0 {
		f.lockedFor[id]--
		f.mu.Unlock()
		return nil, errors.New("database is locked")
	}
	f.mu.Unlock()
	if pan {
		panic("boom")
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	r := &repo_entity.Repository{ID: id, Name: "repo-" + id, LastStatus: repo_entity.StatusOK}
	if rowErr != "" {
		r.LastStatus, r.LastError = repo_entity.StatusError, rowErr
	}
	return r, nil
}

func repoList(ids ...string) []*repo_entity.Repository {
	out := make([]*repo_entity.Repository, len(ids))
	for i, id := range ids {
		out[i] = &repo_entity.Repository{ID: id, Name: "repo-" + id}
	}
	return out
}

// newChecker returns a checker whose runs signal on done.
func newChecker(t *testing.T, f *fakeRepos) (*Checker, chan struct{}) {
	t.Helper()
	c := New(context.Background(), f)
	t.Cleanup(c.Stop)
	done := make(chan struct{}, 16)
	c.onDone = func() { done <- struct{}{} }
	return c, done
}

func waitDone(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the check did not finish")
	}
}

func TestInitialStateIsIdle(t *testing.T) {
	c, _ := newChecker(t, &fakeRepos{})
	s := c.Snapshot()
	if s.State != StateIdle || s.RunID != "" || s.Failed == nil || len(s.Failed) != 0 {
		t.Errorf("%+v", s)
	}
}

func TestARunRefreshesEveryRepositoryInOrder(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b", "c")}
	c, done := newChecker(t, f)
	started, ok := c.Start(TriggerLaunch)
	if !ok || started.State != StateRunning || started.Trigger != TriggerLaunch || started.RunID == "" || started.StartedAt == "" {
		t.Fatalf("%v %+v", ok, started)
	}
	waitDone(t, done)
	s := c.Snapshot()
	if s.State != StateDone || s.Total != 3 || s.Checked != 3 || len(s.Failed) != 0 || s.FinishedAt == "" || s.Error != "" {
		t.Errorf("%+v", s)
	}
	if fmt.Sprint(f.refreshed) != "[a b c]" {
		t.Errorf("order: %v", f.refreshed)
	}
}

func TestAFailingRepositoryDoesNotStopTheRest(t *testing.T) {
	f := &fakeRepos{
		repos:  repoList("a", "b", "c", "d"),
		errs:   map[string]error{"b": errors.New("the server could not be reached")},
		rowErr: map[string]string{"c": "the repository file is not valid"},
	}
	c, done := newChecker(t, f)
	c.Start(TriggerManual)
	waitDone(t, done)
	s := c.Snapshot()
	if s.Checked != 4 || len(s.Failed) != 2 || fmt.Sprint(f.refreshed) != "[a b c d]" {
		t.Fatalf("%+v %v", s, f.refreshed)
	}
	if s.Failed[0].RepositoryID != "b" || s.Failed[0].Error != "the server could not be reached" ||
		s.Failed[1].RepositoryID != "c" || s.Failed[1].Error != "the repository file is not valid" {
		t.Errorf("%+v", s.Failed)
	}
}

func TestStartingTwiceStartsOneRun(t *testing.T) {
	f := &fakeRepos{repos: repoList("a"), block: make(chan struct{})}
	c, done := newChecker(t, f)
	first, ok1 := c.Start(TriggerLaunch)
	second, ok2 := c.Start(TriggerManual)
	if !ok1 || ok2 || second.RunID != first.RunID || second.State != StateRunning || second.Trigger != TriggerLaunch {
		t.Fatalf("%v %v %+v %+v", ok1, ok2, first, second)
	}
	close(f.block)
	waitDone(t, done)
	if len(f.refreshed) != 1 {
		t.Errorf("refreshed %d times", len(f.refreshed))
	}
}

func TestConcurrentStartsStartExactlyOneRun(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b"), block: make(chan struct{})}
	c, done := newChecker(t, f)
	var wg sync.WaitGroup
	var mu sync.Mutex
	started := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := c.Start(TriggerManual); ok {
				mu.Lock()
				started++
				mu.Unlock()
			}
			_ = c.Snapshot()
		}()
	}
	wg.Wait()
	close(f.block)
	waitDone(t, done)
	if started != 1 {
		t.Errorf("%d runs started", started)
	}
}

func TestAStartRightAfterAFinishIsRefusedWithTheFinishedState(t *testing.T) {
	f := &fakeRepos{repos: repoList("a")}
	c, done := newChecker(t, f)
	clock := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return clock }
	c.Start(TriggerLaunch)
	waitDone(t, done)
	first := c.Snapshot()

	clock = clock.Add(2 * time.Second)
	got, ok := c.Start(TriggerManual)
	if ok || got.State != StateDone || got.RunID != first.RunID {
		t.Fatalf("within the pause: %v %+v", ok, got)
	}
	clock = clock.Add(10 * time.Second)
	got, ok = c.Start(TriggerManual)
	if !ok || got.RunID == first.RunID || got.State != StateRunning || got.Trigger != TriggerManual {
		t.Fatalf("after the pause: %v %+v", ok, got)
	}
	waitDone(t, done)
}

func TestTheDeadlineMarksUnreachedRepositories(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b", "c"), block: make(chan struct{})}
	c, done := newChecker(t, f)
	c.deadline = 100 * time.Millisecond
	c.Start(TriggerManual)
	waitDone(t, done)
	s := c.Snapshot()
	if s.State != StateDone || s.Checked != 3 || len(s.Failed) != 3 {
		t.Fatalf("%+v", s)
	}
	if s.Failed[1].Error != "not checked: the check ran out of time" {
		t.Errorf("%+v", s.Failed)
	}
	if len(f.refreshed) != 1 {
		t.Errorf("only the first was tried: %v", f.refreshed)
	}
}

func TestStopCancelsARunningCheck(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b"), block: make(chan struct{})}
	c, done := newChecker(t, f)
	c.Start(TriggerLaunch)
	time.Sleep(50 * time.Millisecond)
	c.Stop()
	waitDone(t, done)
	s := c.Snapshot()
	if s.State != StateDone || len(s.Failed) != 2 || s.Failed[1].Error != "not checked: the app is shutting down" {
		t.Errorf("%+v", s)
	}
}

func TestAPanicIsRecoveredAndEndsTheRunAsFailed(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b"), panicOn: "a"}
	c, done := newChecker(t, f)
	c.Start(TriggerLaunch)
	waitDone(t, done)
	s := c.Snapshot()
	if s.State != StateDone || s.Error != "the check stopped unexpectedly: boom" {
		t.Errorf("%+v", s)
	}
	// and the checker still works afterwards
	c.pause = 0
	f.panicOn = ""
	if _, ok := c.Start(TriggerManual); !ok {
		t.Error("a new run must be possible after a panic")
	}
	waitDone(t, done)
}

func TestAnUnreadableRepositoryListEndsTheRunWithAnError(t *testing.T) {
	c, done := newChecker(t, &fakeRepos{listErr: errors.New("db")})
	c.Start(TriggerLaunch)
	waitDone(t, done)
	if s := c.Snapshot(); s.State != StateDone || s.Error != "could not read the repositories" {
		t.Errorf("%+v", s)
	}
}

func TestNoRepositoriesIsAnImmediateCleanRun(t *testing.T) {
	c, done := newChecker(t, &fakeRepos{})
	c.Start(TriggerLaunch)
	waitDone(t, done)
	if s := c.Snapshot(); s.State != StateDone || s.Total != 0 || s.Checked != 0 || len(s.Failed) != 0 {
		t.Errorf("%+v", s)
	}
}

func TestSnapshotsAreCopies(t *testing.T) {
	f := &fakeRepos{repos: repoList("a"), errs: map[string]error{"a": errors.New("x")}}
	c, done := newChecker(t, f)
	c.Start(TriggerLaunch)
	waitDone(t, done)
	s := c.Snapshot()
	s.Failed[0].Error = "changed"
	if c.Snapshot().Failed[0].Error != "x" {
		t.Error("a snapshot must not alias the checker's state")
	}
}

func TestALockedDatabaseIsRetriedAndThenSucceeds(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b"), lockedFor: map[string]int{"a": 2}}
	c, done := newChecker(t, f)
	c.retryWait = time.Millisecond
	c.Start(TriggerLaunch)
	waitDone(t, done)
	s := c.Snapshot()
	if len(s.Failed) != 0 || s.Checked != 2 {
		t.Errorf("%+v", s)
	}
	if fmt.Sprint(f.refreshed) != "[a a a b]" {
		t.Errorf("a was tried three times: %v", f.refreshed)
	}
}

func TestALockThatNeverClearsIsReportedAfterAFewTries(t *testing.T) {
	f := &fakeRepos{repos: repoList("a", "b"), lockedFor: map[string]int{"a": 100}}
	c, done := newChecker(t, f)
	c.retryWait = time.Millisecond
	c.Start(TriggerLaunch)
	waitDone(t, done)
	s := c.Snapshot()
	if len(s.Failed) != 1 || s.Failed[0].Error != "database is locked" || s.Checked != 2 {
		t.Errorf("%+v", s)
	}
	if n := len(f.refreshed); n != lockedRetries+1 { // four tries of a, one of b
		t.Errorf("%d refreshes: %v", n, f.refreshed)
	}
}
