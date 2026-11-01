// Package profilerules holds a profile's auto-add rules (step 140): "a new job that matches this profile at least
// MinPercent percent is added to the profile's job board with this status". A profile can have several rules, in an order
// the person sets (step 143): a rule fits a score when the score reaches its percentage, and the rule that applies is the
// FIRST fitting rule of the list (RuleFor); the percentage alone no longer decides between two rules that both fit. The
// rules live in jobs.db next to the statuses they point to; the profile lives in career.db, so the callers' checks
// (RequireProfileExists) are made here. See plan/ai/tools/career/step-140-profile-job-match-consumer.md and
// step-143-apply-rules-to-existing-jobs.md.
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
	ErrBadOrder         = errors.New("the order must list exactly this profile's rules, each once")
)

// Rule is one auto-add rule with the name of its status (for display).
type Rule struct {
	ID         string `json:"id"`
	ProfileID  string `json:"profileId"`
	MinPercent int    `json:"minPercent"`
	StatusID   string `json:"statusId"`
	StatusName string `json:"statusName"`
	// Position is the rule's place in the profile's list, 0 first (the lists are returned in this order).
	Position int `json:"position"`
}

const selectRules = `SELECT r.id, r.profile_id, r.min_percent, r.status_id, s.name, r.position
	FROM profile_match_rules r JOIN job_statuses s ON s.id = r.status_id`

// ruleOrder is the order every list is read in: the person's order, with the creation time and the id fixing ties.
const ruleOrder = `r.position, r.created_at, r.id`

func scanRules(rows *sql.Rows) ([]Rule, error) {
	defer rows.Close()
	rules := []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.ProfileID, &r.MinPercent, &r.StatusID, &r.StatusName, &r.Position); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// List returns a profile's rules in their order, the one that is tried first first (never nil). profile.ErrUnknownProfile for a profile that
// does not exist.
func List(profileID string) ([]Rule, error) {
	if err := profile.RequireProfileExists(profileID); err != nil {
		return nil, err
	}
	rows, err := db.JobsDB.Query(selectRules+` WHERE r.profile_id = ? ORDER BY `+ruleOrder, profileID)
	if err != nil {
		return nil, err
	}
	return scanRules(rows)
}

// All returns every profile's rules, by profile id, each list in the rules' order. Profiles without rules are
// absent. This is what the consumer reads once per batch; it does not check that the profiles still exist (a rule of a
// deleted profile is only ever purged, never applied: the consumer scores existing profiles only).
func All() (map[string][]Rule, error) {
	rows, err := db.JobsDB.Query(selectRules + ` ORDER BY r.profile_id, ` + ruleOrder)
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

// RuleFor is the rule that applies to a score: the FIRST rule of the list that fits it (the score is at least the rule's
// percentage). The list must be in the rules' order, which List and All return. ok is false when no rule fits (or there
// are none). A later rule with a higher percentage does not beat an earlier one that fits.
func RuleFor(rules []Rule, score int) (Rule, bool) {
	for _, r := range rules {
		if score >= r.MinPercent {
			return r, true
		}
	}
	return Rule{}, false
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
	err := tx.QueryRow(selectRules+` WHERE r.id = ?`, id).Scan(&r.ID, &r.ProfileID, &r.MinPercent, &r.StatusID, &r.StatusName, &r.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrUnknownRule
	}
	return r, err
}

// Create adds a rule to the end of a profile's list. Errors: profile.ErrUnknownProfile, ErrInvalidPercent, jobboard.ErrUnknownStatus,
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
	// a new rule goes to the end of the list
	if _, err := tx.Exec(
		`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id, position)
		 VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(position) + 1, 0) FROM profile_match_rules WHERE profile_id = ?))`,
		id, profileID, minPercent, statusID, profileID,
	); err != nil {
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

// Reorder sets the order of a profile's rules to the one given (the first id is tried first) and returns the rules in
// that order. The ids must be exactly the profile's rules, each once; anything else (a rule missing, one that is not
// this profile's, one listed twice) is ErrBadOrder and changes nothing, so two people reordering at once, or a rule added
// in another tab, cannot corrupt the order. profile.ErrUnknownProfile for a profile that does not exist.
func Reorder(profileID string, ruleIDs []string) ([]Rule, error) {
	if err := profile.RequireProfileExists(profileID); err != nil {
		return nil, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT id FROM profile_match_rules WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		have[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ruleIDs) != len(have) {
		return nil, ErrBadOrder
	}
	seen := map[string]bool{}
	for _, id := range ruleIDs {
		if !have[id] || seen[id] {
			return nil, ErrBadOrder
		}
		seen[id] = true
	}
	for position, id := range ruleIDs {
		if _, err := tx.Exec(`UPDATE profile_match_rules SET position = ? WHERE id = ? AND profile_id = ?`, position, id, profileID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return List(profileID)
}
