package chromeproc

import (
	"container/list"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The limit of crawl sessions (plan 224, step 5): at most MaxSessions Chrome-driving requests run at the same time, whatever engine they use.
// A "session" is one crawl request, counted where it enters the tool (see LimitSessions): a request drives one page at a time, so this is the
// number of pages the tool has open for crawling, and it can never deadlock on a nested acquisition. The login windows and the visible window
// are driven by a person and do not take a slot; the crawl requests that use them do.
//
// When all slots are taken a new request waits in a first-come first-served queue for up to QueueWait and then fails with ErrTooManySessions.
// A session held longer than StuckAfter is treated as stuck: its request is cancelled and its slot given back, so a leak can never lock the tool.

// MaxSessions is the limit. BROWSER_TOOL_MAX_SESSIONS may only lower it.
const MaxSessions = 5

// QueueWait is how long a request waits for a free slot.
const QueueWait = 60 * time.Second

// StuckAfter is how long a session may be held before it is treated as stuck.
const StuckAfter = 15 * time.Minute

// ErrTooManySessions is returned when no slot became free in time.
var ErrTooManySessions = errors.New("too_many_sessions")

// SessionLimit is the limit in force: MaxSessions, or the lower value of BROWSER_TOOL_MAX_SESSIONS (never below 1, never above MaxSessions).
func SessionLimit() int { return limitFrom(os.Getenv("BROWSER_TOOL_MAX_SESSIONS")) }

func limitFrom(env string) int {
	n, err := strconv.Atoi(strings.TrimSpace(env))
	if err != nil || n < 1 || n > MaxSessions {
		return MaxSessions
	}
	return n
}

type session struct {
	owner   string
	since   time.Time
	onStuck func() // cancels the request; may be nil
}

type waiter struct {
	ready chan *session
	owner string
}

// Limiter is a counting semaphore with a first-come first-served queue and a stuck-session reaper.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	running map[*session]struct{}
	queue   *list.List // of *waiter
	now     func() time.Time

	stuckReleased int
}

// NewLimiter makes a limiter with this many slots (at least 1).
func NewLimiter(limit int, now func() time.Time) *Limiter {
	if limit < 1 {
		limit = 1
	}
	if now == nil {
		now = time.Now
	}
	return &Limiter{limit: limit, running: map[*session]struct{}{}, queue: list.New(), now: now}
}

// LimiterStatus is what the status route says about the sessions.
type LimiterStatus struct {
	Limit   int `json:"limit"`
	Running int `json:"running"`
	Waiting int `json:"waiting"`
}

// Status is the number of slots, the sessions running and the requests waiting.
func (l *Limiter) Status() LimiterStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LimiterStatus{Limit: l.limit, Running: len(l.running), Waiting: l.queue.Len()}
}

// Acquire takes a slot for owner, waiting up to wait in line. onStuck (may be nil) is called, once, if the session is held longer than the
// stuck limit; it must cancel the work. The returned release gives the slot back; calling it more than once does nothing.
func (l *Limiter) Acquire(ctx context.Context, owner string, wait time.Duration, onStuck func()) (release func(), err error) {
	l.mu.Lock()
	if len(l.running) < l.limit && l.queue.Len() == 0 {
		s := l.startLocked(owner, onStuck)
		l.mu.Unlock()
		return l.releaser(s), nil
	}
	w := &waiter{ready: make(chan *session, 1), owner: owner}
	el := l.queue.PushBack(w)
	l.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case s := <-w.ready:
		l.mu.Lock()
		s.onStuck = onStuck
		l.mu.Unlock()
		return l.releaser(s), nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = ErrTooManySessions
	}
	// gave up: leave the line; but a slot may have been handed over in the same instant, then it must be passed on
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case s := <-w.ready:
		delete(l.running, s)
		l.handOverLocked()
	default:
		l.queue.Remove(el)
	}
	return nil, err
}

func (l *Limiter) startLocked(owner string, onStuck func()) *session {
	s := &session{owner: owner, since: l.now(), onStuck: onStuck}
	l.running[s] = struct{}{}
	return s
}

func (l *Limiter) releaser(s *session) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if _, ok := l.running[s]; !ok {
				return // already released by the reaper
			}
			delete(l.running, s)
			l.handOverLocked()
		})
	}
}

// handOverLocked gives free slots to the first waiters in line.
func (l *Limiter) handOverLocked() {
	for len(l.running) < l.limit && l.queue.Len() > 0 {
		front := l.queue.Front()
		w := l.queue.Remove(front).(*waiter)
		s := &session{owner: w.owner, since: l.now()}
		l.running[s] = struct{}{}
		w.ready <- s
	}
}

// ReapStuck cancels and releases every session held longer than after, and returns how many. Cancelling runs outside the lock.
func (l *Limiter) ReapStuck(after time.Duration) int {
	l.mu.Lock()
	now := l.now()
	var stuck []*session
	for s := range l.running {
		if now.Sub(s.since) >= after {
			stuck = append(stuck, s)
		}
	}
	for _, s := range stuck {
		delete(l.running, s)
	}
	l.stuckReleased += len(stuck)
	l.handOverLocked()
	l.mu.Unlock()
	for _, s := range stuck {
		if s.onStuck != nil {
			s.onStuck()
		}
	}
	return len(stuck)
}

// StuckReleased is how many sessions the reaper has released so far.
func (l *Limiter) StuckReleased() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stuckReleased
}

// The tool's one limiter.
var sessions = NewLimiter(SessionLimit(), nil)

// Sessions is the status of the tool's limiter.
func Sessions() LimiterStatus { return sessions.Status() }
