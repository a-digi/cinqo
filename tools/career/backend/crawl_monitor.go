// crawl_monitor.go is the centralized "which portal is being crawled
// right now" view — a single endpoint aggregating every currently-
// running crawl_runs row (of either kind, see db.go's own
// crawl_runs.kind doc comment) across EVERY portal link, joined with
// its own owning portal link and portal, so a human doesn't have to
// open each portal's own accordion individually to find out what's
// active. Read-only; no new data model — a plain join over
// crawl_runs/portal_links/portals, all already existing. See
// plan/ai/tools/career/step-XX-portal-crawl-pacing.md.
package main

import (
	"database/sql"
	"net/http"
)

// activeCrawlRunSummary is one row of the crawl-monitor response — a
// crawlRun plus the portal link/portal context needed to identify it
// without a separate round trip per row.
type activeCrawlRunSummary struct {
	CrawlRunID      string   `json:"crawlRunId"`
	Kind            string   `json:"kind"`
	PortalID        string   `json:"portalId"`
	PortalName      string   `json:"portalName"`
	PortalLinkID    string   `json:"portalLinkId"`
	PortalLinkTitle string   `json:"portalLinkTitle,omitempty"`
	PortalLinkURL   string   `json:"portalLinkUrl"`
	Status          string   `json:"status"`
	StartedAt       string   `json:"startedAt"`
	Phase           *string  `json:"phase"`
	Log             []string `json:"log"`
}

// listActiveCrawlRuns returns every currently-running crawl_runs row,
// oldest-started first, joined with its own owning portal link and
// portal. A crawl_runs row always references a real portal_links row
// (ON DELETE CASCADE, db.go) which in turn always references a real
// portals row, so both joins here are plain JOINs, never LEFT JOINs —
// there is no dangling-reference case to handle.
func listActiveCrawlRuns() ([]activeCrawlRunSummary, error) {
	rows, err := jobsDB.Query(
		`SELECT cr.id, cr.kind, p.id, p.name, pl.id, pl.title, pl.url, cr.status, cr.started_at, cr.phase, cr.log
		 FROM crawl_runs cr
		 JOIN portal_links pl ON pl.id = cr.portal_link_id
		 JOIN portals p ON p.id = pl.portal_id
		 WHERE cr.status = 'running'
		 ORDER BY cr.started_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []activeCrawlRunSummary{}
	for rows.Next() {
		var s activeCrawlRunSummary
		var title, phase sql.NullString
		var log string
		if err := rows.Scan(&s.CrawlRunID, &s.Kind, &s.PortalID, &s.PortalName, &s.PortalLinkID, &title, &s.PortalLinkURL, &s.Status, &s.StartedAt, &phase, &log); err != nil {
			return nil, err
		}
		s.PortalLinkTitle = title.String
		if phase.Valid {
			s.Phase = &phase.String
		}
		s.Log = splitCrawlRunLog(log)
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// crawlMonitorHandler handles GET /portal-links/crawl-runs/active.
func crawlMonitorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runs, err := listActiveCrawlRuns()
	if err != nil {
		http.Error(w, "failed to list active crawl runs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"runs": runs})
}
