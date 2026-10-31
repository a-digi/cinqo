// Package jobboard is the job board: the statuses a job can have and which job sits in
// which status on which profile's board. The four fixed statuses are seeded rows; a
// profile can add its own. An entry is "this profile uses this job, with this status",
// so one job can sit on several boards with a different status on each. Everything is
// in jobs.db (real foreign keys to jobs and job_statuses); profile ids are opaque ids
// into career.db, so the callers check that a profile exists. See
// plan/ai/tools/career/step-126-job-board-statuses.md.
package jobboard

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"career-tool-backend/db"

	"github.com/google/uuid"
)

// The fixed statuses' ids (seeded by db.go's jobsSchema).
const (
	NotReviewedID = "fixed-not-reviewed"
	DeclinedID    = "fixed-declined"
	StoppedID     = "fixed-stopped"
	AppliedID     = "fixed-applied"
)

// The kinds of a status; custom ones are the profile's own.
const (
	KindNotReviewed = "not_reviewed"
	KindDeclined    = "declined"
	KindStopped     = "stopped"
	KindApplied     = "applied"
	KindCustom      = "custom"
)

const (
	// MaxCustomStatuses is how many custom statuses one profile may have.
	MaxCustomStatuses = 12
	// MaxNameLength is the longest status name, in characters.
	MaxNameLength = 40
)

var (
	ErrInvalidName      = errors.New("a status name must be 1 to 40 characters")
	ErrDuplicateName    = errors.New("a status with this name already exists")
	ErrTooManyStatuses  = errors.New("a profile can have at most 12 custom statuses")
	ErrFixedStatus      = errors.New("the fixed statuses cannot be changed")
	ErrUnknownStatus    = errors.New("unknown status")
	ErrUnknownJob       = errors.New("unknown job")
	ErrStatusNotOnBoard = errors.New("this status belongs to another profile")
)

// now is the clock for updated_at; a fixed-width UTC string with milliseconds, so the
// text sorts like the time does ("most recently moved first"). Tests replace it.
var now = func() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// Status is one column of a board.
type Status struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Card is a job as it shows in a column.
type Card struct {
	JobID     string `json:"jobId"`
	Title     string `json:"title"`
	Company   string `json:"company"`
	Location  string `json:"location"`
	UpdatedAt string `json:"updatedAt"`
}

// Column is a status with the jobs that are in it on one board.
type Column struct {
	Status
	Jobs []Card `json:"jobs"`
}

// Board is one profile's board: every column, empty ones included.
type Board struct {
	ProfileID string   `json:"profileId"`
	Columns   []Column `json:"columns"`
}

// Entry is one job on one board.
type Entry struct {
	ProfileID string `json:"profileId"`
	JobID     string `json:"jobId"`
	StatusID  string `json:"statusId"`
	UpdatedAt string `json:"updatedAt"`
}

// Usage is a job's status on one profile's board. The caller adds the profile's name.
type Usage struct {
	ProfileID  string `json:"profileId"`
	StatusID   string `json:"statusId"`
	StatusName string `json:"statusName"`
	Kind       string `json:"kind"`
}

// cleanName trims a status name and checks its length.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxNameLength {
		return "", ErrInvalidName
	}
	return name, nil
}

