// Package review is Cinqo's review engine: star ratings (with an optional short text) about an entity (a Career company, a Media
// file...), kept in one SQLite file per owner, the system or each tool, so different owners' reviews are never mixed. A review
// belongs to an owner (which picks the file), an entity type and an entity UUID (the object id), and there is one review per author
// per entity. This package is storage and validation only; which scale a type uses and who may do what is decided above it. It mirrors
// api/src/comments. See plan/ai/review/review-engine.md.
package review

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// migrations are applied in order and never edited: a change is a new entry at the end. The database's PRAGMA user_version records
// how many have been applied (these files are created on demand, so they cannot use the fixed config/db/migrations).
var migrations = []string{
	// scale is stored in each row so a rating always says what it was measured on; half_steps is the rating in half-stars (3.5 = 7).
	`CREATE TABLE IF NOT EXISTS reviews (
		id          TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id   TEXT NOT NULL,
		scale       INTEGER NOT NULL CHECK (scale IN (3, 5, 10)),
		half_steps  INTEGER NOT NULL CHECK (half_steps BETWEEN 1 AND scale * 2),
		body        TEXT NOT NULL DEFAULT '' CHECK (length(body) <= 2000),
		author_id   TEXT NOT NULL,
		author_name TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT,
		UNIQUE (entity_type, entity_id, author_id)
	);
	CREATE INDEX IF NOT EXISTS idx_reviews_entity ON reviews (entity_type, entity_id, created_at);`,
}

// timeFormat is fixed-width UTC with milliseconds, so text order is time order.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Review is one stored review. Rating is in stars (3.5); UpdatedAt is empty until the author changes it.
type Review struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entityType"`
	EntityID   string  `json:"entityId"`
	Rating     float64 `json:"rating"`
	Scale      int     `json:"scale"`
	Body       string  `json:"body"`
	AuthorID   string  `json:"authorId"`
	AuthorName string  `json:"authorName"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// PutReview is what a caller supplies to set the author's review of an entity. Scale and HalfStars come from the entity type's
// declaration, never from the request; the author comes from the caller's validated token.
type PutReview struct {
	EntityType string
	EntityID   string
	Scale      int
	HalfStars  bool
	Rating     float64
	Body       string
	AuthorID   string
	AuthorName string
}

// Page is one page of an entity's reviews, newest first. Total counts every review of the entity. NextBefore is the id to pass as
// `before` for the next (older) page, empty when there is none.
type Page struct {
	Reviews    []Review `json:"reviews"`
	Total      int      `json:"total"`
	NextBefore string   `json:"nextBefore"`
}

// Summary describes an entity's ratings on one scale. Distribution has 2 x scale entries: entry i is how many reviews gave
// (i+1)/2 stars. Reviews written on another scale (the type's scale changed later) are left out of everything and counted in
// OtherScale.
type Summary struct {
	Count        int     `json:"count"`
	Average      float64 `json:"average"`
	Distribution []int   `json:"distribution"`
	OtherScale   int     `json:"otherScale"`
}

// Brief is the short form used for lists: how many reviews and their average.
type Brief struct {
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

// Store is one owner's reviews database.
type Store struct {
	db    *sql.DB
	now   func() time.Time
	newID func() string

	// lastCreated keeps created_at strictly increasing within this store, so two reviews made in the same millisecond still sort in
	// the order they were made (the id tie-break in List is only a backstop for rows from elsewhere).
	mu          sync.Mutex
	lastCreated time.Time
}

func newUUID() string { return uuid.NewString() }

// nextCreatedAt returns the timestamp for a new review: the clock's, truncated to the stored millisecond precision, but never at or
// before the previous one.
func (s *Store) nextCreatedAt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.now().UTC().Truncate(time.Millisecond)
	if !t.After(s.lastCreated) {
		t = s.lastCreated.Add(time.Millisecond)
	}
	s.lastCreated = t
	return t.Format(timeFormat)
}

func openStore(path string, now func() time.Time, newID func() string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create reviews directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open reviews database: %w", err)
	}
	// One connection: writes are rare and serialising them avoids "database is locked".
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, now: now, newID: newID}, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read reviews schema version: %w", err)
	}
	if version > len(migrations) {
		// A newer Cinqo wrote this file. Do not touch what we do not understand.
		return fmt.Errorf("reviews database is from a newer version (schema %d, this build knows %d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("reviews schema step %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

const columns = `id, entity_type, entity_id, scale, half_steps, body, author_id, author_name, created_at, COALESCE(updated_at, '')`

func scan(row interface{ Scan(...any) error }) (Review, error) {
	var r Review
	var steps int
	if err := row.Scan(&r.ID, &r.EntityType, &r.EntityID, &r.Scale, &steps, &r.Body, &r.AuthorID, &r.AuthorName, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return r, err
	}
	r.Rating = float64(steps) / 2
	return r, nil
}

// Put sets the author's review of an entity: it creates it, or replaces the rating and text of the one the author already has (the
// id, the author and created_at stay, updated_at is stamped). One review per author per entity, so a double click never duplicates.
func (s *Store) Put(in PutReview) (Review, error) {
	entityType, err := cleanEntityType(in.EntityType)
	if err != nil {
		return Review{}, err
	}
	entityID, err := canonicalUUID("entityId", in.EntityID)
	if err != nil {
		return Review{}, err
	}
	steps, err := halfSteps(in.Rating, in.Scale, in.HalfStars)
	if err != nil {
		return Review{}, err
	}
	body, err := cleanBody(in.Body)
	if err != nil {
		return Review{}, err
	}
	authorID, authorName, err := cleanAuthor(in.AuthorID, in.AuthorName)
	if err != nil {
		return Review{}, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO reviews (id, entity_type, entity_id, scale, half_steps, body, author_id, author_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (entity_type, entity_id, author_id) DO UPDATE SET
		   scale = excluded.scale, half_steps = excluded.half_steps, body = excluded.body,
		   author_name = excluded.author_name, updated_at = ?`,
		s.newID(), entityType, entityID, in.Scale, steps, body, authorID, authorName, s.nextCreatedAt(),
		s.now().UTC().Format(timeFormat),
	); err != nil {
		return Review{}, fmt.Errorf("store review: %w", err)
	}
	return s.GetByAuthor(entityType, entityID, authorID)
}

