package shared

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	// DB is this tool's own single SQLite handle — never cinqo's own
	// core database. Opened once by InitDB, below.
	DB *sql.DB
	// CrawlLogsDir holds one JSON file per crawl_logs row (step 39),
	// named <id>.json — set once here, read by crawl_log.go's own
	// save/fetch/delete paths. Lives under TOOL_LOGS_DIR, not
	// TOOL_DB_DIR — this is diagnostic/history data, not this tool's
	// own SQLite database. See
	// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
	CrawlLogsDir string
	// FetchCacheDir holds fetch_page_html's own short-lived on-disk
	// cache (step 45) — one index.json manifest plus one .cache file
	// per distinct cached URL, set once here, read/written by
	// fetch_cache.go's own lookup/store functions. Lives under
	// TOOL_CACHE_DIR, not TOOL_DB_DIR/TOOL_LOGS_DIR — a performance
	// cache is neither this tool's own SQLite database nor diagnostic/
	// history data; it's disposable optimization data this tool can
	// regenerate from scratch at any time. See
	// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
	FetchCacheDir string
	// PageSnapshotsDir holds the sanitised page copies the Career picker shows (plan 247): one .html and one .json per snapshot, deleted when the
	// picker closes, by an age sweep, or by the manual "delete temporary files" button. Always safe to remove.
	PageSnapshotsDir string
	// CacheRoot is the tool's whole cache folder (TOOL_CACHE_DIR, absolute): the temporary files folder that Browser & Engine shows and clears (plan 248).
	// FetchCacheDir, PageSnapshotsDir and the Chrome profile root all live inside it.
	CacheRoot string
	// ChallengeLogsDir holds one subfolder per crawl session's own
	// requestID, each containing one raw-HTML .txt file per tick of the
	// Cloudflare human-wait loop (Debug + Log HTML only) — set once
	// here, read/written by crawler/challenge_log.go. Lives under
	// TOOL_LOGS_DIR, same reasoning as CrawlLogsDir above. Unlike
	// CrawlLogsDir/FetchCacheDir, the per-session subfolder itself is
	// created lazily on first actual write, not eagerly here — most
	// crawls never reach that wait loop at all.
	ChallengeLogsDir string
)

