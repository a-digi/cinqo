package jobs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"career-tool-backend/db"
	"career-tool-backend/jobs/jobmatcher"
)

// Automatic matching of new jobs (step 132). Every profile is scored for a job with the manual engine (no AI) and
// the best one becomes the job's match: job_matches holds one row per job, so storing each profile's result in turn
// would leave whichever profile happened to come last. See plan/ai/tools/career/step-132-job-match-consumer.md.

// The outcomes of AutoMatchJobs for one job.
const (
	AutoMatched         = "matched"          // a match was stored
	AutoSkippedExisting = "skipped_existing" // the job already has a match (an AI run, a manual run, a failed or running one): never replaced
	AutoUnjudged        = "unjudged"         // no profile could judge the job: nothing stored
	AutoUnknownJob      = "unknown_job"      // the job no longer exists
	AutoFailed          = "failed"           // reading, scoring or storing failed for this job (Err says why)
)

// AutoMatchOutcome is what happened to one job.
type AutoMatchOutcome struct {
	JobID     string
	Status    string
	ProfileID string // the profile whose result was stored, for AutoMatched
	Score     int    // its score, for AutoMatched
	Err       error  // for AutoFailed
}

// UnmatchedJobIDs returns the ids of jobs that have no job_matches row at all, newest first, at most limit of them (the
// catch-up for events that were missed, step 134). A job with a row of any kind, an AI match, a manual one, a failed or
// a running one, is not unmatched: it is never offered, so it is never touched.
func UnmatchedJobIDs(limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := db.JobsDB.Query(
		`SELECT j.id FROM jobs j LEFT JOIN job_matches m ON m.job_id = j.id
		 WHERE m.job_id IS NULL ORDER BY j.crawled_at DESC, j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AutoMatchJobs gives each job that has no match yet the best manual match over all profiles. The profiles, their
// personas and the corpus are read once for the whole batch. A job that already has a match is left alone, so a
// redelivered event or a match a person started meanwhile is never overwritten; a repeated id in the batch is handled
// once. An error is returned only when the batch cannot start at all (the profiles cannot be read); a problem with
// one job is that job's AutoFailed outcome and the rest go on. Outcomes come in the order of the first mention of
// each id.
func AutoMatchJobs(ids []string) ([]AutoMatchOutcome, error) {
	var order []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	scorer, err := NewProfileScorer()
	if err != nil {
		return nil, fmt.Errorf("could not read the profiles: %w", err)
	}
	out := make([]AutoMatchOutcome, 0, len(order))
	for _, id := range order {
		out = append(out, autoMatchOne(scorer, id))
	}
	return out, nil
}

func autoMatchOne(scorer *ProfileScorer, jobID string) AutoMatchOutcome {
	fail := func(err error) AutoMatchOutcome { return AutoMatchOutcome{JobID: jobID, Status: AutoFailed, Err: err} }
	var one int
	switch err := db.JobsDB.QueryRow(`SELECT 1 FROM job_matches WHERE job_id = ?`, jobID).Scan(&one); {
	case err == nil:
		return AutoMatchOutcome{JobID: jobID, Status: AutoSkippedExisting}
	case !errors.Is(err, sql.ErrNoRows):
		return fail(err)
	}
	if len(scorer.profiles) == 0 {
		// no profile has a persona, so nothing can judge the job: do not even read it (a catch-up that cannot match
		// anything must cost almost nothing)
		return AutoMatchOutcome{JobID: jobID, Status: AutoUnjudged}
	}
	profileID, result, ok, err := scorer.Best(jobID)
	if errors.Is(err, ErrUnknownJob) {
		return AutoMatchOutcome{JobID: jobID, Status: AutoUnknownJob}
	}
	if err != nil {
		return fail(err)
	}
	if !ok {
		return AutoMatchOutcome{JobID: jobID, Status: AutoUnjudged}
	}
	stored, err := storeManualMatchIfAbsent(jobID, profileID, result)
	if err != nil {
		return fail(err)
	}
	if !stored { // a match appeared between the check and the insert
		return AutoMatchOutcome{JobID: jobID, Status: AutoSkippedExisting}
	}
	return AutoMatchOutcome{JobID: jobID, Status: AutoMatched, ProfileID: profileID, Score: result.Score}
}

// storeManualMatchIfAbsent is storeManualMatch that never replaces: the row and its skills are written in one
// transaction only when the job has no job_matches row. It reports whether it wrote.
func storeManualMatchIfAbsent(jobID, profileID string, r jobmatcher.Result) (bool, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return false, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, conversation_id, error, kind, breakdown_json, updated_at)
		 VALUES (?, ?, ?, ?, 'completed', NULL, NULL, 'manual', ?, datetime('now'))
		 ON CONFLICT(job_id) DO NOTHING`,
		jobID, profileID, r.PersonaID, r.Score, string(raw),
	)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	for _, s := range r.MatchedSkills {
		if _, err := tx.Exec(`INSERT INTO job_match_skills (job_id, skill) VALUES (?, ?)`, jobID, s); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}
