package jobboard

import (
	"sort"
	"time"

	"career-tool-backend/applicationlog"
	"career-tool-backend/db"
)

// Application is one job the profile is applying to, for the overview of applications (step 176, step 5): a job that is in the
// Applied status on the profile's board, or that has entries in the profile's protocol (or both). Status is the job's status on the
// board and is absent when the job is not on it (the protocol was written first). Protocol is absent when there are no entries.
// ActivityAt is when something last happened (the newest entry that has happened, or the move on the board, whichever is later) as a
// UTC ISO time; absent when nothing has happened (only planned entries, not on the board). WaitingForReply is true when the newest
// entry that has happened is a message the profile sent, so the other side owes an answer (moving the card does not change that).
type Application struct {
	JobID           string                  `json:"jobId"`
	Title           string                  `json:"title"`
	Company         string                  `json:"company"`
	Status          *Status                 `json:"status,omitempty"`
	Protocol        *applicationlog.Summary `json:"protocol,omitempty"`
	ActivityAt      string                  `json:"activityAt,omitempty"`
	WaitingForReply bool                    `json:"waitingForReply"`
}

const activityLayout = "2006-01-02T15:04:05Z"

// parseActivity reads the two forms the database hands out: a protocol date ("2026-11-05" or "2026-11-05T14:30:00Z") and a board
// move ("2026-11-05T14:30:00.123Z"). A day counts as its start.
func parseActivity(text string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05.000Z", activityLayout, "2006-01-02"} {
		if t, err := time.Parse(layout, text); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// Applications lists a profile's applications, the one with the most recent activity first (ties and the ones without any by
// title). The caller checks that the profile exists.
func Applications(profileID string) ([]Application, error) {
	summaries, err := applicationlog.Summaries(profileID)
	if err != nil {
		return nil, err
	}
	rows, err := db.JobsDB.Query(
		`SELECT j.id, j.title, COALESCE(j.company, ''), COALESCE(s.id, ''), COALESCE(s.name, ''), COALESCE(s.kind, ''), COALESCE(e.updated_at, '')
		 FROM jobs j
		 LEFT JOIN job_board_entries e ON e.job_id = j.id AND e.profile_id = ?
		 LEFT JOIN job_statuses s ON s.id = e.status_id
		 WHERE s.kind = ? OR EXISTS (SELECT 1 FROM application_messages m WHERE m.job_id = j.id AND m.profile_id = ?)`,
		profileID, KindApplied, profileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		app Application
		at  time.Time
	}
	var items []item
	for rows.Next() {
		var a Application
		var st Status
		var moved string
		if err := rows.Scan(&a.JobID, &a.Title, &a.Company, &st.ID, &st.Name, &st.Kind, &moved); err != nil {
			return nil, err
		}
		if st.ID != "" {
			a.Status = &st
		}
		var at time.Time
		if t, ok := parseActivity(moved); ok {
			at = t
		}
		if s, ok := summaries[a.JobID]; ok {
			sc := s
			a.Protocol = &sc
			a.WaitingForReply = s.LastDirection == "sent"
			if t, ok := parseActivity(s.LastAt); ok && t.After(at) {
				at = t
			}
		}
		if !at.IsZero() {
			a.ActivityAt = at.Format(activityLayout)
		}
		items = append(items, item{a, at})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].at.Equal(items[j].at) {
			return items[i].at.After(items[j].at)
		}
		if items[i].app.Title != items[j].app.Title {
			return items[i].app.Title < items[j].app.Title
		}
		return items[i].app.JobID < items[j].app.JobID
	})
	out := make([]Application, 0, len(items))
	for _, it := range items {
		out = append(out, it.app)
	}
	return out, nil
}
