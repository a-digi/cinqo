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
	"net/http"

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

// migrateCrawlLogsContainer adds crawl_logs.container to a table that
// already existed before step 18 — CREATE TABLE IF NOT EXISTS alone
// never adds a column to an existing table. A no-op once already
// applied (or if crawl_logs doesn't exist yet at all, in which case
// the CREATE TABLE just above already created it with the column).
func migrateCrawlLogsContainer(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(crawl_logs)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasContainer := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == "container" {
			hasContainer = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasContainer {
		return nil
	}

	_, err = db.Exec(`ALTER TABLE crawl_logs ADD COLUMN container TEXT`)
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
func saveCrawlLog(container string, fields []extractField, nextSelector string, requestedMaxPages, effectiveMaxPages int, result paginatedCrawlResponse, pageHTML []string) {
	requestFields := make([]crawlRequestField, len(fields))
	for i, f := range fields {
		requestFields[i] = crawlRequestField{Label: f.Label, Selector: f.Selector, Attribute: f.Attribute, Multiple: f.Multiple}
	}

	fieldsJSON, err := json.Marshal(requestFields)
	if err != nil {
		return
	}

	logPages := make([]crawlLogPageEntry, len(result.Pages))
	for i, p := range result.Pages {
		logPages[i] = crawlLogPageEntry{pageExtractResult: p}
		if i < len(pageHTML) {
			logPages[i].HTML = pageHTML[i]
		}
	}
	pagesJSON, err := json.Marshal(logPages)
	if err != nil {
		return
	}

	id := uuid.NewString()
	_, _ = browserDB.Exec(
		// Millisecond precision (strftime's own %f), not datetime('now')'s
		// second-level precision — caught directly while verifying step
		// 22: two crawls logged within the same second tied on
		// created_at, leaving listCrawlLogs' own "newest first" ORDER BY
		// with no guaranteed order among ties (a real, pre-existing gap,
		// not introduced by step 22, but now something step 22's own
		// "Debug on, Log HTML on" verification actually exercises —
		// several crawls run back-to-back is the normal case for that
		// test, and plausibly for a real user running several crawls in
		// quick succession too). See
		// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
		`INSERT INTO crawl_logs (id, created_at, container, request_fields, next_selector, requested_max_pages, effective_max_pages, pages, stopped_reason, pages_visited)
		 VALUES (?, strftime('%Y-%m-%d %H:%M:%f', 'now'), ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, container, string(fieldsJSON), nextSelector, requestedMaxPages, effectiveMaxPages, string(pagesJSON), result.StoppedReason, result.PagesVisited,
	)

	_, _ = browserDB.Exec(
		`DELETE FROM crawl_logs WHERE id NOT IN (SELECT id FROM crawl_logs ORDER BY created_at DESC LIMIT ?)`,
		maxCrawlLogEntries,
	)
}

// listCrawlLogs returns every retained entry, newest first.
func listCrawlLogs() ([]crawlLogEntry, error) {
	rows, err := browserDB.Query(
		`SELECT id, created_at, container, request_fields, next_selector, requested_max_pages, effective_max_pages, pages, stopped_reason, pages_visited
		 FROM crawl_logs ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []crawlLogEntry{}
	for rows.Next() {
		var e crawlLogEntry
		var container sql.NullString
		var fieldsJSON, pagesJSON string
		if err := rows.Scan(&e.ID, &e.CreatedAt, &container, &fieldsJSON, &e.NextSelector, &e.RequestedMaxPages, &e.EffectiveMaxPages, &pagesJSON, &e.StoppedReason, &e.PagesVisited); err != nil {
			return nil, err
		}
		e.Container = container.String
		if err := json.Unmarshal([]byte(fieldsJSON), &e.Fields); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pagesJSON), &e.Pages); err != nil {
			return nil, err
		}
		logs = append(logs, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// deleteCrawlLog removes one entry by id.
func deleteCrawlLog(id string) error {
	_, err := browserDB.Exec(`DELETE FROM crawl_logs WHERE id = ?`, id)
	return err
}

// deleteAllCrawlLogs clears every entry — only ever reached via the
// explicit ?all=true query parameter (crawlLogsHandler below), never
// as the default of a bare DELETE with no parameters.
func deleteAllCrawlLogs() error {
	_, err := browserDB.Exec(`DELETE FROM crawl_logs`)
	return err
}

// crawlLogsHandler handles GET/DELETE /crawl-logs — the human-facing
// viewer this tool's own frontend calls; not an MCP tool (nothing about
// reading or clearing past crawl history needs the AI's own
// involvement). DELETE (step 22) requires either ?id=<id> (removes one
// entry) or ?all=true (clears every entry) — a bare DELETE with
// neither is a 400, never a silent "delete everything" default. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
func crawlLogsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
