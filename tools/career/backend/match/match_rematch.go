package match

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/match/jobmatcher"
	"career-tool-backend/profile"
)

// Re-matching a job whose text changed (step 164). The job's match in job_matches (one row per job) was made from the
// text the job had then; when a write changes the description ("career.job.description_changed") the manual engine is run
// again, for free. What is never touched: a result an AI run made (it cost tokens and the person asked for it), and a run
// still in progress. See plan/ai/tools/career/step-164-job-description-changed-rematch.md.

// The outcomes of RematchChangedJobs for one job.
const (
	ChangedJobCreated         = "created"          // the job had no match: the best manual match over all profiles was stored, like AutoMatchJobs does
	ChangedJobRescored        = "rescored"         // a manual match was made again, from the new text, for the profile it was made for
	ChangedJobSkippedAI       = "skipped_ai"       // the match is an AI result: never replaced automatically
	ChangedJobSkippedRunning  = "skipped_running"  // a match run is in progress for the job
	ChangedJobSkippedExisting = "skipped_existing" // any other existing row (or one a person's run replaced meanwhile): left alone
	ChangedJobUnjudged        = "unjudged"         // the profile (or, for a job without a row, every profile) cannot judge the job: the row stays as it is
	ChangedJobUnknownJob      = "unknown_job"      // the job no longer exists
	ChangedJobFailed          = "failed"           // reading, scoring or storing failed for this job (Err says why)
)

// ChangedJobOutcome is what happened to one job.
type ChangedJobOutcome struct {
	JobID     string
	Status    string
	ProfileID string // the profile whose result was stored, for ChangedJobCreated and ChangedJobRescored
	Score     int    // its score, for those two
	Err       error  // for ChangedJobFailed
}

// RematchChangedJobs brings the match of each job up to date after its text changed. The profiles, their personas and the corpus are
// read once for the whole batch; a repeated id is handled once; an error is returned only when the batch cannot start (the
// profiles cannot be read), a problem with one job is that job's ChangedJobFailed outcome and the rest go on. Per job:
//   - no row: the best manual match over all profiles is stored (what a new job gets);
//   - a manual result (kind 'manual', completed): scored again for the profile the row was made for (the person's choice or the
//     best one at the time) and replaced, with its skills, in one transaction, and only if the row is still that manual result;
//   - an AI result, a run in progress, or anything else: left alone.
func RematchChangedJobs(ids []string) ([]ChangedJobOutcome, error) {
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
	out := make([]ChangedJobOutcome, 0, len(order))
	for _, id := range order {
		out = append(out, rematchChangedJob(scorer, id))
	}
	return out, nil
}

func rematchChangedJob(scorer *ProfileScorer, jobID string) ChangedJobOutcome {
	fail := func(err error) ChangedJobOutcome {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobFailed, Err: err}
	}
	var profileID, kind, status string
	err := db.JobsDB.QueryRow(`SELECT profile_id, kind, status FROM job_matches WHERE job_id = ?`, jobID).Scan(&profileID, &kind, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return rematchChangedJobCreate(scorer, jobID)
	case err != nil:
		return fail(err)
	case kind == "ai":
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobSkippedAI}
	case status == "matching":
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobSkippedRunning}
	case kind != "manual" || status != "completed":
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobSkippedExisting}
	}
	result, ok, err := scorer.ScoreProfile(jobID, profileID)
	if errors.Is(err, jobs.ErrUnknownJob) {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobUnknownJob}
	}
	if err != nil {
		return fail(err)
	}
	if !ok {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobUnjudged}
	}
	replaced, err := writeUnderProfileLock(profileID, func() (bool, error) { return replaceManualMatchIfStillManual(jobID, profileID, result) })
	if err != nil {
		return fail(err)
	}
	if !replaced { // a person's run took the row over between the check and the write
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobSkippedExisting}
	}
	return ChangedJobOutcome{JobID: jobID, Status: ChangedJobRescored, ProfileID: profileID, Score: result.Score}
}

// rematchChangedJobCreate gives a job without a row the best manual match over all profiles, exactly like AutoMatchJobs.
func rematchChangedJobCreate(scorer *ProfileScorer, jobID string) ChangedJobOutcome {
	fail := func(err error) ChangedJobOutcome {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobFailed, Err: err}
	}
	if len(scorer.profiles) == 0 {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobUnjudged}
	}
	profileID, result, ok, err := scorer.Best(jobID)
	if errors.Is(err, jobs.ErrUnknownJob) {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobUnknownJob}
	}
	if err != nil {
		return fail(err)
	}
	if !ok {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobUnjudged}
	}
	stored, err := writeUnderProfileLock(profileID, func() (bool, error) { return storeManualMatchIfAbsent(jobID, profileID, result) })
	if err != nil {
		return fail(err)
	}
	if !stored {
		return ChangedJobOutcome{JobID: jobID, Status: ChangedJobSkippedExisting}
	}
	return ChangedJobOutcome{JobID: jobID, Status: ChangedJobCreated, ProfileID: profileID, Score: result.Score}
}

// replaceManualMatchIfStillManual writes a fresh manual result over the job's row, but only while that row is still a finished
// manual result of the same profile: a person who started a run meanwhile (an AI conversation, 'matching') is never overwritten.
// The row and its skills change in one transaction. It reports whether it wrote.
func replaceManualMatchIfStillManual(jobID, profileID string, r jobmatcher.Result) (bool, error) {
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
		`UPDATE job_matches SET persona_id = ?, score = ?, breakdown_json = ?, error = NULL, updated_at = datetime('now')
		 WHERE job_id = ? AND profile_id = ? AND kind = 'manual' AND status = 'completed'`,
		r.PersonaID, r.Score, string(raw), jobID, profileID,
	)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.Exec(`DELETE FROM job_match_skills WHERE job_id = ?`, jobID); err != nil {
		return false, err
	}
	for _, s := range r.MatchedSkills {
		if _, err := tx.Exec(`INSERT INTO job_match_skills (job_id, skill) VALUES (?, ?)`, jobID, s); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// writeUnderProfileLock runs a write of one profile's result while holding the profile's lock (the one RematchProfile and the
// purge of a deleted profile's matches use) and only if the profile still exists, so a re-match never leaves a match of a
// profile that is being deleted. A profile that is gone makes the write report false, not an error.
func writeUnderProfileLock(profileID string, write func() (bool, error)) (bool, error) {
	defer jobs.LockProfile(profileID)()
	if err := profile.RequireProfileExists(profileID); err != nil {
		if errors.Is(err, profile.ErrUnknownProfile) {
			return false, nil
		}
		return false, err
	}
	return write()
}
