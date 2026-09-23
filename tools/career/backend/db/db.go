// Package db opens and initializes this tool's own two SQLite
// databases — career.db (the job seeker's own profiles/personas/persona
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
//
// CareerDB/JobsDB and InitDatabases are exported specifically because
// every one of this backend's own domain packages (profile, cvimport,
// companies, jobs, portal, crawl) needs to reach one or both of these
// handles, and this backend was refactored (step XX) from one flat
// package main into these subpackages, mirroring tools/browser/
// backend's own shared/auth/crawler split — package main can never be
// imported by anything, so this package (analogous to browser's own
// shared package) is what every other package imports instead. See
// plan/ai/tools/career/step-XX-package-split.md.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	CareerDB *sql.DB
	JobsDB   *sql.DB
)

// InitDatabases opens (creating if needed) both database files and
// their own schemas. TOOL_DB_DIR itself is never pre-created by the
// host — same convention every other tool's own main() already
// follows — so this tool creates it itself.
func InitDatabases() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	careerDB, err := sql.Open("sqlite", filepath.Join(dbDir, "career.db"))
	if err != nil {
		return fmt.Errorf("failed to open career.db: %w", err)
	}
	careerDB.SetMaxOpenConns(1)
	// migrateCareerDB itself owns turning PRAGMA foreign_keys on —
	// toggled off for the duration of its own table-rebuild surgery,
	// then back on once done (see its own doc comment for why).
	if err := migrateCareerDB(careerDB); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to migrate career.db: %w", err)
	}
	if _, err := careerDB.Exec(careerSchema); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to prepare career.db schema: %w", err)
	}
	CareerDB = careerDB

	jobsDB, err := sql.Open("sqlite", filepath.Join(dbDir, "jobs.db"))
	if err != nil {
		return fmt.Errorf("failed to open jobs.db: %w", err)
	}
	jobsDB.SetMaxOpenConns(1)
	// PRAGMA foreign_keys defaults OFF per SQLite connection — unlike
	// CareerDB (whose migrateCareerDB turns it ON at the end of its own
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
	if _, err := jobsDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		jobsDB.Close()
		return fmt.Errorf("failed to enable foreign keys on jobs.db: %w", err)
	}
	if err := migrateJobsDB(jobsDB); err != nil {
		jobsDB.Close()
		return fmt.Errorf("failed to migrate jobs.db: %w", err)
	}
	if _, err := jobsDB.Exec(jobsSchema); err != nil {
		jobsDB.Close()
		return fmt.Errorf("failed to prepare jobs.db schema: %w", err)
	}
	JobsDB = jobsDB

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
    job_detail_crawl_instructions     TEXT,
    instructions_ai_error             TEXT,
    instructions_ai_error_at          TEXT,
    instructions_ai_conversation_id   TEXT,
    job_detail_instructions_ai_error             TEXT,
    job_detail_instructions_ai_error_at          TEXT,
    job_detail_instructions_ai_conversation_id   TEXT,
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
    crawled_at      TEXT NOT NULL DEFAULT (datetime('now')),
    -- detail_crawl_status (step XX) records the outcome of the most
    -- recent job-DETAIL-page crawl attempt for this job — NULL means
    -- no attempt has ever been made (as opposed to crawled_at, which
    -- is set unconditionally at insert by the LISTING crawl that
    -- produced this row in the first place, and says nothing about
    -- whether the job's own detail page was ever separately visited).
    -- 'failed' | 'success'. See
    -- plan/ai/tools/career/step-XX-job-detail-crawl-status-eye-icon.md.
    detail_crawl_status TEXT
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
    phase          TEXT,
    -- kind (step XX) distinguishes a "Crawl now" run against this
    -- link's own LISTING page ('listing', the original and default)
    -- from a "Crawl job details now" run against every already-saved
    -- job's own detail page ('job_detail') — crawl_runs_one_running_idx
    -- above still applies across both kinds: they share the one
    -- browser tab, so only one of either kind may run at a time per
    -- link. See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
    kind           TEXT NOT NULL DEFAULT 'listing'
);

CREATE INDEX IF NOT EXISTS crawl_runs_portal_link_idx ON crawl_runs(portal_link_id);

CREATE UNIQUE INDEX IF NOT EXISTS crawl_runs_one_running_idx
    ON crawl_runs(portal_link_id) WHERE status = 'running';

