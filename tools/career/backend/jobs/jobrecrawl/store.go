// Package jobrecrawl is the request side of "recrawl jobs for a fresh description" (step 148): a request is a row in
// job_recrawl_requests, one per job, and one "career.job.recrawl_requested" event per job tells the consumer (a later step)
// to crawl it. The row is the memory behind the best-effort event and the progress the Jobs page shows. See
// plan/ai/tools/career/step-148-recrawl-jobs.md.
package jobrecrawl

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"career-tool-backend/db"
)

// Topic is the event sent for each requested job; its payload is {"job_id": "…"}.
const Topic = "career.job.recrawl_requested"

// DescriptionChangedTopic is the event sent when a recrawl changed a job's description (refreshed or filled in); its payload is
// {"job_id": "…"}. The persona-score consumer listens to it and scores the job again, since its scores were made from the old text.
const DescriptionChangedTopic = "career.job.description_changed"

// The states of a request.
const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateDone      = "done"
	StateCancelled = "cancelled"
)

// The outcomes of a finished request.
const (
	OutcomeRefreshed = "refreshed" // the description changed
	OutcomeUnchanged = "unchanged" // the page gave the same text
	OutcomeFilled    = "filled"    // a job added by hand had no description and the crawl filled it
	OutcomeKept      = "kept"      // a job added by hand has its own text, which is never replaced
	OutcomeFailed    = "failed"    // the crawl or the save failed; the old text was kept
	OutcomeSkipped   = "skipped"   // nothing to crawl it with (no portal, no instructions)
)

// Why a job is not accepted into a batch.
const (
	SkipNoSuchJob   = "no such job"
	SkipNoPortal    = "no portal to crawl it from"
	SkipAlreadyBusy = "already being recrawled"
)

// ErrUnknownBatch is returned for a batch id that no request belongs to.
var ErrUnknownBatch = errors.New("unknown batch")

// Skip is a job that was not accepted, and why.
type Skip struct {
	JobID  string `json:"jobId"`
	Reason string `json:"reason"`
}

// Requested is the answer to one request: the new batch, the ids that were queued (their events still have to be sent), and
// the ones that were skipped.
type Requested struct {
	BatchID  string
	Accepted []string
	Skipped  []Skip
}

// Request queues every job in ids that can be recrawled, all in ONE transaction however many there are, and reports the
// rest. An id counts once however often it appears. A job that is already queued or running is skipped; a finished or
// cancelled earlier request of a job is replaced by the new one. No event is sent here.
func Request(ids []string) (Requested, error) {
	res := Requested{BatchID: uuid.NewString(), Accepted: []string{}, Skipped: []Skip{}}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true

		var sourceURL string
		var linkID sql.NullString
		err := tx.QueryRow(
			`SELECT COALESCE(j.source_url, ''), pl.id FROM jobs j LEFT JOIN portal_links pl ON pl.id = j.portal_link_id WHERE j.id = ?`, id,
		).Scan(&sourceURL, &linkID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res.Skipped = append(res.Skipped, Skip{id, SkipNoSuchJob})
			continue
		case err != nil:
			return res, err
		}
		if sourceURL == "" || !linkID.Valid {
			res.Skipped = append(res.Skipped, Skip{id, SkipNoPortal})
			continue
		}

		var state string
		err = tx.QueryRow(`SELECT state FROM job_recrawl_requests WHERE job_id = ?`, id).Scan(&state)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return res, err
		}
		if state == StateQueued || state == StateRunning {
			res.Skipped = append(res.Skipped, Skip{id, SkipAlreadyBusy})
			continue
		}

		if _, err := tx.Exec(
			`INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES (?, ?, ?, 'queued')
			 ON CONFLICT(job_id) DO UPDATE SET batch_id = excluded.batch_id, requested_at = excluded.requested_at, state = 'queued',
			   outcome = NULL, reason = NULL, finished_at = NULL`,
			id, res.BatchID, now,
		); err != nil {
			return res, err
		}
		res.Accepted = append(res.Accepted, id)
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

