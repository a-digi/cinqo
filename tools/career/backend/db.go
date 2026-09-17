// db.go opens and initializes this tool's own two SQLite databases —
// career.db (the job seeker's own profiles/personas/persona
// details/skills/experience) and jobs.db (crawled job postings) —
// kept as two separate files, not two schemas in one, because they
// have genuinely different lifecycles: the profile changes rarely, by
// the user's own hand or the AI helping fill it in; jobs is
// bulk-written by crawling and is realistically the one a user might
// want to reset independently without touching their own profile.
// Both live under TOOL_DB_DIR, via modernc.org/sqlite (pure Go, no
// CGO — the same driver already verified in this codebase to
// cross-compile cleanly for every OS/arch this project ships, reused
// here rather than re-verified from scratch). See
// plan/ai/tools/career/step-02-two-databases.md.
//
// Step 8 introduced Persona — every profile/skill/experience row
// belonged to a persona (career_profile keyed by persona_id itself;
// career_skills/career_experience carrying a persona_id FK). Step 10
// introduced Profile (the actual job seeker) one level above Persona
// — every persona now belongs to a profile — and renamed the old
// per-persona "career_profile" concept to "persona_details" (it was
// never the job seeker's own profile, it was that persona's own
// career positioning; "profile" now means the job seeker). Since
// SQLite cannot retroactively add a primary key or foreign key to an
// existing table, each of these steps needed a real one-time
// migration, run by this binary itself (there is no migration
// framework for tool-owned databases the way
// api/config/db/migrations/ is for the main cinqo API's own
// users.db) — see migrateCareerDB below and
// plan/ai/tools/career/step-08-persona.md /
// plan/ai/tools/career/step-10-job-seeker-profile.md.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// migrateCareerDB itself owns turning PRAGMA foreign_keys on —
	// toggled off for the duration of its own table-rebuild surgery,
	// then back on once done (see its own doc comment for why).
	if err := migrateCareerDB(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate career.db: %w", err)
	}
	if _, err := db.Exec(careerSchema); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare career.db schema: %w", err)
	}
	careerDB = db

	jdb, err := sql.Open("sqlite", filepath.Join(dbDir, "jobs.db"))
	if err != nil {
		return fmt.Errorf("failed to open jobs.db: %w", err)
	}
	jdb.SetMaxOpenConns(1)
	// PRAGMA foreign_keys defaults OFF per SQLite connection — unlike
	// careerDB (whose migrateCareerDB turns it ON at the end of its own
	// migration, a real one-time need this DB never had), jobs.db
	// never previously depended on FK enforcement (jobs.company_id
	// didn't exist before step 14, and step 14's own SET NULL was
	// exercised only through the app's own explicit UPDATE in
	// linkJobToCompany, never through a real DELETE). Step 16 is the
	// first thing in this DB that actually needs a cascade to fire —
	// caught directly (not assumed) by deleting a company with a real
	// linked recruiter and finding the recruiter survived, orphaned,
	// with a company_id pointing at nothing. See
	// plan/ai/tools/career/step-16-recruiters.md.
	if _, err := jdb.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		jdb.Close()
		return fmt.Errorf("failed to enable foreign keys on jobs.db: %w", err)
	}
	if err := migrateJobsDB(jdb); err != nil {
		jdb.Close()
		return fmt.Errorf("failed to migrate jobs.db: %w", err)
	}
	if _, err := jdb.Exec(jobsSchema); err != nil {
		jdb.Close()
		return fmt.Errorf("failed to prepare jobs.db schema: %w", err)
	}
	jobsDB = jdb

	return nil
}

