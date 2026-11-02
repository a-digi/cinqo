package jobs

import (
	"strings"

	"career-tool-backend/db"
)

// stale_matches.go (step 164): the catch-up for "career.job.description_changed" events that never arrived. Events are best
// effort (core drops one after three tries, a full queue refuses one, a killed tool loses what is waiting). A write that
// changes a job's text sets jobs.description_changed_at; a match made before that moment was made from the old text. These
// queries find such jobs so the sweepers can hand them to the same consumers the event goes to. A job whose text never
// changed after it was found (description_changed_at is NULL) is never offered. Timestamps are all datetime('now') text, so
// they compare as text; "at or before" (<=) is deliberate: a match made in the very second of the change cannot be told from
// one made just before it, and handling the job once more costs one cheap scoring and then moves the match past the stamp.
// See plan/ai/tools/career/step-164-job-description-changed-rematch.md.

// StaleManualMatchJobIDs returns up to limit ids of jobs whose job_matches row is a finished manual result made at or before
// the job's last text change, the most recently changed first. An AI result, a run in progress and a job without a row are
// never offered here (the first two are never replaced, the third is the unmatched sweeper's).
func StaleManualMatchJobIDs(limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	return queryIDs(
		`SELECT j.id FROM jobs j JOIN job_matches m ON m.job_id = j.id
		 WHERE j.description_changed_at IS NOT NULL AND m.kind = 'manual' AND m.status = 'completed' AND m.updated_at <= j.description_changed_at
		 ORDER BY j.description_changed_at DESC, j.id LIMIT ?`, limit)
}

// JobsNeedingRulesCheck returns up to limit ids of jobs whose text changed and that the profile rules consumer has not looked
// at since (never, or at or before the change), the most recently changed first.
func JobsNeedingRulesCheck(limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	return queryIDs(
		`SELECT id FROM jobs
		 WHERE description_changed_at IS NOT NULL AND (rules_checked_at IS NULL OR rules_checked_at <= description_changed_at)
		 ORDER BY description_changed_at DESC, id LIMIT ?`, limit)
}

// MarkRulesChecked records that the profile rules consumer looked at these jobs just now, whatever it found. An unknown id is
// ignored. It is what ends the offer of a job in JobsNeedingRulesCheck.
func MarkRulesChecked(ids []string) error {
	var clean []any
	for _, id := range ids {
		if id != "" {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	_, err := db.JobsDB.Exec(
		`UPDATE jobs SET rules_checked_at = datetime('now') WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(clean)), ",")+`)`, clean...)
	return err
}

func queryIDs(query string, limit int) ([]string, error) {
	rows, err := db.JobsDB.Query(query, limit)
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