-- job_matches (step XX) tracks the most recent "Job Match" attempt for
-- one job — one row per job (job_id itself is the primary key, no
-- separate id/history), re-matching overwrites. profile_id/persona_id
-- are OPAQUE ids into career.db's own profiles/personas tables — this
-- tool's own two SQLite databases (career.db, jobs.db) are genuinely
-- separate files/connections (see InitDatabases), so no real SQL
-- FOREIGN KEY or JOIN is possible across them; these are plain,
-- unenforced TEXT references. score/persona_id are written ONLY by
-- the AI-facing save_job_match MCP tool (job_match.go) — the
-- human/frontend-facing PUT /jobs/match endpoint structurally cannot
-- set either, so a compromised or buggy frontend call can never
-- fabricate a match result. See
-- plan/ai/tools/career/step-XX-job-match.md.
CREATE TABLE IF NOT EXISTS job_matches (
    job_id          TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    profile_id      TEXT NOT NULL,
    persona_id      TEXT,
    score           INTEGER,
    status          TEXT NOT NULL DEFAULT 'matching'
                      CHECK (status IN ('matching','completed','failed')),
    conversation_id TEXT,
    error           TEXT,
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    -- kind/semantic_model_id (step XX) — VESTIGIAL. These originally
    -- distinguished an AI-conversation-driven match ('ai') from a
    -- deterministic, no-AI "Match now" mechanism ('deterministic') that
    -- also had an optional semantic-similarity model behind it — both
    -- were removed in favor of keeping only the AI-driven match. Left
    -- in place (rather than dropped via a migration) purely so
    -- existing rows already carrying a non-default value stay
    -- readable/inspectable; Go code never writes anything but each
    -- column's own default ('ai'/NULL) going forward. See
    -- plan/ai/tools/career/step-XX-remove-deterministic-job-match.md.
    kind              TEXT NOT NULL DEFAULT 'ai',
    semantic_model_id TEXT
);

