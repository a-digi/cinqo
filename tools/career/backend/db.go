// db.go opens and initializes this tool's own two SQLite databases —
// career.db (the user's own profile/skills/experience) and jobs.db
// (crawled job postings) — kept as two separate files, not two
// schemas in one, because they have genuinely different lifecycles:
// the profile changes rarely, by the user's own hand or the AI
// helping fill it in; jobs is bulk-written by crawling and is
// realistically the one a user might want to reset independently
// without touching their own profile. Both live under TOOL_DB_DIR,
// via modernc.org/sqlite (pure Go, no CGO — the same driver already
// verified in this codebase to cross-compile cleanly for every
// OS/arch this project ships, reused here rather than re-verified
// from scratch). See plan/ai/tools/career/step-02-two-databases.md.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var (
	careerDB *sql.DB
	jobsDB   *sql.DB
)

// initDatabases opens (creating if needed) both database files and
// their own schemas. TOOL_DB_DIR itself is never pre-created by the
// host — same convention every other tool's own main() already
// follows — so this tool creates it itself.
func initDatabases() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	db, err := openDB(filepath.Join(dbDir, "career.db"), careerSchema)
	if err != nil {
		return fmt.Errorf("failed to open career.db: %w", err)
	}
	careerDB = db

	db, err = openDB(filepath.Join(dbDir, "jobs.db"), jobsSchema)
	if err != nil {
		return fmt.Errorf("failed to open jobs.db: %w", err)
	}
	jobsDB = db

	return nil
}

const careerSchema = `
CREATE TABLE IF NOT EXISTS career_profile (
    id                 TEXT PRIMARY KEY,
    full_name          TEXT,
    headline           TEXT,
    summary            TEXT,
    location           TEXT,
    desired_titles     TEXT,
    desired_locations  TEXT,
    min_salary         INTEGER,
    updated_at         TEXT
);

CREATE TABLE IF NOT EXISTS career_skills (
    id    TEXT PRIMARY KEY,
    skill TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS career_experience (
    id          TEXT PRIMARY KEY,
    company     TEXT NOT NULL,
    title       TEXT NOT NULL,
    start_date  TEXT,
    end_date    TEXT,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
`

const jobsSchema = `
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    source_url  TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    company     TEXT,
    location    TEXT,
    description TEXT,
    posted_at   TEXT,
    crawled_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
`

// openDB opens a single SQLite file and applies its own schema —
// MaxOpenConns(1) matches browser's own login_credentials.go
// reasoning verbatim: this tool's own write volume is small (a human
// occasionally editing their profile, an AI occasionally saving a
// batch of crawled jobs), so a single connection avoids any
// "database is locked" surprise from modernc.org/sqlite's own
// concurrent-writer behavior, not worth tuning around here.
func openDB(path, schema string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare schema: %w", err)
	}

	return db, nil
}
