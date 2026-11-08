package match

import (
	"career-tool-backend/db"
)

// JobIDsAfter lists up to limit job ids greater than after, in id order: the way to visit every job in chunks (the persona
// recompute and the "apply to existing jobs" of the auto-add rules do), each exactly once whatever is added or deleted
// meanwhile.
func JobIDsAfter(after string, limit int) ([]string, error) { return jobIDsAfter(after, limit) }

// jobIDsAfter lists up to limit job ids greater than after, in id order (so every job is visited once, whatever is
// added or deleted meanwhile).
func jobIDsAfter(after string, limit int) ([]string, error) {
	rows, err := db.JobsDB.Query(`SELECT id FROM jobs WHERE id > ? ORDER BY id LIMIT ?`, after, limit)
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
