// Package profilerules holds a profile's auto-add rules (step 140): "a new job that matches this profile at least
// MinPercent percent is added to the profile's job board with this status". A profile can have several rules; for a
// score the rule with the highest threshold the score reaches applies (RuleFor). The rules live in jobs.db next to the
// statuses they point to; the profile lives in career.db, so the callers' checks (RequireProfileExists) are made here.
// See plan/ai/tools/career/step-140-profile-job-match-consumer.md.
package profilerules

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
)

const (
	// MinPercent and MaxPercent bound a rule's threshold.
	MinPercent = 1
	MaxPercent = 100
	// MaxRulesPerProfile is how many rules one profile may have.
	MaxRulesPerProfile = 10
)

var (
	ErrInvalidPercent   = errors.New("the percentage must be a whole number from 1 to 100")
	ErrDuplicatePercent = errors.New("this profile already has a rule for this percentage")
	ErrTooManyRules     = errors.New("a profile can have at most 10 rules")
	ErrUnknownRule      = errors.New("unknown rule")
)

// Rule is one auto-add rule with the name of its status (for display).
type Rule struct {
	ID         string `json:"id"`
	ProfileID  string `json:"profileId"`
	MinPercent int    `json:"minPercent"`
	StatusID   string `json:"statusId"`
	StatusName string `json:"statusName"`
}

const selectRules = `SELECT r.id, r.profile_id, r.min_percent, r.status_id, s.name
	FROM profile_match_rules r JOIN job_statuses s ON s.id = r.status_id`

func scanRules(rows *sql.Rows) ([]Rule, error) {
	defer rows.Close()
	rules := []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.ProfileID, &r.MinPercent, &r.StatusID, &r.StatusName); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// List returns a profile's rules, highest percentage first (never nil). profile.ErrUnknownProfile for a profile that
// does not exist.
func List(profileID string) ([]Rule, error) {
	if err := profile.RequireProfileExists(profileID); err != nil {
		return nil, err
	}
	rows, err := db.JobsDB.Query(selectRules+` WHERE r.profile_id = ? ORDER BY r.min_percent DESC`, profileID)
	if err != nil {
		return nil, err
	}
	return scanRules(rows)
}

// All returns every profile's rules, by profile id, each list highest percentage first. Profiles without rules are
// absent. This is what the consumer reads once per batch; it does not check that the profiles still exist (a rule of a
// deleted profile is only ever purged, never applied: the consumer scores existing profiles only).
func All() (map[string][]Rule, error) {
	rows, err := db.JobsDB.Query(selectRules + ` ORDER BY r.profile_id, r.min_percent DESC`)
	if err != nil {
		return nil, err
	}
	rules, err := scanRules(rows)
	if err != nil {
		return nil, err
	}
	out := map[string][]Rule{}
	for _, r := range rules {
		out[r.ProfileID] = append(out[r.ProfileID], r)
	}
	return out, nil
}

// RuleFor is the rule that applies to a score: the one with the highest percentage that the score reaches (score at
// least that percentage). ok is false when the score is below every rule (or there are none). The order of rules does
// not matter.
func RuleFor(rules []Rule, score int) (Rule, bool) {
	var best Rule
	found := false
	for _, r := range rules {
		if score >= r.MinPercent && (!found || r.MinPercent > best.MinPercent) {
			best, found = r, true
		}
	}
	return best, found
}

// checkStatus makes sure a status exists and may be used by the profile: a fixed one, or one of its own custom ones.
func checkStatus(tx *sql.Tx, profileID, statusID string) error {
	var owner sql.NullString
	err := tx.QueryRow(`SELECT profile_id FROM job_statuses WHERE id = ?`, statusID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return jobboard.ErrUnknownStatus
	}
	if err != nil {
		return err
	}
	if owner.Valid && owner.String != profileID {
		return jobboard.ErrStatusNotOnBoard
	}
	return nil
}

func validPercent(p int) bool { return p >= MinPercent && p <= MaxPercent }

func get(tx *sql.Tx, id string) (Rule, error) {
	var r Rule
	err := tx.QueryRow(selectRules+` WHERE r.id = ?`, id).Scan(&r.ID, &r.ProfileID, &r.MinPercent, &r.StatusID, &r.StatusName)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrUnknownRule
	}
	return r, err
}

// Create adds a rule to a profile. Errors: profile.ErrUnknownProfile, ErrInvalidPercent, jobboard.ErrUnknownStatus,
// jobboard.ErrStatusNotOnBoard (a custom status of another profile), ErrTooManyRules, ErrDuplicatePercent.
func Create(profileID string, minPercent int, statusID string) (Rule, error) {
	if err := profile.RequireProfileExists(profileID); err != nil {
		return Rule{}, err
	}
	if !validPercent(minPercent) {
		return Rule{}, ErrInvalidPercent
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := checkStatus(tx, profileID, statusID); err != nil {
		return Rule{}, err
	}
	var count, same int
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(min_percent = ?), 0) FROM profile_match_rules WHERE profile_id = ?`, minPercent, profileID).Scan(&count, &same); err != nil {
		return Rule{}, err
	}
	if same > 0 {
		return Rule{}, ErrDuplicatePercent
	}
	if count >= MaxRulesPerProfile {
		return Rule{}, ErrTooManyRules
	}
	id := uuid.NewString()
	if _, err := tx.Exec(`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES (?, ?, ?, ?)`, id, profileID, minPercent, statusID); err != nil {
		return Rule{}, err
	}
	rule, err := get(tx, id)
	if err != nil {
		return Rule{}, err
	}
	return rule, tx.Commit()
}

// Update changes a rule's percentage and status (never its profile). Errors: ErrUnknownRule, ErrInvalidPercent,
// jobboard.ErrUnknownStatus, jobboard.ErrStatusNotOnBoard, ErrDuplicatePercent (another rule of the profile has it).
func Update(id string, minPercent int, statusID string) (Rule, error) {
	if !validPercent(minPercent) {
		return Rule{}, ErrInvalidPercent
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := get(tx, id)
	if err != nil {
		return Rule{}, err
	}
	if err := checkStatus(tx, current.ProfileID, statusID); err != nil {
		return Rule{}, err
	}
	var same int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ? AND min_percent = ? AND id <> ?`, current.ProfileID, minPercent, id).Scan(&same); err != nil {
		return Rule{}, err
	}
	if same > 0 {
		return Rule{}, ErrDuplicatePercent
	}
	if _, err := tx.Exec(`UPDATE profile_match_rules SET min_percent = ?, status_id = ?, updated_at = datetime('now') WHERE id = ?`, minPercent, statusID, id); err != nil {
		return Rule{}, err
	}
	rule, err := get(tx, id)
	if err != nil {
		return Rule{}, err
	}
	return rule, tx.Commit()
}

// Delete removes a rule; a benign no-op for an unknown id, like every delete in this tool.
func Delete(id string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM profile_match_rules WHERE id = ?`, id)
	return err
}
