package profilematch

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"career-tool-backend/db"
)

// The catch-up state of the profile re-match (step 151). A profile's fingerprint is a hash of the ids of its personas: a profile or a
// persona that was created changes it, an edit of a persona (a skill, the details) does not, because editing a persona is not
// something this feature re-matches for (plan open question 5). RematchProfile stores the fingerprint it started from when a run
// completes; the sweeper hands over the profiles whose fingerprint now differs. See
// plan/ai/tools/career/step-151-profile-persona-created-rematch.md.

// ProfileFingerprint is the fingerprint of a profile's personas as they are now; ok is false when the profile does not exist.
func ProfileFingerprint(profileID string) (fingerprint string, ok bool, err error) {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, profileID).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	ids, err := personaIDsOf(profileID)
	if err != nil {
		return "", false, err
	}
	return fingerprintOfIDs(ids), true, nil
}

func personaIDsOf(profileID string) ([]string, error) {
	rows, err := db.CareerDB.Query(`SELECT id FROM personas WHERE profile_id = ?`, profileID)
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

func fingerprintOfIDs(ids []string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

func setProfileMatchState(profileID, fingerprint string) error {
	_, err := db.JobsDB.Exec(
		`INSERT INTO profile_match_state (profile_id, fingerprint, matched_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(profile_id) DO UPDATE SET fingerprint = excluded.fingerprint, matched_at = excluded.matched_at`,
		profileID, fingerprint)
	return err
}

func allProfileMatchStates() (map[string]string, error) {
	rows, err := db.JobsDB.Query(`SELECT profile_id, fingerprint FROM profile_match_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, fp string
		if err := rows.Scan(&id, &fp); err != nil {
			return nil, err
		}
		out[id] = fp
	}
	return out, rows.Err()
}

// profileFingerprints reads every profile with its fingerprint, oldest profile first.
func profileFingerprints() (ids []string, fps map[string]string, err error) {
	rows, err := db.CareerDB.Query(`SELECT id FROM profiles ORDER BY created_at, id`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	fps = make(map[string]string, len(ids))
	for _, id := range ids {
		pids, err := personaIDsOf(id)
		if err != nil {
			return nil, nil, err
		}
		fps[id] = fingerprintOfIDs(pids)
	}
	return ids, fps, nil
}

// SeedProfileMatchState gives the profiles that exist the fingerprint they have now, without re-matching anything, but only the first
// time it runs (a marker row remembers that): installing the feature changes none of the existing matches, and only profiles and
// personas created afterwards are re-matched. It reports how many profiles it seeded (0 when it had already run). A profile that
// already has a state keeps it.
func SeedProfileMatchState() (int, error) {
	var one int
	switch err := db.JobsDB.QueryRow(`SELECT 1 FROM profile_match_seed WHERE id = 1`).Scan(&one); {
	case err == nil:
		return 0, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, err
	}
	ids, fps, err := profileFingerprints()
	if err != nil {
		return 0, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	seeded := 0
	for _, id := range ids {
		res, err := tx.Exec(`INSERT INTO profile_match_state (profile_id, fingerprint) VALUES (?, ?) ON CONFLICT(profile_id) DO NOTHING`, id, fps[id])
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			seeded++
		}
	}
	if _, err := tx.Exec(`INSERT INTO profile_match_seed (id) VALUES (1) ON CONFLICT(id) DO NOTHING`); err != nil {
		return 0, err
	}
	return seeded, tx.Commit()
}

// StaleProfiles lists the profiles whose jobs are not known to be matched for their personas as they are now: the ones with no state
// and the ones whose stored fingerprint differs, oldest profile first. It also removes the state of profiles that no longer exist.
func StaleProfiles() ([]string, error) {
	ids, fps, err := profileFingerprints()
	if err != nil {
		return nil, err
	}
	states, err := allProfileMatchStates()
	if err != nil {
		return nil, err
	}
	stale := []string{}
	for _, id := range ids {
		if fp, ok := states[id]; !ok || fp != fps[id] {
			stale = append(stale, id)
		}
	}
	for id := range states {
		if _, ok := fps[id]; !ok {
			if _, err := db.JobsDB.Exec(`DELETE FROM profile_match_state WHERE profile_id = ?`, id); err != nil {
				return nil, err
			}
		}
	}
	return stale, nil
}
