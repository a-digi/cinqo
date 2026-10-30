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
	"sort"
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
	// Replies: a comment may have a parent. root_id is the thread's top-level comment
	// (its own id for a top-level one) and depth how far down it is; both are written
	// once at creation, from the parent, and never change. Existing comments are
	// top-level comments of their own threads. See plan/ai/comments/comment-replies.md.
	`ALTER TABLE comments ADD COLUMN parent_id TEXT;
	ALTER TABLE comments ADD COLUMN root_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE comments ADD COLUMN depth INTEGER NOT NULL DEFAULT 0 CHECK (depth BETWEEN 0 AND 5);
	UPDATE comments SET root_id = id;
	CREATE INDEX IF NOT EXISTS idx_comments_root ON comments (entity_type, entity_id, root_id);
	CREATE INDEX IF NOT EXISTS idx_comments_parent ON comments (parent_id);`,
}

// timeFormat is fixed-width UTC with milliseconds, so text order is time order.
const timeFormat = "2006-01-02T15:04:05.000Z"

// Comment is one stored comment. UpdatedAt is empty until it is edited. ParentID is
// empty for a top-level comment; Depth is 0 for one and its parent's depth + 1 for
// a reply. RootID (the top-level comment of the thread) is internal.
type Comment struct {
	ID         string `json:"id"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Body       string `json:"body"`
	AuthorID   string `json:"authorId"`
	AuthorName string `json:"authorName"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	ParentID   string `json:"parentId"`
	Depth      int    `json:"depth"`
	RootID     string `json:"-"`
	// Context is true only for a comment shown because a reply below it matched the
	// list's filters, not because it matched itself.
	Context bool `json:"context"`
}

// NewComment is what a caller supplies to add one. The author comes from the
// caller's validated token, never from the request body.
type NewComment struct {
	EntityType string
	EntityID   string
	Body       string
	AuthorID   string
	AuthorName string
	// ParentID is the comment replied to ("" for a top-level comment). It must be a
	// comment of the same entity.
	ParentID string
}

// Page is one page of an entity's comments: whole threads, newest thread first, each
// thread depth-first with replies oldest first, so a reply always follows its
// parent. NextBefore is the id to pass as `before` for the next (older) page of
// threads, empty when there is none.
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

const columns = `id, entity_type, entity_id, body, author_id, author_name, created_at, COALESCE(updated_at, ''), COALESCE(parent_id, ''), depth, root_id`