-- job_match_skills (step XX) holds the specific skills (verbatim
-- strings, from that persona's own career.db skills list) that
-- explain a job_matches row's own score — a proper one-to-many child
-- table, not a JSON blob column, matching career_skills' own
-- established convention for "a list of skill strings" elsewhere in
-- this tool. Replaced wholesale (delete + re-insert) on every
-- save_job_match call, same "no history, most recent overwrite"
-- semantics job_matches itself already has. No FK enforcement against
-- career.db's own career_skills possible (separate SQLite file) —
-- saveJobMatchResult (job_match.go) validates each skill against that
-- persona's own real skills in Go code instead, at write time. See
-- plan/ai/tools/career/step-XX-job-match-skills.md.
--
-- match_kind (step XX) — VESTIGIAL, same reasoning as job_matches.kind/
-- semantic_model_id above: originally distinguished a literal keyword
-- hit from a semantic (meaning-based) one, back when the now-removed
-- deterministic match mechanism could produce either. Every row this
-- tool writes now is 'literal' (its own column default) — kept only so
-- existing rows stay readable.
CREATE TABLE IF NOT EXISTS job_match_skills (
    job_id     TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    skill      TEXT NOT NULL,
    match_kind TEXT NOT NULL DEFAULT 'literal',
    PRIMARY KEY (job_id, skill)
);

-- job_cv_pdfs (step XX) tracks the most recent "Generate CV PDF"
-- attempt for one job — one row per job (job_id itself is the primary
-- key, no separate id/history), re-generating overwrites, same
-- "single slot per job, most recent wins" convention job_matches
-- already established. profile_id is an OPAQUE id into career.db's
-- own profiles table (see job_matches' own doc comment for why no
-- real FK/JOIN across the two databases is possible).
--
-- The AI-facing save_cv_pdf MCP tool (jobs/cv_pdf.go) moves a row
-- straight from 'generating' to 'completed', setting media_file_id —
-- it never talks to Media over HTTP itself: the CORE app's own
-- conversation orchestrator (api/src/conversation/chat.go) promotes
-- the pdfResource argument into a real, permanent Media file id
-- in-process (its own trusted, same-process write, never an HTTP
-- round trip) BEFORE this tool call ever runs, using the manifest's
-- own declarative promote_media_param mechanism — so by the time this
-- Go code sees pdfResource, it's already a real Media file id, not a
-- transient pdf_tools resource reference. See
-- plan/ai/tools/career/step-XX-cv-pdf.md.
CREATE TABLE IF NOT EXISTS job_cv_pdfs (
    job_id             TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    profile_id         TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'generating'
                         CHECK (status IN ('generating','completed','failed')),
    conversation_id    TEXT,
    media_file_id      TEXT,
    error              TEXT,
    updated_at         TEXT NOT NULL DEFAULT (datetime('now'))
);
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
	if err := migratePortalLinksJobDetailCrawlInstructions(db); err != nil {
		return err
	}
	if err := migratePortalLinksJobDetailInstructionsAIStatus(db); err != nil {
		return err
	}
	if err := migratePortalLinksRequestFlags(db); err != nil {
		return err
	}
	if err := migratePortalLinksErrorDismissedAt(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsPhase(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsKind(db); err != nil {
		return err
	}
	if err := migrateJobsDetailCrawlStatus(db); err != nil {
		return err
	}
	if err := migrateJobMatchesKind(db); err != nil {
		return err
	}
	if err := migrateJobMatchesSemanticModelID(db); err != nil {
		return err
	}
	if err := migrateJobMatchSkillsMatchKind(db); err != nil {
		return err
	}
	return migrateJobCvPdfsSchema(db)
}

// migrateJobCvPdfsSchema detects an early-shape job_cv_pdfs table (one
// still carrying the now-removed pdf_tools_file_id column, from before
// the promote_media_param mechanism made the AI-facing save_cv_pdf
// tool receive an already-real Media file id directly) and drops it —
// this feature was still under active development with no real user
// data in that shape yet, so a rebuild-from-empty is correct here,
// unlike every other migration in this file (which all preserve
// existing rows). jobsSchema's own CREATE TABLE IF NOT EXISTS then
// creates the current, simpler shape immediately after this runs. See
// plan/ai/tools/career/step-XX-cv-pdf.md.
func migrateJobCvPdfsSchema(db *sql.DB) error {
	exists, hasOldColumn, err := tableHasColumn(db, "job_cv_pdfs", "pdf_tools_file_id")
	if err != nil {
		return err
	}
	if !exists || !hasOldColumn {
		return nil
	}
	_, err = db.Exec(`DROP TABLE job_cv_pdfs`)
	return err
}

// migrateJobMatchesSemanticModelID adds the nullable semantic_model_id
// column (step XX) to an already-installed job_matches table — same
// plain ALTER TABLE ADD COLUMN shape as migrateJobMatchesKind above,
// guarded the same way. NULL backfills every pre-existing row
// correctly: the semantic model feature didn't exist before this step,
// so no earlier match could possibly have used one. See
// plan/ai/tools/career/step-XX-semantic-match-observability.md.
func migrateJobMatchesSemanticModelID(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "job_matches", "semantic_model_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE job_matches ADD COLUMN semantic_model_id TEXT`)
	return err
}

// migrateJobMatchSkillsMatchKind adds the match_kind column (step XX)
// to an already-installed job_match_skills table — same plain ALTER
// TABLE ADD COLUMN shape as every other column addition in this file,
// guarded the same way. DEFAULT 'literal' backfills every pre-existing
// row correctly — see this column's own doc comment on the
// CREATE TABLE above for why. See
// plan/ai/tools/career/step-XX-semantic-match-observability.md.
func migrateJobMatchSkillsMatchKind(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "job_match_skills", "match_kind")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE job_match_skills ADD COLUMN match_kind TEXT NOT NULL DEFAULT 'literal'`)
	return err
}

// migrateJobMatchesKind adds the kind column (step XX) to an
// already-installed job_matches table — same plain ALTER TABLE ADD
// COLUMN shape as migrateCrawlRunsKind above, no CHECK constraint,
// guarded the same way. DEFAULT 'ai' backfills every pre-existing row
// correctly: every job_matches row before this step was, by
// definition, AI-driven — the deterministic mechanism didn't exist
// yet. See plan/ai/tools/career/step-XX-deterministic-job-match.md.
func migrateJobMatchesKind(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "job_matches", "kind")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE job_matches ADD COLUMN kind TEXT NOT NULL DEFAULT 'ai'`)
	return err
}