// GetByAuthor returns the author's review of an entity, or ErrNotFound.
func (s *Store) GetByAuthor(entityType, entityID, authorID string) (Review, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return Review{}, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return Review{}, err
	}
	authorID, _, err = cleanAuthor(authorID, "")
	if err != nil {
		return Review{}, err
	}
	r, err := scan(s.db.QueryRow(
		`SELECT `+columns+` FROM reviews WHERE entity_type = ? AND entity_id = ? AND author_id = ?`, entityType, entityID, authorID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Review{}, ErrNotFound
	}
	return r, err
}

// Get returns one review of the given entity. A review is only reachable through the entity it belongs to: the same id under another
// entity is not found.
func (s *Store) Get(entityType, entityID, id string) (Review, error) {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return Review{}, err
	}
	r, err := scan(s.db.QueryRow(
		`SELECT `+columns+` FROM reviews WHERE id = ? AND entity_type = ? AND entity_id = ?`, id, entityType, entityID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Review{}, ErrNotFound
	}
	return r, err
}

// ListOptions shapes one List call. The zero value means "the newest DefaultPageSize reviews".
type ListOptions struct {
	// Limit <= 0 means DefaultPageSize, and it is capped at MaxPageSize.
	Limit int
	// Before is the id of the last review of the previous page ("" for the first page); the page continues with the older ones.
	Before string
}

