// Package comments is Cinqo's comments engine: comments about an entity (a Career
// job, a Media file…), kept in one SQLite file per owner — the system, or each
// tool — so different owners' comments are never mixed. A comment belongs to an
// owner (which picks the file), an entity type and an entity UUID. This package is
// storage and validation only; who may do what is decided above it. See
// plan/ai/comments/comments-engine.md.
package comments

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// migrations are applied in order and never edited: a change is a new entry at the
// end. The database's PRAGMA user_version records how many have been applied (these
// files are created on demand, so they cannot use the fixed config/db/migrations).
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS comments (
		id          TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id   TEXT NOT NULL,
		body        TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
		author_id   TEXT NOT NULL,
		author_name TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_comments_entity ON comments (entity_type, entity_id, created_at);`,
}

// timeFormat is fixed-width UTC with milliseconds, so text order is time order.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Comment is one stored comment. UpdatedAt is empty until it is edited.
type Comment struct {
	ID         string `json:"id"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Body       string `json:"body"`
	AuthorID   string `json:"authorId"`
	AuthorName string `json:"authorName"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// NewComment is what a caller supplies to add one. The author comes from the
// caller's validated token, never from the request body.
type NewComment struct {
	EntityType string
	EntityID   string
	Body       string
	AuthorID   string
	AuthorName string
}

// Page is one page of an entity's comments, newest first. NextBefore is the id to
// pass as `before` for the next (older) page, empty when there is none.
type Page struct {
	Comments   []Comment `json:"comments"`
	Total      int       `json:"total"`
	NextBefore string    `json:"nextBefore"`
}

// Store is one owner's comments database.
type Store struct {
	db    *sql.DB
	now   func() time.Time
	newID func() string

	// lastCreated keeps created_at strictly increasing within this store, so two
	// comments made in the same millisecond still sort in the order they were made
	// (the id tie-break in List is only a backstop for rows from elsewhere).
	mu          sync.Mutex
	lastCreated time.Time
}

func newUUID() string { return uuid.NewString() }

// nextCreatedAt returns the timestamp for a new comment: the clock's, truncated to
// the stored millisecond precision, but never at or before the previous one.
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
		return nil, fmt.Errorf("create comments directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open comments database: %w", err)
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
		return fmt.Errorf("read comments schema version: %w", err)
	}
	if version > len(migrations) {
		// A newer Cinqo wrote this file. Do not touch what we do not understand.
		return fmt.Errorf("comments database is from a newer version (schema %d, this build knows %d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("comments schema step %d: %w", i+1, err)
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

const columns = `id, entity_type, entity_id, body, author_id, author_name, created_at, COALESCE(updated_at, '')`

func scan(row interface{ Scan(...any) error }) (Comment, error) {
	var c Comment
	err := row.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Body, &c.AuthorID, &c.AuthorName, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// Create validates and stores a comment and returns it.
func (s *Store) Create(in NewComment) (Comment, error) {
	entityType, err := cleanEntityType(in.EntityType)
	if err != nil {
		return Comment{}, err
	}
	entityID, err := canonicalUUID("entityId", in.EntityID)
	if err != nil {
		return Comment{}, err
	}
	body, err := cleanBody(in.Body)
	if err != nil {
		return Comment{}, err
	}
	authorID, authorName, err := cleanAuthor(in.AuthorID, in.AuthorName)
	if err != nil {
		return Comment{}, err
	}

	c := Comment{
		ID: s.newID(), EntityType: entityType, EntityID: entityID, Body: body,
		AuthorID: authorID, AuthorName: authorName, CreatedAt: s.nextCreatedAt(),
	}
	if _, err := s.db.Exec(
		`INSERT INTO comments (id, entity_type, entity_id, body, author_id, author_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.EntityType, c.EntityID, c.Body, c.AuthorID, c.AuthorName, c.CreatedAt,
	); err != nil {
		return Comment{}, fmt.Errorf("store comment: %w", err)
	}
	return c, nil
}

