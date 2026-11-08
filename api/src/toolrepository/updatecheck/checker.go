// Package updatecheck refreshes every tool repository in the background and
// works out which installed tools have an update — see
// plan/ai/tools/repository/check-for-updates.md.
//
// A check is "refresh every repository", one after another in a single
// goroutine (SQLite is opened without a busy timeout, so writes are never
// concurrent). Whether a tool has an update is always computed from the stored
// catalogs (Updates), so it is correct while a run is going and after a restart.
// The run state itself lives in memory only.
package updatecheck

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
)

const (
	StateIdle    = "idle"
	StateRunning = "running"
	StateDone    = "done"

	TriggerLaunch = "launch"
	TriggerManual = "manual"

	// DefaultDeadline bounds a whole run.
	DefaultDeadline = 5 * time.Minute
	lockedRetries   = 4

	// DefaultPause is how long after a run ends before another may start.
	DefaultPause = 5 * time.Second
)

// Repositories is what a check needs from the repository service.
type Repositories interface {
	List() ([]*repo_entity.Repository, error)
	Refresh(ctx context.Context, id string) (*repo_entity.Repository, error)
}

// Failure is one repository that could not be checked.
type Failure struct {
	RepositoryID string `json:"repositoryId"`
	Name         string `json:"name"`
	Error        string `json:"error"`
}

// Check is the state of the latest run.
type Check struct {
	State      string    `json:"state"`
	RunID      string    `json:"runId"`
	Trigger    string    `json:"trigger"`
	StartedAt  string    `json:"startedAt"`
	FinishedAt string    `json:"finishedAt"`
	Total      int       `json:"totalRepositories"`
	Checked    int       `json:"checkedRepositories"`
	Failed     []Failure `json:"failed"`
	// Error is set when the run itself broke (for example a panic), as opposed to
	// single repositories failing.
	Error string `json:"error"`
}

// Checker runs one check at a time.
type Checker struct {
	repos     Repositories
	ctx       context.Context
	cancel    context.CancelFunc
	deadline  time.Duration
	pause     time.Duration
	retryWait time.Duration
	now       func() time.Time
	onDone    func() // test hook, called after a run's state is final

	mu         sync.Mutex
	cur        Check
	lastFinish time.Time
}

// New makes a Checker. Cancelling parent (or calling Stop) cancels a running check.
func New(parent context.Context, repos Repositories) *Checker {
	ctx, cancel := context.WithCancel(parent)
	return &Checker{
		repos: repos, ctx: ctx, cancel: cancel,
		deadline: DefaultDeadline, pause: DefaultPause, retryWait: 250 * time.Millisecond, now: time.Now,
		cur: Check{State: StateIdle, Failed: []Failure{}},
	}
}

// WithPause sets how long after a run ends another may start (tests use 0).
func (c *Checker) WithPause(d time.Duration) *Checker { c.pause = d; return c }

// WithDeadline sets the bound on a whole run.
func (c *Checker) WithDeadline(d time.Duration) *Checker { c.deadline = d; return c }

// Stop cancels a running check; used at shutdown.
func (c *Checker) Stop() { c.cancel() }

// Snapshot returns the latest run's state.
func (c *Checker) Snapshot() Check {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.copyLocked()
}

func (c *Checker) copyLocked() Check {
	out := c.cur
	out.Failed = append([]Failure{}, c.cur.Failed...)
	return out
}

// Start begins a check unless one is running or one ended less than the pause
// ago; it returns the state to show and whether a new run was started. A refused
// start is not an error: the caller shows the running or just-finished run.
func (c *Checker) Start(trigger string) (Check, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur.State == StateRunning {
		return c.copyLocked(), false
	}
	if !c.lastFinish.IsZero() && c.now().Sub(c.lastFinish) < c.pause {
		return c.copyLocked(), false
	}
	c.cur = Check{
		State: StateRunning, RunID: uuid.NewString(), Trigger: trigger,
		StartedAt: stamp(c.now()), Failed: []Failure{},
	}
	runID := c.cur.RunID
	started := c.copyLocked()
	go c.run(runID)
	return started, true
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (c *Checker) run(runID string) {
	defer func() {
		if r := recover(); r != nil {
			c.finish(runID, fmt.Sprintf("the check stopped unexpectedly: %v", r))
			return
		}
		c.finish(runID, "")
	}()

	repos, err := c.repos.List()
	if err != nil {
		c.update(runID, func(ch *Check) { ch.Error = "could not read the repositories" })
		return
	}
	c.update(runID, func(ch *Check) { ch.Total = len(repos) })

	runCtx, cancel := context.WithTimeout(c.ctx, c.deadline)
	defer cancel()
	for _, repo := range repos {
		if runCtx.Err() != nil {
			reason := "the check ran out of time"
			if c.ctx.Err() != nil {
				reason = "the app is shutting down"
			}
			c.update(runID, func(ch *Check) {
				ch.Checked++
				ch.Failed = append(ch.Failed, Failure{repo.ID, repo.Name, "not checked: " + reason})
			})
			continue
		}
		failure := c.checkOne(runCtx, repo)
		c.update(runID, func(ch *Check) {
			ch.Checked++
			if failure != nil {
				ch.Failed = append(ch.Failed, *failure)
			}
		})
	}
}

// checkOne refreshes one repository. Refresh records its own failure on the row
// and still returns the repository, so a "refresh failed" status counts too.
func (c *Checker) checkOne(ctx context.Context, repo *repo_entity.Repository) *Failure {
	refreshed, err := c.refreshWithRetry(ctx, repo.ID)
	switch {
	case err != nil:
		return &Failure{repo.ID, repo.Name, err.Error()}
	case refreshed != nil && refreshed.LastStatus == repo_entity.StatusError:
		return &Failure{repo.ID, refreshed.Name, refreshed.LastError}
	}
	return nil
}

// refreshWithRetry retries a refresh that failed only because another connection
// held SQLite's write lock (the database is opened without a busy timeout, and a
// tool's own status write can land at the same moment), a few times and briefly.
func (c *Checker) refreshWithRetry(ctx context.Context, id string) (*repo_entity.Repository, error) {
	var repo *repo_entity.Repository
	var err error
	for attempt := 0; attempt < lockedRetries; attempt++ {
		repo, err = c.repos.Refresh(ctx, id)
		if err == nil || !strings.Contains(err.Error(), "database is locked") {
			return repo, err
		}
		select {
		case <-ctx.Done():
			return repo, err
		case <-time.After(c.retryWait):
		}
	}
	return repo, err
}

// update changes the state of run runID only; a stale run never touches a newer one.
func (c *Checker) update(runID string, fn func(*Check)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur.RunID == runID {
		fn(&c.cur)
	}
}

func (c *Checker) finish(runID, runError string) {
	c.mu.Lock()
	if c.cur.RunID == runID {
		c.cur.State = StateDone
		c.cur.FinishedAt = stamp(c.now())
		if runError != "" {
			c.cur.Error = runError
		}
		c.lastFinish = c.now()
	}
	hook := c.onDone
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
}
