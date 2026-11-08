package applicationlog

import (
	"strings"

	"career-tool-backend/db"
)

// Summary is what a job's card on a profile's board says about its protocol: how many entries there are, how many of them are still
// to come (a planned call), and the newest one that has already happened (its date in the same form as Message.OccurredAt, and its
// direction, so "waiting for a reply" can be seen). LastAt and LastDirection are empty when nothing has happened yet.
type Summary struct {
	Count         int    `json:"count"`
	Planned       int    `json:"planned"`
	LastAt        string `json:"lastAt,omitempty"`
	LastDirection string `json:"lastDirection,omitempty"`
}

// Summaries returns the summary of every job that has at least one entry on this profile's protocol, keyed by job id. A job
// without entries is not in the map. An entry counts as planned when its stored time is later than now; a day without a time is
// stored as that day's midnight (UTC), so today's entry has happened.
func Summaries(profileID string) (map[string]Summary, error) {
	rows, err := db.JobsDB.Query(
		`SELECT job_id, direction, occurred_at, has_time FROM application_messages WHERE profile_id = ? ORDER BY job_id, occurred_at, rowid`,
		profileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Summary{}
	at := now()
	for rows.Next() {
		var jobID, direction, occurred string
		var hasTime int
		if err := rows.Scan(&jobID, &direction, &occurred, &hasTime); err != nil {
			return nil, err
		}
		s := out[jobID]
		s.Count++
		if occurred > at {
			s.Planned++
		} else {
			// Ordered oldest first, so the last one that has happened wins.
			if hasTime == 1 {
				s.LastAt = strings.Replace(occurred, " ", "T", 1) + "Z"
			} else {
				s.LastAt = occurred[:10]
			}
			s.LastDirection = direction
		}
		out[jobID] = s
	}
	return out, rows.Err()
}
