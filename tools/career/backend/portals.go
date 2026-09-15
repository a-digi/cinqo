// portals.go is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own portals/portal_links tables — a small, curated list of
// job-board sources the AI should crawl, and the URLs on each one to
// visit. A Portal Link is forced to be mapped to an existing Portal,
// never implicit — the same pattern profile.go/persona.go/
// recruiters.go already use for their own parent references.
//
// Step 19 added crawl_instructions read/write — an AI-authored YAML
// document in the exact same shape browser's own crawl_paginated MCP
// tool expects (fields[] + pagination), so the AI's own next step is
// handing it straight to that tool. career cannot import browser's own
// validation (two fully separate Go modules/binaries, no shared
// package) — validateCrawlInstructions below is a small, independent
// reimplementation of the *shape* check only, never a guarantee the
// instruction actually works against a real page. See
// plan/ai/tools/career/step-18-portals.md and
// plan/ai/tools/career/step-19-portal-crawl-instructions.md.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

var errUnknownPortal = errors.New("unknown portal id")

func requirePortalExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM portals WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownPortal
	case err != nil:
		return err
	}
	return nil
}

var errUnknownPortalLink = errors.New("unknown portal link id")

func requirePortalLinkExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM portal_links WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownPortalLink
	case err != nil:
		return err
	}
	return nil
}

var errDuplicatePortalLink = errors.New("this url is already a link on this portal")

type portalLink struct {
	ID                string `json:"id"`
	PortalID          string `json:"portalId"`
	URL               string `json:"url"`
	Title             string `json:"title,omitempty"`
	CrawlInstructions string `json:"crawlInstructions,omitempty"`
	// LastCrawledAt (step 26) is the most recent jobs.crawled_at
	// among every job ever saved against this link — read-only,
	// derived, never itself written; empty when no job has ever been
	// saved for this link. See
	// plan/ai/tools/career/step-26-last-crawled-hint.md.
	LastCrawledAt string `json:"lastCrawledAt,omitempty"`
	// InstructionsAIError/InstructionsAIErrorAt (step 33) record the
	// most recent failure of an AI-driven crawl-instructions
	// generation/edit attempt for this link — both empty when no
	// failure is currently recorded (the default, and the state after
	// a successful attempt clears them). See
	// plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
	InstructionsAIError   string `json:"instructionsAiError,omitempty"`
	InstructionsAIErrorAt string `json:"instructionsAiErrorAt,omitempty"`
	// HasActiveCrawlRun (step 37) is true while a detached "Crawl now"
	// run is in progress for this link — read-only, derived from
	// crawl_runs, never itself written. Lets the frontend resume
	// watching a run still going after a page reload/reopen without a
	// separate round trip per link. See
	// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
	HasActiveCrawlRun bool   `json:"hasActiveCrawlRun"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type portal struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Links     []portalLink `json:"links"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt,omitempty"`
}

