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
	"encoding/json"
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
	// After the base schema, which creates the photos table (step 110).
	if err := migratePhotosLibrary(careerDB); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to fill the photo library: %w", err)
	}
	if err := migrateProfilePhotos(careerDB); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to fill the profiles' photos: %w", err)
	}
	if err := migratePersonasAiRunID(careerDB); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to add personas.ai_run_id: %w", err)
	}
	if err := migratePersonaAiRunDecisions(careerDB); err != nil {
		careerDB.Close()
		return fmt.Errorf("failed to add the persona AI run decisions: %w", err)
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
-- image_media_file_id (step XX) is a plain string reference to the
-- core Media feature's own file id, not a real FK — same
-- "Media lives in cinqo's own database, not this one" convention
-- cv_documents.media_file_id/cv_import_runs.media_file_id already
-- established. NULL means "no photo uploaded yet"; a profile has at
-- most one, re-uploading replaces it in place (see profileimage
-- package). See plan/ai/career/profile-image/step-01-overview.md.
CREATE TABLE IF NOT EXISTS profiles (
    id                  TEXT PRIMARY KEY,
    first_name          TEXT NOT NULL DEFAULT '',
    last_name           TEXT NOT NULL DEFAULT '',
    image_media_file_id TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at          TEXT
);

-- The photo library (step 110): one row per photo, keyed by its Media file id (a
-- plain string reference, no FK, like profiles.image_media_file_id above). A photo
-- exists on its own; any number of profiles can link it through
-- profiles.image_media_file_id.
CREATE TABLE IF NOT EXISTS photos (
    media_file_id TEXT PRIMARY KEY,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Which photos belong to a profile (step 111): any number, and a photo may belong to many
-- profiles. profiles.image_media_file_id is the profile's default photo and is always one of
-- these (kept by the profileimage package). media_file_id is a plain reference to the photo
-- library, no FK, like the other Media references in this tool.
CREATE TABLE IF NOT EXISTS profile_photos (
    profile_id    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    media_file_id TEXT NOT NULL,
    added_at      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (profile_id, media_file_id)
);

CREATE TABLE IF NOT EXISTS profile_external_links (
    id         TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    platform   TEXT NOT NULL,
    url        TEXT NOT NULL,
    UNIQUE(profile_id, platform)
);

-- ai_run_id (step 112) is the "create personas with AI" run a persona came from; NULL for
-- every persona that did not. Deleting the run keeps the persona.
CREATE TABLE IF NOT EXISTS personas (
    id          TEXT PRIMARY KEY,
    profile_id  TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT,
    ai_run_id   TEXT REFERENCES persona_ai_runs(id) ON DELETE SET NULL
);

-- One "create personas with AI" attempt of a profile (step 112): the hidden conversation that
-- did it (a plain reference into the core conversations, no FK), what the user asked for, how it
-- ended, and the AI's proposal as JSON. The history of these is shown on the Personas page only.
CREATE TABLE IF NOT EXISTS persona_ai_runs (
    id              TEXT PRIMARY KEY,
    profile_id      TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    focus           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'generating' CHECK (status IN ('generating','ready','failed')),
    proposal_json   TEXT,
    error           TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at     TEXT,
    -- Step 113: when the user decided on the whole proposal (accepted or declined every persona),
    -- and the proposed personas they declined, as proposed: [{"name": "..."}]. NULL until decided.
    decided_at      TEXT,
    declined_json   TEXT,
    -- Which proposed personas (their positions in the proposal) the user has decided on so far, as a
    -- JSON array. The user decides one persona at a time; the run is closed (decided_at) once every
    -- persona has a decision. NULL means none yet.
    decided_indexes TEXT
);

CREATE INDEX IF NOT EXISTS persona_ai_runs_profile_idx ON persona_ai_runs(profile_id);

-- The personas the AI suggested during a run, one row each, stored by its suggest_persona tool
-- (which creates no persona) in the order they were made. Finishing the run turns them into its
-- proposal.
CREATE TABLE IF NOT EXISTS persona_ai_suggestions (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES persona_ai_runs(id) ON DELETE CASCADE,
    position     INTEGER NOT NULL,
    persona_json TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(run_id, position)
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

-- cv_documents (CV Builder) is a deterministic, manual "generate a CV
-- PDF from this persona's own data, through a chosen design" library —
-- distinct from job_cv_pdfs (jobsSchema, an AI-tailored CV per job
-- posting); both coexist untouched. data_json is the exact CvData
-- (cvbuilder/templates.CvData, JSON-marshaled) actually used to render
-- this row, INCLUDING any per-CV edits made in the builder's own
-- sidebar that never touch persona_details/career_skills/
-- career_experience — the only record of what a given CV actually
-- contained, and what "edit this CV" (re-open, tweak, regenerate as a
-- new row) starts from. media_file_id is a plain string reference to
-- the core Media feature's own file id, not a real FK (same reasoning
-- cv_import_runs' own media_file_id above already established — Media
-- lives in cinqo's own database, not this one). See
-- plan/ai/career/cv-builder/step-01-overview-and-data-model.md.
-- job_id (AI-driven CV generation, step XX) is a plain, unenforced
-- reference to jobs.db's own jobs table — NULL for every document
-- created through the human-facing wizard, set only when the AI
-- generates a CV for a specific job posting (jobs/cv_pdf.go's own
-- save_cv_document). Same "two genuinely separate SQLite files, no
-- real cross-database FK possible" reasoning job_matches.profile_id
-- already established. See
-- plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
CREATE TABLE IF NOT EXISTS cv_documents (
    id            TEXT PRIMARY KEY,
    persona_id    TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    template_id   TEXT NOT NULL,
    title         TEXT NOT NULL,
    data_json     TEXT NOT NULL,
    media_file_id TEXT NOT NULL,
    job_id        TEXT,
    -- The photo the CV was made with (step 111), one of the profile's photos; NULL = none was
    -- chosen, the profile's default then. A plain reference to the photo library, no FK.
    photo_media_file_id TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT
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
    -- ai_platform_id / ai_model (step 102): this portal's own AI platform and
    -- model for crawling; '' means "no override", so the tool-wide choice
    -- (ai_settings) is used. A model without a platform is refused by the
    -- setter. Also added to existing installs by migratePortalsAiOverride.
    ai_platform_id TEXT NOT NULL DEFAULT '',
    ai_model       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT
);

-- portal_links' own column list below is kept in sync with every
-- column an existing install ends up with after every migration
-- function further down this file has run (listing_crawl_requested,
-- job_detail_instructions_ai_requested,
-- instructions_ai_error_dismissed_at,
-- job_detail_instructions_ai_error_dismissed_at,
-- job_detail_crawl_requested, auto_discovery_enabled,
-- auto_discovery_interval_minutes, auto_discovery_last_run_at) — a
-- genuinely brand-new install never goes through any of those ALTER
-- TABLE migrations at all (each one's own tableHasColumn guard sees
-- portal_links doesn't exist yet, at the point migrateJobsDB runs, and
-- no-ops), so this CREATE TABLE IF NOT EXISTS is the ONLY thing that
-- ever actually creates the table for that install — found live, not
-- assumed, when this file's own auto_discovery_enabled addition (step
-- 84 follow-up) broke a real test against a freshly initialized
-- database with "no such column: auto_discovery_enabled" until every
-- other already-migrated-for-upgrades column was added here too. Both
-- auto_discovery_interval_minutes/auto_discovery_last_run_at (step 85)
-- were added here from the start, not just their own migration, to
-- avoid repeating that exact mistake a second time.
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
    listing_crawl_requested                       INTEGER NOT NULL DEFAULT 0,
    job_detail_instructions_ai_requested           INTEGER NOT NULL DEFAULT 0,
    instructions_ai_error_dismissed_at             TEXT,
    job_detail_instructions_ai_error_dismissed_at  TEXT,
    job_detail_crawl_requested                     INTEGER NOT NULL DEFAULT 0,
    auto_discovery_enabled                         INTEGER NOT NULL DEFAULT 1,
    auto_discovery_interval_minutes                INTEGER,
    auto_discovery_last_run_at                     INTEGER,
    -- empty_crawl_acknowledged_at (step 90): when the user marked this
    -- link's current run of zero-job crawls as "expected, this page has
    -- no jobs" (RFC3339 UTC). Only a timestamp: whether it still applies
    -- is decided by comparing it with the last time a crawl found a job.
    -- Also added to existing installs by
    -- migratePortalLinksEmptyCrawlAcknowledgedAt.
    empty_crawl_acknowledged_at                    TEXT,
    -- jobs_question_asked_at / jobs_question_note (step 91): a pending
    -- "does this page list jobs?" question the generation agent asked the
    -- user after finding 0 items (RFC3339 UTC; NULL = nothing pending),
    -- and the agent's one-line observation. The answer is not stored —
    -- "no" becomes empty_crawl_acknowledged_at. Also added to existing
    -- installs by migratePortalLinksJobsQuestion.
    jobs_question_asked_at                         TEXT,
    jobs_question_note                             TEXT,
    -- ai_platform_id / ai_model (step 102): this link's own AI platform and
    -- model; '' means "no override" (the portal's, then the tool-wide, choice
    -- applies). Also added to existing installs by migratePortalsAiOverride.
    ai_platform_id                                 TEXT NOT NULL DEFAULT '',
    ai_model                                       TEXT NOT NULL DEFAULT '',
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
    detail_crawl_status TEXT,
    -- source (step 97): 'crawled' for every job a crawl or the AI saved (the
    -- default, so existing rows keep their meaning), 'manual' for a job a
    -- person added by hand. A manual job with no posting URL stores
    -- source_url = 'manual:<id>' (the column is NOT NULL UNIQUE); reads blank
    -- that placeholder. Also added to existing installs by migrateJobsSource.
    source TEXT NOT NULL DEFAULT 'crawled' CHECK (source IN ('crawled','manual'))
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
    kind           TEXT NOT NULL DEFAULT 'listing',
    -- triggered_by (step 84) distinguishes a human's own "Crawl now"/
    -- "Crawl job details now" click ('manual', the original and only
    -- value before this step) from one the auto-discovery manager
    -- started on its own schedule ('auto_discovery') — lets Crawl
    -- Monitor show which crawls were automatic, using data this
    -- feature already produces, no new logging table needed. See
    -- plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
    triggered_by   TEXT NOT NULL DEFAULT 'manual',
    -- Listing-crawl outcome (step 90): filled only when a 'listing' run
    -- COMPLETES; NULL for every other run and for runs that finished
    -- before these columns existed, meaning "no data". jobs_saved/
    -- jobs_updated/jobs_skipped are IngestCrawlResults' own counts
    -- (a skipped item had no title or no URL); pages_visited and
    -- stopped_reason come from the browser's own response;
    -- not_found_fields is a JSON array of the field labels that matched
    -- nothing on EVERY visited page ('[]' when none). Lets the UI tell
    -- "found nothing" apart from "found jobs", which the free-text
    -- result_summary cannot. Also added to existing installs by
    -- migrateCrawlRunsListingOutcome. See
    -- plan/ai/tools/career/step-90-zero-jobs-crawl-flag.md.
    jobs_saved       INTEGER,
    jobs_updated     INTEGER,
    jobs_skipped     INTEGER,
    pages_visited    INTEGER,
    stopped_reason   TEXT,
    not_found_fields TEXT
);

CREATE INDEX IF NOT EXISTS crawl_runs_portal_link_idx ON crawl_runs(portal_link_id);

CREATE UNIQUE INDEX IF NOT EXISTS crawl_runs_one_running_idx
    ON crawl_runs(portal_link_id) WHERE status = 'running';

-- instruction_attempts (step 86) counts every "Generate with AI" run
-- (kind 'listing' = crawl_instructions, 'job_detail' =
-- job_detail_crawl_instructions) per portal link, so the UI can show
-- how many succeeded or failed and alert when attempts were made but
-- the link still has no instructions. saved_at is set ONLY by the
-- save MCP tools (never by a frontend call), so 'succeeded' cannot be
-- claimed by a client; an attempt that ends without saved_at is
-- 'failed' (the AI errored or gave up). The partial unique index
-- allows at most one running attempt per link and kind, same idiom as
-- crawl_runs_one_running_idx above. A new table, so this base schema
-- text alone covers both fresh and existing installs. See
-- plan/ai/tools/career/step-86-instruction-attempt-tracking.md.
CREATE TABLE IF NOT EXISTS instruction_attempts (
    id              TEXT PRIMARY KEY,
    portal_link_id  TEXT NOT NULL REFERENCES portal_links(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('listing','job_detail')),
    triggered_by    TEXT NOT NULL DEFAULT 'manual' CHECK (triggered_by IN ('manual','auto')),
    status          TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','succeeded','failed')),
    conversation_id TEXT,
    started_at      TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at     TEXT,
    saved_at        TEXT,
    error_message   TEXT,
    -- failure_reason (step 87) says WHY a failed attempt failed:
    -- 'not_saved' = the run ended cleanly but the AI saved no
    -- instructions (it gave up — the only kind that suggests the page
    -- can't be crawled), 'error' = the turn itself errored (provider
    -- outage, no key, no permission), 'interrupted' = the tab or app
    -- closed mid-run. NULL for running and succeeded rows. Also added
    -- to existing installs by migrateInstructionAttemptsFailureReason.
    failure_reason  TEXT CHECK (failure_reason IS NULL OR failure_reason IN ('not_saved','error','interrupted'))
);

CREATE INDEX IF NOT EXISTS instruction_attempts_link_kind_idx
    ON instruction_attempts(portal_link_id, kind);

CREATE UNIQUE INDEX IF NOT EXISTS instruction_attempts_one_running_idx
    ON instruction_attempts(portal_link_id, kind) WHERE status = 'running';

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
-- The AI-facing save_cv_document MCP tool (jobs/cv_pdf.go) moves a row
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
    updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
    -- cv_document_id/source (step 98): where the job's current CV came from.
    -- 'ai' is the Generate CV PDF conversation; 'manual' a CV built in the CV
    -- Builder for this job; 'existing' a CV picked from the library. A plain,
    -- unenforced reference into career.db's cv_documents (no cross-file FK).
    cv_document_id     TEXT,
    source             TEXT NOT NULL DEFAULT 'ai' CHECK (source IN ('ai','manual','existing'))
);

