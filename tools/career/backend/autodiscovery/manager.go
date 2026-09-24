// manager.go is the auto-discovery scheduler itself — the piece that
// actually decides WHEN to crawl, never HOW (that's runCrawlNow,
// completely unchanged, reused as-is via crawl.RunListingCrawlNow). See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md,
// including that plan's own "what was verified" section on why a
// goroutine has no PID to track and no zombie-process failure mode to
// defend against — StartAutoDiscoveryManager below follows the exact
// same "no stop mechanism, runs for the process's own lifetime"
// convention crawl.StartStaleCrawlRunReaper already established.
package autodiscovery

import (
	"log"
	"sync"
	"time"

	"career-tool-backend/crawl"
	"career-tool-backend/portal"
)

// tickInterval is how often the manager checks whether a sweep is due
// — independent of the configured interval_minutes itself; this only
// bounds how late a due sweep can actually start (at most tickInterval
// late). var, not const, matching this codebase's own established
// convention for a value a test might want to shrink (crawl_runs.go's
// own staleCrawlRunReapInterval).
var tickInterval = 1 * time.Minute

// sweepRunning guards against two sweeps overlapping — a single flag
// is enough since only this manager's own one ticker goroutine ever
// starts a sweep; it's set true for a sweep's entire duration
// (potentially many minutes, one link at a time) and checked at the
// start of every tick.
var (
	sweepMu      sync.Mutex
	sweepRunning bool
)

// StartAutoDiscoveryManager launches the scheduler as a background
// goroutine for this process's entire lifetime.
func StartAutoDiscoveryManager() {
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for range ticker.C {
			checkAndRunSweep()
		}
	}()
}

// checkAndRunSweep is one tick's own work: skip if a sweep is already
// running, skip if disabled, skip if not due yet — otherwise run one.
func checkAndRunSweep() {
	sweepMu.Lock()
	if sweepRunning {
		sweepMu.Unlock()
		return
	}
	sweepMu.Unlock()

	settings, err := GetSettings()
	if err != nil {
		log.Printf("auto-discovery: failed to load settings: %v", err)
		return
	}
	if !settings.Enabled {
		return
	}
	now := time.Now().Unix()
	if settings.LastRunAt != nil && now-*settings.LastRunAt < int64(settings.IntervalMinutes)*60 {
		return
	}

	sweepMu.Lock()
	sweepRunning = true
	sweepMu.Unlock()
	defer func() {
		sweepMu.Lock()
		sweepRunning = false
		sweepMu.Unlock()
	}()

	runSweep()
}

// runSweep records itself as having started (see recordSweepStart's
// own doc comment for why start, not end), then visits every eligible
// portal link SEQUENTIALLY, never in parallel — browser tool's own
// crawl path already serializes on a single shared session lock
// (crawl_now.go's own comments reference sessionMu), so concurrent
// auto-discovery crawls would just queue there anyway; doing it here
// instead is simpler to log and reason about for no real throughput
// cost. Stops the whole sweep early the moment no fresh cached token is
// available — every remaining link would fail the exact same way, so
// there's nothing to gain from attempting each one individually only
// to log the same failure repeatedly.
func runSweep() {
	if err := recordSweepStart(); err != nil {
		log.Printf("auto-discovery: failed to record sweep start: %v", err)
		return
	}

	linkIDs, err := portal.ListPortalLinksEligibleForAutoDiscovery()
	if err != nil {
		log.Printf("auto-discovery: failed to list eligible portal links: %v", err)
		return
	}
	if len(linkIDs) == 0 {
		log.Printf("auto-discovery: sweep started — no eligible portal links (need crawl instructions set and not individually opted out)")
		return
	}

	log.Printf("auto-discovery: sweep started — %d eligible portal link(s)", len(linkIDs))
	for _, linkID := range linkIDs {
		token, fresh := CachedToken()
		if !fresh {
			log.Printf("auto-discovery: skipping remaining sweep — no live session token available " +
				"(open any Career page to refresh one; this sweep will pick back up automatically on a later tick)")
			return
		}
		runID, err := crawl.RunListingCrawlNow(linkID, token, "auto_discovery")
		if err != nil {
			log.Printf("auto-discovery: failed to start crawl for portal link %s: %v", linkID, err)
			continue
		}
		log.Printf("auto-discovery: finished crawl %s for portal link %s (see crawl_runs for its own outcome)", runID, linkID)
	}
	log.Printf("auto-discovery: sweep finished")
}
