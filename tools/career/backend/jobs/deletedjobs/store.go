package deletedjobs

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"career-tool-backend/db"
)

// The reasons a job is remembered as deleted. A job the cleanup's age rule removes is deliberately not remembered (a
// job that aged out and comes back is a genuine repost), so there is no reason for it.
const (
	ReasonDeleted  = "deleted"   // a person or the AI deleted it
	ReasonLowMatch = "low_match" // the cleanup removed it for matching no profile well
)

var (
	// ErrNoMemory is returned when deleted_jobs.db could not be opened at start: nothing can be remembered.
	ErrNoMemory = errors.New("the deleted jobs database is not available")
	// ErrNoKey is returned for a job whose URL is blank: it has no key to be remembered by.
	ErrNoKey = errors.New("the job has no URL to remember it by")
	// ErrUnknownReason is returned for a reason that is not one of the two above.
	ErrUnknownReason = errors.New("unknown reason")
)

// Entry is what is remembered about a deleted job. Only SourceURL is used to recognize it (as a hash); the rest is
// the readable snapshot.
type Entry struct {
	SourceURL  string
	Title      string
	Company    string
	Location   string
	PortalName string
	PostedAt   string
	MatchScore *int // the last manual or AI score, nil if it had none
	Reason     string
}

// Remember stores a deleted job, one row per URL: remembering the same URL again (written in any equivalent way)
// refreshes its snapshot, reason and time instead of adding a row. created reports whether the row is new, so a caller
// whose delete then removes nothing can take back only what it added.
func Remember(e Entry) (created bool, err error) {
	if db.DeletedJobsDB == nil {
		return false, ErrNoMemory
	}
	if e.Reason != ReasonDeleted && e.Reason != ReasonLowMatch {
		return false, ErrUnknownReason
	}
	hash := URLHash(e.SourceURL)
	if hash == "" {
		return false, ErrNoKey
	}
	var one int
	switch err := db.DeletedJobsDB.QueryRow(`SELECT 1 FROM deleted_jobs WHERE url_hash = ?`, hash).Scan(&one); {
	case err == nil:
	case errors.Is(err, errNoRows):
		created = true
	default:
		return false, err
	}
	_, err = db.DeletedJobsDB.Exec(
		`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, company, location, portal_name, posted_at, match_score, reason, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(url_hash) DO UPDATE SET
			key_version = excluded.key_version, source_url = excluded.source_url, title = excluded.title,
			company = excluded.company, location = excluded.location, portal_name = excluded.portal_name,
			posted_at = excluded.posted_at, match_score = excluded.match_score, reason = excluded.reason,
			deleted_at = excluded.deleted_at`,
		hash, KeyVersion, e.SourceURL, e.Title, e.Company, e.Location, e.PortalName, e.PostedAt, e.MatchScore, e.Reason,
		now().UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return false, err
	}
	return created, nil
}

// now is the clock, a var so a test can fix it.
var now = time.Now

