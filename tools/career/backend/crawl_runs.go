// crawl_runs.go is the data-layer for tracking one "Crawl now" attempt
// against a portal link, independently of any HTTP request's own
// lifetime — the foundation step-37's detached goroutine orchestration
// builds on. Deliberately mirrors the AI conversation feature's own
// turn_runs table (api/src/conversation/) in shape and reasoning: a
// coarse, append-only log column, a terminal status with an optional
// result/error, and a partial unique index that is the real
// concurrency guard (not an app-level check-then-insert, which would
// race under two concurrent "Crawl now" clicks for the same link). See
// plan/ai/tools/career/step-36-crawl-run-data-model.md.
package main

import (
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// errCrawlAlreadyRunning is returned by startCrawlRun when
// crawl_runs_one_running_idx (db.go) would be violated — a caller
// mistake (the UI's own busy-state should already prevent this), not a
// server failure.
var errCrawlAlreadyRunning = errors.New("a crawl is already running for this link")

type crawlRun struct {
	ID           string
	PortalLinkID string
	// Kind distinguishes a "Crawl now" run against this link's own
	// listing page ('listing', the default) from a "Crawl job details
	// now" run against every already-saved job's own detail page
	// ('job_detail') — see db.go's own crawl_runs.kind doc comment.
	Kind       string
	Status     string
	StartedAt  string
	FinishedAt *string
	// Log is the raw, newline-delimited "RFC3339<TAB>message" trace —
	// split into individual entries only at the HTTP response layer
	// (step 37), matching how turn_runs.log is treated on the
	// conversation side.
	Log           string
	ResultSummary *string
	ErrorMessage  *string
	// Phase (step 39) is the single most recent fine-grained step this
	// run has reached — nil until the first setCrawlRunPhase call
	// lands, never cleared afterward (see db.go's own crawl_runs
	// schema comment for why).
	Phase *string
}

// startCrawlRun inserts a new running row for portalLinkID, of the
// given kind ("listing" or "job_detail" — see db.go's own crawl_runs.kind
// doc comment). errCrawlAlreadyRunning if one is already running for
// this link, of EITHER kind (crawl_runs_one_running_idx's own
// violation, detected the same way addPortalLink detects
// errDuplicatePortalLink above) — the two kinds share the one browser
// tab, so they can never run concurrently for the same link.
// errUnknownPortalLink if the link itself doesn't exist.
func startCrawlRun(portalLinkID, kind string) (*crawlRun, error) {
	if err := requirePortalLinkExists(portalLinkID); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	startedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := jobsDB.Exec(
		`INSERT INTO crawl_runs (id, portal_link_id, kind, status, started_at, log) VALUES (?, ?, ?, 'running', ?, '')`,
		id, portalLinkID, kind, startedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errCrawlAlreadyRunning
		}
		return nil, err
	}
	return &crawlRun{ID: id, PortalLinkID: portalLinkID, Kind: kind, Status: "running", StartedAt: startedAt}, nil
}

// appendCrawlRunLog appends one line atomically (log = log || ?) — no
// read-modify-write race, matching turn_runs' own AppendLog contract.
func appendCrawlRunLog(id, line string) error {
	entry := time.Now().UTC().Format(time.RFC3339) + "\t" + line + "\n"
	_, err := jobsDB.Exec(`UPDATE crawl_runs SET log = log || ? WHERE id = ?`, entry, id)
	return err
}

// setCrawlRunPhase updates crawl_runs.phase and appends a matching log
// line in one call (step 39) — every phase transition is a log-worthy
// event by definition, so there is no legitimate case for updating one
// without the other. Superset of a plain appendCrawlRunLog call.
func setCrawlRunPhase(id, phase, message string) error {
	entry := time.Now().UTC().Format(time.RFC3339) + "\t" + message + "\n"
	_, err := jobsDB.Exec(`UPDATE crawl_runs SET phase = ?, log = log || ? WHERE id = ?`, phase, entry, id)
	return err
}