func scan(row interface{ Scan(...any) error }) (Comment, error) {
	var c Comment
	err := row.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Body, &c.AuthorID, &c.AuthorName, &c.CreatedAt, &c.UpdatedAt, &c.ParentID, &c.Depth, &c.RootID)
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

	var parentID string
	if strings.TrimSpace(in.ParentID) != "" {
		if parentID, err = canonicalUUID("parentId", strings.TrimSpace(in.ParentID)); err != nil {
			return Comment{}, err
		}
	}

	c := Comment{
		ID: s.newID(), EntityType: entityType, EntityID: entityID, Body: body,
		AuthorID: authorID, AuthorName: authorName, CreatedAt: s.nextCreatedAt(),
		ParentID: parentID,
	}
	c.RootID = c.ID

	// One transaction, so a parent deleted meanwhile cannot get a reply.
	tx, err := s.db.Begin()
	if err != nil {
		return Comment{}, fmt.Errorf("store comment: %w", err)
	}
	defer tx.Rollback()
	if parentID != "" {
		parent, err := scan(tx.QueryRow(
			`SELECT `+columns+` FROM comments WHERE id = ? AND entity_type = ? AND entity_id = ?`, parentID, entityType, entityID,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return Comment{}, invalid("parentId", "is not a comment of this entity")
		}
		if err != nil {
			return Comment{}, fmt.Errorf("store comment: %w", err)
		}
		if parent.Depth+1 > MaxDepth {
			return Comment{}, invalid("parentId", fmt.Sprintf("replies can go at most %d levels deep", MaxDepth))
		}
		var inThread int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM comments WHERE entity_type = ? AND entity_id = ? AND root_id = ?`, entityType, entityID, parent.RootID,
		).Scan(&inThread); err != nil {
			return Comment{}, fmt.Errorf("store comment: %w", err)
		}
		if inThread >= MaxThreadSize {
			return Comment{}, invalid("parentId", fmt.Sprintf("this thread is full (%d comments)", MaxThreadSize))
		}
		c.RootID, c.Depth = parent.RootID, parent.Depth+1
	}
	var parent any // NULL for a top-level comment
	if parentID != "" {
		parent = parentID
	}
	if _, err := tx.Exec(
		`INSERT INTO comments (id, entity_type, entity_id, body, author_id, author_name, created_at, parent_id, root_id, depth) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.EntityType, c.EntityID, c.Body, c.AuthorID, c.AuthorName, c.CreatedAt, parent, c.RootID, c.Depth,
	); err != nil {
		return Comment{}, fmt.Errorf("store comment: %w", err)
	}
	if err := tx.Commit(); err != nil {
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
// DefaultPageSize threads, unfiltered".
type ListOptions struct {
	// Limit is the number of threads (top-level comments with everything under
	// them); <= 0 means DefaultPageSize, and it is capped at MaxPageSize.
	Limit int
	// Before is the id of a comment of the last thread of the previous page ("" for
	// the first page); the page continues with the threads older than that one.
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

// filter is the validated author / time conditions of a List call.
type filter struct {
	author   string
	from, to string
}

func (f filter) active() bool { return f.author != "" || f.from != "" || f.to != "" }

// sql returns the conditions (each starting with " AND ") for columns of the given
// table alias ("" or "m."), and their arguments. Empty when no filter is set.
func (f filter) sql(alias string) (string, []any) {
	var out string
	var args []any
	if f.author != "" {
		out += ` AND ` + alias + `author_id = ?`
		args = append(args, f.author)
	}
	if f.from != "" {
		out += ` AND ` + alias + `created_at >= ?`
		args = append(args, f.from)
	}
	if f.to != "" {
		out += ` AND ` + alias + `created_at < ?`
		args = append(args, f.to)
	}
	return out, args
}

func newFilter(opts ListOptions) (filter, error) {
	var f filter
	if author := strings.TrimSpace(opts.AuthorID); author != "" {
		if utf8.RuneCountInString(author) > maxAuthorIDRunes || strings.ContainsRune(author, 0) {
			return f, invalid("authorId", "is not valid")
		}
		f.author = author
	}
	var err error
	if f.from, err = parseBound("createdFrom", opts.CreatedFrom); err != nil {
		return f, err
	}
	if f.to, err = parseBound("createdTo", opts.CreatedTo); err != nil {
		return f, err
	}
	if f.from != "" && f.to != "" && f.from >= f.to {
		return f, invalid("createdTo", "must be after createdFrom")
	}
	return f, nil
}

// List returns one page of an entity's comments as whole threads (see Page),
// optionally filtered by author and creation time (see ListOptions). With a filter,
// a thread is shown when any comment in it matches, and the page keeps the matches
// plus their ancestors (marked Context) so a reply is never shown without its
// parent. Total counts the comments that match the filters, in the whole entity.
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
	f, err := newFilter(opts)
	if err != nil {
		return Page{}, err
	}

	page := Page{Comments: []Comment{}}
	plain, plainArgs := f.sql("")
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE entity_type = ? AND entity_id = ?`+plain,
		append([]any{entityType, entityID}, plainArgs...)...,
	).Scan(&page.Total); err != nil {
		return Page{}, err
	}

	// Which threads: top-level comments, newest first, that have a matching comment
	// somewhere in their thread, after the cursor's thread.
	inThread, threadArgs := f.sql("m.")
	query := `SELECT r.id FROM comments r WHERE r.entity_type = ? AND r.entity_id = ? AND r.parent_id IS NULL` +
		` AND EXISTS (SELECT 1 FROM comments m WHERE m.entity_type = r.entity_type AND m.entity_id = r.entity_id AND m.root_id = r.id` + inThread + `)`
	args := append([]any{entityType, entityID}, threadArgs...)
	if opts.Before != "" {
		anchor, err := s.Get(entityType, entityID, opts.Before)
		if err == nil && anchor.ParentID != "" {
			anchor, err = s.Get(entityType, entityID, anchor.RootID)
		}
		if _, bad := IsValidation(err); bad || errors.Is(err, ErrNotFound) {
			return Page{}, invalid("before", "is not a comment of this entity")
		}
		if err != nil {
			return Page{}, err
		}
		query += ` AND (r.created_at < ? OR (r.created_at = ? AND r.id < ?))`
		args = append(args, anchor.CreatedAt, anchor.CreatedAt, anchor.ID)
	}
	// One extra row tells whether another page of threads exists.
	query += ` ORDER BY r.created_at DESC, r.id DESC LIMIT ?`
	args = append(args, limit+1)
	roots, err := s.queryIDs(query, args...)
	if err != nil {
		return Page{}, err
	}
	if len(roots) > limit {
		roots = roots[:limit]
		page.NextBefore = roots[limit-1]
	}
	if len(roots) == 0 {
		return page, nil
	}

	// Their comments (all of each thread; arrangeThreads applies the filters).
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(roots)), ", ")
	rowArgs := []any{entityType, entityID}
	for _, id := range roots {
		rowArgs = append(rowArgs, id)
	}
	rows, err := s.db.Query(`SELECT `+columns+` FROM comments WHERE entity_type = ? AND entity_id = ? AND root_id IN (`+marks+`)`, rowArgs...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	var all []Comment
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return Page{}, err
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	page.Comments = arrangeThreads(all, roots, f, plainMatcher(f))
	return page, nil
}

// plainMatcher reports whether a comment satisfies the filters by itself.
func plainMatcher(f filter) func(Comment) bool {
	return func(c Comment) bool {
		return (f.author == "" || c.AuthorID == f.author) &&
			(f.from == "" || c.CreatedAt >= f.from) &&
			(f.to == "" || c.CreatedAt < f.to)
	}
}

func (s *Store) queryIDs(query string, args ...any) ([]string, error) {
	rows, err := s.db.Query(query, args...)
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

// arrangeThreads puts the comments in display order: threads in the order of roots
// (newest first), each depth-first with siblings oldest first, so every reply is
// directly below its parent. With a filter only matching comments and their
// ancestors are kept, the ancestors that do not match marked Context.
func arrangeThreads(all []Comment, roots []string, f filter, matches func(Comment) bool) []Comment {
	byID := make(map[string]Comment, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}
	keep := make(map[string]bool, len(all))
	for _, c := range all {
		if f.active() && !matches(c) {
			continue
		}
		// Keep the match and every ancestor, so no reply is shown without its parent.
		for cur, ok := c, true; ok && !keep[cur.ID]; cur, ok = byID[cur.ParentID] {
			keep[cur.ID] = true
		}
	}
	children := make(map[string][]Comment)
	for _, c := range all {
		if keep[c.ID] {
			children[c.ParentID] = append(children[c.ParentID], c)
		}
	}
	for _, kids := range children {
		sort.Slice(kids, func(i, j int) bool {
			if kids[i].CreatedAt != kids[j].CreatedAt {
				return kids[i].CreatedAt < kids[j].CreatedAt
			}
			return kids[i].ID < kids[j].ID
		})
	}
	out := make([]Comment, 0, len(all))
	var walk func(c Comment)
	walk = func(c Comment) {
		// Kept without matching itself: shown only as the parent of a match.
		c.Context = f.active() && !matches(c)
		out = append(out, c)
		for _, kid := range children[c.ID] {
			walk(kid)
		}
	}
	for _, id := range roots {
		if root, ok := byID[id]; ok && keep[id] {
			walk(root)
		}
	}
	return out
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

// Delete removes one comment of the given entity together with all its replies.
func (s *Store) Delete(entityType, entityID, id string) error {
	_, err := s.DeleteThread(entityType, entityID, id)
	return err
}

// DeleteThread is Delete that also says how many comments went (the comment and
// every reply under it, at any depth).
func (s *Store) DeleteThread(entityType, entityID, id string) (int, error) {
	entityType, entityID, id, err := cleanKey(entityType, entityID, id)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(
		`DELETE FROM comments WHERE entity_type = ? AND entity_id = ? AND id IN (
			WITH RECURSIVE subtree(id) AS (
				SELECT id FROM comments WHERE id = ? AND entity_type = ? AND entity_id = ?
				UNION ALL
				SELECT c.id FROM comments c JOIN subtree ON c.parent_id = subtree.id
				WHERE c.entity_type = ? AND c.entity_id = ?
			) SELECT id FROM subtree)`,
		entityType, entityID, id, entityType, entityID, entityType, entityID,
	)
	if err != nil {
		return 0, fmt.Errorf("delete comment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, ErrNotFound
	}
	return int(n), nil
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
