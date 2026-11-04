package livelog

import (
	"sync"
	"time"
)

// FollowerCap bounds how many different logs one person follows at once. A tab left open polls for hours, so without a bound one person
// could keep any number of logs read for ever. A log counts as followed while it was polled within the window; a poll of a log that is
// already followed never counts twice; once the window passes without a poll the place is free again, so no one has to "close" anything.
// It keeps a little memory per active person and nothing for anyone who stopped.
type FollowerCap struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	users  map[string]map[string]time.Time
}

const (
	// DefaultMaxFollowers is how many different logs one person may follow at once.
	DefaultMaxFollowers = 5
	// DefaultFollowWindow is how long a log stays "followed" after its last poll: three of the slowest poll interval (5 s).
	DefaultFollowWindow = 15 * time.Second
	// sweepAbove is how many people the map may hold before it is cleaned of everyone who stopped.
	sweepAbove = 1024
)

// NewFollowerCap makes a cap; zero or negative values mean the defaults.
func NewFollowerCap(max int, window time.Duration) *FollowerCap {
	if max <= 0 {
		max = DefaultMaxFollowers
	}
	if window <= 0 {
		window = DefaultFollowWindow
	}
	return &FollowerCap{max: max, window: window, users: map[string]map[string]time.Time{}}
}

// Acquire reports whether user may follow the log key now, and records the poll when so. A false answer records nothing.
func (c *FollowerCap) Acquire(user, key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.users) > sweepAbove {
		c.sweepLocked(now)
	}
	logs := c.users[user]
	if logs == nil {
		logs = map[string]time.Time{}
	}
	for k, last := range logs {
		if now.Sub(last) >= c.window {
			delete(logs, k)
		}
	}
	if _, following := logs[key]; !following && len(logs) >= c.max {
		if len(logs) == 0 {
			delete(c.users, user)
		}
		return false
	}
	logs[key] = now
	c.users[user] = logs
	return true
}

// Active is how many logs user follows at now.
func (c *FollowerCap) Active(user string, now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, last := range c.users[user] {
		if now.Sub(last) < c.window {
			n++
		}
	}
	return n
}

func (c *FollowerCap) sweepLocked(now time.Time) {
	for user, logs := range c.users {
		for k, last := range logs {
			if now.Sub(last) >= c.window {
				delete(logs, k)
			}
		}
		if len(logs) == 0 {
			delete(c.users, user)
		}
	}
}