-- job_cv_generations (CV generation audit trail) is the append-only
-- counterpart to job_cv_pdfs above: that table is deliberately one row
-- PER JOB (job_id is its own primary key) tracking only the CURRENT
-- generation attempt — SaveGeneratedCvPdf/UpdateCvGenerationStatus both
-- wipe its own conversation_id back to NULL the moment an attempt
-- finishes, success or failure, so it retains no history at all once a
-- run completes. This table is one row PER ATTEMPT instead (id is its
-- own primary key, job_id is just an indexed foreign key) — inserted
-- once when a generation starts and updated exactly once more in place
-- when it finishes, never deleted or overwritten — so every conversation
-- that ever ran for a job's CV can still be found and audited long after
-- the job's own "current" CV has since been regenerated. cv_document_id/
-- media_file_id are plain, unenforced references into career.db's own
-- cv_documents table (same "two separate SQLite files, no real
-- cross-database FK possible" reasoning job_matches.persona_id already
-- established) — kept so a past attempt's own link points at exactly
-- the CV/file IT produced, not whatever the job's current one happens to
-- be now. See plan/ai/career/cv-generation-audit/step-02-data-model.md.
CREATE TABLE IF NOT EXISTS job_cv_generations (
    id              TEXT PRIMARY KEY,
    job_id          TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    profile_id      TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'generating'
                      CHECK (status IN ('generating','completed','failed')),
    cv_document_id  TEXT,
    media_file_id   TEXT,
    error           TEXT,
    started_at      TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at     TEXT
);