// InitDB opens (creating if needed) browser.db under TOOL_DB_DIR,
// CrawlLogsDir/ChallengeLogsDir under TOOL_LOGS_DIR, and FetchCacheDir
// under TOOL_CACHE_DIR — none pre-created by the host — same
// convention pdf_generator's own main() already established for
// TOOL_TMP_DIR/TOOL_UPLOADS_DIR — so this tool creates them itself.
func InitDB() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	logsDir := os.Getenv("TOOL_LOGS_DIR")
	if logsDir == "" {
		return fmt.Errorf("TOOL_LOGS_DIR is not set")
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_LOGS_DIR: %w", err)
	}

	CrawlLogsDir = filepath.Join(logsDir, "crawl_logs")
	if err := os.MkdirAll(CrawlLogsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create crawl_logs directory: %w", err)
	}

	cacheDir := os.Getenv("TOOL_CACHE_DIR")
	if cacheDir == "" {
		return fmt.Errorf("TOOL_CACHE_DIR is not set")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_CACHE_DIR: %w", err)
	}

	CacheRoot = filepath.Clean(cacheDir)

	FetchCacheDir = filepath.Join(cacheDir, "fetch_cache")
	if err := os.MkdirAll(FetchCacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create fetch_cache directory: %w", err)
	}

	PageSnapshotsDir = filepath.Join(cacheDir, "page_snapshots")
	if err := os.MkdirAll(PageSnapshotsDir, 0o700); err != nil {
		return fmt.Errorf("failed to create page_snapshots directory: %w", err)
	}

	ChallengeLogsDir = filepath.Join(logsDir, "challenge_logs")
	if err := os.MkdirAll(ChallengeLogsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create challenge_logs directory: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		return fmt.Errorf("failed to open browser database: %w", err)
	}
	// This tool's own write volume is tiny (a human occasionally
	// changing a setting) — a single connection avoids
	// any "database is locked" surprise from modernc.org/sqlite's own
	// concurrent-writer behavior, which isn't worth tuning around here.
	db.SetMaxOpenConns(1)

	// This tool stores NO login: the person logs in on the site's own page in a throw-away window (plan 182). An older version kept
	// encrypted credentials; they are removed here, with their key file.
	removeStoredCredentials(db, dbDir)

	// crawl_logs (step 17, restructured step 39) — diagnostic record of
	// every /crawl-paginated call, AI-driven or deterministic. Holds
	// only what the list view's own collapsed row needs to render
	// without ever touching a file — the full entry (container, fields,
	// next selector, max pages, and each page's own results/notFound/
	// Cloudflare flags/HTML) lives in log_file_path instead, one JSON
	// file per row under CrawlLogsDir, named by this row's own id. See
	// crawler/crawl_log.go and
	// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS crawl_logs (
		id              TEXT PRIMARY KEY,
		created_at      TEXT NOT NULL,
		stopped_reason  TEXT NOT NULL,
		pages_visited   INTEGER NOT NULL,
		first_page_url  TEXT,
		not_found_count INTEGER NOT NULL DEFAULT 0,
		log_file_path   TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare crawl_logs schema: %w", err)
	}
	if err := migrateCrawlLogsFileStorage(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate crawl_logs schema: %w", err)
	}

	// browser_settings (step 22) — a singleton row (id = 1, enforced by
	// the CHECK constraint) holding the Debug toggle crawl logging is
	// now gated behind. INSERT OR IGNORE right after CREATE TABLE IF NOT
	// EXISTS so both a fresh install and an existing one always end up
	// with exactly one row, defaulted OFF — every read downstream is a
	// plain SELECT with no "no rows yet" special-casing. debug_log_challenge
	// (step 68) is a separate, independent toggle from debug_log_html —
	// the challenge-detector HTML dump (waitForHumanToClearCloudflare,
	// crawler/crawl.go) is its own concern from the regular crawl_logs
	// HTML capture (paginatedCrawlHandler), not implied by it. See
	// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS browser_settings (
		id                  INTEGER PRIMARY KEY CHECK (id = 1),
		debug_enabled       INTEGER NOT NULL DEFAULT 0,
		debug_log_html      INTEGER NOT NULL DEFAULT 0,
		debug_log_challenge INTEGER NOT NULL DEFAULT 0,
		-- fetch_engine (step 50): which engine serves the primary single-page
		-- fetch — 'katana' (the default since plan 189) or 'rod' (pure Go over
		-- CDP). Read on every fetch by the crawler. A fresh install gets it here; an
		-- existing table gets it from migrateBrowserSettingsFetchEngineColumn
		-- and its CHECK from the migrations after it (the last one,
		-- migrateBrowserSettingsDropPlaywright, plan 187, removed the value
		-- 'playwright'). See plan/ai/tools/browser/step-50-fetch-engine-setting-page.md.
		fetch_engine        TEXT NOT NULL DEFAULT 'katana' CHECK (fetch_engine IN ('rod','katana')),
		-- human_assist (step 96): when an AI call meets a challenge only a
		-- person can clear, open a browser window for them (instead of just
		-- failing). On by default; an existing table gets it from
		-- migrateBrowserSettingsHumanAssistColumn.
		human_assist        INTEGER NOT NULL DEFAULT 1,
		updated_at          TEXT
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare browser_settings schema: %w", err)
	}
	if err := migrateBrowserSettingsChallengeColumn(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings schema: %w", err)
	}
	if err := migrateBrowserSettingsFetchEngineColumn(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings fetch_engine column: %w", err)
	}
	if err := migrateBrowserSettingsHumanAssistColumn(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings human_assist column: %w", err)
	}
	if err := migrateBrowserSettingsAllowKatana(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings for the katana engine: %w", err)
	}
	if err := migrateBrowserSettingsDropPlaywright(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings away from the playwright engine: %w", err)
	}
	if err := migrateBrowserSettingsDefaultKatana(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to migrate browser_settings to the katana default: %w", err)
	}
	// The engine is named here on purpose: the seed must not depend on the column default, which an older
	// table (rebuilt or altered by an earlier migration) still has as 'rod'.
	if _, err := db.Exec(`INSERT OR IGNORE INTO browser_settings (id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine) VALUES (1, 0, 0, 0, ?)`, DefaultFetchEngine); err != nil {
		db.Close()
		return fmt.Errorf("failed to seed browser_settings: %w", err)
	}

	// cloudflare_domains (step 28) — a persistent record of every
	// hostname this tool has ever seen give the headless session an
	// unresolved Cloudflare challenge, so future crawls against the
	// same domain can skip straight to the headed fallback (step 29)
	// instead of re-discovering the same failure every time. See
	// plan/ai/tools/browser/step-28-cloudflare-domain-cache.md.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS cloudflare_domains (
		domain            TEXT PRIMARY KEY,
		reason            TEXT NOT NULL,
		first_detected_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare cloudflare_domains schema: %w", err)
	}

	// domain_clearance (step 96) — the cf_clearance cookie a person earned by
	// clearing a site's challenge, kept so agents and auto-discovery can reuse
	// it until it expires (across restarts). Only cf_clearance, never exposed
	// by any route. See
	// plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS domain_clearance (
		domain      TEXT NOT NULL,
		name        TEXT NOT NULL,
		value       TEXT NOT NULL,
		path        TEXT NOT NULL,
		secure      INTEGER NOT NULL,
		http_only   INTEGER NOT NULL,
		same_site   TEXT,
		expires_at  TEXT NOT NULL,
		user_agent  TEXT NOT NULL,
		source      TEXT NOT NULL CHECK (source IN ('human','automatic')),
		obtained_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (domain, name, path)
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare domain_clearance schema: %w", err)
	}

	// login_sessions (plan 195, step 1) — the sealed cookies of a site's logged-in session a person chose to keep for Katana. Never a
	// password; see loginsession.go for the rules. Expired rows are deleted here, at every start.
	if err := prepareLoginSessions(db, dbDir); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare login_sessions schema: %w", err)
	}

	DB = db
	return nil
}

