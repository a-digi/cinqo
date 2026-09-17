package shared

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var (
	// DB is this tool's own single SQLite handle — never cinqo's own
	// core database. Opened once by InitDB, below.
	DB *sql.DB
	// CrawlLogsDir holds one JSON file per crawl_logs row (step 39),
	// named <id>.json — set once here, read by crawl_log.go's own
	// save/fetch/delete paths. See
	// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
	CrawlLogsDir string
	// FetchCacheDir holds fetch_page_html's own short-lived on-disk
	// cache (step 45) — one index.json manifest plus one .cache file
	// per distinct cached URL, set once here, read/written by
	// fetch_cache.go's own lookup/store functions. See
	// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
	FetchCacheDir string
)

// InitDB opens (creating if needed) browser.db, both under
// TOOL_DB_DIR. TOOL_DB_DIR itself is never pre-created by the host —
// same convention pdf_generator's own main() already established for
// TOOL_TMP_DIR/TOOL_UPLOADS_DIR — so this tool creates it itself. Does
// NOT load/generate this tool's own credential-encryption key — that
// stays login_credentials.go's own concern (main package), loaded
// separately via its own initCryptoKey, since it has nothing to do
// with the DB/schema this function owns.
func InitDB() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	CrawlLogsDir = filepath.Join(dbDir, "crawl_logs")
	if err := os.MkdirAll(CrawlLogsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create crawl_logs directory: %w", err)
	}

	FetchCacheDir = filepath.Join(dbDir, "fetch_cache")
	if err := os.MkdirAll(FetchCacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create fetch_cache directory: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		return fmt.Errorf("failed to open browser database: %w", err)
	}
	// This tool's own write volume is tiny (a human occasionally
	// registering/revoking a credential) — a single connection avoids
	// any "database is locked" surprise from modernc.org/sqlite's own
	// concurrent-writer behavior, which isn't worth tuning around here.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS login_credentials (
		domain             TEXT PRIMARY KEY,
		username           TEXT NOT NULL,
		encrypted_password TEXT NOT NULL,
		created_at         TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at         TEXT
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare login_credentials schema: %w", err)
	}

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
	// plain SELECT with no "no rows yet" special-casing. See
	// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS browser_settings (
		id             INTEGER PRIMARY KEY CHECK (id = 1),
		debug_enabled  INTEGER NOT NULL DEFAULT 0,
		debug_log_html INTEGER NOT NULL DEFAULT 0,
		updated_at     TEXT
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare browser_settings schema: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO browser_settings (id, debug_enabled, debug_log_html) VALUES (1, 0, 0)`); err != nil {
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

	DB = db
	return nil
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