// orNull turns a *string into a value database/sql writes as either
// the string or NULL — explicit rather than relying on database/sql's
// own pointer-dereferencing conversion, matching this file's own
// established preference (see updatePortalLinkInstructionsAIStatus)
// for explicit nil handling over implicit driver behavior.
func orNull(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// finishCrawlRun marks a run terminal — status plus finished_at, and
// exactly one of resultSummary/errorMessage populated (the caller's
// own responsibility; this function itself doesn't enforce which).
//
// AND status = 'running' (step 63.5) — a general idempotency
// hardening, not specific to any one feature: this table should never
// allow a second terminal write to silently overwrite an
// already-terminal row. Made necessary in practice by step 63's own
// "Stop crawl" handler (crawl_now.go) and runCrawlNow's own goroutine
// now being able to both attempt a terminal write for the same row at
// roughly the same moment (the handler writes 'cancelled' while the
// goroutine, moments later, may still be mid-flight and try to write
// something else) — without this guard, whichever writes LAST would
// win, silently replacing a correct 'cancelled' outcome. With it,
// whichever writes FIRST wins and the second call becomes a harmless,
// zero-row-affected no-op.
func finishCrawlRun(id, status string, resultSummary, errorMessage *string) error {
	finishedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := jobsDB.Exec(
		`UPDATE crawl_runs SET status = ?, finished_at = ?, result_summary = ?, error_message = ? WHERE id = ? AND status = 'running'`,
		status, finishedAt, orNull(resultSummary), orNull(errorMessage), id,
	)
	return err
}

// splitCrawlRunLog splits a crawl_runs.log column's own raw,
// newline-delimited "RFC3339<TAB>message" string into individual
// entries — shared by toCrawlRunResponse (crawl_now.go) and the
// crawl-monitor endpoint (crawl_monitor.go), both of which need the
// same "one log line per entry, no trailing empty line" shape. Never
// nil — an empty log becomes [], not null, so callers' own JSON
// encoding is consistent regardless of whether any log lines exist
// yet.
func splitCrawlRunLog(log string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(log, "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if lines == nil {
		lines = []string{}
	}
	return lines
}

// scanCrawlRun is the one shared Scan shape findActiveCrawlRun/
// findMostRecentCrawlRun both use.
func scanCrawlRun(row *sql.Row) (*crawlRun, error) {
	var r crawlRun
	var finishedAt, resultSummary, errorMessage, phase sql.NullString
	if err := row.Scan(&r.ID, &r.PortalLinkID, &r.Kind, &r.Status, &r.StartedAt, &finishedAt, &r.Log, &resultSummary, &errorMessage, &phase); err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		r.FinishedAt = &finishedAt.String
	}
	if resultSummary.Valid {
		r.ResultSummary = &resultSummary.String
	}
	if errorMessage.Valid {
		r.ErrorMessage = &errorMessage.String
	}
	if phase.Valid {
		r.Phase = &phase.String
	}
	return &r, nil
}

// findActiveCrawlRun returns the one status='running' row for
// portalLinkID, if any — sql.ErrNoRows otherwise (callers compare
// directly, same convention getPortalLinkCrawlInstructions' own
// sql.NullString handling elsewhere in this package already
// establishes for "absent" values).
func findActiveCrawlRun(portalLinkID string) (*crawlRun, error) {
	row := jobsDB.QueryRow(
		`SELECT id, portal_link_id, kind, status, started_at, finished_at, log, result_summary, error_message, phase
		 FROM crawl_runs WHERE portal_link_id = ? AND status = 'running'`,
		portalLinkID,
	)
	return scanCrawlRun(row)
}

// findMostRecentCrawlRun returns portalLinkID's own most recent row
// regardless of status — sql.ErrNoRows if none has ever existed. Used
// by the polling endpoint (step 38) so a poll racing the exact moment
// a run finishes still observes the real terminal status instead of a
// 404 (mirrors TurnRunQueryRepo.FindMostRecentByConversationID's own
// reasoning).
//
// Orders by rowid, not started_at: started_at is second-granularity
// RFC3339 text, so two runs for the same link started within the same
// second (a real, caught-by-testing case — completing one run and
// immediately starting the next can land in the same second) tie
// under a started_at-only ordering, and SQLite doesn't guarantee which
// tied row a plain ORDER BY returns. crawl_runs has no WITHOUT ROWID
// clause, so its implicit rowid is monotonically increasing by
// insertion order regardless of timestamp precision, making it the
// correct tiebreaker (here, the only ordering key at all).
func findMostRecentCrawlRun(portalLinkID string) (*crawlRun, error) {
	row := jobsDB.QueryRow(
		`SELECT id, portal_link_id, kind, status, started_at, finished_at, log, result_summary, error_message, phase
		 FROM crawl_runs WHERE portal_link_id = ? ORDER BY rowid DESC LIMIT 1`,
		portalLinkID,
	)
	return scanCrawlRun(row)
}