CREATE INDEX IF NOT EXISTS job_cv_generations_job_idx ON job_cv_generations(job_id);

-- auto_discovery_settings (step 84) is a deliberate singleton — exactly
-- one global on/off + interval setting covering every portal link with
-- crawl instructions already set, not a per-link schedule (see this
-- step's own plan doc for why). CHECK (id = 1) is the real, DB-enforced
-- "never more than one row" guard, not an app-level check-then-insert
-- (which would race under two concurrent writers). last_run_at is a
-- unix-seconds INTEGER, not the usual RFC3339 TEXT this file's other
-- timestamp columns use — the auto-discovery manager's own due-check is
-- a plain integer subtraction against interval_minutes*60, and unix
-- seconds is what makes that arithmetic direct instead of a
-- parse-then-subtract on every tick. NULL means auto-discovery has
-- never actually run yet — treated as "due immediately" the first time
-- it's enabled, same as this table's own seed row starts out. See
-- plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
CREATE TABLE IF NOT EXISTS auto_discovery_settings (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    enabled          INTEGER NOT NULL DEFAULT 0,
    interval_minutes INTEGER NOT NULL DEFAULT 60,
    last_run_at      INTEGER,
    updated_at       TEXT
);

INSERT OR IGNORE INTO auto_discovery_settings (id, enabled, interval_minutes) VALUES (1, 0, 60);

-- ai_settings is a singleton like auto_discovery_settings: the one AI platform and
-- model this whole tool uses (crawling portals, generating crawl instructions, Job
-- Match, Generate CV, Import CV), chosen on the Settings page. Both columns empty
-- means "not chosen": the tool then falls back to the first platform that has an API
-- key. Only the platform id and model name are stored here; the keys live in core. A
-- stored id that no longer exists in core is simply treated as not chosen by the
-- frontend. CHECK (id = 1) is the DB-enforced "never more than one row" guard.
CREATE TABLE IF NOT EXISTS ai_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    platform_id TEXT NOT NULL DEFAULT '',
    model       TEXT NOT NULL DEFAULT '',
    updated_at  TEXT
);