const careerSchema = `
CREATE TABLE IF NOT EXISTS profiles (
    id          TEXT PRIMARY KEY,
    first_name  TEXT NOT NULL DEFAULT '',
    last_name   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

CREATE TABLE IF NOT EXISTS profile_external_links (
    id         TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    platform   TEXT NOT NULL,
    url        TEXT NOT NULL,
    UNIQUE(profile_id, platform)
);

CREATE TABLE IF NOT EXISTS personas (
    id          TEXT PRIMARY KEY,
    profile_id  TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

CREATE TABLE IF NOT EXISTS persona_details (
    persona_id        TEXT PRIMARY KEY REFERENCES personas(id) ON DELETE CASCADE,
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

-- cv_import_files (Import CV, step 2) tracks one uploaded CV awaiting
-- AI analysis. token_hash is sha256(token) — the raw token is never
-- stored, only ever returned once, at upload time, to the caller who
-- embeds it in the capability URL. This is the one deliberate
-- exception to "every route enforces its own scope": pdf_tools' own
-- fetch has no session to present, so cv-import/file's own handler
-- checks this token instead of the normal scope gate for that one
-- route. See plan/ai/tools/career/import-cv/step-02-cv-upload-and-capability-token-serving.md.
CREATE TABLE IF NOT EXISTS cv_import_files (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL,
    file_path   TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- cv_import_runs tracks one completed CV analysis: the AI's own
-- proposal, and — once the user acts on it — what was actually saved.
-- Created only once the AI's reply has been successfully parsed (an
-- abandoned upload that never got a reply leaves no row here, matching
-- cv_import_files' own TTL-bounded, otherwise-forgotten lifecycle).
-- media_file_id is a plain string reference to the core Media
-- feature's own file id, not a real FK (Media lives in cinqo's own
-- database, not this one) — kept so "process again" can reuse the same
-- file and Delete can best-effort forward a cleanup call to Media.
-- original_filename is denormalized here specifically so history stays
-- legible even after the Media row itself expires or is deleted.
-- save_summary_json is NULL until the user's first "Insert" attempt —
-- a real, distinct history state from "inserted everything" or
-- "inserted some, some failed". See
-- plan/ai/media/step-05-career-history.md.
CREATE TABLE IF NOT EXISTS cv_import_runs (
    id                TEXT PRIMARY KEY,
    media_file_id     TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    conversation_id   TEXT NOT NULL,
    ai_proposal_json  TEXT NOT NULL,
    save_summary_json TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT
);
`

const jobsSchema = `
CREATE TABLE IF NOT EXISTS companies (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

CREATE TABLE IF NOT EXISTS portals (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

CREATE TABLE IF NOT EXISTS portal_links (
    id                                TEXT PRIMARY KEY,
    portal_id                         TEXT NOT NULL REFERENCES portals(id) ON DELETE CASCADE,
    url                               TEXT NOT NULL,
    title                             TEXT,
    crawl_instructions                TEXT,
    instructions_ai_error             TEXT,
    instructions_ai_error_at          TEXT,
    instructions_ai_conversation_id   TEXT,
    created_at                        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at                        TEXT,
    UNIQUE(portal_id, url)
);

CREATE TABLE IF NOT EXISTS jobs (
    id              TEXT PRIMARY KEY,
    source_url      TEXT NOT NULL UNIQUE,
    title           TEXT NOT NULL,
    company         TEXT,
    company_id      TEXT REFERENCES companies(id) ON DELETE SET NULL,
    portal_link_id  TEXT REFERENCES portal_links(id) ON DELETE SET NULL,
    location        TEXT,
    description     TEXT,
    posted_at       TEXT,
    crawled_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS recruiters (
    id          TEXT PRIMARY KEY,
    company_id  TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    first_name  TEXT NOT NULL DEFAULT '',
    last_name   TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

-- crawl_runs (step 36) tracks one "Crawl now" attempt, run detached
-- from any HTTP request (step 37) so it survives the initiating tab
-- closing. Mirrors the AI conversation feature's own turn_runs table
-- (api/src/conversation/entity/turn_run.go) shape and reasoning
-- exactly. The partial unique index is the real, DB-enforced
-- guarantee — not an app-level check-then-insert, which would race
-- under two concurrent "Crawl now" clicks for the same link — that
-- only one run can be in flight per link at a time. See
-- plan/ai/tools/career/step-36-crawl-run-data-model.md.
-- phase (step 39) is the single most recent fine-grained step this run
-- has reached — building_request/navigating/checking_cloudflare/
-- awaiting_human_challenge/extracting/ingesting_jobs — layered on top
-- of status (which stays the coarse running/completed/failed/cancelled
-- control-flow signal). Deliberately never cleared on completion: the
-- last phase a FAILED run reached is real diagnostic information
-- ("failed while: awaiting_human_challenge" tells a different story
-- than "failed while: ingesting_jobs"). See
-- plan/ai/tools/career/step-39-fine-grained-crawl-phases.md.
CREATE TABLE IF NOT EXISTS crawl_runs (
    id             TEXT PRIMARY KEY,
    portal_link_id TEXT NOT NULL REFERENCES portal_links(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'running'
                     CHECK (status IN ('running','completed','failed','cancelled')),
    started_at     TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at    TEXT,
    log            TEXT NOT NULL DEFAULT '',
    result_summary TEXT,
    error_message  TEXT,
    phase          TEXT
);

CREATE INDEX IF NOT EXISTS crawl_runs_portal_link_idx ON crawl_runs(portal_link_id);

CREATE UNIQUE INDEX IF NOT EXISTS crawl_runs_one_running_idx
    ON crawl_runs(portal_link_id) WHERE status = 'running';
`

