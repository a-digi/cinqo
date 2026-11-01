// Package personamatch holds how well each persona fits each job (step 141): one row per (job, persona) in jobs.db. A
// persona that could not judge the job has no row. The package imports only db, so persona and profile can call the
// purges when a persona or a profile is deleted (jobs.db cannot cascade from career.db). The rows of a deleted job go
// with it (ON DELETE CASCADE). See plan/ai/tools/career/step-141-persona-job-match.md.
package personamatch

import (
	"database/sql"
	"errors"
	"strings"

	"career-tool-backend/db"
)

// ErrUnknownJob is returned when the job does not exist (it was deleted).
var ErrUnknownJob = errors.New("unknown job")

// Row is one persona's score for a job.
type Row struct {
	PersonaID     string `json:"personaId"`
	ProfileID     string `json:"profileId"`
	Score         int    `json:"score"`
	EngineVersion string `json:"engineVersion,omitempty"`
}

// Replace makes rows the whole set of a job's persona scores, in one transaction: the old rows go, the new ones are
// written, and a reader never sees half a set. An empty set removes the job's rows. A persona listed twice keeps its
// first entry. ErrUnknownJob when the job does not exist (nothing is written); a score outside 0 to 100 is refused by
// the table and rolls everything back.
func Replace(jobID string, rows []Row) error {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM jobs WHERE id = ?`, jobID).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return ErrUnknownJob
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_persona_matches WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.PersonaID] {
			continue
		}
		seen[r.PersonaID] = true
		if _, err := tx.Exec(
			`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score, engine_version) VALUES (?, ?, ?, ?, ?)`,
			jobID, r.PersonaID, r.ProfileID, r.Score, r.EngineVersion,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanRows(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.PersonaID, &r.ProfileID, &r.Score, &r.EngineVersion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ForJob returns every persona's score for a job, highest first; equal scores come in persona id order (the display
// layer breaks ties by the order the personas were created, which only it knows). Never nil.
func ForJob(jobID string) ([]Row, error) {
	rows, err := db.JobsDB.Query(
		`SELECT persona_id, profile_id, score, engine_version FROM job_persona_matches WHERE job_id = ? ORDER BY score DESC, persona_id`, jobID)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

// chunk is how many job ids one query takes (SQLite limits the number of parameters).
const chunk = 400

// ForJobs returns, by job id, the scores of one profile's personas for the given jobs, each list highest first (equal
// scores in persona id order). A job without rows is absent from the map.
func ForJobs(profileID string, jobIDs []string) (map[string][]Row, error) {
	out := map[string][]Row{}
	for start := 0; start < len(jobIDs); start += chunk {
		part := jobIDs[start:min(len(jobIDs), start+chunk)]
		args := make([]any, 0, len(part)+1)
		args = append(args, profileID)
		for _, id := range part {
			args = append(args, id)
		}
		rows, err := db.JobsDB.Query(
			`SELECT job_id, persona_id, profile_id, score, engine_version FROM job_persona_matches
			 WHERE profile_id = ? AND job_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")+`)
			 ORDER BY job_id, score DESC, persona_id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var jobID string
			var r Row
			if err := rows.Scan(&jobID, &r.PersonaID, &r.ProfileID, &r.Score, &r.EngineVersion); err != nil {
				rows.Close()
				return nil, err
			}
			out[jobID] = append(out[jobID], r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// PurgePersona removes every row of a deleted persona and its score state; a no-op for one that has none.
func PurgePersona(personaID string) error {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM job_persona_matches WHERE persona_id = ?`, personaID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM persona_score_state WHERE persona_id = ?`, personaID); err != nil {
		return err
	}
	return tx.Commit()
}

// PurgeProfile removes every row and score state of a deleted profile's personas.
func PurgeProfile(profileID string) error {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM job_persona_matches WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM persona_score_state WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteForJob removes every row of a job (what the job-deleted consumer does; the cascade has normally done it
// already); a no-op for a job that has none.
func DeleteForJob(jobID string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM job_persona_matches WHERE job_id = ?`, jobID)
	return err
}

// JobsWithoutRows returns up to limit job ids that have no row at all, newest crawled first (then by id): the jobs the
// catch-up sweeper hands to the consumer. A job that no persona can judge stays in this list (it never gets a row), so
// the caller must expect to see it again.
func JobsWithoutRows(limit int) ([]string, error) {
	rows, err := db.JobsDB.Query(
		`SELECT j.id FROM jobs j WHERE NOT EXISTS (SELECT 1 FROM job_persona_matches m WHERE m.job_id = j.id)
		 ORDER BY j.crawled_at DESC, j.id LIMIT ?`, limit)
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

// PersonaJobScore is one job's result for one persona: Judged false means the persona could not judge the job, so its row
// (if it has one) goes.
type PersonaJobScore struct {
	JobID         string
	Judged        bool
	Score         int
	EngineVersion string
}

// ReplaceForPersona brings one persona's rows up to date for the listed jobs, in one transaction: a judged job gets its
// row written (or updated), a job the persona could not judge loses its row, and every other row (of other jobs, of
// other personas) is left alone. A job that no longer exists is skipped without an error (a delete during a long run).
// A score outside 0 to 100 is refused by the table and rolls the whole call back.
func ReplaceForPersona(personaID, profileID string, scores []PersonaJobScore) error {
	if len(scores) == 0 {
		return nil
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, sc := range scores {
		if _, err := tx.Exec(`DELETE FROM job_persona_matches WHERE job_id = ? AND persona_id = ?`, sc.JobID, personaID); err != nil {
			return err
		}
		if !sc.Judged {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score, engine_version)
			 SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM jobs WHERE id = ?)`,
			sc.JobID, personaID, profileID, sc.Score, sc.EngineVersion, sc.JobID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// State is when and from what a persona's scores for all jobs were last made.
type State struct {
	Fingerprint string
	ScoredAt    string
}

// GetState reads a persona's score state; ok is false for a persona that has none.
func GetState(personaID string) (State, bool, error) {
	var s State
	err := db.JobsDB.QueryRow(`SELECT fingerprint, scored_at FROM persona_score_state WHERE persona_id = ?`, personaID).Scan(&s.Fingerprint, &s.ScoredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	return s, true, nil
}

// SetState records that a persona's scores for all jobs were made from data with this fingerprint, now.
func SetState(personaID, profileID, fingerprint string) error {
	_, err := db.JobsDB.Exec(
		`INSERT INTO persona_score_state (persona_id, profile_id, fingerprint, scored_at) VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(persona_id) DO UPDATE SET profile_id = excluded.profile_id, fingerprint = excluded.fingerprint, scored_at = excluded.scored_at`,
		personaID, profileID, fingerprint)
	return err
}

// AllStates reads every persona's score state, by persona id (the sweeper compares them with the personas' current
// fingerprints). Never nil.
func AllStates() (map[string]State, error) {
	rows, err := db.JobsDB.Query(`SELECT persona_id, fingerprint, scored_at FROM persona_score_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]State{}
	for rows.Next() {
		var id string
		var s State
		if err := rows.Scan(&id, &s.Fingerprint, &s.ScoredAt); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}