func createPortal(name string) (string, error) {
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO portals (id, name, created_at) VALUES (?, ?, datetime('now'))`,
		id, name,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// lastCrawledByPortalLink (step 26) reads the most recent
// jobs.crawled_at per portal_link_id, in one query — the same jobsDB
// handle listPortals already uses, no new database or cross-DB join.
// A link with no saved job at all simply has no entry in the returned
// map (not a zero-value string), so callers' own map lookup naturally
// leaves portalLink.LastCrawledAt empty/omitted.
func lastCrawledByPortalLink() (map[string]string, error) {
	rows, err := jobsDB.Query(
		`SELECT portal_link_id, MAX(crawled_at) FROM jobs
		 WHERE portal_link_id IS NOT NULL
		 GROUP BY portal_link_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var linkID, lastCrawledAt string
		if err := rows.Scan(&linkID, &lastCrawledAt); err != nil {
			return nil, err
		}
		result[linkID] = lastCrawledAt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// listPortals returns every portal with its own links nested — same
// "list is enough, no standalone getter" reasoning listProfiles/
// listCompanies already use; a portal's own realistic link count is
// small.
func listPortals() ([]portal, error) {
	rows, err := jobsDB.Query(
		`SELECT id, name, created_at, updated_at FROM portals ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	portals := []portal{}
	for rows.Next() {
		var p portal
		var updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.UpdatedAt = updatedAt.String
		p.Links = []portalLink{}
		portals = append(portals, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	lastCrawled, err := lastCrawledByPortalLink()
	if err != nil {
		return nil, err
	}
	activeCrawlRuns, err := activeCrawlRunPortalLinkIDs()
	if err != nil {
		return nil, err
	}

	linkRows, err := jobsDB.Query(
		`SELECT id, portal_id, url, title, crawl_instructions, instructions_ai_error, instructions_ai_error_at, created_at, updated_at
		 FROM portal_links ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	byPortal := make(map[string][]portalLink, len(portals))
	for linkRows.Next() {
		var l portalLink
		var title, crawlInstructions, instructionsAIError, instructionsAIErrorAt, updatedAt sql.NullString
		if err := linkRows.Scan(&l.ID, &l.PortalID, &l.URL, &title, &crawlInstructions, &instructionsAIError, &instructionsAIErrorAt, &l.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		l.Title = title.String
		l.CrawlInstructions = crawlInstructions.String
		l.InstructionsAIError = instructionsAIError.String
		l.InstructionsAIErrorAt = instructionsAIErrorAt.String
		l.UpdatedAt = updatedAt.String
		l.LastCrawledAt = lastCrawled[l.ID]
		l.HasActiveCrawlRun = activeCrawlRuns[l.ID]
		byPortal[l.PortalID] = append(byPortal[l.PortalID], l)
	}
	if err := linkRows.Err(); err != nil {
		return nil, err
	}
	for i := range portals {
		if links, ok := byPortal[portals[i].ID]; ok {
			portals[i].Links = links
		}
	}
	return portals, nil
}

func updatePortal(id, name string) error {
	if err := requirePortalExists(id); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portals SET name = ?, updated_at = datetime('now') WHERE id = ?`,
		name, id,
	)
	return err
}

// deletePortal cascades (ON DELETE CASCADE, db.go's own schema) to
// every link it owns; step 20 makes this also un-link (never delete)
// any jobs those links produced. A benign no-op if id is unknown,
// matching every other delete-by-id operation in this tool.
func deletePortal(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM portals WHERE id = ?`, id)
	return err
}

func addPortalLink(portalId, url, title string) (string, error) {
	if err := requirePortalExists(portalId); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO portal_links (id, portal_id, url, title, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		id, portalId, url, title,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return "", errDuplicatePortalLink
		}
		return "", err
	}
	return id, nil
}

// updatePortalLink corrects a link's own url and/or title — both
// independently optional, partial-update style (matching
// updateCompany/updateRecruiter's own established shape). crawl_
// instructions (step 19) is deliberately not touched by this function
// — that column's own validated read/write path is step 19's own
// contribution, kept separate so this step can't accidentally store
// an unvalidated YAML document through a generic update path.
func updatePortalLink(id string, url, title *string) error {
	if err := requirePortalLinkExists(id); err != nil {
		return err
	}
	if url == nil && title == nil {
		return nil
	}

	setClauses := ""
	args := []any{}
	if url != nil {
		setClauses += "url = ?, "
		args = append(args, *url)
	}
	if title != nil {
		setClauses += "title = ?, "
		args = append(args, *title)
	}
	setClauses += "updated_at = datetime('now')"
	args = append(args, id)

	_, err := jobsDB.Exec(`UPDATE portal_links SET `+setClauses+` WHERE id = ?`, args...)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return errDuplicatePortalLink
	}
	return err
}

func removePortalLink(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM portal_links WHERE id = ?`, id)
	return err
}

// updatePortalLinkInstructionsAIStatus records (or clears) the outcome
// of the most recent AI-driven crawl-instructions generation/edit
// attempt for this link — errText nil or empty clears any previously
// recorded failure (a fresh attempt succeeded, or the caller is
// explicitly resetting it); non-empty records it, alongside the
// current timestamp. See
// plan/ai/tools/career/step-33-ai-generated-crawl-instructions.md.
func updatePortalLinkInstructionsAIStatus(id string, errText *string) error {
	if err := requirePortalLinkExists(id); err != nil {
		return err
	}
	if errText == nil || *errText == "" {
		_, err := jobsDB.Exec(`UPDATE portal_links SET instructions_ai_error = NULL, instructions_ai_error_at = NULL WHERE id = ?`, id)
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portal_links SET instructions_ai_error = ?, instructions_ai_error_at = datetime('now') WHERE id = ?`,
		*errText, id,
	)
	return err
}