// nameTaken reports whether the profile already has a status of this name (ignoring
// case), or the name is one of the fixed names. exceptID is a status to leave out (the
// one being renamed).
func nameTaken(tx *sql.Tx, profileID, name, exceptID string) (bool, error) {
	var one int
	err := tx.QueryRow(
		`SELECT 1 FROM job_statuses WHERE name = ? COLLATE NOCASE AND id <> ? AND (profile_id IS NULL OR profile_id = ?) LIMIT 1`,
		name, exceptID, profileID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CreateStatus adds a custom status to a profile's board.
func CreateStatus(profileID, name string) (Status, error) {
	name, err := cleanName(name)
	if err != nil {
		return Status{}, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM job_statuses WHERE profile_id = ?`, profileID).Scan(&count); err != nil {
		return Status{}, err
	}
	if count >= MaxCustomStatuses {
		return Status{}, ErrTooManyStatuses
	}
	if taken, err := nameTaken(tx, profileID, name, ""); err != nil {
		return Status{}, err
	} else if taken {
		return Status{}, ErrDuplicateName
	}
	s := Status{ID: uuid.NewString(), Name: name, Kind: KindCustom}
	if _, err := tx.Exec(
		`INSERT INTO job_statuses (id, profile_id, name, kind) VALUES (?, ?, ?, ?)`,
		s.ID, profileID, s.Name, s.Kind,
	); err != nil {
		return Status{}, err
	}
	return s, tx.Commit()
}

// RenameStatus renames a custom status.
func RenameStatus(id, name string) (Status, error) {
	name, err := cleanName(name)
	if err != nil {
		return Status{}, err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var profileID sql.NullString
	var kind string
	err = tx.QueryRow(`SELECT profile_id, kind FROM job_statuses WHERE id = ?`, id).Scan(&profileID, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{}, ErrUnknownStatus
	}
	if err != nil {
		return Status{}, err
	}
	if kind != KindCustom {
		return Status{}, ErrFixedStatus
	}
	if taken, err := nameTaken(tx, profileID.String, name, id); err != nil {
		return Status{}, err
	} else if taken {
		return Status{}, ErrDuplicateName
	}
	if _, err := tx.Exec(`UPDATE job_statuses SET name = ? WHERE id = ?`, name, id); err != nil {
		return Status{}, err
	}
	return Status{ID: id, Name: name, Kind: KindCustom}, tx.Commit()
}

// DeleteStatus deletes a custom status; its jobs move to Not Reviewed on the boards they
// are on. A benign no-op for an unknown id, like every delete here.
func DeleteStatus(id string) error {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var kind string
	err = tx.QueryRow(`SELECT kind FROM job_statuses WHERE id = ?`, id).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if kind != KindCustom {
		return ErrFixedStatus
	}
	if _, err := tx.Exec(
		`UPDATE job_board_entries SET status_id = ?, updated_at = ? WHERE status_id = ?`,
		NotReviewedID, now(), id,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_statuses WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetEntry puts a job on a profile's board in a status: it adds the job when it is not
// on the board yet and moves it otherwise. A custom status must be the profile's own.
func SetEntry(profileID, jobID, statusID string) (Entry, error) {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var one int
	if err := tx.QueryRow(`SELECT 1 FROM jobs WHERE id = ?`, jobID).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrUnknownJob
	} else if err != nil {
		return Entry{}, err
	}
	var owner sql.NullString
	err = tx.QueryRow(`SELECT profile_id FROM job_statuses WHERE id = ?`, statusID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrUnknownStatus
	}
	if err != nil {
		return Entry{}, err
	}
	if owner.Valid && owner.String != profileID {
		return Entry{}, ErrStatusNotOnBoard
	}
	e := Entry{ProfileID: profileID, JobID: jobID, StatusID: statusID, UpdatedAt: now()}
	if _, err := tx.Exec(
		`INSERT INTO job_board_entries (profile_id, job_id, status_id, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(profile_id, job_id) DO UPDATE SET status_id = excluded.status_id, updated_at = excluded.updated_at`,
		e.ProfileID, e.JobID, e.StatusID, e.UpdatedAt,
	); err != nil {
		return Entry{}, err
	}
	return e, tx.Commit()
}

// RemoveEntry takes a job off a profile's board; a no-op when it is not on it.
func RemoveEntry(profileID, jobID string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM job_board_entries WHERE profile_id = ? AND job_id = ?`, profileID, jobID)
	return err
}

// kindRank orders the columns: Not Reviewed, the profile's own, then Applied, Declined,
// Stopped application.
var kindRank = map[string]int{KindNotReviewed: 0, KindCustom: 1, KindApplied: 2, KindDeclined: 3, KindStopped: 4}

// GetBoard reads a profile's board: every column (empty ones too) in order, each with
// its jobs, most recently moved first.
func GetBoard(profileID string) (Board, error) {
	rows, err := db.JobsDB.Query(
		`SELECT id, name, kind FROM job_statuses WHERE profile_id IS NULL OR profile_id = ? ORDER BY rowid`,
		profileID,
	)
	if err != nil {
		return Board{}, err
	}
	var all []Column
	for rows.Next() {
		c := Column{Jobs: []Card{}}
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind); err != nil {
			rows.Close()
			return Board{}, err
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Board{}, err
	}
	rows.Close()

	board := Board{ProfileID: profileID, Columns: make([]Column, 0, len(all))}
	index := map[string]int{}
	// Stable by kind rank: fixed ones keep their seed order, custom ones their creation order.
	for rank := 0; rank <= 4; rank++ {
		for _, c := range all {
			if kindRank[c.Kind] == rank {
				index[c.ID] = len(board.Columns)
				board.Columns = append(board.Columns, c)
			}
		}
	}

	jobRows, err := db.JobsDB.Query(
		`SELECT e.status_id, j.id, j.title, COALESCE(j.company, ''), COALESCE(j.location, ''), e.updated_at
		 FROM job_board_entries e JOIN jobs j ON j.id = e.job_id
		 WHERE e.profile_id = ?
		 ORDER BY e.updated_at DESC, j.title, j.id`,
		profileID,
	)
	if err != nil {
		return Board{}, err
	}
	defer jobRows.Close()
	for jobRows.Next() {
		var statusID string
		var card Card
		if err := jobRows.Scan(&statusID, &card.JobID, &card.Title, &card.Company, &card.Location, &card.UpdatedAt); err != nil {
			return Board{}, err
		}
		i, ok := index[statusID]
		if !ok {
			return Board{}, fmt.Errorf("job %s is in status %s, which is not on this board", card.JobID, statusID)
		}
		board.Columns[i].Jobs = append(board.Columns[i].Jobs, card)
	}
	return board, jobRows.Err()
}

// StatusInfo is a status with the profile that owns it (empty for a fixed one).
type StatusInfo struct {
	Status
	ProfileID string `json:"profileId,omitempty"`
}

// ListStatuses reads every status: the four fixed ones, then the profiles' own, each group in the order
// they were created. A page that lets a job be put on any profile's board uses it to offer each profile
// its own list (the fixed ones plus that profile's).
func ListStatuses() ([]StatusInfo, error) {
	rows, err := db.JobsDB.Query(`SELECT id, name, kind, COALESCE(profile_id, '') FROM job_statuses ORDER BY (profile_id IS NOT NULL), rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []StatusInfo{}
	for rows.Next() {
		var s StatusInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.Kind, &s.ProfileID); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// GetUsage reads a job's status on every board that has it (the caller adds each
// profile's name; a profile that no longer exists is the caller's to skip).
func GetUsage(jobID string) ([]Usage, error) {
	rows, err := db.JobsDB.Query(
		`SELECT e.profile_id, s.id, s.name, s.kind
		 FROM job_board_entries e JOIN job_statuses s ON s.id = e.status_id
		 WHERE e.job_id = ? ORDER BY e.updated_at DESC, e.profile_id`,
		jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	usage := []Usage{}
	for rows.Next() {
		var u Usage
		if err := rows.Scan(&u.ProfileID, &u.StatusID, &u.StatusName, &u.Kind); err != nil {
			return nil, err
		}
		usage = append(usage, u)
	}
	return usage, rows.Err()
}

// PurgeProfile removes a deleted profile's board: its entries and its custom statuses.
// jobs.db cannot cascade from career.db, so profile.DeleteProfile calls this.
func PurgeProfile(profileID string) error {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM job_board_entries WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_statuses WHERE profile_id = ?`, profileID); err != nil {
		return err
	}
	return tx.Commit()
}
