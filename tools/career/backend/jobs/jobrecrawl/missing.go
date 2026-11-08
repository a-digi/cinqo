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
	// States says, for every job without a successful detail crawl, where it stands (the Details state constants below): the Jobs page marks
	// such a job and tells the person why its details are not there and whether they are coming. Never null.
	States map[string]string `json:"states"`
}

// Where a job without details stands (Missing.States). The first four say the job cannot be crawled yet and why; the rest say it can.
const (
	DetailsNoPortal       = "noPortal"
	DetailsNeedsLogin     = "needsLogin"
	DetailsNoInstructions = "noInstructions"
	// A crawl of this job is waiting in a recrawl batch, or running (in a batch, or as the detail crawl of its link).
	DetailsQueued  = "queued"
	DetailsRunning = "running"
	// Its link has a job-detail crawl requested (the pipeline starts it next).
	DetailsScheduled = "scheduled"
	// It can be crawled and nothing is scheduled: only the person's "Crawl missing details" gets it.
	DetailsReady = "ready"
)

// FindMissing lists the jobs whose detail crawl never succeeded (never tried, or the last try failed), and counts those that cannot be
// crawled with a reason.
func FindMissing() (Missing, error) {
	res := Missing{JobIDs: []string{}, States: map[string]string{}}
	rows, err := db.JobsDB.Query(
		`SELECT j.id, COALESCE(j.source_url, ''), pl.id, COALESCE(TRIM(pl.job_detail_crawl_instructions), ''), COALESCE(pl.requires_login, 0) = 1 AND NOT (COALESCE(pl.login_session_id, '') <> '' AND COALESCE(pl.login_session_state, '') <> 'expired'),
		        COALESCE(r.state, ''),
		        COALESCE(pl.job_detail_crawl_requested, 0) <> 0,
		        EXISTS (SELECT 1 FROM crawl_runs cr WHERE cr.portal_link_id = pl.id AND cr.kind = 'job_detail' AND cr.status = 'running')
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
		var requested, detailRunning bool
		if err := rows.Scan(&id, &url, &linkID, &instructions, &login, &state, &requested, &detailRunning); err != nil {
			return res, err
		}
		res.Total++
		switch {
		case url == "" || !linkID.Valid:
			res.NoPortal++
			res.States[id] = DetailsNoPortal
		case login != 0:
			res.NeedsLogin++
			res.States[id] = DetailsNeedsLogin
		case instructions == "":
			res.NoInstructions++
			res.States[id] = DetailsNoInstructions
		case state == StateQueued || state == StateRunning:
			res.AlreadyBeingCrawled++
			if state == StateRunning {
				res.States[id] = DetailsRunning
			} else {
				res.States[id] = DetailsQueued
			}
		default:
			res.JobIDs = append(res.JobIDs, id)
			switch {
			case detailRunning:
				res.States[id] = DetailsRunning
			case requested:
				res.States[id] = DetailsScheduled
			default:
				res.States[id] = DetailsReady
			}
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