// --- crawl instructions (step 19) ---

// crawlInstructionsField/crawlInstructionsPagination originally
// mirrored only the keys step 19's own shape check validated —
// Attribute/Multiple were tolerated but ignored, since only
// browser's own crawl_paginated ever consumed the full instruction.
// Step 27 changes that: this tool's own backend now also *runs* a
// crawl (deterministically, via buildCrawlRequest below), so it needs
// every field crawl_paginated itself needs, not just the two this
// step validates the presence of. See
// plan/ai/tools/career/step-27-ai-free-manual-crawl.md.
type crawlInstructionsField struct {
	Label     string `yaml:"label"`
	Selector  string `yaml:"selector"`
	Attribute string `yaml:"attribute,omitempty"`
	Multiple  bool   `yaml:"multiple,omitempty"`
}

type crawlInstructionsPagination struct {
	NextSelector string `yaml:"nextSelector"`
	MaxPages     int    `yaml:"maxPages"`
}

type crawlInstructionsDoc struct {
	// Container (step 30) is optional — a CSS selector for each
	// repeating item's own wrapping element (e.g. one job listing's
	// own <div>). When set, every field's own selector is evaluated
	// relative to each matched container instead of the whole page,
	// and ingestCrawlResults below consumes the resulting Items
	// directly instead of zipping parallel arrays by index. See
	// plan/ai/tools/career/step-30-grouped-crawl-ingestion.md and
	// plan/ai/tools/browser/step-18-grouped-container-extraction.md.
	Container string                   `yaml:"container,omitempty"`
	Fields    []crawlInstructionsField `yaml:"fields"`
	// Mapping (step 32) is optional {sourceLabel: targetKey} — renames
	// a field's own label to a specific output key before
	// ingestCrawlResults ever sees it (browser's own crawl_paginated
	// applies the rename, not this tool — see
	// plan/ai/tools/browser/step-20-output-field-mapping.md). Lets an
	// AI author fields under whatever labels are natural for a given
	// page (e.g. "job_title") while still satisfying the fixed
	// "title"/"url"/... vocabulary ingestCrawlResults reads by.
	// validateCrawlInstructions below checks the *effective* output
	// (label, or its mapping target when present) produces the
	// required keys. See
	// plan/ai/tools/career/step-32-required-schema-and-mapping.md.
	Mapping    map[string]string            `yaml:"mapping,omitempty"`
	Pagination *crawlInstructionsPagination `yaml:"pagination"`
}

// requiredCrawlOutputKeys mirrors ingestCrawlResults' own hard
// requirement exactly (title == "" || sourceURL == "" → skipped,
// company/location/description/postedAt all optional) — the single
// source of truth for what validateCrawlInstructions checks the
// effective (post-mapping) field labels against.
var requiredCrawlOutputKeys = []string{"title", "url"}

// errInvalidCrawlInstructions wraps a specific, actionable detail
// message (via fmt.Errorf's own %w) — a shape check, not a
// correctness check: this only confirms the required keys are present,
// never that the instruction actually works against a real page (that
// can only be proven by the AI itself later calling browser's own
// crawl_paginated). See
// plan/ai/tools/career/step-19-portal-crawl-instructions.md.
var errInvalidCrawlInstructions = errors.New("invalid crawl instructions")

func validateCrawlInstructions(yamlText string) error {
	var doc crawlInstructionsDoc
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return fmt.Errorf("%w: not valid YAML: %v", errInvalidCrawlInstructions, err)
	}
	if len(doc.Fields) == 0 {
		return fmt.Errorf("%w: fields must have at least one entry", errInvalidCrawlInstructions)
	}
	for i, f := range doc.Fields {
		if f.Label == "" {
			return fmt.Errorf("%w: fields[%d].label is required", errInvalidCrawlInstructions, i)
		}
		if f.Selector == "" {
			return fmt.Errorf("%w: fields[%d].selector is required", errInvalidCrawlInstructions, i)
		}
	}
	if doc.Pagination == nil {
		return fmt.Errorf("%w: pagination is required", errInvalidCrawlInstructions)
	}
	if doc.Pagination.NextSelector == "" {
		return fmt.Errorf("%w: pagination.nextSelector is required", errInvalidCrawlInstructions)
	}
	if doc.Pagination.MaxPages <= 0 {
		return fmt.Errorf("%w: pagination.maxPages is required", errInvalidCrawlInstructions)
	}
	if err := validateCrawlOutputSchema(doc); err != nil {
		return err
	}
	return nil
}

