package jobs

import (
	"sort"
	"strings"

	"career-tool-backend/db"
)

// The orphans: match rows that name a profile or a persona that no longer exists (step 152). The consumers of "career.profile.deleted"
// and "career.persona.deleted" remove the matches of a deletion as it happens; this is the catch-up for the ones they could not: a
// lost event, a tool that was stopped, an AI match that finished after its profile was deleted, and the matches that were left behind
// before the consumers existed. The sweeper of package matchorphans calls PurgeOrphans; OrphanMatches is the same detection without
// removing anything (a preview). See plan/ai/tools/career/step-152-profile-persona-deleted-events.md.

// OrphanMatch is one job match that is about to be, or was, removed: the job, what the match says and why it is an orphan.
type OrphanMatch struct {
	JobID     string
	Title     string
	ProfileID string
	PersonaID string // empty when the match has no persona (a running AI match)
	Kind      string
	Status    string
	Score     int // 0 when the match has no score
	HasScore  bool
	Reason    string // "profile deleted" or "persona deleted"
}

// OrphanReport says what PurgeOrphans removed.
type OrphanReport struct {
	Profiles      int // orphan profiles found (ids that no longer exist)
	Personas      int // orphan personas found under profiles that do exist
	Matches       int // job_matches rows removed
	PersonaScores int // job_persona_matches rows removed
	States        int // persona_score_state and profile_match_state rows removed
}

type personaPair struct{ persona, profile string }

// orphanSet is what the detection found: ids that do not exist any more.
type orphanSet struct {
	profiles map[string]bool // profile ids absent from career.db
	pairs    []personaPair   // (persona, profile) pairs of an existing profile whose persona is gone (or belongs to another profile)
}

func (o orphanSet) empty() bool { return len(o.profiles) == 0 && len(o.pairs) == 0 }

// afterOrphanListing runs after the ids were read from jobs.db and before career.db is asked whether they exist: a hook for tests.
var afterOrphanListing = func() {}

// findOrphans reads the profile and persona ids the match tables mention, and then asks career.db which of them exist. The order is what
// makes it safe: an id is only judged after it was seen in a match table, and matches are only written for profiles that exist, so a
// profile created while this runs is never in the list and is never mistaken for an orphan.
func findOrphans() (orphanSet, error) {
	profiles := map[string]bool{}
	pairs := map[personaPair]bool{}
	for _, q := range []string{
		`SELECT profile_id FROM job_matches UNION SELECT profile_id FROM job_persona_matches UNION SELECT profile_id FROM persona_score_state UNION SELECT profile_id FROM profile_match_state`,
	} {
		ids, err := queryStrings(q)
		if err != nil {
			return orphanSet{}, err
		}
		for _, id := range ids {
			profiles[id] = true
		}
	}
	rows, err := db.JobsDB.Query(
		`SELECT persona_id, profile_id FROM job_matches WHERE persona_id IS NOT NULL AND persona_id <> ''
		 UNION SELECT persona_id, profile_id FROM job_persona_matches UNION SELECT persona_id, profile_id FROM persona_score_state`)
	if err != nil {
		return orphanSet{}, err
	}
	for rows.Next() {
		var p personaPair
		if err := rows.Scan(&p.persona, &p.profile); err != nil {
			rows.Close()
			return orphanSet{}, err
		}
		pairs[p] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return orphanSet{}, err
	}
	rows.Close()
	afterOrphanListing()

	existing := map[string]bool{}
	if len(profiles) > 0 {
		ids := make([]string, 0, len(profiles))
		for id := range profiles {
			ids = append(ids, id)
		}
		for _, chunk := range chunks(ids, 400) {
			found, err := careerStrings(`SELECT id FROM profiles WHERE id IN (`+placeholders(len(chunk))+`)`, chunk)
			if err != nil {
				return orphanSet{}, err
			}
			for _, id := range found {
				existing[id] = true
			}
		}
	}
	set := orphanSet{profiles: map[string]bool{}}
	for id := range profiles {
		if !existing[id] {
			set.profiles[id] = true
		}
	}
	// A pair under a profile that does not exist goes with the profile; only pairs of existing profiles are judged on their own.
	var judged []personaPair
	for p := range pairs {
		if existing[p.profile] {
			judged = append(judged, p)
		}
	}
	valid := map[personaPair]bool{}
	personaIDs := make([]string, 0, len(judged))
	for _, p := range judged {
		personaIDs = append(personaIDs, p.persona)
	}
	for _, chunk := range chunks(personaIDs, 400) {
		rows, err := db.CareerDB.Query(`SELECT id, profile_id FROM personas WHERE id IN (`+placeholders(len(chunk))+`)`, toAny(chunk)...)
		if err != nil {
			return orphanSet{}, err
		}
		for rows.Next() {
			var p personaPair
			if err := rows.Scan(&p.persona, &p.profile); err != nil {
				rows.Close()
				return orphanSet{}, err
			}
			valid[p] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return orphanSet{}, err
		}
		rows.Close()
	}
	for _, p := range judged {
		if !valid[p] {
			set.pairs = append(set.pairs, p)
		}
	}
	sort.Slice(set.pairs, func(i, j int) bool {
		if set.pairs[i].profile != set.pairs[j].profile {
			return set.pairs[i].profile < set.pairs[j].profile
		}
		return set.pairs[i].persona < set.pairs[j].persona
	})
	return set, nil
}

