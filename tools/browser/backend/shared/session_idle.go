package shared

import (
	"log"
	"sync/atomic"
	"time"

	"browser-tool-backend/shared/chromeproc"
)

// The shared headless browser must not outlive the work that needed it: a
// crawl uses it page after page with short gaps, and once nothing has used it
// for SessionIdleStop it is closed (a later use starts a fresh one, exactly
// as on first use — EnsureSharedSession is lazy). Nothing is ever closed
// under a user: the primary tab is busy while Mu is held, a worker tab while
// it holds one of the worker slots, and every entry point first records
// activity, so a browser that was just asked for is never "idle".

// SessionIdleStop is how long the shared browser may sit unused. A var so
// tests can shorten it.
var SessionIdleStop = 10 * time.Second

// sessionReapInterval is how often the reaper looks.
const sessionReapInterval = 2 * time.Second

var lastSessionActivity atomic.Int64 // unix nanoseconds

// touchSessionActivity records that the shared browser was just asked for or
// just released.
func touchSessionActivity() {
	lastSessionActivity.Store(time.Now().UnixNano())
	chromeproc.TouchKind(chromeproc.KindSharedHeadless)
}

// reapIdleSession closes the shared headless browser when it is running, not
// in use and not used since SessionIdleStop. Reports whether it closed it.
// Never blocks: a held Mu means the primary tab is in use, so it simply
// looks again later.
func reapIdleSession(now time.Time) bool {
	if len(workerSlotPool()) > 0 {
		return false
	}
	if !Mu.TryLock() {
		// The primary tab is in use: the browser is not idle, and idleness is
		// counted from when it was last seen busy (the handlers unlock Mu
		// themselves, so this is where their finish shows up).
		touchSessionActivity()
		return false
	}
	defer Mu.Unlock()
	if Ctx == nil {
		return false
	}
	if now.Sub(time.Unix(0, lastSessionActivity.Load())) < SessionIdleStop {
		return false
	}
	if len(workerSlotPool()) > 0 { // a worker tab opened while we looked
		return false
	}
	stopHeadlessLocked()
	log.Printf("shared browser: closed after %s without use", SessionIdleStop)
	return true
}

// StartIdleReaper runs the reaper for the life of the process.
func StartIdleReaper() {
	go func() {
		t := time.NewTicker(sessionReapInterval)
		defer t.Stop()
		for now := range t.C {
			reapIdleSession(now)
		}
	}()
}