INSERT OR IGNORE INTO ai_settings (id) VALUES (1);
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
	if err := migrateProfilesImageMediaFileID(db); err != nil {
		return err
	}
	if err := migrateCvDocumentsJobID(db); err != nil {
		return err
	}
	if err := migrateCvDocumentsPhoto(db); err != nil {
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

// migrateProfilesImageMediaFileID adds the nullable image_media_file_id
// column (profile-image feature) to an already-installed profiles
// table — same plain ALTER TABLE ADD COLUMN shape as every other
// column addition in this file, guarded the same way. NULL backfills
// every pre-existing row correctly: no profile could have had a photo
// before this column existed. A no-op on a brand-new install (the
// inline CREATE TABLE IF NOT EXISTS above already includes it). See
// plan/ai/career/profile-image/step-01-overview.md.
func migrateProfilesImageMediaFileID(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "profiles", "image_media_file_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE profiles ADD COLUMN image_media_file_id TEXT`)
	return err
}

// migratePhotosLibrary puts every photo a profile already links into the photos
// library (step 110), so the existing links stay valid and the photos can be
// picked by other profiles. Idempotent: it only adds what is missing, so it is
// safe on every start. It runs right after the base schema, which creates the
// photos table, not inside migrateCareerDB (that runs before the schema).
func migratePhotosLibrary(db *sql.DB) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO photos (media_file_id)
		SELECT DISTINCT image_media_file_id FROM profiles
		WHERE image_media_file_id IS NOT NULL AND image_media_file_id != ''`)
	return err
}

// migrateProfilePhotos makes every profile's current photo (its default from now on) one
// of the profile's photos (step 111). Idempotent: it only adds what is missing, so it is
// safe on every start, and it keeps the invariant "a default is always one of the
// profile's photos". Runs right after the base schema, like migratePhotosLibrary.
func migrateProfilePhotos(db *sql.DB) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO profile_photos (profile_id, media_file_id)
		SELECT id, image_media_file_id FROM profiles
		WHERE image_media_file_id IS NOT NULL AND image_media_file_id != ''`)
	return err
}

