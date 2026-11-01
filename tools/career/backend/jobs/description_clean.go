package jobs

import (
	"database/sql"
	"fmt"

	"career-tool-backend/db"
	"career-tool-backend/textclean"
)

// descriptionCleanFix is the name of the one-time repair in the data_fixes table.
const descriptionCleanFix = "clean-crawled-descriptions"

// backupJobsDB is a seam: tests replace it.
var backupJobsDB = db.BackupJobsDB

// DescriptionCleanResult says what CleanStoredDescriptions did.
type DescriptionCleanResult struct {
	AlreadyDone bool   // the repair had been done before; nothing was looked at
	Scanned     int    // crawled jobs with a description that were read
	Changed     int    // descriptions that were rewritten
	Backup      string // the copy of jobs.db made before rewriting; empty when nothing changed
}

// CleanStoredDescriptions is the one-time repair of step 147: before descriptions were cleaned on the way in, crawls stored
// markup and stylesheets in them. It runs every crawled job's description through textclean.Description and writes back only
// the ones that change. A job a person typed (source 'manual') is never touched. It is done once: a data_fixes row is the
// marker, written in the same transaction as the rewrite, and a copy of jobs.db is made first (nothing is rewritten when the
// copy fails). Run at start-up; see plan/ai/tools/career/step-147-clean-job-description-text.md.
func CleanStoredDescriptions() (DescriptionCleanResult, error) {
	var res DescriptionCleanResult
	var done int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM data_fixes WHERE name = ?`, descriptionCleanFix).Scan(&done); err != nil {
		return res, err
	}
	if done > 0 {
		res.AlreadyDone = true
		return res, nil
	}

	type change struct{ id, text string }
	var changes []change
	rows, err := db.JobsDB.Query(`SELECT id, description FROM jobs WHERE source = 'crawled' AND COALESCE(description, '') <> ''`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			rows.Close()
			return res, err
		}
		res.Scanned++
		if cleaned := textclean.Description(text); cleaned != text {
			changes = append(changes, change{id, cleaned})
		}
	}
	err = rows.Err()
	rows.Close() // jobs.db has a single connection: the rewrite below needs it free
	if err != nil {
		return res, err
	}

	if len(changes) > 0 {
		if res.Backup, err = backupJobsDB("description-clean"); err != nil {
			return res, err
		}
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, c := range changes {
		if _, err := tx.Exec(`UPDATE jobs SET description = ? WHERE id = ?`, c.text, c.id); err != nil {
			return res, fmt.Errorf("failed to clean the description of job %s: %w", c.id, err)
		}
	}
	res.Changed = len(changes)
	detail := fmt.Sprintf("%d of %d crawled descriptions rewritten; backup %s", res.Changed, res.Scanned, res.Backup)
	if err := markFixDone(tx, descriptionCleanFix, detail); err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

func markFixDone(tx *sql.Tx, name, detail string) error {
	_, err := tx.Exec(`INSERT INTO data_fixes (name, done_at, detail) VALUES (?, datetime('now'), ?)`, name, detail)
	return err
}