// activeCrawlRunPortalLinkIDs returns the set of portal link ids that
// currently have a running crawl_runs row — one query, mirroring
// lastCrawledByPortalLink's own batch-lookup shape (portals.go), so
// listPortals doesn't need a per-link round trip to populate
// hasActiveCrawlRun.
func activeCrawlRunPortalLinkIDs() (map[string]bool, error) {
	rows, err := jobsDB.Query(`SELECT portal_link_id FROM crawl_runs WHERE status = 'running'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

// reconcileOrphanedCrawlRuns runs once at backend startup — a
// goroutine, unlike an OS process, has no PID to find or reattach
// after a restart, so any row still 'running' from before this
// process started is definitely orphaned. Mirrors
// conversation.ReconcileOrphanedTurnRuns exactly. See
// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
func reconcileOrphanedCrawlRuns() error {
	_, err := jobsDB.Exec(
		`UPDATE crawl_runs SET status = 'failed', finished_at = ?, error_message = 'Interrupted by a server restart'
		 WHERE status = 'running'`,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// staleCrawlRunThreshold (step 47.4) bounds how long a crawl_runs row
// may legitimately stay 'running' before the periodic reaper below
// treats it as stuck and marks it 'failed' — WITHOUT requiring a
// process restart the way reconcileOrphanedCrawlRuns (above) does. Set
// comfortably above the longest single attempt this process can
// legitimately take: browser's own normalSessionPaginatedCrawlTimeout
// (32 minutes — the headed Cloudflare fallback, including up to 30
// minutes of human-solve wait) is the real ceiling; every retry
// runCrawlNow's own loop performs (step 38/47.3) is for a fast-failing
// transient error, never a second full-budget wait stacked on top of
// the first. var, not const, so a test can shrink it — matching this
// codebase's own established convention (crawlNowSessionRetryInterval,
// above; browser's own humanSolveRetryInterval).
var staleCrawlRunThreshold = 40 * time.Minute

// staleCrawlRunReapInterval (step 47.4) is how often the reaper below
// sweeps for stuck rows — far less frequent than staleCrawlRunThreshold
// itself needs to be enforced precisely; a few minutes of slack before
// a stuck row is actually caught is an acceptable trade for not
// hammering the database on a tight loop. var, same testing reason as
// staleCrawlRunThreshold.
var staleCrawlRunReapInterval = 5 * time.Minute

// reapStaleCrawlRuns marks every crawl_runs row still 'running' well
// past staleCrawlRunThreshold as 'failed' — the in-process complement
// to reconcileOrphanedCrawlRuns (which only ever runs once, at boot):
// this catches a goroutine that is hung but never crashed the process
// (step 47's own root cause — a chromedp.Run call blocked past its own
// context deadline because the underlying renderer itself is wedged),
// which reconcileOrphanedCrawlRuns can never see since nothing about
// this process actually restarted. Returns the number of rows reaped,
// for observability/testing — 0 is the ordinary, expected case. Two
// separate `time.Now()` reads (one for the cutoff comparison, one for
// finished_at) are deliberate, not a bug: they're each other's own
// truthful value for what they represent (when a row is considered
// stale vs. when this sweep actually ran), not required to match.
func reapStaleCrawlRuns() (int, error) {
	cutoff := time.Now().UTC().Add(-staleCrawlRunThreshold).Format(time.RFC3339)
	result, err := jobsDB.Exec(
		`UPDATE crawl_runs SET status = 'failed', finished_at = ?, error_message = 'Crawl timed out — stuck for longer than the maximum expected duration and was automatically marked failed'
		 WHERE status = 'running' AND started_at < ?`,
		time.Now().UTC().Format(time.RFC3339), cutoff,
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// startStaleCrawlRunReaper launches the periodic sweep above as a
// background goroutine for this process's entire lifetime — no stop
// mechanism, matching reconcileOrphanedCrawlRuns being meant to run
// exactly once at boot: this one is meant to run for as long as the
// process itself does.
func startStaleCrawlRunReaper() {
	go func() {
		ticker := time.NewTicker(staleCrawlRunReapInterval)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := reapStaleCrawlRuns(); err != nil {
				log.Printf("stale crawl run reaper: sweep failed: %v", err)
			} else if n > 0 {
				log.Printf("stale crawl run reaper: marked %d stuck crawl run(s) as failed", n)
			}
		}
	}()
}