// Counts is how a batch's finished requests ended.
type Counts struct {
	Refreshed int `json:"refreshed"`
	Unchanged int `json:"unchanged"`
	Filled    int `json:"filled"`
	Kept      int `json:"kept"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// JobStatus is one request of a batch, with the job's title and whether it was added by hand.
type JobStatus struct {
	JobID   string `json:"jobId"`
	Title   string `json:"title"`
	Manual  bool   `json:"manual"`
	State   string `json:"state"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// BatchStatus is the progress of a batch.
type BatchStatus struct {
	Total     int         `json:"total"`
	Queued    int         `json:"queued"`
	Running   int         `json:"running"`
	Done      int         `json:"done"`
	Cancelled int         `json:"cancelled"`
	Counts    Counts      `json:"counts"`
	Jobs      []JobStatus `json:"jobs"`
}

// Status reads a batch: the numbers for all of it and one page (limit, offset) of its jobs, oldest request first.
func Status(batchID string, limit, offset int) (BatchStatus, error) {
	st := BatchStatus{Jobs: []JobStatus{}}
	rows, err := db.JobsDB.Query(`SELECT state, COALESCE(outcome, ''), COUNT(*) FROM job_recrawl_requests WHERE batch_id = ? GROUP BY state, COALESCE(outcome, '')`, batchID)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var state, outcome string
		var n int
		if err := rows.Scan(&state, &outcome, &n); err != nil {
			rows.Close()
			return st, err
		}
		st.Total += n
		switch state {
		case StateQueued:
			st.Queued += n
		case StateRunning:
			st.Running += n
		case StateCancelled:
			st.Cancelled += n
		case StateDone:
			st.Done += n
			switch outcome {
			case OutcomeRefreshed:
				st.Counts.Refreshed += n
			case OutcomeUnchanged:
				st.Counts.Unchanged += n
			case OutcomeFilled:
				st.Counts.Filled += n
			case OutcomeKept:
				st.Counts.Kept += n
			case OutcomeFailed:
				st.Counts.Failed += n
			case OutcomeSkipped:
				st.Counts.Skipped += n
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return st, err
	}
	if st.Total == 0 {
		return st, ErrUnknownBatch
	}

	page, err := db.JobsDB.Query(
		`SELECT r.job_id, j.title, j.source, r.state, COALESCE(r.outcome, ''), COALESCE(r.reason, '')
		 FROM job_recrawl_requests r JOIN jobs j ON j.id = r.job_id
		 WHERE r.batch_id = ? ORDER BY r.requested_at, j.title, r.job_id LIMIT ? OFFSET ?`, batchID, limit, offset,
	)
	if err != nil {
		return st, err
	}
	defer page.Close()
	for page.Next() {
		var js JobStatus
		var source string
		if err := page.Scan(&js.JobID, &js.Title, &source, &js.State, &js.Outcome, &js.Reason); err != nil {
			return st, err
		}
		js.Manual = source == "manual"
		st.Jobs = append(st.Jobs, js)
	}
	return st, page.Err()
}

// Cancel turns every queued request of the batch into cancelled and returns how many; a request that is already running
// finishes its page, one that is done stays as it is. ErrUnknownBatch when no request belongs to the batch.
func Cancel(batchID string) (int, error) {
	var existing int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_recrawl_requests WHERE batch_id = ?`, batchID).Scan(&existing); err != nil {
		return 0, err
	}
	if existing == 0 {
		return 0, ErrUnknownBatch
	}
	res, err := db.JobsDB.Exec(
		`UPDATE job_recrawl_requests SET state = 'cancelled', finished_at = ? WHERE batch_id = ? AND state = 'queued'`,
		time.Now().UTC().Format(time.RFC3339), batchID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Claim takes a queued request for the worker: queued becomes running and the old reason is cleared. It reports whether this
// call took it; false means the request is not queued (cancelled, done, already running, or gone), so the caller drops it.
func Claim(jobID string) (bool, error) {
	res, err := db.JobsDB.Exec(`UPDATE job_recrawl_requests SET state = 'running', reason = NULL WHERE job_id = ? AND state = 'queued'`, jobID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// Release puts a running request back to queued with a reason that the Jobs page shows ("waiting for a signed-in session"), for
// the sweeper to hand out again later.
func Release(jobID, reason string) error {
	_, err := db.JobsDB.Exec(`UPDATE job_recrawl_requests SET state = 'queued', reason = ? WHERE job_id = ? AND state = 'running'`, nullIfEmpty(reason), jobID)
	return err
}

// Finish ends a running request with its outcome (and a reason for the outcomes that have one).
func Finish(jobID, outcome, reason string) error {
	_, err := db.JobsDB.Exec(
		`UPDATE job_recrawl_requests SET state = 'done', outcome = ?, reason = ?, finished_at = ? WHERE job_id = ? AND state = 'running'`,
		outcome, nullIfEmpty(reason), time.Now().UTC().Format(time.RFC3339), jobID,
	)
	return err
}

// DueQueued lists up to limit queued requests that have waited longer than olderThan, oldest first: the ones whose event never
// reached the consumer, or that were put back to wait.
func DueQueued(olderThan time.Duration, limit int) ([]string, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	rows, err := db.JobsDB.Query(`SELECT job_id FROM job_recrawl_requests WHERE state = 'queued' AND requested_at <= ? ORDER BY requested_at, job_id LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ReviveRunning puts every running request back to queued and returns how many: at start-up no worker is running, so a request
// still marked running was cut off by a restart.
func ReviveRunning() (int, error) {
	res, err := db.JobsDB.Exec(`UPDATE job_recrawl_requests SET state = 'queued' WHERE state = 'running'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