// migrateJobsDetailCrawlStatus adds the nullable detail_crawl_status
// column (step XX) to an already-installed jobs table — same plain
// ALTER TABLE ADD COLUMN shape as every other column addition in this
// file, guarded the same way. NULL backfills every pre-existing row
// correctly: none of them have ever had a detail-crawl attempt
// recorded, since this column didn't exist before this step. See
// plan/ai/tools/career/step-XX-job-detail-crawl-status-eye-icon.md.
func migrateJobsDetailCrawlStatus(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "jobs", "detail_crawl_status")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE jobs ADD COLUMN detail_crawl_status TEXT`)
	return err
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
    job_detail_crawl_instructions     TEXT,
    instructions_ai_error             TEXT,
    instructions_ai_error_at          TEXT,
    instructions_ai_conversation_id   TEXT,
    job_detail_instructions_ai_error             TEXT,
    job_detail_instructions_ai_error_at          TEXT,
    job_detail_instructions_ai_conversation_id   TEXT,
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

// migratePortalLinksJobDetailCrawlInstructions adds the nullable
// column holding a link's own second, separate instruction document —
// how to extract job-position-relevant text off a single job's own
// detail page, as opposed to crawl_instructions' own listing-page
// fields+pagination shape. Same plain ALTER TABLE ADD COLUMN shape as
// every other column addition in this file, guarded the same way. See
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
func migratePortalLinksJobDetailCrawlInstructions(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "job_detail_crawl_instructions")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_crawl_instructions TEXT`)
	return err
}

// migratePortalLinksJobDetailInstructionsAIStatus adds the three
// nullable columns that record the outcome of the most recent
// AI-driven job-detail-crawl-instructions generation/edit attempt for
// a link — the SEPARATE, job-detail-specific counterpart to
// migratePortalLinksInstructionsAIStatus/
// migratePortalLinksInstructionsAIConversationID above, which track
// the LISTING instructions' own AI generation instead. Kept as their
// own independent columns (not shared) so generating one document's
// instructions with AI can never clobber the other's own error/
// in-flight-conversation tracking. Same plain ALTER TABLE ADD COLUMN
// shape, guarded the same way. See
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
func migratePortalLinksJobDetailInstructionsAIStatus(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "job_detail_instructions_ai_error")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_instructions_ai_error TEXT`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_instructions_ai_error_at TEXT`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_instructions_ai_conversation_id TEXT`)
	return err
}

// migratePortalLinksRequestFlags adds the two boolean request-flag
// columns a Domain Events listener (career-tool-backend's own
// events/listing-instructions-ready and events/jobs-crawled HTTP
// handlers) sets to ask the frontend to perform a privileged action
// (start a crawl, start an AI conversation) under the next real user's
// own live session — a backend event handler has no live end-user
// request to act as, so it can never perform either action itself. Same
// plain ALTER TABLE ADD COLUMN shape as every other column addition in
// this file, guarded the same way. DEFAULT 0 backfills every
// pre-existing row correctly: no such request could have existed before
// this step. See plan/ai/tools/career/step-65-career-event-listeners.md.
func migratePortalLinksRequestFlags(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "listing_crawl_requested")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN listing_crawl_requested INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_instructions_ai_requested INTEGER NOT NULL DEFAULT 0`)
	return err
}

// migratePortalLinksErrorDismissedAt adds the two nullable columns
// that record when the user last dismissed an AI-generation error
// banner for this link's own listing/job-detail instructions,
// respectively — compared against that document's own *_ai_error_at
// column at render time (dismissed_at >= error_at means "still
// dismissed"; a NEWER error_at means a fresh failure that should show
// again). Same plain ALTER TABLE ADD COLUMN shape as every other
// column addition in this file, guarded the same way. See
// plan/ai/tools/career/step-71-dismiss-tracking-data-model.md.
func migratePortalLinksErrorDismissedAt(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "instructions_ai_error_dismissed_at")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN instructions_ai_error_dismissed_at TEXT`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_instructions_ai_error_dismissed_at TEXT`)
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

// migrateCrawlRunsKind adds the kind column (step XX) to an
// already-installed crawl_runs table — same plain ALTER TABLE ADD
// COLUMN shape as migrateCrawlRunsPhase above, guarded the same way.
// DEFAULT 'listing' backfills every pre-existing row correctly: every
// crawl_runs row before this step was, by definition, a listing crawl
// — job_detail didn't exist yet. See
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
func migrateCrawlRunsKind(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "crawl_runs", "kind")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE crawl_runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'listing'`)
	return err
}
