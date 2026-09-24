// manager.go is the auto-discovery scheduler itself — the piece that
// actually decides WHEN to crawl, never HOW (that's runCrawlNow,
// completely unchanged, reused as-is via crawl.RunListingCrawlNow). See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md,
// including that plan's own "what was verified" section on why a
// goroutine has no PID to track and no zombie-process failure mode to
// defend against — StartAutoDiscoveryManager below follows the exact
// same "no stop mechanism, runs for the process's own lifetime"
// convention crawl.StartStaleCrawlRunReaper already established.
//
// checkAndRunTick (step 85) handles TWO independent due-checks on
// every tick, not just one: a link with its own custom interval
// (portal.ListDuePortalLinksWithCustomInterval) is checked against its
// OWN clock regardless of the shared global one, while the GLOBAL
// interval's own eligible links (portal.ListPortalLinksEligibleForAutoDiscovery
// — which already excludes any link with its own custom interval set)
// are still checked as one group against the shared
// auto_discovery_settings row, exactly as before step 85. The global
// on/off toggle is a master kill switch for BOTH categories — turning
// auto-discovery off stops everything, including a link with its own
// schedule (plan's own "Open design decision" 1(a)). See
// plan/ai/tools/career/step-85-auto-discovery-per-link-custom-interval.md.
package autodiscovery

import (
	"log"
	"sync"
	"time"

	"career-tool-backend/crawl"
	"career-tool-backend/portal"
)

// tickInterval is how often the manager checks whether anything is due
// — independent of any configured interval_minutes itself; this only
// bounds how late due work can actually start (at most tickInterval
// late). var, not const, matching this codebase's own established
// convention for a value a test might want to shrink (crawl_runs.go's
// own staleCrawlRunReapInterval).
var tickInterval = 1 * time.Minute

// sweepRunning guards against two ticks' own work overlapping — in
// practice this never actually happens (the ticker loop below calls
// checkAndRunTick synchronously, so a new tick can't start until the
// previous call returns), but it's kept as a defensive, self-
// documenting guard, same as the original step-84 design.
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
			checkAndRunTick()
		}
	}()
}

// workItem is one link this tick has decided to crawl, tagged with
// whether it came from the custom-interval group (in which case ITS
// OWN clock needs stamping after a successful crawl) or the global
// group (whose shared clock was already stamped once, up front, for
// the whole group together — see recordSweepStart's own doc comment
// for why start, not end, is what's correct there).
type workItem struct {
	linkID         string
	customInterval bool
}

// checkAndRunTick is one tick's own work. Skips entirely if disabled
// (the master kill switch). Otherwise builds one combined worklist —
// every custom-interval link that's individually due, plus, only if
// the GLOBAL due-check also passes, every global-interval eligible
// link — and processes the whole thing sequentially, one link at a
// time (browser tool's own crawl path already serializes on a single
// shared session lock, so concurrent auto-discovery crawls would just
// queue there anyway; doing it here instead is simpler to log and
// reason about for no real throughput cost). Stops the rest of the
// tick early the moment no fresh cached token is available — every
// remaining link would fail the exact same way.
func checkAndRunTick() {
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

	var work []workItem

	customLinkIDs, err := portal.ListDuePortalLinksWithCustomInterval()
	if err != nil {
		log.Printf("auto-discovery: failed to list due custom-interval portal links: %v", err)
	} else {
		for _, id := range customLinkIDs {
			work = append(work, workItem{linkID: id, customInterval: true})
		}
	}

	now := time.Now().Unix()
	globalDue := settings.LastRunAt == nil || now-*settings.LastRunAt >= int64(settings.IntervalMinutes)*60
	globalCount := 0
	if globalDue {
		// Recorded BEFORE fetching the group's own link list, and
		// before any of them actually run — the same "record at
		// start, not end" reasoning as before step 85, now scoped
		// correctly to just this group rather than the whole tick: a
		// long-running custom-interval link elsewhere in this same
		// tick must never delay the GLOBAL group's own next due-check
		// by however long that unrelated link's crawl took.
		if err := recordSweepStart(); err != nil {
			log.Printf("auto-discovery: failed to record global sweep start — skipping this tick's global portion: %v", err)
		} else {
			globalLinkIDs, err := portal.ListPortalLinksEligibleForAutoDiscovery()
			if err != nil {
				log.Printf("auto-discovery: failed to list eligible portal links: %v", err)
			} else {
				for _, id := range globalLinkIDs {
					work = append(work, workItem{linkID: id})
				}
				globalCount = len(globalLinkIDs)
			}
		}
	}

	if len(work) == 0 {
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

	log.Printf("auto-discovery: tick processing %d due link(s) (%d on their own custom interval, %d on the global interval)",
		len(work), len(customLinkIDs), globalCount)
	for _, item := range work {
		token, fresh := CachedToken()
		if !fresh {
			log.Printf("auto-discovery: skipping remaining tick — no live session token available " +
				"(open any Career page to refresh one; this will pick back up automatically on a later tick)")
			return
		}
		runID, err := crawl.RunListingCrawlNow(item.linkID, token, "auto_discovery")
		if err != nil {
			log.Printf("auto-discovery: failed to start crawl for portal link %s: %v", item.linkID, err)
			continue
		}
		log.Printf("auto-discovery: finished crawl %s for portal link %s (see crawl_runs for its own outcome)", runID, item.linkID)
		if item.customInterval {
			if err := portal.RecordPortalLinkAutoDiscoveryRun(item.linkID); err != nil {
				log.Printf("auto-discovery: failed to record auto-discovery run for portal link %s: %v", item.linkID, err)
			}
		}
	}
	log.Printf("auto-discovery: tick finished")
}