// validateCrawlOutputSchema checks that the *effective* output of these
// instructions — each field's own label, or its mapping target when
// doc.Mapping renames it — would actually produce every key in
// requiredCrawlOutputKeys. This is what closes the real gap step 32 was
// written for: previously, instructions with fields labeled e.g.
// "job_title"/"employer" (a perfectly reasonable, arguably better choice
// for a real page's own vocabulary) passed this function's shape checks
// cleanly, saved without error, and then silently produced zero saved
// jobs on every future deterministic crawl — ingestCrawlResults only
// ever reads the literal keys "title"/"url"/etc. and has no fallback.
// Still a shape check, not a correctness check, per this function's own
// sibling comment above: this only proves the *keys* would be right,
// never that the selectors actually match real content on the page. See
// plan/ai/tools/career/step-32-required-schema-and-mapping.md.
func validateCrawlOutputSchema(doc crawlInstructionsDoc) error {
	effective := make(map[string]bool, len(doc.Fields))
	for _, f := range doc.Fields {
		key := f.Label
		if target, ok := doc.Mapping[f.Label]; ok {
			key = target
		}
		effective[key] = true
	}
	for _, required := range requiredCrawlOutputKeys {
		if !effective[required] {
			return fmt.Errorf(
				"%w: no field produces the required %q key (add a field labeled %q, or add a mapping entry renaming one of your existing fields — e.g. mapping: { <your label>: %s })",
				errInvalidCrawlInstructions, required, required, required,
			)
		}
	}
	return nil
}

// getPortalLinkURL (step 37) reads just a link's own url — the
// detached crawl-now orchestration needs it to navigate browser's
// shared session, the one field listPortals' own per-link struct
// already exposes to the frontend but no standalone reader returned
// until now.
func getPortalLinkURL(id string) (string, error) {
	if err := requirePortalLinkExists(id); err != nil {
		return "", err
	}
	var url string
	if err := jobsDB.QueryRow(`SELECT url FROM portal_links WHERE id = ?`, id).Scan(&url); err != nil {
		return "", err
	}
	return url, nil
}

func getPortalLinkCrawlInstructions(id string) (string, error) {
	if err := requirePortalLinkExists(id); err != nil {
		return "", err
	}
	var crawlInstructions sql.NullString
	err := jobsDB.QueryRow(`SELECT crawl_instructions FROM portal_links WHERE id = ?`, id).Scan(&crawlInstructions)
	if err != nil {
		return "", err
	}
	return crawlInstructions.String, nil
}