// migrateCareerDB runs, in order, every past schema migration this
// tool has needed. Each phase detects its own "old shape" and is a
// no-op if that shape isn't present — safe to run on a brand-new
// install (both phases no-op; the schema block right after this call
// creates the current shape directly), an already-fully-migrated
// database (both phases no-op), or a database caught mid-history at
// any prior version (only the phases still needed actually run).
//
// Foreign keys are turned off for the duration: verified directly
// (not assumed) that SQLite's own DROP TABLE, when foreign_keys is
// on, performs an implicit delete of every row in any table that
// references the one being dropped — so rebuilding "personas" (step
// 10's own migratePreStep10Schema) would silently cascade-delete
// every row in persona_details/career_skills/career_experience
// before this migration ever got a chance to copy them forward. A
// real bug caught by testing the migration against a real copy of
// this tool's own live database before running it for real — not a
// hypothetical. PRAGMA foreign_keys can't be changed inside a
// transaction, so it's toggled here, outside both phases' own
// transactions, not inside migratePreStep8Schema/migratePreStep10Schema
// themselves. foreign_key_check afterward confirms no dangling
// reference was introduced by any of this table surgery.
func migrateCareerDB(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}

	if err := migratePreStep8Schema(db); err != nil {
		return err
	}
	if err := migratePreStep10Schema(db); err != nil {
		return err
	}

	if err := checkForeignKeys(db); err != nil {
		return err
	}

	_, err := db.Exec(`PRAGMA foreign_keys = ON`)
	return err
}

// checkForeignKeys runs SQLite's own foreign_key_check — a real
// integrity verification, not just a hope, that the table surgery
// above didn't leave any row pointing at a parent id that no longer
// exists.
func checkForeignKeys(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("post-migration foreign key check failed — data integrity issue, refusing to continue")
	}
	return rows.Err()
}

// tableHasColumn is the shared detection primitive both migration
// phases use — PRAGMA table_info is SQLite's own way to inspect a
// table's real current columns, the only reliable way to tell "old
// shape" from "already migrated" without a separate version-tracking
// table.
func tableHasColumn(db *sql.DB, table, column string) (exists, hasColumn bool, err error) {
	var tableExists int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
	).Scan(&tableExists); err != nil {
		return false, false, err
	}
	if tableExists == 0 {
		return false, false, nil
	}

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return true, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return true, false, err
		}
		if name == column {
			hasColumn = true
		}
	}
	return true, hasColumn, rows.Err()
}