// OrphanMatches lists the job matches PurgeOrphans would remove, without removing anything: the preview. Newest-first order is not
// promised; the rows are ordered by job id.
func OrphanMatches() ([]OrphanMatch, error) {
	set, err := findOrphans()
	if err != nil {
		return nil, err
	}
	if set.empty() {
		return nil, nil
	}
	rows, err := db.JobsDB.Query(
		`SELECT m.job_id, COALESCE(j.title, ''), m.profile_id, COALESCE(m.persona_id, ''), m.kind, m.status, m.score
		 FROM job_matches m LEFT JOIN jobs j ON j.id = m.job_id ORDER BY m.job_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pairGone := map[personaPair]bool{}
	for _, p := range set.pairs {
		pairGone[p] = true
	}
	var out []OrphanMatch
	for rows.Next() {
		var m OrphanMatch
		var score *int
		if err := rows.Scan(&m.JobID, &m.Title, &m.ProfileID, &m.PersonaID, &m.Kind, &m.Status, &score); err != nil {
			return nil, err
		}
		if score != nil {
			m.Score, m.HasScore = *score, true
		}
		switch {
		case set.profiles[m.ProfileID]:
			m.Reason = "profile deleted"
		case m.PersonaID != "" && pairGone[personaPair{m.PersonaID, m.ProfileID}]:
			m.Reason = "persona deleted"
		default:
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PurgeOrphans removes every match row that names a profile or persona that no longer exists, in one transaction and under the re-match
// lock of each profile concerned: the job_matches rows with their skills, the persona scores and score states, and the re-match state
// of profiles that are gone. It returns what it removed and the ids of the jobs that lost their match (sorted), so the caller can have
// the profiles that remain match them. Idempotent; it touches nothing that belongs to a profile and persona that exist.
func PurgeOrphans() (OrphanReport, []string, error) {
	var report OrphanReport
	set, err := findOrphans()
	if err != nil || set.empty() {
		return report, nil, err
	}
	report.Profiles, report.Personas = len(set.profiles), len(set.pairs)
	locked := map[string]bool{}
	var lockIDs []string
	for id := range set.profiles {
		lockIDs = append(lockIDs, id)
	}
	for _, p := range set.pairs {
		if !locked[p.profile] && !set.profiles[p.profile] {
			locked[p.profile] = true
			lockIDs = append(lockIDs, p.profile)
		}
	}
	sort.Strings(lockIDs) // a fixed order, so two sweeps can never wait for each other
	for _, id := range lockIDs {
		defer lockProfile(id)()
	}

	tx, err := db.JobsDB.Begin()
	if err != nil {
		return report, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	freed := map[string]bool{}
	exec := func(query string, args ...any) (int, error) {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	collect := func(query string, args ...any) error {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			freed[id] = true
		}
		return rows.Err()
	}
	for id := range set.profiles {
		if err := collect(`SELECT job_id FROM job_matches WHERE profile_id = ?`, id); err != nil {
			return report, nil, err
		}
		if _, err := exec(`DELETE FROM job_match_skills WHERE job_id IN (SELECT job_id FROM job_matches WHERE profile_id = ?)`, id); err != nil {
			return report, nil, err
		}
		n, err := exec(`DELETE FROM job_matches WHERE profile_id = ?`, id)
		if err != nil {
			return report, nil, err
		}
		report.Matches += n
		if n, err = exec(`DELETE FROM job_persona_matches WHERE profile_id = ?`, id); err != nil {
			return report, nil, err
		}
		report.PersonaScores += n
		for _, q := range []string{`DELETE FROM persona_score_state WHERE profile_id = ?`, `DELETE FROM profile_match_state WHERE profile_id = ?`} {
			if n, err = exec(q, id); err != nil {
				return report, nil, err
			}
			report.States += n
		}
	}
	for _, p := range set.pairs {
		if err := collect(`SELECT job_id FROM job_matches WHERE persona_id = ? AND profile_id = ?`, p.persona, p.profile); err != nil {
			return report, nil, err
		}
		if _, err := exec(`DELETE FROM job_match_skills WHERE job_id IN (SELECT job_id FROM job_matches WHERE persona_id = ? AND profile_id = ?)`, p.persona, p.profile); err != nil {
			return report, nil, err
		}
		n, err := exec(`DELETE FROM job_matches WHERE persona_id = ? AND profile_id = ?`, p.persona, p.profile)
		if err != nil {
			return report, nil, err
		}
		report.Matches += n
		if n, err = exec(`DELETE FROM job_persona_matches WHERE persona_id = ? AND profile_id = ?`, p.persona, p.profile); err != nil {
			return report, nil, err
		}
		report.PersonaScores += n
		if n, err = exec(`DELETE FROM persona_score_state WHERE persona_id = ? AND profile_id = ?`, p.persona, p.profile); err != nil {
			return report, nil, err
		}
		report.States += n
	}
	if err := tx.Commit(); err != nil {
		return OrphanReport{}, nil, err
	}
	ids := make([]string, 0, len(freed))
	for id := range freed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return report, ids, nil
}

func queryStrings(query string, args ...any) ([]string, error) {
	rows, err := db.JobsDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func careerStrings(query string, args []string) ([]string, error) {
	rows, err := db.CareerDB.Query(query, toAny(args)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func toAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func chunks(in []string, size int) [][]string {
	var out [][]string
	for len(in) > size {
		out = append(out, in[:size])
		in = in[size:]
	}
	if len(in) > 0 {
		out = append(out, in)
	}
	return out
}
