package jobs

import (
	"errors"
	"sort"

	"career-tool-backend/db"
)

// The removal of the matches of a deleted profile or persona (step 152). The matches live in jobs.db, which career.db cannot cascade
// to, so a deleted profile or persona leaves them behind ("zombie" matches) unless they are removed by code. The consumers of
// "career.profile.deleted" and "career.persona.deleted" call these. See
// plan/ai/tools/career/step-152-profile-persona-deleted-events.md.

// ErrNoID is returned when a purge is asked for an empty id: it would match nothing useful, and a purge must never be wider than asked.
var ErrNoID = errors.New("an id is required")

// PurgeProfileMatches removes everything the matches of a deleted profile left behind, in one transaction and under the profile's
// re-match lock (so a re-match that is running finishes first, and one that starts afterwards finds no profile and writes nothing):
//   - every job_matches row of the profile, whatever its kind or status (a running or failed AI match too), with its job_match_skills;
//   - the profile's persona scores (job_persona_matches) and score states (persona_score_state);
//   - the profile's re-match state (profile_match_state).
//
// It returns the ids of the jobs that lost their match, sorted, so the caller can have them matched again by the profiles that remain.
// It is idempotent (a second call removes nothing and returns nothing) and touches no other profile's rows.
func PurgeProfileMatches(profileID string) ([]string, error) {
	if profileID == "" {
		return nil, ErrNoID
	}
	defer lockProfile(profileID)()
	return purge(
		`SELECT job_id FROM job_matches WHERE profile_id = ?`, []any{profileID},
		[]step{
			{`DELETE FROM job_match_skills WHERE job_id IN (SELECT job_id FROM job_matches WHERE profile_id = ?)`, []any{profileID}},
			{`DELETE FROM job_matches WHERE profile_id = ?`, []any{profileID}},
			{`DELETE FROM job_persona_matches WHERE profile_id = ?`, []any{profileID}},
			{`DELETE FROM persona_score_state WHERE profile_id = ?`, []any{profileID}},
			{`DELETE FROM profile_match_state WHERE profile_id = ?`, []any{profileID}},
		})
}

// PurgePersonaMatches removes everything the matches of a deleted persona left behind, in one transaction and under its profile's
// re-match lock:
//   - the job_matches rows of that profile whose best persona was this one, with their job_match_skills (a job whose stored match came
//     from another persona of the profile, or from another profile, is untouched; so is a running AI match, which has no persona yet);
//   - the persona's scores (job_persona_matches) and score state (persona_score_state).
//
// It returns the ids of the jobs that lost their match, sorted. Both ids are required (the profile scopes the removal). Idempotent.
func PurgePersonaMatches(personaID, profileID string) ([]string, error) {
	if personaID == "" || profileID == "" {
		return nil, ErrNoID
	}
	defer lockProfile(profileID)()
	return purge(
		`SELECT job_id FROM job_matches WHERE persona_id = ? AND profile_id = ?`, []any{personaID, profileID},
		[]step{
			{`DELETE FROM job_match_skills WHERE job_id IN (SELECT job_id FROM job_matches WHERE persona_id = ? AND profile_id = ?)`, []any{personaID, profileID}},
			{`DELETE FROM job_matches WHERE persona_id = ? AND profile_id = ?`, []any{personaID, profileID}},
			{`DELETE FROM job_persona_matches WHERE persona_id = ?`, []any{personaID}},
			{`DELETE FROM persona_score_state WHERE persona_id = ?`, []any{personaID}},
		})
}

type step struct {
	query string
	args  []any
}

// purge reads the ids of the jobs about to lose their match, then runs the deletes, all in one transaction: either everything goes or
// nothing does.
func purge(freedQuery string, freedArgs []any, steps []step) ([]string, error) {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(freedQuery, freedArgs...)
	if err != nil {
		return nil, err
	}
	var freed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		freed = append(freed, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, s := range steps {
		if _, err := tx.Exec(s.query, s.args...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	sort.Strings(freed)
	return freed, nil
}
