// instruction_attempts.go (step 86) tracks every "Generate with AI"
// run that tries to write a portal link's crawl instructions (kind
// 'listing' = crawl_instructions, 'job_detail' =
// job_detail_crawl_instructions), so the UI can show how many
// attempts succeeded or failed and alert when attempts were made but
// the link still has no instructions.
//
// "Succeeded" is decided here, server-side, from saved_at — which only
// MarkInstructionSaved (called by the save MCP tools) ever sets — never
// from anything a client reports: a turn that ended cleanly but saved
// nothing (the AI gave up per its own prompt) is a failure. See
// plan/ai/tools/career/step-86-instruction-attempt-tracking.md.
package portal

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

const (
	InstructionKindListing   = "listing"
	InstructionKindJobDetail = "job_detail"

	InstructionTriggerManual = "manual"
	InstructionTriggerAuto   = "auto"

	InstructionAttemptSucceeded = "succeeded"
	InstructionAttemptFailed    = "failed"
	// InstructionAttemptNone is FinishInstructionAttempt's result when
	// no attempt was running — not an error, so the fresh-run and
	// resumed-after-reload paths can both call it without double
	// counting.
	InstructionAttemptNone = "none"

	// maxInstructionAttemptTextLength caps the AI/turn-supplied
	// error_message and conversation_id stored per attempt.
	maxInstructionAttemptTextLength = 2000

	defaultInstructionAttemptError = "The AI finished without saving instructions."

	interruptedInstructionAttemptError = "Interrupted: the app or tab closed before the run finished."

	// Why a failed attempt failed (instruction_attempts.failure_reason,
	// step 87). Only InstructionFailureNotSaved — the run ended cleanly
	// but the AI saved nothing — says anything about the page itself.
	InstructionFailureNotSaved    = "not_saved"
	InstructionFailureError       = "error"
	InstructionFailureInterrupted = "interrupted"
)

// staleInstructionAttemptThreshold bounds how long an attempt may stay
// 'running'. A hidden generation turn lives in the core app, not in
// this process, so a Career backend restart does NOT orphan it — only
// age is a safe signal. 30 minutes is double the 15-minute turn limit
// (conversation.maxTurnRunDuration), so a healthy run is never reaped.
// var, not const, so a test can shrink it — this codebase's own
// convention (staleCrawlRunThreshold).
var staleInstructionAttemptThreshold = 30 * time.Minute

var (
	ErrInvalidInstructionKind    = errors.New("kind must be 'listing' or 'job_detail'")
	ErrInvalidInstructionTrigger = errors.New("triggeredBy must be 'manual' or 'auto'")
)

