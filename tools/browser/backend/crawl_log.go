// crawl_log.go (step 17) is a diagnostic-only record of every
// /crawl-paginated call — the instructions actually used and the real
// per-page result (including which field labels matched nothing on
// the page), so "why did this crawl find nothing" is answerable after
// the fact instead of unknowable. Written for both callers of
// /crawl-paginated: the AI's own --mcp adapter (crawl_paginated MCP
// tool) and career's own deterministic "Crawl now" (step 27), since
// both ultimately hit this same HTTP handler. A logging failure here
// never fails the crawl itself — this is observability, not a
// correctness dependency. See
// plan/ai/tools/browser/step-17-crawl-diagnostic-logging.md.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// maxCrawlLogEntries bounds crawl_logs' own growth — this is
// diagnostic/debug data, not a record anyone needs kept forever.
const maxCrawlLogEntries = 100

type crawlLogEntry struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	// Container (step 18) — empty when the crawl used flat (non-
	// grouped) extraction; a non-empty value here is what tells the
	// log viewer to expect Items (not Results) on each page below.
	Container         string              `json:"container,omitempty"`
	Fields            []crawlRequestField `json:"fields"`
	NextSelector      string              `json:"nextSelector"`
	RequestedMaxPages int                 `json:"requestedMaxPages"`
	EffectiveMaxPages int                 `json:"effectiveMaxPages"`
	Pages             []crawlLogPageEntry `json:"pages"`
	StoppedReason     string              `json:"stoppedReason"`
	PagesVisited      int                 `json:"pagesVisited"`
}

// crawlLogPageEntry is the log's own per-page storage shape —
// everything pageExtractResult already has, plus an optional HTML
// capture (step 22's own "Log HTML" debug setting) that must never
// reach the live paginatedCrawlResponse returned to the AI/tool caller
// — that would silently bloat every crawl_paginated result with full
// page HTML the caller never asked for. Kept as a distinct type for
// exactly that reason: marshaled only here, for persistence, never
// embedded in paginatedCrawlResponse itself. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
type crawlLogPageEntry struct {
	pageExtractResult
	HTML string `json:"html,omitempty"`
}

// migrateCrawlLogsFileStorage detects a pre-step-39 crawl_logs table
// (has a "pages" column — every entry's full body stored inline) and
// drops it, letting the CREATE TABLE IF NOT EXISTS just above (in
// initBrowserDB) recreate it fresh in the new, file-backed shape.
// crawl_logs is this tool's own explicitly disposable diagnostic data
// (capped at maxCrawlLogEntries, "not a record anyone needs kept
// forever" per this file's own top comment) and this tool has no
// migration runner at all, so a real per-row migration — reading each
// old row's own inline pages JSON and writing it out to a new file —
// isn't worth the complexity here. This deliberately discards any
// existing crawl log history on upgrade. A no-op on a brand-new
// install (the CREATE TABLE above already created the current shape)
// or an already-migrated one. See
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

// crawlRequestField mirrors extractField (extract.go) — declared
// separately here rather than reused directly, so this log's own wire
// shape doesn't silently change if extractField's own jsonschema tags
// (meant for the AI, not for storage) ever do.
type crawlRequestField struct {
	Label     string `json:"label"`
	Selector  string `json:"selector"`
	Attribute string `json:"attribute,omitempty"`
	Multiple  bool   `json:"multiple,omitempty"`
}

