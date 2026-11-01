package crawlcriteria

import (
	"log"
	"net/http"
	"strconv"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// KeptIgnored is how many ignored jobs the log keeps (the newest); older ones are pruned when a new one is recorded.
const KeptIgnored = 200

// RecordIgnored writes a job a matcher refused into the log (jobs.UseFilteredOutLog injects it): one row per posting URL, a job refused
// again by the next crawl updates its row, and only the newest KeptIgnored rows stay. A failure is logged and never fails the crawl.
func RecordIgnored(c jobs.Candidate, refused *jobs.FilteredOutError) {
	if _, err := db.JobsDB.Exec(
		`INSERT INTO crawler_ignored_jobs (source_url, title, company, location, portal_link_id, matcher, reason, ignored_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_url) DO UPDATE SET title = excluded.title, company = excluded.company, location = excluded.location,
		   portal_link_id = excluded.portal_link_id, matcher = excluded.matcher, reason = excluded.reason, ignored_at = excluded.ignored_at`,
		c.SourceURL, c.Title, c.Company, c.Location, c.PortalLinkID, refused.Matcher, refused.Reason, now().UTC().Format(timeFormat),
	); err != nil {
		log.Printf("crawler criteria: could not log the ignored job %q: %v", c.Title, err)
		return
	}
	if _, err := db.JobsDB.Exec(
		`DELETE FROM crawler_ignored_jobs WHERE id NOT IN (SELECT id FROM crawler_ignored_jobs ORDER BY ignored_at DESC, id DESC LIMIT ?)`, KeptIgnored,
	); err != nil {
		log.Printf("crawler criteria: could not prune the log of ignored jobs: %v", err)
	}
}

// IgnoredJob is one row of the log, with the name of the portal the job came from when it is still known.
type IgnoredJob struct {
	ID         int64  `json:"id"`
	SourceURL  string `json:"sourceUrl"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	Location   string `json:"location"`
	PortalName string `json:"portalName"`
	Matcher    string `json:"matcher"`
	Reason     string `json:"reason"`
	IgnoredAt  string `json:"ignoredAt"`
}

// IgnoredList is the log: the newest `limit` rows and how many rows there are in all.
type IgnoredList struct {
	Items []IgnoredJob `json:"items"`
	Total int          `json:"total"`
}

// ListIgnored reads the newest `limit` ignored jobs, newest first.
func ListIgnored(limit int) (IgnoredList, error) {
	out := IgnoredList{Items: []IgnoredJob{}}
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM crawler_ignored_jobs`).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := db.JobsDB.Query(
		`SELECT i.id, i.source_url, i.title, i.company, i.location, COALESCE(p.name, ''), i.matcher, i.reason, i.ignored_at
		 FROM crawler_ignored_jobs i
		 LEFT JOIN portal_links pl ON pl.id = i.portal_link_id
		 LEFT JOIN portals p ON p.id = pl.portal_id
		 ORDER BY i.ignored_at DESC, i.id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var j IgnoredJob
		if err := rows.Scan(&j.ID, &j.SourceURL, &j.Title, &j.Company, &j.Location, &j.PortalName, &j.Matcher, &j.Reason, &j.IgnoredAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, j)
	}
	return out, rows.Err()
}

// IgnoredHandler handles GET /settings/crawler/ignored?limit=: the log of ignored jobs, newest first (limit 1 to KeptIgnored, 50 when
// absent; a number outside the range is clamped, a value that is not a number is a 400).
func IgnoredHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "limit must be a whole number", http.StatusBadRequest)
			return
		}
		limit = max(1, min(n, KeptIgnored))
	}
	list, err := ListIgnored(limit)
	if err != nil {
		http.Error(w, "failed to read the ignored jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, list)
}
