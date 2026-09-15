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
	Status       string
	StartedAt    string
	FinishedAt   *string
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

// startCrawlRun inserts a new running row for portalLinkID.
// errCrawlAlreadyRunning if one is already running for this link
// (crawl_runs_one_running_idx's own violation, detected the same way
// addPortalLink detects errDuplicatePortalLink above); errUnknownPortalLink
// if the link itself doesn't exist.
func startCrawlRun(portalLinkID string) (*crawlRun, error) {
	if err := requirePortalLinkExists(portalLinkID); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	startedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := jobsDB.Exec(
		`INSERT INTO crawl_runs (id, portal_link_id, status, started_at, log) VALUES (?, ?, 'running', ?, '')`,
		id, portalLinkID, startedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errCrawlAlreadyRunning
		}
		return nil, err
	}
	return &crawlRun{ID: id, PortalLinkID: portalLinkID, Status: "running", StartedAt: startedAt}, nil
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
func finishCrawlRun(id, status string, resultSummary, errorMessage *string) error {
	finishedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := jobsDB.Exec(
		`UPDATE crawl_runs SET status = ?, finished_at = ?, result_summary = ?, error_message = ? WHERE id = ?`,
		status, finishedAt, orNull(resultSummary), orNull(errorMessage), id,
	)
	return err
}

// scanCrawlRun is the one shared Scan shape findActiveCrawlRun/
// findMostRecentCrawlRun both use.
func scanCrawlRun(row *sql.Row) (*crawlRun, error) {
	var r crawlRun
	var finishedAt, resultSummary, errorMessage, phase sql.NullString
	if err := row.Scan(&r.ID, &r.PortalLinkID, &r.Status, &r.StartedAt, &finishedAt, &r.Log, &resultSummary, &errorMessage, &phase); err != nil {
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
		`SELECT id, portal_link_id, status, started_at, finished_at, log, result_summary, error_message, phase
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
		`SELECT id, portal_link_id, status, started_at, finished_at, log, result_summary, error_message, phase
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