// migratePersonasAiRunID adds the nullable ai_run_id column (step 112, the AI run a persona came
// from) to an already-installed personas table. NULL is right for every existing persona. A no-op
// on a brand-new install (the base schema already has it). It runs right after the base schema,
// not inside migrateCareerDB, because the column references persona_ai_runs, which the schema
// creates.
func migratePersonasAiRunID(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "personas", "ai_run_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE personas ADD COLUMN ai_run_id TEXT REFERENCES persona_ai_runs(id) ON DELETE SET NULL`)
	return err
}

// migratePersonaAiRunDecisions adds decided_at, declined_json and decided_indexes to an installed persona_ai_runs
// table (step 113) and closes the runs that already have personas created from them: those were
// decided before the columns existed. Idempotent, so it is safe on every start; a run with no
// personas stays undecided and can still be reviewed. A no-op on a brand-new install (the base
// schema already has the columns). Runs right after the base schema, which creates the table.
func migratePersonaAiRunDecisions(db *sql.DB) error {
	for _, col := range []string{"decided_at", "declined_json", "decided_indexes"} {
		exists, hasColumn, err := tableHasColumn(db, "persona_ai_runs", col)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if !hasColumn {
			if _, err := db.Exec(`ALTER TABLE persona_ai_runs ADD COLUMN ` + col + ` TEXT`); err != nil {
				return err
			}
		}
	}
	if _, err := db.Exec(`UPDATE persona_ai_runs SET decided_at = COALESCE(finished_at, created_at)
		WHERE decided_at IS NULL AND id IN (SELECT ai_run_id FROM personas WHERE ai_run_id IS NOT NULL)`); err != nil {
		return err
	}
	// A run that was decided as a whole (before personas could be decided one at a time) has every
	// persona of its proposal decided.
	rows, err := db.Query(`SELECT id, proposal_json FROM persona_ai_runs
		WHERE decided_at IS NOT NULL AND decided_indexes IS NULL AND proposal_json IS NOT NULL`)
	if err != nil {
		return err
	}
	type fix struct {
		id      string
		indexes string
	}
	var fixes []fix
	for rows.Next() {
		var id, proposal string
		if err := rows.Scan(&id, &proposal); err != nil {
			rows.Close()
			return err
		}
		var p struct {
			Personas []json.RawMessage `json:"personas"`
		}
		if json.Unmarshal([]byte(proposal), &p) != nil {
			continue
		}
		indexes := make([]int, len(p.Personas))
		for i := range indexes {
			indexes[i] = i
		}
		raw, _ := json.Marshal(indexes)
		fixes = append(fixes, fix{id, string(raw)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, f := range fixes {
		if _, err := db.Exec(`UPDATE persona_ai_runs SET decided_indexes = ? WHERE id = ?`, f.indexes, f.id); err != nil {
			return err
		}
	}
	return nil
}

// migrateCvDocumentsJobID adds the nullable job_id column (AI-generated
// CV documents) to an already-installed cv_documents table — same
// plain ALTER TABLE ADD COLUMN shape as every other column addition in
// this file, guarded the same way. NULL backfills every pre-existing
// row correctly: no document could have been generated for a job
// before this column existed. A no-op on a brand-new install (the
// inline CREATE TABLE IF NOT EXISTS above already includes it). See
// plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
func migrateCvDocumentsJobID(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "cv_documents", "job_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE cv_documents ADD COLUMN job_id TEXT`)
	return err
}

// migrateCvDocumentsPhoto adds the nullable photo_media_file_id column (step 111, the photo a
// CV was made with) to an already-installed cv_documents table, guarded like the other column
// additions here. NULL is right for every existing row: they keep using the profile's default.
// A no-op on a brand-new install (the base schema already has it).
func migrateCvDocumentsPhoto(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "cv_documents", "photo_media_file_id")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE cv_documents ADD COLUMN photo_media_file_id TEXT`)
	return err
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
	if err := migratePortalLinksJobDetailCrawlRequested(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsPhase(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsKind(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsTriggeredBy(db); err != nil {
		return err
	}
	if err := migrateCrawlRunsListingOutcome(db); err != nil {
		return err
	}
	if err := migratePortalLinksEmptyCrawlAcknowledgedAt(db); err != nil {
		return err
	}
	if err := migratePortalLinksJobsQuestion(db); err != nil {
		return err
	}
	if err := migratePortalLinksAutoDiscoveryEnabled(db); err != nil {
		return err
	}
	if err := migratePortalLinksAutoDiscoveryInterval(db); err != nil {
		return err
	}
	if err := migratePortalsAiOverride(db); err != nil {
		return err
	}
	if err := migrateInstructionAttemptsFailureReason(db); err != nil {
		return err
	}
	if err := migrateJobsDetailCrawlStatus(db); err != nil {
		return err
	}
	if err := migrateJobsSource(db); err != nil {
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
	if err := migrateJobCvPdfsSchema(db); err != nil {
		return err
	}
	return migrateJobCvPdfsLink(db)
}

// migrateJobCvPdfsLink adds job_cv_pdfs.cv_document_id and .source (step 98)
// to a jobs.db that already has the table. Existing rows are AI-generated
// CVs, which is exactly what the 'ai' default says. A no-op on a brand-new
// install (the inline CREATE TABLE already has both) and when the table does
// not exist yet. See plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
func migrateJobCvPdfsLink(db *sql.DB) error {
	exists, hasDocumentID, err := tableHasColumn(db, "job_cv_pdfs", "cv_document_id")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if !hasDocumentID {
		if _, err := db.Exec(`ALTER TABLE job_cv_pdfs ADD COLUMN cv_document_id TEXT`); err != nil {
			return err
		}
	}
	_, hasSource, err := tableHasColumn(db, "job_cv_pdfs", "source")
	if err != nil {
		return err
	}
	if !hasSource {
		if _, err := db.Exec(`ALTER TABLE job_cv_pdfs ADD COLUMN source TEXT NOT NULL DEFAULT 'ai' CHECK (source IN ('ai','manual','existing'))`); err != nil {
			return err
		}
	}
	return nil
}

// migrateJobCvPdfsSchema detects an early-shape job_cv_pdfs table (one
// still carrying the now-removed pdf_tools_file_id column, from before
// the promote_media_param mechanism made the AI-facing save_cv_document
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

// migrateJobsSource (step 97) adds jobs.source to an install that already has
// jobs (a fresh one gets it from the base schema). Every existing row is a job a
// crawl or the AI saved, which is what the default 'crawled' says. A no-op when
// the column is present.
func migrateJobsSource(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "jobs", "source")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE jobs ADD COLUMN source TEXT NOT NULL DEFAULT 'crawled' CHECK (source IN ('crawled','manual'))`)
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

// migratePortalLinksJobDetailCrawlRequested adds the boolean
// request-flag column the existing events/jobs-crawled listener now
// also sets unconditionally, the moment a listing crawl finishes — the
// same "backend flags intent, frontend executes under the real user's
// own live session" pattern migratePortalLinksRequestFlags' own two
// columns already use. Whether it's actually SAFE to start that crawl
// yet (job-detail crawl instructions might not exist) is deliberately
// not this flag's own concern — the frontend's own consuming effect is
// what waits for readiness. Same plain ALTER TABLE ADD COLUMN shape as
// every other column addition in this file, guarded the same way. See
// plan/ai/tools/career/step-76-job-detail-crawl-requested-backend.md.
func migratePortalLinksJobDetailCrawlRequested(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "job_detail_crawl_requested")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN job_detail_crawl_requested INTEGER NOT NULL DEFAULT 0`)
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

// migrateCrawlRunsTriggeredBy adds the triggered_by column (step 84) to
// an already-installed crawl_runs table — same plain ALTER TABLE ADD
// COLUMN shape as migrateCrawlRunsPhase/migrateCrawlRunsKind above,
// guarded the same way. DEFAULT 'manual' backfills every pre-existing
// row correctly: every crawl_runs row before this step was, by
// definition, a human clicking "Crawl now" — auto-discovery didn't
// exist yet. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
func migrateCrawlRunsTriggeredBy(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "crawl_runs", "triggered_by")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE crawl_runs ADD COLUMN triggered_by TEXT NOT NULL DEFAULT 'manual'`)
	return err
}

