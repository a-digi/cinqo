// db.go opens and initializes this tool's own two SQLite databases —
// career.db (the user's own personas, profile/skills/experience) and
// jobs.db (crawled job postings) — kept as two separate files, not two
// schemas in one, because they have genuinely different lifecycles:
// the profile changes rarely, by the user's own hand or the AI
// helping fill it in; jobs is bulk-written by crawling and is
// realistically the one a user might want to reset independently
// without touching their own profile. Both live under TOOL_DB_DIR,
// via modernc.org/sqlite (pure Go, no CGO — the same driver already
// verified in this codebase to cross-compile cleanly for every
// OS/arch this project ships, reused here rather than re-verified
// from scratch). See plan/ai/tools/career/step-02-two-databases.md.
//
// Step 8 introduced Persona — every profile/skill/experience row now
// belongs to a persona (career_profile is keyed by persona_id itself;
// career_skills/career_experience carry a persona_id FK). Since
// SQLite cannot retroactively add a primary key or foreign key to an
// existing table, an already-installed pre-step-8 career.db needs a
// real one-time migration, run by this binary itself (there is no
// migration framework for tool-owned databases the way
// api/config/db/migrations/ is for the main cinqo API's own
// users.db) — see migrateCareerDB below and
// plan/ai/tools/career/step-08-persona.md.
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

	db, err := sql.Open("sqlite", filepath.Join(dbDir, "career.db"))
	if err != nil {
		return fmt.Errorf("failed to open career.db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return fmt.Errorf("failed to enable foreign keys on career.db: %w", err)
	}
	if err := migrateCareerDB(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate career.db to persona schema: %w", err)
	}
	if _, err := db.Exec(careerSchema); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare career.db schema: %w", err)
	}
	careerDB = db

	jdb, err := openDB(filepath.Join(dbDir, "jobs.db"), jobsSchema)
	if err != nil {
		return fmt.Errorf("failed to open jobs.db: %w", err)
	}
	jobsDB = jdb

	return nil
}

const careerSchema = `
CREATE TABLE IF NOT EXISTS personas (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

CREATE TABLE IF NOT EXISTS career_profile (
    persona_id        TEXT PRIMARY KEY REFERENCES personas(id) ON DELETE CASCADE,
    full_name         TEXT NOT NULL DEFAULT '',
    headline          TEXT NOT NULL DEFAULT '',
    summary           TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    desired_titles    TEXT NOT NULL DEFAULT '',
    desired_locations TEXT NOT NULL DEFAULT '',
    min_salary        INTEGER,
    updated_at        TEXT
);

CREATE TABLE IF NOT EXISTS career_skills (
    id         TEXT PRIMARY KEY,
    persona_id TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    skill      TEXT NOT NULL,
    UNIQUE(persona_id, skill)
);

CREATE TABLE IF NOT EXISTS career_experience (
    id          TEXT PRIMARY KEY,
    persona_id  TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
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

// migrateCareerDB detects a pre-step-8 career.db (a career_profile
// table with no persona_id column) and, if found, rebuilds all three
// tables under one seeded "default" persona, preserving every
// existing row. A no-op for a brand-new install (no career_profile
// table yet — the schema block right after this call creates it
// directly in the new shape) and a no-op for an already-migrated
// database (persona_id already present).
func migrateCareerDB(db *sql.DB) error {
	var tableExists int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'career_profile'`,
	).Scan(&tableExists)
	if err != nil {
		return err
	}
	if tableExists == 0 {
		return nil
	}

	rows, err := db.Query(`PRAGMA table_info(career_profile)`)
	if err != nil {
		return err
	}
	hasPersonaID := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "persona_id" {
			hasPersonaID = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if hasPersonaID {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const migration = `
CREATE TABLE IF NOT EXISTS personas (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);
INSERT INTO personas (id, name, created_at) VALUES ('default', 'Default', datetime('now'));

CREATE TABLE career_profile_new (
    persona_id        TEXT PRIMARY KEY REFERENCES personas(id) ON DELETE CASCADE,
    full_name         TEXT NOT NULL DEFAULT '',
    headline          TEXT NOT NULL DEFAULT '',
    summary           TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    desired_titles    TEXT NOT NULL DEFAULT '',
    desired_locations TEXT NOT NULL DEFAULT '',
    min_salary        INTEGER,
    updated_at        TEXT
);
INSERT INTO career_profile_new (persona_id, full_name, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at)
SELECT 'default', full_name, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at FROM career_profile;
DROP TABLE career_profile;
ALTER TABLE career_profile_new RENAME TO career_profile;

CREATE TABLE career_skills_new (
    id         TEXT PRIMARY KEY,
    persona_id TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    skill      TEXT NOT NULL,
    UNIQUE(persona_id, skill)
);
INSERT INTO career_skills_new (id, persona_id, skill) SELECT id, 'default', skill FROM career_skills;
DROP TABLE career_skills;
ALTER TABLE career_skills_new RENAME TO career_skills;

CREATE TABLE career_experience_new (
    id          TEXT PRIMARY KEY,
    persona_id  TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    company     TEXT NOT NULL,
    title       TEXT NOT NULL,
    start_date  TEXT,
    end_date    TEXT,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO career_experience_new (id, persona_id, company, title, start_date, end_date, description, created_at)
SELECT id, 'default', company, title, start_date, end_date, description, created_at FROM career_experience;
DROP TABLE career_experience;
ALTER TABLE career_experience_new RENAME TO career_experience;
`
	if _, err := tx.Exec(migration); err != nil {
		return err
	}
	return tx.Commit()
}

// openDB opens a single SQLite file and applies its own schema — used
// for jobs.db, which needs neither the FK pragma nor a migration
// routine. career.db's own open path is inlined in initDatabases
// above since it needs both. MaxOpenConns(1) matches browser's own
// login_credentials.go reasoning verbatim: this tool's own write
// volume is small (a human occasionally editing their profile, an AI
// occasionally saving a batch of crawled jobs), so a single connection
// avoids any "database is locked" surprise from modernc.org/sqlite's
// own concurrent-writer behavior, not worth tuning around here.
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
