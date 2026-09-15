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
	Pages             []pageExtractResult `json:"pages"`
	StoppedReason     string              `json:"stoppedReason"`
	PagesVisited      int                 `json:"pagesVisited"`
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
func saveCrawlLog(container string, fields []extractField, nextSelector string, requestedMaxPages, effectiveMaxPages int, result paginatedCrawlResponse) {
	requestFields := make([]crawlRequestField, len(fields))
	for i, f := range fields {
		requestFields[i] = crawlRequestField{Label: f.Label, Selector: f.Selector, Attribute: f.Attribute, Multiple: f.Multiple}
	}

	fieldsJSON, err := json.Marshal(requestFields)
	if err != nil {
		return
	}
	pagesJSON, err := json.Marshal(result.Pages)
	if err != nil {
		return
	}

	id := uuid.NewString()
	_, _ = browserDB.Exec(
		`INSERT INTO crawl_logs (id, created_at, container, request_fields, next_selector, requested_max_pages, effective_max_pages, pages, stopped_reason, pages_visited)
		 VALUES (?, datetime('now'), ?, ?, ?, ?, ?, ?, ?, ?)`,
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

// crawlLogsHandler handles GET /crawl-logs — the human-facing viewer
// this tool's own frontend calls; not an MCP tool (nothing about
// reading past crawl history needs the AI's own involvement).
func crawlLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logs, err := listCrawlLogs()
	if err != nil {
		http.Error(w, "failed to load crawl logs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs})
}