// migrateBrowserSettingsHumanAssistColumn (step 96) adds human_assist to an
// existing browser_settings table that predates it. The existing row takes the
// column default, 1 (on). A no-op if the column is already present.
func migrateBrowserSettingsHumanAssistColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(browser_settings)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "human_assist" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = db.Exec(`ALTER TABLE browser_settings ADD COLUMN human_assist INTEGER NOT NULL DEFAULT 1`)
	return err
}

// migrateCrawlLogsFileStorage detects a pre-step-39 crawl_logs table
// (has a "pages" column — every entry's full body stored inline) and
// drops it, letting the CREATE TABLE IF NOT EXISTS just above (in
// InitDB) recreate it fresh in the new, file-backed shape. crawl_logs
// is this tool's own explicitly disposable diagnostic data (capped at
// maxCrawlLogEntries, crawler/crawl_log.go's own top comment) and this
// tool has no migration runner at all, so a real per-row migration —
// reading each old row's own inline pages JSON and writing it out to a
// new file — isn't worth the complexity here. This deliberately
// discards any existing crawl log history on upgrade. A no-op on a
// brand-new install (the CREATE TABLE above already created the
// current shape) or an already-migrated one. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
func migrateCrawlLogsFileStorage(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(crawl_logs)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasPages := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "pages" {
			hasPages = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !hasPages {
		return nil
	}

	if _, err := db.Exec(`DROP TABLE crawl_logs`); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE crawl_logs (
		id              TEXT PRIMARY KEY,
		created_at      TEXT NOT NULL,
		stopped_reason  TEXT NOT NULL,
		pages_visited   INTEGER NOT NULL,
		first_page_url  TEXT,
		not_found_count INTEGER NOT NULL DEFAULT 0,
		log_file_path   TEXT NOT NULL
	)`)
	return err
}

// migrateBrowserSettingsFetchEngineColumn (step 50) adds fetch_engine to
// an existing browser_settings table that predates it (a fresh install
// gets it from the CREATE TABLE above). The existing singleton row
// takes the column default, 'rod', which is the default engine; the
// Debug values already stored are untouched. A no-op if the column is
// already present.
func migrateBrowserSettingsFetchEngineColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(browser_settings)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "fetch_engine" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	// Historical on purpose (plan 187): this runs only on a database that predates the column, and
	// its CHECK still names the engine that was removed. migrateBrowserSettingsDropPlaywright runs
	// after the migrations and rebuilds the table without it.
	_, err = db.Exec(`ALTER TABLE browser_settings ADD COLUMN fetch_engine TEXT NOT NULL DEFAULT 'rod' CHECK (fetch_engine IN ('rod','playwright'))`)
	return err
}

// migrateBrowserSettingsAllowKatana (plan 184) lets an existing
// browser_settings table store the value 'katana'. The fetch_engine
// column carries a CHECK that SQLite cannot alter, so a table whose
// stored definition does not mention 'katana' is rebuilt: a new table
// with the current definition, the one row copied, the old one dropped,
// the new one renamed — all in one transaction, so a failure leaves the
// old table untouched. A no-op once the definition allows 'katana'.
func migrateBrowserSettingsAllowKatana(db *sql.DB) error {
	var def string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'browser_settings'`).Scan(&def); err != nil {
		return err
	}
	if strings.Contains(def, "'katana'") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Historical on purpose (plan 187): the CHECK below is the one plan 184 wrote; the migration
	// after this one (migrateBrowserSettingsDropPlaywright) rebuilds the table without 'playwright'.
	for _, q := range []string{
		`CREATE TABLE browser_settings_new (
			id                  INTEGER PRIMARY KEY CHECK (id = 1),
			debug_enabled       INTEGER NOT NULL DEFAULT 0,
			debug_log_html      INTEGER NOT NULL DEFAULT 0,
			debug_log_challenge INTEGER NOT NULL DEFAULT 0,
			fetch_engine        TEXT NOT NULL DEFAULT 'rod' CHECK (fetch_engine IN ('rod','playwright','katana')),
			human_assist        INTEGER NOT NULL DEFAULT 1,
			updated_at          TEXT
		)`,
		`INSERT INTO browser_settings_new (id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist, updated_at)
			SELECT id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist, updated_at FROM browser_settings`,
		`DROP TABLE browser_settings`,
		`ALTER TABLE browser_settings_new RENAME TO browser_settings`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateBrowserSettingsDropPlaywright (plan 187) removes the engine value
// 'playwright' from an existing browser_settings table: the Playwright engine
// no longer exists. The fetch_engine CHECK cannot be altered in SQLite, so a
// table whose stored definition still names 'playwright' is rebuilt (the same
// way as migrateBrowserSettingsAllowKatana): a new table with the current
// definition, the one row copied with a stored 'playwright' turned into 'rod',
// the old table dropped, the new one renamed — all in one transaction, so a
// failure leaves the old table untouched. A no-op once the definition no
// longer names 'playwright'. The two older migrations above it keep writing
// the old CHECK on purpose: they only run on very old databases, and this one
// runs right after them.
func migrateBrowserSettingsDropPlaywright(db *sql.DB) error {
	var def string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'browser_settings'`).Scan(&def); err != nil {
		return err
	}
	if !strings.Contains(def, "'playwright'") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE browser_settings_new (
			id                  INTEGER PRIMARY KEY CHECK (id = 1),
			debug_enabled       INTEGER NOT NULL DEFAULT 0,
			debug_log_html      INTEGER NOT NULL DEFAULT 0,
			debug_log_challenge INTEGER NOT NULL DEFAULT 0,
			fetch_engine        TEXT NOT NULL DEFAULT 'rod' CHECK (fetch_engine IN ('rod','katana')),
			human_assist        INTEGER NOT NULL DEFAULT 1,
			updated_at          TEXT
		)`,
		`INSERT INTO browser_settings_new (id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist, updated_at)
			SELECT id, debug_enabled, debug_log_html, debug_log_challenge,
				CASE WHEN fetch_engine = 'katana' THEN 'katana' ELSE 'rod' END,
				human_assist, updated_at FROM browser_settings`,
		`DROP TABLE browser_settings`,
		`ALTER TABLE browser_settings_new RENAME TO browser_settings`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateBrowserSettingsDefaultKatana (plan 189) makes Katana the engine of an install nobody ever
// configured. A stored 'rod' is either a person's choice or only the old default, and the table cannot
// tell the two apart except by one fact: the seed row has updated_at NULL, and ANY save through the
// Engine or Debug page sets it. So only a never-saved row that still says 'rod' is moved; a row that was
// saved keeps its engine (a person who saved the Debug toggles but never looked at the engine keeps 'rod'
// and finds the "Recommended" mark on Katana on the Engine page). Idempotent: once the row says
// 'katana' it matches nothing, and a person who later picks Rod through the page sets updated_at, so
// this can never flip that choice back.
func migrateBrowserSettingsDefaultKatana(db *sql.DB) error {
	_, err := db.Exec(`UPDATE browser_settings SET fetch_engine = 'katana' WHERE id = 1 AND fetch_engine = 'rod' AND updated_at IS NULL`)
	return err
}

// migrateBrowserSettingsChallengeColumn adds debug_log_challenge to an
// existing browser_settings table that predates it — the CREATE TABLE
// IF NOT EXISTS above only creates the column's current, full shape on
// a brand-new install; an already-existing table (debug_enabled/
// debug_log_html only) needs this ALTER TABLE to catch up. Defaults
// existing rows to 0 (off), same as a fresh install's own seed row. A
// no-op if the column is already present.
func migrateBrowserSettingsChallengeColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(browser_settings)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "debug_log_challenge" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	_, err = db.Exec(`ALTER TABLE browser_settings ADD COLUMN debug_log_challenge INTEGER NOT NULL DEFAULT 0`)
	return err
}

// credentialsKeyFile is the name of the key file an older version of this tool kept next to its database to encrypt stored credentials.
const credentialsKeyFile = "credentials.key"

// removeStoredCredentials deletes what an older version of this tool stored for logins: the table login_credentials (its rows are
// counted in the log, never read) and the key file that encrypted them. This tool keeps no login of any kind any more (plan 182: the
// person logs in on the site itself, in a window that is thrown away). Best effort and idempotent: a failure is logged, never fatal, and
// a database or folder without them is left alone.
func removeStoredCredentials(db *sql.DB, dbDir string) {
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_credentials`).Scan(&rows); err == nil {
		if _, err := db.Exec(`DROP TABLE IF EXISTS login_credentials`); err != nil {
			log.Printf("browser tool: could not remove the old stored-credentials table: %v", err)
		} else {
			log.Printf("browser tool: removed the old stored-credentials table (%d stored credential(s) deleted)", rows)
		}
	}
	if err := os.Remove(filepath.Join(dbDir, credentialsKeyFile)); err == nil {
		log.Print("browser tool: removed the old credential encryption key")
	} else if !os.IsNotExist(err) {
		log.Printf("browser tool: could not remove the old credential encryption key: %v", err)
	}
}
