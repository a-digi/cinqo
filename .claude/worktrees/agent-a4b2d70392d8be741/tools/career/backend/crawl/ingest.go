// ingest.go turns a completed deterministic crawl's own raw results —
// browser's own /crawl-paginated response body — into saved job rows.
//
// Lives in this package, not portals (where it originally sat beside
// the crawl-instruction code that builds the request), because
// ingesting means SAVING JOBS: it calls jobs.SavePortalJob, while the
// jobs package itself already depends on portals for its own
// portal-link lookups. Keeping it in portals would have made those two
// packages import each other. This package is the one that already
// imports both, and it is where both callers live: the HTTP handler
// below, for the frontend-orchestrated flow, and runCrawlNow
// (crawl_now.go), for the detached backend-orchestrated one.
package crawl

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/jobs"
	"career-tool-backend/portals"
	"career-tool-backend/shared"
)

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
// one through the exact same jobs.SavePortalJob (step 20) the AI-based
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
// (jobs.SavePortalJob's own `duplicate` result is intentionally not
// tracked as a separate outcome here, matching the pre-existing
// behavior exactly — a duplicate is counted as JobsUpdated, same as a
// genuine update).
func ingestCrawlResults(portalLinkID string, pages []crawlResultPage) (ingestCrawlResultsResult, error) {
	if err := portals.RequirePortalLinkExists(portalLinkID); err != nil {
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
				_, created, _, err := jobs.SavePortalJob(portalLinkID, sourceURL, title, company, location, description, postedAt)
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
			_, created, _, err := jobs.SavePortalJob(portalLinkID, sourceURL, title, at(companies, i), at(locations, i), at(descriptions, i), at(postedAts, i))
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

// --- POST /portal-links/ingest-crawl-results ---

type ingestCrawlResultsRequest struct {
	PortalLinkID string            `json:"portalLinkId"`
	Pages        []crawlResultPage `json:"pages"`
}

// IngestCrawlResultsHandler handles POST /portal-links/ingest-crawl-
// results — the frontend-orchestrated deterministic flow's last step:
// the frontend forwards browser's own /crawl-paginated response body
// (plus which portal link it was crawling) here unmodified, and this
// tool maps it onto job rows via ingestCrawlResults' own fixed label
// vocabulary. That flow's FIRST step, GET /portal-links/crawl-request,
// is portals.CrawlRequestHandler.
func IngestCrawlResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body ingestCrawlResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	result, err := ingestCrawlResults(body.PortalLinkID, body.Pages)
	if err != nil {
		portals.WritePortalLinkAwareError(w, "ingest crawl results", err)
		return
	}
	shared.WriteJSON(w, result)
}