func validInstructionKind(kind string) bool {
	return kind == InstructionKindListing || kind == InstructionKindJobDetail
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// ReconcileStaleInstructionAttempts ends every attempt still 'running'
// after staleInstructionAttemptThreshold — a tab closed or an app
// crashed before the finish call, which would otherwise leave the row
// 'running' forever: blocking the idempotent start from beginning a
// fresh attempt, and (via stats.running) suppressing the alert pill.
// One that already has saved_at is a real success whose finish call
// was merely lost, so it is closed as 'succeeded'; the rest are
// 'failed'. Returns how many rows it ended. Called at boot and from
// the start and stats paths, so a stuck row heals without a restart.
func ReconcileStaleInstructionAttempts() (int64, error) {
	cutoff := fmt.Sprintf("-%d seconds", int64(staleInstructionAttemptThreshold/time.Second))

	succeeded, err := db.JobsDB.Exec(
		`UPDATE instruction_attempts SET status = 'succeeded', finished_at = datetime('now'), error_message = NULL
		 WHERE status = 'running' AND saved_at IS NOT NULL AND started_at < datetime('now', ?)`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	failed, err := db.JobsDB.Exec(
		`UPDATE instruction_attempts SET status = 'failed', finished_at = datetime('now'), error_message = ?, failure_reason = ?
		 WHERE status = 'running' AND saved_at IS NULL AND started_at < datetime('now', ?)`,
		interruptedInstructionAttemptError, InstructionFailureInterrupted, cutoff,
	)
	if err != nil {
		return 0, err
	}
	n1, _ := succeeded.RowsAffected()
	n2, _ := failed.RowsAffected()
	return n1 + n2, nil
}

// StartInstructionAttempt records the start of one attempt for a link
// and kind. If one is already running it is left alone and its id is
// returned with started == false (idempotent; the partial unique index
// instruction_attempts_one_running_idx enforces the same invariant).
func StartInstructionAttempt(portalLinkID, kind, triggeredBy, conversationID string) (attemptID string, started bool, err error) {
	if !validInstructionKind(kind) {
		return "", false, ErrInvalidInstructionKind
	}
	if triggeredBy != InstructionTriggerManual && triggeredBy != InstructionTriggerAuto {
		return "", false, ErrInvalidInstructionTrigger
	}
	if err := RequirePortalLinkExists(portalLinkID); err != nil {
		return "", false, err
	}
	// A stale 'running' row must not swallow this start.
	if _, err := ReconcileStaleInstructionAttempts(); err != nil {
		return "", false, err
	}

	findRunning := func() (string, error) {
		var id string
		err := db.JobsDB.QueryRow(
			`SELECT id FROM instruction_attempts WHERE portal_link_id = ? AND kind = ? AND status = 'running'`,
			portalLinkID, kind,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return id, err
	}

	if id, err := findRunning(); err != nil || id != "" {
		return id, false, err
	}

	var conv any
	if c := truncateRunes(conversationID, maxInstructionAttemptTextLength); c != "" {
		conv = c
	}
	id := uuid.NewString()
	if _, err := db.JobsDB.Exec(
		`INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, conversation_id) VALUES (?, ?, ?, ?, ?)`,
		id, portalLinkID, kind, triggeredBy, conv,
	); err != nil {
		// Lost a race with a concurrent start: the unique index
		// rejected ours, so report the winner instead of failing.
		if existing, findErr := findRunning(); findErr == nil && existing != "" {
			return existing, false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

// FinishInstructionAttempt ends the link's running attempt for kind.
// The outcome is decided here: saved_at set (by MarkInstructionSaved)
// means "succeeded"; otherwise "failed", with errText if given, else a
// default message. Returns InstructionAttemptNone if nothing was
// running.
func FinishInstructionAttempt(portalLinkID, kind, errText string) (status string, err error) {
	if !validInstructionKind(kind) {
		return "", ErrInvalidInstructionKind
	}
	if err := RequirePortalLinkExists(portalLinkID); err != nil {
		return "", err
	}

	var id string
	var savedAt sql.NullString
	err = db.JobsDB.QueryRow(
		`SELECT id, saved_at FROM instruction_attempts WHERE portal_link_id = ? AND kind = ? AND status = 'running'`,
		portalLinkID, kind,
	).Scan(&id, &savedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return InstructionAttemptNone, nil
	}
	if err != nil {
		return "", err
	}

	if savedAt.Valid {
		_, err = db.JobsDB.Exec(
			`UPDATE instruction_attempts SET status = 'succeeded', finished_at = datetime('now'), error_message = NULL WHERE id = ? AND status = 'running'`,
			id,
		)
		return InstructionAttemptSucceeded, err
	}

	// An error text from the client means the turn itself failed; none
	// means it ended cleanly without saving (the AI gave up).
	msg := truncateRunes(errText, maxInstructionAttemptTextLength)
	reason := InstructionFailureError
	if msg == "" {
		msg = defaultInstructionAttemptError
		reason = InstructionFailureNotSaved
	}
	_, err = db.JobsDB.Exec(
		`UPDATE instruction_attempts SET status = 'failed', finished_at = datetime('now'), error_message = ?, failure_reason = ? WHERE id = ? AND status = 'running'`,
		msg, reason, id,
	)
	return InstructionAttemptFailed, err
}

// MarkInstructionSaved records that instructions of this kind were
// just saved by the AI, on the link's running attempt if there is one.
// A harmless no-op when nothing is running (a save outside any
// tracked attempt) — never an error for that reason.
func MarkInstructionSaved(portalLinkID, kind string) error {
	if !validInstructionKind(kind) {
		return ErrInvalidInstructionKind
	}
	_, err := db.JobsDB.Exec(
		`UPDATE instruction_attempts SET saved_at = datetime('now') WHERE portal_link_id = ? AND kind = ? AND status = 'running' AND saved_at IS NULL`,
		portalLinkID, kind,
	)
	return err
}

// markInstructionsSavedByAI is what the AI-facing save tools call
// after a successful save. Best-effort: the instructions are already
// saved, so a failure to record it is logged (stderr — stdout is the
// MCP transport) and never turned into a tool error; the attempt then
// simply finishes as failed, which is the safe direction.
func markInstructionsSavedByAI(portalLinkID, kind string) {
	if err := MarkInstructionSaved(portalLinkID, kind); err != nil {
		log.Printf("instruction attempt: failed to record save for link %s (%s): %v", portalLinkID, kind, err)
	}
}

// InstructionKindStats summarizes one link's attempts for one kind.
// Attempts counts finished ones (Succeeded + Failed); a run still in
// flight is reported via Running and not counted until it ends.
type InstructionKindStats struct {
	Attempts      int    `json:"attempts"`
	Succeeded     int    `json:"succeeded"`
	Failed        int    `json:"failed"`
	Running       bool   `json:"running"`
	LastFailureAt string `json:"lastFailureAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	// ConsecutiveFailures (step 87) is how many failures of kind
	// 'not_saved' (the run ended cleanly but the AI saved nothing) this
	// link has had since its most recent success — the signal that a
	// page probably can't be crawled. A failure caused by an error or an
	// interruption is neutral: it neither adds to the streak nor resets
	// it. See plan/ai/tools/career/step-87-likely-uncrawlable-pill.md.
	ConsecutiveFailures int `json:"consecutiveFailures"`
}

type InstructionStats struct {
	Listing   InstructionKindStats `json:"listing"`
	JobDetail InstructionKindStats `json:"jobDetail"`
}

func (s *InstructionStats) forKind(kind string) *InstructionKindStats {
	if kind == InstructionKindJobDetail {
		return &s.JobDetail
	}
	return &s.Listing
}

// InstructionStatsByLink returns the stats for every link that has at
// least one attempt, keyed by portal link id — two queries total, never
// one per link. A link with no attempts is simply absent (callers treat
// that as the zero value).
func InstructionStatsByLink() (map[string]InstructionStats, error) {
	out := map[string]InstructionStats{}

	// A stale 'running' row must not hide the alert pill.
	if _, err := ReconcileStaleInstructionAttempts(); err != nil {
		return nil, err
	}

	rows, err := db.JobsDB.Query(
		`SELECT portal_link_id, kind,
		        COALESCE(SUM(status = 'succeeded'), 0),
		        COALESCE(SUM(status = 'failed'), 0),
		        COALESCE(SUM(status = 'running'), 0)
		 FROM instruction_attempts
		 GROUP BY portal_link_id, kind`,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var linkID, kind string
		var succeeded, failed, running int
		if err := rows.Scan(&linkID, &kind, &succeeded, &failed, &running); err != nil {
			rows.Close()
			return nil, err
		}
		stats := out[linkID]
		k := stats.forKind(kind)
		k.Succeeded, k.Failed, k.Attempts, k.Running = succeeded, failed, succeeded+failed, running > 0
		out[linkID] = stats
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	failRows, err := db.JobsDB.Query(
		`SELECT portal_link_id, kind, COALESCE(finished_at, ''), COALESCE(error_message, '')
		 FROM (
		   SELECT portal_link_id, kind, finished_at, error_message,
		          ROW_NUMBER() OVER (PARTITION BY portal_link_id, kind ORDER BY finished_at DESC, rowid DESC) AS rn
		   FROM instruction_attempts WHERE status = 'failed'
		 ) WHERE rn = 1`,
	)
	if err != nil {
		return nil, err
	}
	for failRows.Next() {
		var linkID, kind, at, msg string
		if err := failRows.Scan(&linkID, &kind, &at, &msg); err != nil {
			failRows.Close()
			return nil, err
		}
		stats := out[linkID]
		k := stats.forKind(kind)
		k.LastFailureAt, k.LastError = at, msg
		out[linkID] = stats
	}
	if err := failRows.Err(); err != nil {
		failRows.Close()
		return nil, err
	}
	// Closed before the next query: jobs.db is a single connection.
	failRows.Close()

	// The streak: 'not_saved' failures after the link's latest success
	// (rowid is insert order, and a link and kind never run two attempts
	// at once, so it is also finish order). Errors and interruptions are
	// excluded by the filter, which is what makes them neutral.
	streakRows, err := db.JobsDB.Query(
		`SELECT a.portal_link_id, a.kind, COUNT(*)
		 FROM instruction_attempts a
		 WHERE a.status = 'failed' AND a.failure_reason = 'not_saved'
		   AND a.rowid > COALESCE((
		         SELECT MAX(s.rowid) FROM instruction_attempts s
		         WHERE s.portal_link_id = a.portal_link_id AND s.kind = a.kind AND s.status = 'succeeded'), 0)
		 GROUP BY a.portal_link_id, a.kind`,
	)
	if err != nil {
		return nil, err
	}
	defer streakRows.Close()
	for streakRows.Next() {
		var linkID, kind string
		var streak int
		if err := streakRows.Scan(&linkID, &kind, &streak); err != nil {
			return nil, err
		}
		stats := out[linkID]
		stats.forKind(kind).ConsecutiveFailures = streak
		out[linkID] = stats
	}
	return out, streakRows.Err()
}