// migrateCrawlRunsListingOutcome (step 90) adds the structured listing-
// crawl outcome columns to an install that already has crawl_runs (a
// fresh install gets them from the base schema). All nullable, no
// backfill: runs that finished before this have no structured data, and
// nothing is guessed from their free-text summary. See
// plan/ai/tools/career/step-90-zero-jobs-crawl-flag.md.
func migrateCrawlRunsListingOutcome(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "crawl_runs", "jobs_saved")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`ALTER TABLE crawl_runs ADD COLUMN jobs_saved INTEGER`,
		`ALTER TABLE crawl_runs ADD COLUMN jobs_updated INTEGER`,
		`ALTER TABLE crawl_runs ADD COLUMN jobs_skipped INTEGER`,
		`ALTER TABLE crawl_runs ADD COLUMN pages_visited INTEGER`,
		`ALTER TABLE crawl_runs ADD COLUMN stopped_reason TEXT`,
		`ALTER TABLE crawl_runs ADD COLUMN not_found_fields TEXT`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migratePortalLinksEmptyCrawlAcknowledgedAt (step 90) adds the
// "marked as expected" timestamp to an install that already has
// portal_links (a fresh one gets it from the base schema). Nullable, no
// backfill: nothing has been acknowledged yet.
func migratePortalLinksEmptyCrawlAcknowledgedAt(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "empty_crawl_acknowledged_at")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN empty_crawl_acknowledged_at TEXT`)
	return err
}

// migratePortalLinksJobsQuestion (step 91) adds the pending-question
// columns to an install that already has portal_links (a fresh one gets
// them from the base schema). Nullable, no backfill: nothing is pending.
func migratePortalLinksJobsQuestion(db *sql.DB) error {
	for _, col := range []string{"jobs_question_asked_at", "jobs_question_note"} {
		exists, hasColumn, err := tableHasColumn(db, "portal_links", col)
		if err != nil {
			return err
		}
		if !exists || hasColumn {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN ` + col + ` TEXT`); err != nil {
			return err
		}
	}
	return nil
}

