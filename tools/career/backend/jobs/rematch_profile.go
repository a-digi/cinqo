package jobs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/jobs/jobmatcher"
)

// Re-matching the jobs for one profile (step 151). A profile or persona that is new can fit a job better than the profile whose
// result job_matches holds, and AutoMatchJobs only ever fills jobs that have no match. RematchProfile scores one profile against
// every job with the manual engine (no AI) and lets its result in only where the rules below allow it. See
// plan/ai/tools/career/step-151-profile-persona-created-rematch.md.
//
//	stored job_matches row                               | what happens
//	none                                                 | this profile's result is stored
//	manual, completed, a lower score than this profile's | replaced by this profile's result
//	manual, completed, an equal or higher score          | left alone
//	ai, matching or failed                               | never touched

// rematchChunk is how many job ids are read at a time, so the job list is never held open while jobs are scored.
var rematchChunk = 200

// afterRematchChunk runs after each chunk of jobs was handled: a hook for tests (create a persona while a run goes on).
var afterRematchChunk = func() {}

// RematchReport says what RematchProfile did; the counts add up to Jobs.
type RematchReport struct {
	Jobs             int // the jobs looked at
	Stored           int // jobs that had no match and got this profile's
	Improved         int // jobs whose manual match was replaced by a higher score of this profile
	Unchanged        int // jobs whose manual match is at least as good
	SkippedProtected int // jobs whose match is an AI, running or failed one
	Unjudged         int // jobs this profile could not judge (or that were deleted meanwhile)
	Failed           int // jobs that could not be read, scored or stored
}

var rematchLocks sync.Map // profile id -> *sync.Mutex

// lockProfile takes the profile's lock (a re-match and a purge of the profile's matches never overlap) and returns the unlock.
func lockProfile(profileID string) (unlock func()) {
	lock, _ := rematchLocks.LoadOrStore(profileID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// RematchProfile re-matches every job for one profile. It is idempotent (a second run changes nothing) and two runs for the same
// profile never overlap (the second waits). A profile that does not exist, or that cannot judge (no persona, too little data), changes
// nothing. An error is returned only when the run cannot start (the profile or the job list cannot be read); a problem with one job
// is counted in Failed and the rest go on.
func RematchProfile(profileID string) (RematchReport, error) {
	var report RematchReport
	if profileID == "" {
		return report, nil
	}
	defer lockProfile(profileID)()

	// The fingerprint is read before anything is scored, so a persona created while the run goes on leaves the profile stale for the sweeper.
	fingerprint, exists, err := ProfileFingerprint(profileID)
	if err != nil {
		return report, err
	}
	scorer, err := NewProfileScorerFor(map[string]bool{profileID: true})
	if err != nil {
		return report, err
	}
	after := ""
	for {
		ids, err := jobIDsAfter(after, rematchChunk)
		if err != nil {
			return report, err
		}
		if len(ids) == 0 {
			// The run is complete (a job that failed is counted and logged by the caller, not retried for ever): remember what it was made from.
			if exists {
				if err := setProfileMatchState(profileID, fingerprint); err != nil {
					return report, err
				}
			}
			return report, nil
		}
		for _, id := range ids {
			report.Jobs++
			switch rematchOne(scorer, profileID, id) {
			case rematchStored:
				report.Stored++
			case rematchImproved:
				report.Improved++
			case rematchUnchanged:
				report.Unchanged++
			case rematchProtected:
				report.SkippedProtected++
			case rematchUnjudged:
				report.Unjudged++
			default:
				report.Failed++
			}
		}
		after = ids[len(ids)-1]
		afterRematchChunk()
	}
}

type rematchOutcome int

const (
	rematchFailed rematchOutcome = iota
	rematchStored
	rematchImproved
	rematchUnchanged
	rematchProtected
	rematchUnjudged
)

func rematchOne(scorer *ProfileScorer, profileID, jobID string) rematchOutcome {
	if len(scorer.profiles) == 0 {
		return rematchUnjudged // this profile cannot judge anything: the job is not even read
	}
	// A protected row is found before any scoring, so it costs nothing.
	var kind, status string
	var score sql.NullInt64
	err := db.JobsDB.QueryRow(`SELECT kind, status, score FROM job_matches WHERE job_id = ?`, jobID).Scan(&kind, &status, &score)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return rematchFailed
	case kind != "manual" || status != "completed":
		return rematchProtected
	}
	_, result, ok, err := scorer.Best(jobID)
	if errors.Is(err, ErrUnknownJob) {
		return rematchUnjudged
	}
	if err != nil {
		return rematchFailed
	}
	if !ok {
		return rematchUnjudged
	}
	outcome, err := storeIfBetter(jobID, profileID, result)
	if err != nil {
		return rematchFailed
	}
	return outcome
}

// storeIfBetter writes the result in one transaction under the rules of the table above, deciding from the row as it is inside the
// transaction, so a match that appeared or changed since the first look is respected (an AI match that started meanwhile is never
// overwritten).
func storeIfBetter(jobID, profileID string, r jobmatcher.Result) (rematchOutcome, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return rematchFailed, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return rematchFailed, err
	}
	defer func() { _ = tx.Rollback() }()
	var kind, status string
	var score sql.NullInt64
	outcome := rematchStored
	switch err := tx.QueryRow(`SELECT kind, status, score FROM job_matches WHERE job_id = ?`, jobID).Scan(&kind, &status, &score); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return rematchFailed, err
	case kind != "manual" || status != "completed":
		return rematchProtected, nil
	case score.Valid && int64(r.Score) <= score.Int64:
		return rematchUnchanged, nil
	default:
		outcome = rematchImproved
	}
	if _, err := tx.Exec(
		`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, conversation_id, error, kind, breakdown_json, updated_at)
		 VALUES (?, ?, ?, ?, 'completed', NULL, NULL, 'manual', ?, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			persona_id = excluded.persona_id,
			score = excluded.score,
			status = 'completed',
			conversation_id = NULL,
			error = NULL,
			kind = 'manual',
			breakdown_json = excluded.breakdown_json,
			updated_at = datetime('now')`,
		jobID, profileID, r.PersonaID, r.Score, string(raw),
	); err != nil {
		return rematchFailed, err
	}
	if _, err := tx.Exec(`DELETE FROM job_match_skills WHERE job_id = ?`, jobID); err != nil {
		return rematchFailed, err
	}
	for _, s := range r.MatchedSkills {
		if _, err := tx.Exec(`INSERT INTO job_match_skills (job_id, skill) VALUES (?, ?)`, jobID, s); err != nil {
			return rematchFailed, err
		}
	}
	if err := tx.Commit(); err != nil {
		return rematchFailed, err
	}
	return outcome, nil
}