// migratePreStep8Schema detects a pre-step-8 career.db (a
// career_profile table with no persona_id column) and, if found,
// rebuilds it (plus career_skills/career_experience) under one seeded
// "default" persona, preserving every existing row. See
// plan/ai/tools/career/step-08-persona.md.
func migratePreStep8Schema(db *sql.DB) error {
	exists, hasPersonaID, err := tableHasColumn(db, "career_profile", "persona_id")
	if err != nil {
		return err
	}
	if !exists || hasPersonaID {
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
SELECT 'default', COALESCE(full_name, ''), COALESCE(headline, ''), COALESCE(summary, ''), COALESCE(location, ''), COALESCE(desired_titles, ''), COALESCE(desired_locations, ''), min_salary, updated_at FROM career_profile;
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

// migratePreStep10Schema detects a step-8/9-shape career.db (a
// personas table with no profile_id column) and, if found, seeds one
// Profile (best-effort backfilling its name from whatever full_name
// already exists on career_profile — see this step's own design doc
// for why that's a deliberately simple, explicitly lossy rule, not a
// real name-parser), links every existing persona to it, and renames
// career_profile to persona_details, dropping full_name (that's the
// job seeker's own identity now, not a per-persona field). See
// plan/ai/tools/career/step-10-job-seeker-profile.md.
func migratePreStep10Schema(db *sql.DB) error {
	exists, hasProfileID, err := tableHasColumn(db, "personas", "profile_id")
	if err != nil {
		return err
	}
	if !exists || hasProfileID {
		return nil
	}

	var firstName, lastName string
	var fullName sql.NullString
	err = db.QueryRow(
		`SELECT full_name FROM career_profile WHERE full_name IS NOT NULL AND full_name != '' LIMIT 1`,
	).Scan(&fullName)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if fullName.Valid {
		parts := strings.SplitN(strings.TrimSpace(fullName.String), " ", 2)
		firstName = parts[0]
		if len(parts) == 2 {
			lastName = parts[1]
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS profiles (
    id          TEXT PRIMARY KEY,
    first_name  TEXT NOT NULL DEFAULT '',
    last_name   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);
CREATE TABLE IF NOT EXISTS profile_external_links (
    id         TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    platform   TEXT NOT NULL,
    url        TEXT NOT NULL,
    UNIQUE(profile_id, platform)
);
`); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO profiles (id, first_name, last_name, created_at) VALUES ('default', ?, ?, datetime('now'))`,
		firstName, lastName,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`
CREATE TABLE personas_new (
    id          TEXT PRIMARY KEY,
    profile_id  TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);
INSERT INTO personas_new (id, profile_id, name, description, created_at, updated_at)
SELECT id, 'default', name, description, created_at, updated_at FROM personas;
DROP TABLE personas;
ALTER TABLE personas_new RENAME TO personas;

CREATE TABLE persona_details (
    persona_id        TEXT PRIMARY KEY REFERENCES personas(id) ON DELETE CASCADE,
    headline          TEXT NOT NULL DEFAULT '',
    summary           TEXT NOT NULL DEFAULT '',
    location          TEXT NOT NULL DEFAULT '',
    desired_titles    TEXT NOT NULL DEFAULT '',
    desired_locations TEXT NOT NULL DEFAULT '',
    min_salary        INTEGER,
    updated_at        TEXT
);
INSERT INTO persona_details (persona_id, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at)
SELECT persona_id, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at FROM career_profile;
DROP TABLE career_profile;
`); err != nil {
		return err
	}

	return tx.Commit()
}

// migrateJobsDB detects a pre-company jobs.db (a jobs table with no
// company_id column) and, if found, creates the companies table (so
// the column's own REFERENCES target exists before the column is
// added) and adds company_id via a plain ALTER TABLE ADD COLUMN — no
// table rebuild needed, unlike career.db's own persona/profile
// migrations, since adding a nullable column with no UNIQUE/PK
// constraint is one of the ALTER TABLE forms SQLite supports
// directly. No-op on a brand-new install (jobs doesn't exist yet —
// jobsSchema creates it correctly shaped from the start) and on an
// already-migrated one. See plan/ai/tools/career/step-14-companies.md.
func migrateJobsDB(db *sql.DB) error {
	if err := migrateJobsCompanyID(db); err != nil {
		return err
	}
	if err := migrateJobsPortalLinkID(db); err != nil {
		return err
	}
	if err := migratePortalLinksTitle(db); err != nil {
		return err
	}
	if err := migratePortalLinksInstructionsAIStatus(db); err != nil {
		return err
	}
	if err := migratePortalLinksInstructionsAIConversationID(db); err != nil {
		return err
	}
	return migrateCrawlRunsPhase(db)
}

func migrateJobsCompanyID(db *sql.DB) error {
	exists, hasCompanyID, err := tableHasColumn(db, "jobs", "company_id")
	if err != nil {
		return err
	}
	if !exists || hasCompanyID {
		return nil
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS companies (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);
`); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE jobs ADD COLUMN company_id TEXT REFERENCES companies(id) ON DELETE SET NULL`)
	return err
}

// migrateJobsPortalLinkID mirrors migrateJobsCompanyID exactly, one
// step later: detects a pre-portal jobs table (no portal_link_id
// column) and, if found, creates portals/portal_links (so the
// column's own REFERENCES target exists — a no-op via CREATE TABLE IF
// NOT EXISTS on an install that already has them from steps 18/19) and
// adds portal_link_id via a plain ALTER TABLE ADD COLUMN. See
// plan/ai/tools/career/step-20-portal-job-ingestion.md.
func migrateJobsPortalLinkID(db *sql.DB) error {
	exists, hasPortalLinkID, err := tableHasColumn(db, "jobs", "portal_link_id")
	if err != nil {
		return err
	}
	if !exists || hasPortalLinkID {
		return nil
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS portals (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);
CREATE TABLE IF NOT EXISTS portal_links (
    id                                TEXT PRIMARY KEY,
    portal_id                         TEXT NOT NULL REFERENCES portals(id) ON DELETE CASCADE,
    url                               TEXT NOT NULL,
    title                             TEXT,
    crawl_instructions                TEXT,
    instructions_ai_error             TEXT,
    instructions_ai_error_at          TEXT,
    instructions_ai_conversation_id   TEXT,
    created_at                        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at                        TEXT,
    UNIQUE(portal_id, url)
);
`); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE jobs ADD COLUMN portal_link_id TEXT REFERENCES portal_links(id) ON DELETE SET NULL`)
	return err
}

// migratePortalLinksTitle detects a pre-title portal_links table (a
// real table with no title column — includes the brand-new-table case
// migrateJobsPortalLinkID may have just created above, which already
// has title from its own inline CREATE TABLE, so this is correctly a
// no-op there too) and adds it via a plain ALTER TABLE ADD COLUMN — no
// rebuild needed, same shape as every other column addition to this
// database. Existing rows get title = NULL, never a synthesized
// default. See plan/ai/tools/career/step-22-portal-link-titles.md.
func migratePortalLinksTitle(db *sql.DB) error {
	exists, hasTitle, err := tableHasColumn(db, "portal_links", "title")
	if err != nil {
		return err
	}
	if !exists || hasTitle {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN title TEXT`)
	return err
}

// migratePortalLinksInstructionsAIStatus adds the two nullable columns
// that record the outcome of the most recent AI-driven crawl-
// instructions generation/edit attempt for a link (step 33) — same
// plain ALTER TABLE ADD COLUMN shape as migratePortalLinksTitle above,
// guarded the same way. Both NULL means "no recorded failure," the
// default and the state after a successful attempt clears them. See
// plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
func migratePortalLinksInstructionsAIStatus(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "instructions_ai_error")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN instructions_ai_error TEXT`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN instructions_ai_error_at TEXT`)
	return err
}

// migratePortalLinksInstructionsAIConversationID adds the nullable
// column tracking which hidden conversation is currently generating/
// updating a link's own crawl instructions, if one is in flight — same
// plain ALTER TABLE ADD COLUMN shape as
// migratePortalLinksInstructionsAIStatus above, guarded the same way.
// See plan/ai/tools/career/step-60-generate-with-ai-live-chat-window.md.
func migratePortalLinksInstructionsAIConversationID(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "instructions_ai_conversation_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN instructions_ai_conversation_id TEXT`)
	return err
}

// migrateCrawlRunsPhase adds the fine-grained phase column (step 39)
// to an already-installed crawl_runs table — same plain ALTER TABLE
// ADD COLUMN shape as every other column addition in this file, a
// no-op on a brand-new install (crawl_runs' own inline CREATE TABLE IF
// NOT EXISTS above already includes phase) or an already-migrated one.
// See plan/ai/tools/career/step-39-fine-grained-crawl-phases.md.
func migrateCrawlRunsPhase(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "crawl_runs", "phase")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE crawl_runs ADD COLUMN phase TEXT`)
	return err
}