// migratePortalLinksAutoDiscoveryEnabled adds the per-link opt-out flag
// (step 84 follow-up: some links people might not want auto-discovered
// even while the global schedule is on) — same plain ALTER TABLE ADD
// COLUMN shape every other portal_links column since step 22 already
// uses, guarded the same way. DEFAULT 1 (enabled) backfills every
// pre-existing row correctly — "by default auto-discovery is on" is
// the explicit instruction this column exists to satisfy, and no link
// that existed before this step could have opted out of a feature that
// didn't exist yet. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
func migratePortalLinksAutoDiscoveryEnabled(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "auto_discovery_enabled")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN auto_discovery_enabled INTEGER NOT NULL DEFAULT 1`)
	return err
}

// migrateInstructionAttemptsFailureReason (step 87) adds
// instruction_attempts.failure_reason to an install that already has
// the table (a fresh install gets it from the base schema instead) and
// backfills it for the failures already recorded. The backfill keys on
// the two fixed messages step 86 wrote — "The AI finished without
// saving instructions." and the "Interrupted: …" stale-run text — and
// calls every other failure 'error'; no history is guessed. Runs the
// backfill only in the same call that adds the column, so a later start
// never rewrites rows. See
// plan/ai/tools/career/step-87-likely-uncrawlable-pill.md.
func migrateInstructionAttemptsFailureReason(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "instruction_attempts", "failure_reason")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`ALTER TABLE instruction_attempts ADD COLUMN failure_reason TEXT CHECK (failure_reason IS NULL OR failure_reason IN ('not_saved','error','interrupted'))`,
		`UPDATE instruction_attempts SET failure_reason = 'not_saved' WHERE status = 'failed' AND error_message = 'The AI finished without saving instructions.'`,
		`UPDATE instruction_attempts SET failure_reason = 'interrupted' WHERE status = 'failed' AND error_message LIKE 'Interrupted:%'`,
		`UPDATE instruction_attempts SET failure_reason = 'error' WHERE status = 'failed' AND failure_reason IS NULL`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migratePortalLinksAutoDiscoveryInterval adds this link's own optional
// override columns (step 85: "it can also have a custom own period to
// auto-discovery") — same plain ALTER TABLE ADD COLUMN shape every
// other portal_links column already uses, guarded the same way (both
// columns land or neither does, checked via the first one's own
// presence, matching migratePortalLinksJobDetailInstructionsAIStatus's
// own two-column precedent above). Both NULL for every pre-existing
// row: "no custom period set — use the global interval" is every
// link's own default, and auto_discovery_last_run_at (this link's own
// independent clock, written only by the auto-discovery manager) has
// no meaning at all until a custom interval is actually set. See
// plan/ai/tools/career/step-85-auto-discovery-per-link-custom-interval.md.
func migratePortalLinksAutoDiscoveryInterval(db *sql.DB) error {
	exists, hasColumn, err := tableHasColumn(db, "portal_links", "auto_discovery_interval_minutes")
	if err != nil {
		return err
	}
	if !exists || hasColumn {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE portal_links ADD COLUMN auto_discovery_interval_minutes INTEGER`); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE portal_links ADD COLUMN auto_discovery_last_run_at INTEGER`)
	return err
}

// migratePortalsAiOverride (step 102) adds ai_platform_id and ai_model to portals
// and to portal_links on an install that already has them (a fresh one gets them
// from the base schema). Default ” means "no override", so nothing changes for
// existing rows. Runs after migrateJobsPortalLinkID, which may create the tables.
func migratePortalsAiOverride(db *sql.DB) error {
	for _, table := range []string{"portals", "portal_links"} {
		for _, col := range []string{"ai_platform_id", "ai_model"} {
			exists, hasColumn, err := tableHasColumn(db, table, col)
			if err != nil {
				return err
			}
			if !exists || hasColumn {
				continue
			}
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	return nil
}
