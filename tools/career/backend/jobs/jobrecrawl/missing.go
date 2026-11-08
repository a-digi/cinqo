package jobrecrawl

import (
	"database/sql"
	"net/http"

	"career-tool-backend/db"
)

// "Crawl missing details" (the Jobs page button): the jobs whose detail page was never crawled successfully, and which of them can be
// crawled at all. The crawling itself is the recrawl batch (Request, the event per job, the consumer): this only names the jobs. Nothing here
// looks at crawl_runs: a crawl that is running does not matter, the button works whatever else runs.

// Missing is what the Jobs page needs to label and run the button.
type Missing struct {
	// JobIDs are the jobs that can be crawled now, oldest crawled first: a posting address, a portal link with job detail instructions,
	// no login needed.
	JobIDs []string `json:"jobIds"`
	// Total is every job without a successful detail crawl; the rest are told apart by why they cannot be crawled.
	Total               int `json:"total"`
	NoPortal            int `json:"noPortal"`
	NoInstructions      int `json:"noInstructions"`
	NeedsLogin          int `json:"needsLogin"`
	AlreadyBeingCrawled int `json:"alreadyBeingCrawled"`
}

// FindMissing lists the jobs whose detail crawl never succeeded (never tried, or the last try failed), and counts those that cannot be
// crawled with a reason.
func FindMissing() (Missing, error) {
	res := Missing{JobIDs: []string{}}
	rows, err := db.JobsDB.Query(
		`SELECT j.id, COALESCE(j.source_url, ''), pl.id, COALESCE(TRIM(pl.job_detail_crawl_instructions), ''), COALESCE(pl.requires_login, 0) = 1 AND NOT (COALESCE(pl.login_session_id, '') <> '' AND COALESCE(pl.login_session_state, '') <> 'expired'),
		        COALESCE(r.state, '')
		 FROM jobs j
		 LEFT JOIN portal_links pl ON pl.id = j.portal_link_id
		 LEFT JOIN job_recrawl_requests r ON r.job_id = j.id
		 WHERE j.detail_crawl_status IS NULL OR j.detail_crawl_status <> 'success'
		 ORDER BY j.crawled_at ASC, j.rowid ASC`)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, url, instructions, state string
		var linkID sql.NullString
		var login int
		if err := rows.Scan(&id, &url, &linkID, &instructions, &login, &state); err != nil {
			return res, err
		}
		res.Total++
		switch {
		case url == "" || !linkID.Valid:
			res.NoPortal++
		case login != 0:
			res.NeedsLogin++
		case instructions == "":
			res.NoInstructions++
		case state == StateQueued || state == StateRunning:
			res.AlreadyBeingCrawled++
		default:
			res.JobIDs = append(res.JobIDs, id)
		}
	}
	return res, rows.Err()
}

// MissingHandler handles GET jobs/recrawl/missing.
func MissingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := FindMissing()
	if err != nil {
		http.Error(w, "failed to look for jobs without details: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, res)
}