// writeFileAtomically writes data to path via a temp file + rename —
// cheap insurance against a truncated/corrupt file if the process is
// killed mid-write, a real (if rare) possibility given a captured
// page's own HTML is written here untruncated (step 40) and can be
// several MB for a real page. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md and
// plan/ai/tools/browser/step-40-log-html-truncation-fix.md.
func writeFileAtomically(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// saveCrawlLog persists one completed /crawl-paginated call and prunes
// down to maxCrawlLogEntries, oldest first. Best-effort: called after
// the real crawl has already succeeded, so a failure here is logged
// nowhere further and never surfaces to the crawl's own caller — this
// must never turn a successful crawl into a failed HTTP response.
// Called only when Debug is on at all (step 22) — paginatedCrawlHandler
// itself decides that, once, before ever calling this. pageHTML is nil
// unless "Log HTML" was also on, in which case it's parallel to
// result.Pages (one entry per page) — attached per page below, never
// into result.Pages itself (that would leak into the live AI-facing
// response, which this function never touches). See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
//
// Step 39 — the full entry (container/fields/next selector/max pages/
// every page's own results, notFound, Cloudflare flags, and HTML) is
// written to its own file under crawlLogsDir, named by this entry's
// own id; crawl_logs itself only ever stores what listCrawlLogs' own
// collapsed row needs (firstPageURL/notFoundCount, computed here once
// from result.Pages, replace what the frontend used to compute
// client-side against the full payload) plus the file's path. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
func saveCrawlLog(container string, fields []extractField, nextSelector string, requestedMaxPages, effectiveMaxPages int, result paginatedCrawlResponse, pageHTML []string) {
	requestFields := make([]crawlRequestField, len(fields))
	for i, f := range fields {
		requestFields[i] = crawlRequestField{Label: f.Label, Selector: f.Selector, Attribute: f.Attribute, Multiple: f.Multiple}
	}

	logPages := make([]crawlLogPageEntry, len(result.Pages))
	notFoundCount := 0
	for i, p := range result.Pages {
		logPages[i] = crawlLogPageEntry{pageExtractResult: p}
		if i < len(pageHTML) {
			logPages[i].HTML = pageHTML[i]
		}
		notFoundCount += len(p.NotFound)
	}

	firstPageURL := ""
	if len(result.Pages) > 0 {
		firstPageURL = result.Pages[0].URL
	}

	// Millisecond precision, not datetime('now')'s second-level one —
	// caught directly while verifying step 22: two crawls logged within
	// the same second tied on created_at, leaving listCrawlLogs' own
	// "newest first" ORDER BY with no guaranteed order among ties.
	// Computed once, here, and reused for both the file and the DB row
	// (rather than letting SQLite generate its own via strftime) so the
	// two never disagree. See
	// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
	createdAt := time.Now().UTC().Format("2006-01-02 15:04:05.000")

	id := uuid.NewString()
	entry := crawlLogEntry{
		ID:                id,
		CreatedAt:         createdAt,
		Container:         container,
		Fields:            requestFields,
		NextSelector:      nextSelector,
		RequestedMaxPages: requestedMaxPages,
		EffectiveMaxPages: effectiveMaxPages,
		Pages:             logPages,
		StoppedReason:     result.StoppedReason,
		PagesVisited:      result.PagesVisited,
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return
	}

	path := filepath.Join(crawlLogsDir, id+".json")
	if err := writeFileAtomically(path, entryJSON); err != nil {
		return
	}

	if _, err := browserDB.Exec(
		`INSERT INTO crawl_logs (id, created_at, stopped_reason, pages_visited, first_page_url, not_found_count, log_file_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, createdAt, result.StoppedReason, result.PagesVisited, firstPageURL, notFoundCount, path,
	); err != nil {
		// The DB row is the source of truth; a file with no matching row
		// is just orphaned disk space, not a correctness problem — but no
		// reason to leave it around when the insert itself failed.
		_ = os.Remove(path)
		return
	}

	prunedPaths, _ := crawlLogPathsBeyondLimit(maxCrawlLogEntries)
	_, _ = browserDB.Exec(
		`DELETE FROM crawl_logs WHERE id NOT IN (SELECT id FROM crawl_logs ORDER BY created_at DESC LIMIT ?)`,
		maxCrawlLogEntries,
	)
	deleteCrawlLogFilesAsync(prunedPaths)
}

// crawlLogPathsBeyondLimit returns the log_file_path of every row that
// a prune to the given limit (oldest first) would remove — read
// before the DELETE runs, since the row (and its own path) is gone
// once it does.
func crawlLogPathsBeyondLimit(limit int) ([]string, error) {
	rows, err := browserDB.Query(
		`SELECT log_file_path FROM crawl_logs WHERE id NOT IN (SELECT id FROM crawl_logs ORDER BY created_at DESC LIMIT ?)`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// deleteCrawlLogFilesAsync launches one goroutine to os.Remove every
// path given — never blocks the caller (the DB row is already gone by
// the time this is called; a slow or failed filesystem delete must
// never hold up the HTTP response, especially for deleteAllCrawlLogs'
// own up-to-maxCrawlLogEntries files at once) and never surfaces a
// failure anywhere — best-effort, matching this file's own existing
// convention (saveCrawlLog itself is already "a logging failure here
// never fails the crawl"): an already-missing or permission-denied
// file just stays an orphan, same as any other best-effort cleanup in
// this file. See plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
func deleteCrawlLogFilesAsync(paths []string) {
	if len(paths) == 0 {
		return
	}
	go func(paths []string) {
		for _, p := range paths {
			_ = os.Remove(p)
		}
	}(paths)
}

// crawlLogSummary is what listCrawlLogs returns — exactly what the
// list view's own collapsed row needs (see CrawlLogsPage.tsx),
// computed once at write time (saveCrawlLog) so listing never opens a
// single crawl_logs/<id>.json file. The full entry (crawlLogEntry) is
// fetched separately, only for whichever one row a caller actually
// expands — see fetchCrawlLogDetail below. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
type crawlLogSummary struct {
	ID            string `json:"id"`
	CreatedAt     string `json:"createdAt"`
	StoppedReason string `json:"stoppedReason"`
	PagesVisited  int    `json:"pagesVisited"`
	FirstPageURL  string `json:"firstPageUrl,omitempty"`
	NotFoundCount int    `json:"notFoundCount"`
}

// listCrawlLogs returns every retained entry's own lightweight
// summary, newest first — no file I/O at all.
func listCrawlLogs() ([]crawlLogSummary, error) {
	rows, err := browserDB.Query(
		`SELECT id, created_at, stopped_reason, pages_visited, first_page_url, not_found_count
		 FROM crawl_logs ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []crawlLogSummary{}
	for rows.Next() {
		var s crawlLogSummary
		var firstPageURL sql.NullString
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.StoppedReason, &s.PagesVisited, &firstPageURL, &s.NotFoundCount); err != nil {
			return nil, err
		}
		s.FirstPageURL = firstPageURL.String
		logs = append(logs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// errCrawlLogNotFound is a 404, not a 500 — a caller asking for an id
// that never existed, or one already deleted, is not a server failure.
var errCrawlLogNotFound = errors.New("crawl log not found")

// fetchCrawlLogDetail resolves id's own log_file_path and opens that
// file — the caller (crawlLogsHandler) streams it directly to the HTTP
// response via http.ServeContent rather than this function
// unmarshaling/remarshaling it, so a huge captured-HTML entry is never
// held whole in memory twice over on this path. The caller is
// responsible for closing the returned *os.File.
func fetchCrawlLogDetail(id string) (*os.File, os.FileInfo, error) {
	var path string
	err := browserDB.QueryRow(`SELECT log_file_path FROM crawl_logs WHERE id = ?`, id).Scan(&path)
	if err == sql.ErrNoRows {
		return nil, nil, errCrawlLogNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil, errCrawlLogNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

// deleteCrawlLog removes one entry by id, then asynchronously removes
// its own file (step 39) — the path is read before the DELETE, since
// the row (and its own log_file_path) is gone once it runs.
func deleteCrawlLog(id string) error {
	var path string
	if err := browserDB.QueryRow(`SELECT log_file_path FROM crawl_logs WHERE id = ?`, id).Scan(&path); err != nil {
		return err
	}
	if _, err := browserDB.Exec(`DELETE FROM crawl_logs WHERE id = ?`, id); err != nil {
		return err
	}
	deleteCrawlLogFilesAsync([]string{path})
	return nil
}

// deleteAllCrawlLogs clears every entry — only ever reached via the
// explicit ?all=true query parameter (crawlLogsHandler below), never
// as the default of a bare DELETE with no parameters. Step 39 — also
// asynchronously removes every entry's own file, same reasoning as
// deleteCrawlLog above.
func deleteAllCrawlLogs() error {
	rows, err := browserDB.Query(`SELECT log_file_path FROM crawl_logs`)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if _, err := browserDB.Exec(`DELETE FROM crawl_logs`); err != nil {
		return err
	}
	deleteCrawlLogFilesAsync(paths)
	return nil
}

// crawlLogsHandler handles GET/DELETE /crawl-logs — the human-facing
// viewer this tool's own frontend calls; not an MCP tool (nothing about
// reading or clearing past crawl history needs the AI's own
// involvement). DELETE (step 22) requires either ?id=<id> (removes one
// entry) or ?all=true (clears every entry) — a bare DELETE with
// neither is a 400, never a silent "delete everything" default. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
//
// Step 39 — GET also accepts ?id=<id>, mirroring DELETE's own
// convention: a bare GET returns the lightweight list, GET?id=
// streams one entry's full file (container/fields/every page's own
// results/notFound/Cloudflare flags/HTML) straight from disk via
// http.ServeContent, never loading it into a Go struct first. See
// plan/ai/tools/browser/step-39-crawl-log-file-storage.md.
func crawlLogsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if id := r.URL.Query().Get("id"); id != "" {
			f, info, err := fetchCrawlLogDetail(id)
			if errors.Is(err, errCrawlLogNotFound) {
				http.Error(w, "crawl log not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "failed to load crawl log: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", "application/json")
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
			return
		}

		logs, err := listCrawlLogs()
		if err != nil {
			http.Error(w, "failed to load crawl logs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs})

	case http.MethodDelete:
		if r.URL.Query().Get("all") == "true" {
			if err := deleteAllCrawlLogs(); err != nil {
				http.Error(w, "failed to clear crawl logs: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required (or all=true to clear every log)", http.StatusBadRequest)
			return
		}
		if err := deleteCrawlLog(id); err != nil {
			http.Error(w, "failed to delete crawl log: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