// Get returns one comment of the given entity. A comment is only reachable
// through the entity it belongs to: the same id under another entity is not found.
func (s *Store) Get(entityType, entityID, id string) (Comment, error) {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return Comment{}, err
	}
	c, err := scan(s.db.QueryRow(
		`SELECT `+columns+` FROM comments WHERE id = ? AND entity_type = ? AND entity_id = ?`, id, entityType, entityID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	return c, err
}

// ListOptions shapes one List call. The zero value means "the newest
// DefaultPageSize comments, unfiltered".
type ListOptions struct {
	// Limit <= 0 means DefaultPageSize; it is capped at MaxPageSize.
	Limit int
	// Before is the id of the last comment of the previous page ("" for the first).
	Before string
	// AuthorID keeps only comments written by this user (the token subject).
	AuthorID string
	// CreatedFrom (inclusive) and CreatedTo (exclusive) keep only comments created
	// in [from, to). Each is an RFC 3339 timestamp (any offset) or a plain date
	// (YYYY-MM-DD, meaning 00:00 UTC). Empty means no bound.
	CreatedFrom string
	CreatedTo   string
}

// parseBound turns a filter bound into the stored timestamp format, or "" for none.
func parseBound(field, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(timeFormat), nil
		}
	}
	return "", invalid(field, "must be an RFC 3339 timestamp or a date (YYYY-MM-DD)")
}

// List returns one page of an entity's comments, newest first, optionally
// filtered by author and creation time (see ListOptions). Total counts the
// comments that match the filters, not just this page.
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

	where := ` WHERE entity_type = ? AND entity_id = ?`
	args := []any{entityType, entityID}

	if author := strings.TrimSpace(opts.AuthorID); author != "" {
		if utf8.RuneCountInString(author) > maxAuthorIDRunes || strings.ContainsRune(author, 0) {
			return Page{}, invalid("authorId", "is not valid")
		}
		where += ` AND author_id = ?`
		args = append(args, author)
	}
	from, err := parseBound("createdFrom", opts.CreatedFrom)
	if err != nil {
		return Page{}, err
	}
	to, err := parseBound("createdTo", opts.CreatedTo)
	if err != nil {
		return Page{}, err
	}
	if from != "" && to != "" && from >= to {
		return Page{}, invalid("createdTo", "must be after createdFrom")
	}
	if from != "" {
		where += ` AND created_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		where += ` AND created_at < ?`
		args = append(args, to)
	}

	page := Page{Comments: []Comment{}}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments`+where, args...).Scan(&page.Total); err != nil {
		return Page{}, err
	}

	if opts.Before != "" {
		anchor, err := s.Get(entityType, entityID, opts.Before)
		if _, bad := IsValidation(err); bad || errors.Is(err, ErrNotFound) {
			return Page{}, invalid("before", "is not a comment of this entity")
		}
		if err != nil {
			return Page{}, err
		}
		where += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, anchor.CreatedAt, anchor.CreatedAt, anchor.ID)
	}
	// One extra row tells whether another page exists.
	query := `SELECT ` + columns + ` FROM comments` + where + ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		page.Comments = append(page.Comments, c)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Comments) > limit {
		page.Comments = page.Comments[:limit]
		page.NextBefore = page.Comments[limit-1].ID
	}
	return page, nil
}

// Update replaces a comment's text and stamps updated_at. Who may do this is the
// caller's decision; created_at and the author never change.
func (s *Store) Update(entityType, entityID, id, body string) (Comment, error) {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return Comment{}, err
	}
	body, err = cleanBody(body)
	if err != nil {
		return Comment{}, err
	}
	res, err := s.db.Exec(
		`UPDATE comments SET body = ?, updated_at = ? WHERE id = ? AND entity_type = ? AND entity_id = ?`,
		body, s.now().UTC().Format(timeFormat), id, entityType, entityID,
	)
	if err != nil {
		return Comment{}, fmt.Errorf("update comment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Comment{}, ErrNotFound
	}
	return s.Get(entityType, entityID, id)
}

// Delete removes one comment of the given entity.
func (s *Store) Delete(entityType, entityID, id string) error {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM comments WHERE id = ? AND entity_type = ? AND entity_id = ?`, id, entityType, entityID)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeEntity removes every comment of one entity (it no longer exists) and says
// how many there were. Purging an entity with no comments is not an error.
func (s *Store) PurgeEntity(entityType, entityID string) (int, error) {
	entityType, err := cleanEntityType(entityType)
	if err != nil {
		return 0, err
	}
	entityID, err = canonicalUUID("entityId", entityID)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM comments WHERE entity_type = ? AND entity_id = ?`, entityType, entityID)
	if err != nil {
		return 0, fmt.Errorf("purge comments: %w", err)
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