// updatePortalLinkCrawlInstructions is the one shared persistent-layer
// function both the HTTP human-editing path (PUT /portal-links'
// own optional crawlInstructions field) and the AI-facing
// set_portal_link_crawl_instructions MCP tool call — validation lives
// here, once, so neither path can bypass it.
func updatePortalLinkCrawlInstructions(id, yamlText string) error {
	if err := requirePortalLinkExists(id); err != nil {
		return err
	}
	if err := validateCrawlInstructions(yamlText); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portal_links SET crawl_instructions = ?, updated_at = datetime('now') WHERE id = ?`,
		yamlText, id,
	)
	return err
}

// --- deterministic ("Crawl now") crawl support (step 27) ---

// errNoCrawlInstructions is a 400, not a 500 — this is a caller
// mistake (the UI's own "Crawl now" button is already gated on
// link.crawlInstructions being set), not a server failure.
var errNoCrawlInstructions = errors.New("this portal link has no crawl instructions set")

// maxAllowedPaginationPages mirrors browser's own identically-named
// constant (tools/browser/backend/paginate.go) exactly — that ceiling
// is normally only enforced by the AI-facing --mcp adapter in front of
// browser's own plain POST /crawl-paginated, which this tool's
// deterministic path calls directly, bypassing that adapter entirely.
// Duplicated (not imported — two fully separate Go modules/binaries,
// same reasoning as validateCrawlInstructions' own header comment)
// here so an oversized stored maxPages still can't run an unbounded
// crawl now that no AI-side safeguard is in the loop. See open
// question 3, plan/ai/tools/career/step-27-ai-free-manual-crawl.md.
const maxAllowedPaginationPages = 10

// crawlRequestField/crawlRequest are the exact JSON shape browser's
// own POST /crawl-paginated expects as its request body
// (tools/browser/backend/paginate.go's paginatedCrawlRequest) — this
// tool has no dependency on that package, so the shape is
// independently declared here, kept in sync by hand.
type crawlRequestField struct {
	Label     string `json:"label"`
	Selector  string `json:"selector"`
	Attribute string `json:"attribute,omitempty"`
	Multiple  bool   `json:"multiple,omitempty"`
}

type crawlRequest struct {
	Container         string              `json:"container,omitempty"`
	Fields            []crawlRequestField `json:"fields"`
	Mapping           map[string]string   `json:"mapping,omitempty"`
	NextSelector      string              `json:"nextSelector"`
	RequestedMaxPages int                 `json:"requestedMaxPages"`
	EffectiveMaxPages int                 `json:"effectiveMaxPages"`
}

// buildCrawlRequest turns a portal link's own stored crawl_instructions
// YAML into the JSON shape browser's own /crawl-paginated route
// expects — reusing validateCrawlInstructions' own parsing/shape
// check (defensive: normally already validated at write time, but this
// re-checks in case a row predates that check or was hand-edited)
// rather than re-implementing it. Returns errNoCrawlInstructions when
// the link has no crawl instructions at all — the deterministic
// "Crawl now" button has nothing to run in that case.
func buildCrawlRequest(id string) (crawlRequest, error) {
	yamlText, err := getPortalLinkCrawlInstructions(id)
	if err != nil {
		return crawlRequest{}, err
	}
	if yamlText == "" {
		return crawlRequest{}, errNoCrawlInstructions
	}
	if err := validateCrawlInstructions(yamlText); err != nil {
		return crawlRequest{}, err
	}

	var doc crawlInstructionsDoc
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		// Unreachable in practice — validateCrawlInstructions above
		// already parsed this same text successfully.
		return crawlRequest{}, fmt.Errorf("%w: %v", errInvalidCrawlInstructions, err)
	}

	requested := doc.Pagination.MaxPages
	effective := requested
	if effective > maxAllowedPaginationPages {
		effective = maxAllowedPaginationPages
	}

	fields := make([]crawlRequestField, len(doc.Fields))
	for i, f := range doc.Fields {
		fields[i] = crawlRequestField{Label: f.Label, Selector: f.Selector, Attribute: f.Attribute, Multiple: f.Multiple}
	}

	return crawlRequest{
		Container:         doc.Container,
		Fields:            fields,
		Mapping:           doc.Mapping,
		NextSelector:      doc.Pagination.NextSelector,
		RequestedMaxPages: requested,
		EffectiveMaxPages: effective,
	}, nil
}

// crawlResultPage mirrors browser's own pageExtractResult
// (tools/browser/backend/paginate.go) — one page's worth of extracted
// values, keyed by the field label the crawl instructions declared.
// Results[label] is either a single string (Multiple: false) or a
// []any of strings (Multiple: true) — extractResultStrings below
// normalizes either shape to a []string. Items (step 30) is populated
// instead of Results when the crawl instructions set a container —
// each entry is already one real job's own correctly-grouped fields;
// exactly one of Results/Items is ever non-empty, mirroring browser's
// own contract exactly.
type crawlResultPage struct {
	URL      string           `json:"url"`
	Results  map[string]any   `json:"results,omitempty"`
	Items    []map[string]any `json:"items,omitempty"`
	NotFound []string         `json:"notFound"`
}

// extractResultStrings normalizes one field's extracted value (a
// plain string for a non-multiple field, a []any for a multiple one,
// or absent) to a []string — the one shape ingestCrawlResults' own
// per-index zip below needs regardless of which the field was
// authored as.
func extractResultStrings(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// at returns s[i], or "" when i is out of range — lets
// ingestCrawlResults zip several independently-lengthed field arrays
// (a page listing 12 jobs might have 12 titles and 12 urls, but only
// 9 of them also matched a "location" selector) without a bounds
// check at every call site.
func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

type ingestCrawlResultsResult struct {
	JobsSaved   int `json:"jobsSaved"`
	JobsUpdated int `json:"jobsUpdated"`
	JobsSkipped int `json:"jobsSkipped"`
}

// ingestCrawlResults maps a completed deterministic crawl's own raw
// results (browser's own /crawl-paginated response body, forwarded
// here unmodified by the frontend) onto job rows via the fixed label
// vocabulary decided in step 27 (title/url required; company/
// location/description/postedAt optional, used verbatim with no date
// normalization or other AI-style interpretation) and persists each
// one through the exact same savePortalJob (step 20) the AI-based
// "Crawl with AI" path already uses — one shared insert path for both
// crawl mechanisms, only how a job's own fields were produced differs.
//
// A page listing many jobs (not just one detail page) is the common
// case for a real job board. Grouped pages (step 30, page.Items set —
// the crawl instructions declared a container) are handled first, per
// page: each item is already one real job's own correctly-grouped
// fields, no positional zipping needed at all — this is the actual
// fix for the misalignment flat mode could silently produce. A page
// with no Items (no container was set for this link, or an older
// crawl instruction predating step 30) falls back to flat mode
// unchanged: "title"/"url" are typically authored with Multiple:
// true, yielding one array per field rather than one scalar; this
// walks index 0..len(titles) (title is the required anchor field) and
// zips every other field positionally via at() — a title with no
// matching url at the same index is skipped (JobsSkipped), never
// inserted with an empty sourceUrl. Both branches count a save the
// same way flat mode always has: only `created` is ever checked
// (savePortalJob's own `duplicate` result is intentionally not
// tracked as a separate outcome here, matching the pre-existing
// behavior exactly — a duplicate is counted as JobsUpdated, same as a
// genuine update).
func ingestCrawlResults(portalLinkID string, pages []crawlResultPage) (ingestCrawlResultsResult, error) {
	if err := requirePortalLinkExists(portalLinkID); err != nil {
		return ingestCrawlResultsResult{}, err
	}

	var result ingestCrawlResultsResult
	for _, page := range pages {
		if len(page.Items) > 0 {
			for _, item := range page.Items {
				title := at(extractResultStrings(item["title"]), 0)
				sourceURL := at(extractResultStrings(item["url"]), 0)
				if title == "" || sourceURL == "" {
					result.JobsSkipped++
					continue
				}
				company := at(extractResultStrings(item["company"]), 0)
				location := at(extractResultStrings(item["location"]), 0)
				description := at(extractResultStrings(item["description"]), 0)
				postedAt := at(extractResultStrings(item["postedAt"]), 0)
				_, created, _, err := savePortalJob(portalLinkID, sourceURL, title, company, location, description, postedAt)
				if err != nil {
					return result, err
				}
				if created {
					result.JobsSaved++
				} else {
					result.JobsUpdated++
				}
			}
			continue
		}

		titles := extractResultStrings(page.Results["title"])
		urls := extractResultStrings(page.Results["url"])
		companies := extractResultStrings(page.Results["company"])
		locations := extractResultStrings(page.Results["location"])
		descriptions := extractResultStrings(page.Results["description"])
		postedAts := extractResultStrings(page.Results["postedAt"])

		for i, title := range titles {
			sourceURL := at(urls, i)
			if title == "" || sourceURL == "" {
				result.JobsSkipped++
				continue
			}
			_, created, _, err := savePortalJob(portalLinkID, sourceURL, title, at(companies, i), at(locations, i), at(descriptions, i), at(postedAts, i))
			if err != nil {
				return result, err
			}
			if created {
				result.JobsSaved++
			} else {
				result.JobsUpdated++
			}
		}
	}
	return result, nil
}

// --- MCP registration ---

type createPortalArgs struct {
	Name string `json:"name" jsonschema:"the portal's own name, e.g. the job board's name"`
}

func registerCreatePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_portal",
		Description: "Create a new job portal — a source to crawl for job postings. Only call this after " +
			"list_portals confirms no existing portal already matches what the user described (e.g. the same " +
			"job board under a slightly different name) — if one does, use that portal's own id instead of " +
			"creating a duplicate.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createPortalArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		id, err := createPortal(args.Name)
		if err != nil {
			return errResult(fmt.Sprintf("failed to create portal: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id, "name": args.Name})
	})
}

type listPortalsArgs struct{}

func registerListPortals(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_portals",
		Description: "List every job portal, including each one's own crawl target links (id, title, url, " +
			"crawlInstructions). Always call this first before create_portal, add_portal_link, or " +
			"set_portal_link_crawl_instructions — check whether a portal or link matching what the user " +
			"described already exists (by name/title, not just an exact URL match) before creating or asking " +
			"to create anything new.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listPortalsArgs) (*mcp.CallToolResult, any, error) {
		portals, err := listPortals()
		if err != nil {
			return errResult(fmt.Sprintf("failed to list portals: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"portals": portals})
	})
}

type updatePortalArgs struct {
	ID   string `json:"id" jsonschema:"the portal's own id, from create_portal or list_portals"`
	Name string `json:"name" jsonschema:"the portal's own new name"`
}

func registerUpdatePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_portal",
		Description: "Rename a job portal. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePortalArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		if err := updatePortal(args.ID, args.Name); err != nil {
			if errors.Is(err, errUnknownPortal) {
				return errResult(fmt.Sprintf("unknown portal id %q", args.ID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update portal: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deletePortalArgs struct {
	ID string `json:"id" jsonschema:"the portal's own id — deleting it also deletes every link it owns"`
}

func registerDeletePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_portal",
		Description: "Delete a job portal. This permanently deletes every crawl target link it owns.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deletePortalArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := deletePortal(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to delete portal: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

type addPortalLinkArgs struct {
	PortalID string `json:"portalId" jsonschema:"the portal to add this crawl target to — must already exist"`
	URL      string `json:"url" jsonschema:"the URL to crawl for job postings"`
	Title    string `json:"title" jsonschema:"a short, descriptive title for this link (e.g. 'Software Engineer jobs, Hamburg') so it stays recognizable later"`
}

func registerAddPortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_portal_link",
		Description: "Add a URL to crawl to an existing job portal, with a short, descriptive title (e.g. " +
			"'Software Engineer jobs, Hamburg') so it stays recognizable later — title is required. Call " +
			"list_portals first: if a link with a matching or very similar title/URL already exists under this " +
			"portal, do not add a duplicate — use update_portal_link or set_portal_link_crawl_instructions " +
			"against its existing id instead. If the user's request could reasonably match more than one " +
			"existing link and it isn't clear which one they mean, ask the user to clarify before adding or " +
			"editing anything — never guess.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addPortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalID == "" {
			return errResult("portalId is required"), nil, nil
		}
		if args.URL == "" {
			return errResult("url is required"), nil, nil
		}
		if args.Title == "" {
			return errResult("title is required"), nil, nil
		}
		id, err := addPortalLink(args.PortalID, args.URL, args.Title)
		if err != nil {
			switch {
			case errors.Is(err, errUnknownPortal):
				return errResult(fmt.Sprintf("unknown portal id %q", args.PortalID)), nil, nil
			case errors.Is(err, errDuplicatePortalLink):
				return errResult("this url is already a link on this portal"), nil, nil
			}
			return errResult(fmt.Sprintf("failed to add portal link: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})
}

type updatePortalLinkArgs struct {
	ID    string  `json:"id" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
	URL   *string `json:"url,omitempty" jsonschema:"the link's own new URL"`
	Title *string `json:"title,omitempty" jsonschema:"the link's own new title"`
}

func registerUpdatePortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_portal_link",
		Description: "Correct an existing portal link's own title and/or url. This is the right tool when a " +
			"link close to what's wanted already exists — use it instead of add_portal_link to avoid creating " +
			"a duplicate. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if args.URL == nil && args.Title == nil {
			return errResult("url or title is required"), nil, nil
		}
		if args.URL != nil && *args.URL == "" {
			return errResult("url cannot be empty"), nil, nil
		}
		if args.Title != nil && *args.Title == "" {
			return errResult("title cannot be empty"), nil, nil
		}
		if err := updatePortalLink(args.ID, args.URL, args.Title); err != nil {
			switch {
			case errors.Is(err, errUnknownPortalLink):
				return errResult(fmt.Sprintf("unknown portal link id %q", args.ID)), nil, nil
			case errors.Is(err, errDuplicatePortalLink):
				return errResult("this url is already a link on this portal"), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update portal link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type removePortalLinkArgs struct {
	ID string `json:"id" jsonschema:"the portal link's own id"`
}

func registerRemovePortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_portal_link",
		Description: "Remove a crawl target link from a portal.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removePortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := removePortalLink(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to remove portal link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

type getPortalLinkCrawlInstructionsArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
}

func registerGetPortalLinkCrawlInstructions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_portal_link_crawl_instructions",
		Description: "Read the currently saved YAML crawl instructions for one portal link (null if none saved yet).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getPortalLinkCrawlInstructionsArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return errResult("portalLinkId is required"), nil, nil
		}
		instructions, err := getPortalLinkCrawlInstructions(args.PortalLinkID)
		if err != nil {
			if errors.Is(err, errUnknownPortalLink) {
				return errResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to read crawl instructions: %v", err)), nil, nil
		}
		if instructions == "" {
			return jsonResult(map[string]any{"crawlInstructions": nil})
		}
		return jsonResult(map[string]any{"crawlInstructions": instructions})
	})
}

type setPortalLinkCrawlInstructionsArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
	Instructions string `json:"instructions" jsonschema:"YAML: fields (>=1 entry, each with label+selector), pagination (nextSelector+maxPages), and optionally container/mapping — the exact shape crawl_paginated expects. The effective output (a field's own label, or its mapping target) must include title and url, see this tool's own description for the full required schema."`
}

func registerSetPortalLinkCrawlInstructions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "set_portal_link_crawl_instructions",
		Description: "Save the YAML crawl instructions for one portal link — works equally on a link you just " +
			"created or one that already existed before this conversation; call list_portals first to find the " +
			"target link's own id (match by its title) if you don't already have it. " +
			"BEFORE WRITING fields, decide which kind of page this is: (1) a LISTING page — a search-results/" +
			"job-board/catalog page showing MULTIPLE similar items at once (this is the common case for a job " +
			"portal's own crawl target), or (2) a single item's own DETAIL page describing just one thing. " +
			"For a listing page, ALWAYS set a top-level container — a CSS selector for one item's own repeating " +
			"wrapping element (the <div>/<li>/<article> that repeats once per item on the page) — this is the " +
			"correct default for any repeating list, not an optional fix applied only after something looks " +
			"wrong. Every field's own selector is then evaluated relative to each matched container, and the " +
			"result is one correctly-grouped object per real item. Without container on a listing page, fields " +
			"describing multiple items are instead saved as separate same-length arrays whose positions may NOT " +
			"actually correspond to the same real item — silently pairing one item's title with a different " +
			"item's own url/company/etc. Only omit container for a genuine single-item detail page, where there " +
			"is nothing to group. The exact same shape crawl_paginated expects (fields: [...] + pagination: " +
			"{nextSelector, maxPages}, plus the optional top-level container), so it can be handed to that tool " +
			"directly later.\n\n" +
			"Required output schema (checked at save time, not just field shape): every field's own label — or its mapping target, see below — must, taken together, include title (required) and url (required); company, location, description, and postedAt are all optional and used verbatim with no reformatting. This is the exact vocabulary the deterministic \"Crawl now\" button reads by; if this page's own natural field names don't already match (e.g. \"job_title\" instead of \"title\"), add a top-level mapping block renaming them — the same mechanism crawl_paginated itself now supports — rather than forcing awkward labels onto your fields.\n\nExample — LISTING page (the common case):\ncontainer: \".job-result\"\nfields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\n  - label: company\n    selector: .company\npagination:\n  " +
			"nextSelector: a.next-page\n  maxPages: 5\n\n" +
			"Example — LISTING page with natural field names, mapped to the required schema:\ncontainer: \".job-result\"\nfields:\n  - label: job_title\n    selector: h2\n  - label: employer\n    selector: .company\nmapping:\n  job_title: title\n  employer: company\npagination:\n  " +
			"nextSelector: a.next-page\n  maxPages: 5\n\n" +
			"Example — single DETAIL page (no container needed):\nfields:\n  - label: title\n    selector: h1\npagination:\n  " +
			"nextSelector: a.next-page\n  maxPages: 5\n\n" +
			"Rejected if it doesn't parse, is missing required keys, or the effective output (after mapping) " +
			"wouldn't produce both title and url — this only checks shape, not that it actually works against " +
			"the real page.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setPortalLinkCrawlInstructionsArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return errResult("portalLinkId is required"), nil, nil
		}
		if args.Instructions == "" {
			return errResult("instructions is required"), nil, nil
		}
		if err := updatePortalLinkCrawlInstructions(args.PortalLinkID, args.Instructions); err != nil {
			if errors.Is(err, errUnknownPortalLink) {
				return errResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			// errInvalidCrawlInstructions and any other failure already
			// carry a specific, actionable message of their own.
			return errResult(err.Error()), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}