// Has reports whether a job with this URL (written in any equivalent way) was deleted and is remembered. A blank URL has
// no key and is never remembered. ErrNoMemory means the database is not available: the caller decides what that means
// (the crawl treats it as "nothing is remembered").
func Has(sourceURL string) (bool, error) {
	if db.DeletedJobsDB == nil {
		return false, ErrNoMemory
	}
	hash := URLHash(sourceURL)
	if hash == "" {
		return false, nil
	}
	var one int
	err := db.DeletedJobsDB.QueryRow(`SELECT 1 FROM deleted_jobs WHERE url_hash = ?`, hash).Scan(&one)
	if errors.Is(err, errNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Forget removes the row of a URL (any equivalent spelling of it). Forgetting something that is not remembered is not
// an error.
func Forget(sourceURL string) error {
	if db.DeletedJobsDB == nil {
		return ErrNoMemory
	}
	hash := URLHash(sourceURL)
	if hash == "" {
		return ErrNoKey
	}
	_, err := db.DeletedJobsDB.Exec(`DELETE FROM deleted_jobs WHERE url_hash = ?`, hash)
	return err
}

// Rehash recomputes the hash of every row made with other normalization rules than the ones in the code (a row whose
// key_version differs from KeyVersion), from its readable source_url. Two rows that now normalize to the same URL
// become one: the most recently deleted survives. It is meant to run once at start, after the databases are open, and
// does nothing when every row is current. It returns how many rows it rewrote.
func Rehash() (int, error) {
	if db.DeletedJobsDB == nil {
		return 0, ErrNoMemory
	}
	rows, err := db.DeletedJobsDB.Query(`SELECT id, source_url FROM deleted_jobs WHERE key_version <> ? ORDER BY deleted_at DESC, id DESC`, KeyVersion)
	if err != nil {
		return 0, err
	}
	type old struct {
		id  int64
		url string
	}
	var stale []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.url); err != nil {
			rows.Close()
			return 0, err
		}
		stale = append(stale, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(stale) == 0 {
		return 0, nil
	}

	tx, err := db.DeletedJobsDB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	n := 0
	for _, o := range stale {
		hash := URLHash(o.url)
		var taken int64
		err := tx.QueryRow(`SELECT id FROM deleted_jobs WHERE url_hash = ? AND id <> ?`, hash, o.id).Scan(&taken)
		switch {
		case err == nil:
			// another row (newer, as they are taken newest first, or already current) holds this URL: this one is the older duplicate
			if _, err := tx.Exec(`DELETE FROM deleted_jobs WHERE id = ?`, o.id); err != nil {
				return 0, err
			}
		case errors.Is(err, errNoRows):
			if _, err := tx.Exec(`UPDATE deleted_jobs SET url_hash = ?, key_version = ? WHERE id = ?`, hash, KeyVersion, o.id); err != nil {
				return 0, err
			}
		default:
			return 0, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	log.Printf("deleted jobs: recomputed %d key(s) for normalization rules version %d", n, KeyVersion)
	return n, nil
}

var errNoRows = sql.ErrNoRows

// Item is one remembered job as the list shows it.
type Item struct {
	ID         int64  `json:"id"`
	SourceURL  string `json:"sourceUrl"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	Location   string `json:"location"`
	PortalName string `json:"portalName"`
	PostedAt   string `json:"postedAt"`
	MatchScore *int   `json:"matchScore"`
	Reason     string `json:"reason"`
	DeletedAt  string `json:"deletedAt"`
}

// MaxListLimit is the most a list returns, DefaultListLimit what it returns without a limit.
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// List returns the remembered jobs, the most recently deleted first, at most limit of them (the default when limit is
// not positive, never more than MaxListLimit), and how many are remembered in all.
func List(limit int) (items []Item, total int, err error) {
	if db.DeletedJobsDB == nil {
		return nil, 0, ErrNoMemory
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	limit = min(limit, MaxListLimit)
	if err := db.DeletedJobsDB.QueryRow(`SELECT COUNT(*) FROM deleted_jobs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.DeletedJobsDB.Query(
		`SELECT id, source_url, title, company, location, portal_name, posted_at, match_score, reason, deleted_at
		 FROM deleted_jobs ORDER BY deleted_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items = []Item{}
	for rows.Next() {
		var it Item
		var score sql.NullInt64
		if err := rows.Scan(&it.ID, &it.SourceURL, &it.Title, &it.Company, &it.Location, &it.PortalName, &it.PostedAt, &score, &it.Reason, &it.DeletedAt); err != nil {
			return nil, 0, err
		}
		if score.Valid {
			v := int(score.Int64)
			it.MatchScore = &v
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

// ForgetByID allows one remembered job again (the next crawl may add it) and reports whether there was such a row.
func ForgetByID(id int64) (bool, error) {
	if db.DeletedJobsDB == nil {
		return false, ErrNoMemory
	}
	res, err := db.DeletedJobsDB.Exec(`DELETE FROM deleted_jobs WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ForgetAll allows every remembered job again and returns how many it released.
func ForgetAll() (int, error) {
	if db.DeletedJobsDB == nil {
		return 0, ErrNoMemory
	}
	res, err := db.DeletedJobsDB.Exec(`DELETE FROM deleted_jobs`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