// List returns one page of an entity's reviews, newest first.
func (s *Store) List(entityType, entityID string, opts ListOptions) (Page, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return Page{}, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return Page{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	page := Page{Reviews: []Review{}}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE entity_type = ? AND entity_id = ?`, entityType, entityID,
	).Scan(&page.Total); err != nil {
		return Page{}, err
	}
	query := `SELECT ` + columns + ` FROM reviews WHERE entity_type = ? AND entity_id = ?`
	args := []any{entityType, entityID}
	if opts.Before != "" {
		anchor, err := s.Get(entityType, entityID, opts.Before)
		if _, bad := IsValidation(err); bad || errors.Is(err, ErrNotFound) {
			return Page{}, invalid("before", "is not a review of this entity")
		}
		if err != nil {
			return Page{}, err
		}
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, anchor.CreatedAt, anchor.CreatedAt, anchor.ID)
	}
	// One extra row tells whether another page exists.
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		page.Reviews = append(page.Reviews, r)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Reviews) > limit {
		page.Reviews = page.Reviews[:limit]
		page.NextBefore = page.Reviews[limit-1].ID
	}
	return page, nil
}

func average(halfStepSum, count int) float64 {
	if count == 0 {
		return 0
	}
	return math.Round(float64(halfStepSum)/2/float64(count)*100) / 100
}

// Summary describes an entity's ratings on the given scale (the type's current one); see Summary for what is left out.
func (s *Store) Summary(entityType, entityID string, scale int) (Summary, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return Summary{}, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return Summary{}, err
	}
	if !ValidScale(scale) {
		return Summary{}, invalid("scale", "must be 3, 5 or 10")
	}
	sum := Summary{Distribution: make([]int, scale*2)}
	rows, err := s.db.Query(
		`SELECT half_steps, COUNT(*) FROM reviews WHERE entity_type = ? AND entity_id = ? AND scale = ? GROUP BY half_steps`,
		entityType, entityID, scale,
	)
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var steps, n int
		if err := rows.Scan(&steps, &n); err != nil {
			return Summary{}, err
		}
		sum.Distribution[steps-1] = n
		sum.Count += n
		total += steps * n
	}
	if err := rows.Err(); err != nil {
		return Summary{}, err
	}
	sum.Average = average(total, sum.Count)
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviews WHERE entity_type = ? AND entity_id = ? AND scale <> ?`, entityType, entityID, scale,
	).Scan(&sum.OtherScale); err != nil {
		return Summary{}, err
	}
	return sum, nil
}

// Briefs returns the count and average of each of up to MaxSummaryIDs entities of one type, on the given scale. Every requested id
// is in the result (zero reviews for one nobody rated), so a list can draw a row without checking.
func (s *Store) Briefs(entityType string, ids []string, scale int) (map[string]Brief, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return nil, err
	}
	if !ValidScale(scale) {
		return nil, invalid("scale", "must be 3, 5 or 10")
	}
	if len(ids) > MaxSummaryIDs {
		return nil, invalid("ids", fmt.Sprintf("can have at most %d ids", MaxSummaryIDs))
	}
	out := make(map[string]Brief, len(ids))
	args := []any{entityType, scale}
	for _, raw := range ids {
		id, err := canonicalUUID("ids", raw)
		if err != nil {
			return nil, err
		}
		if _, dup := out[id]; !dup {
			out[id] = Brief{}
			args = append(args, id)
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(args)-2), ", ")
	rows, err := s.db.Query(
		`SELECT entity_id, COUNT(*), SUM(half_steps) FROM reviews WHERE entity_type = ? AND scale = ? AND entity_id IN (`+marks+`) GROUP BY entity_id`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n, total int
		if err := rows.Scan(&id, &n, &total); err != nil {
			return nil, err
		}
		out[id] = Brief{Count: n, Average: average(total, n)}
	}
	return out, rows.Err()
}

// DeleteMine removes the author's review of an entity and says whether there was one (not having one is not an error, so deleting is
// idempotent).
func (s *Store) DeleteMine(entityType, entityID, authorID string) (bool, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return false, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return false, err
	}
	authorID, _, err = cleanAuthor(authorID, "")
	if err != nil {
		return false, err
	}
	res, err := s.db.Exec(
		`DELETE FROM reviews WHERE entity_type = ? AND entity_id = ? AND author_id = ?`, entityType, entityID, authorID,
	)
	if err != nil {
		return false, fmt.Errorf("delete review: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Delete removes one review of the given entity by id (for an administrator removing someone else's).
func (s *Store) Delete(entityType, entityID, id string) error {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM reviews WHERE id = ? AND entity_type = ? AND entity_id = ?`, id, entityType, entityID)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeEntity removes every review of one entity (it no longer exists) and says how many there were. Purging an entity with no
// reviews is not an error.
func (s *Store) PurgeEntity(entityType, entityID string) (int, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return 0, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM reviews WHERE entity_type = ? AND entity_id = ?`, entityType, entityID)
	if err != nil {
		return 0, fmt.Errorf("purge reviews: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func cleanKey(entityType, entityID, id string) (string, string, string, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return "", "", "", err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return "", "", "", err
	}
	id, err = canonicalUUID("id", strings.TrimSpace(id))
	if err != nil {
		return "", "", "", err
	}
	return entityType, entityID, id, nil
}
