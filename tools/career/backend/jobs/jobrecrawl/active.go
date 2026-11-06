package jobrecrawl

import (
	"database/sql"
	"net/http"

	"career-tool-backend/db"
)

// What the job recrawl is doing right now, for the Crawl Monitor (plan 194): the batches that still have work and one row for each job that
// is being crawled at this moment. A recrawl has no crawl_runs row, so the monitor's own list never showed it; this reads the requests.

// ActiveBatch is a batch with something still waiting or being crawled.
type ActiveBatch struct {
	BatchID   string `json:"batchId"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Queued    int    `json:"queued"`
	Running   int    `json:"running"`
	Cancelled int    `json:"cancelled"`
	StartedAt string `json:"startedAt"`
	// Stopping: Stop was clicked, nothing waits any more and some crawls are still being finished.
	Stopping bool `json:"stopping"`
}

// ActiveWorker is a job that is being crawled now.
type ActiveWorker struct {
	JobID           string `json:"jobId"`
	JobTitle        string `json:"jobTitle"`
	PortalID        string `json:"portalId,omitempty"`
	PortalName      string `json:"portalName,omitempty"`
	PortalLinkID    string `json:"portalLinkId,omitempty"`
	PortalLinkTitle string `json:"portalLinkTitle,omitempty"`
	URL             string `json:"url"`
	// Slot is the crawler that took it: 0 is the main crawler, 1 to 4 the helpers. -1 when the row says nothing (an old row).
	Slot      int    `json:"slot"`
	StartedAt string `json:"startedAt"`
	// Phase and Message are the Browser tool's own words for what the page load is doing; empty while it has said nothing (it says nothing
	// while it waits for the platform's turn).
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// Active is the answer of GET jobs/recrawl/active. Both lists are empty, never null, when nothing runs.
type Active struct {
	Batches []ActiveBatch  `json:"batches"`
	Workers []ActiveWorker `json:"workers"`
}

// FindActive reads the batches with work left and the jobs being crawled now (oldest started first).
func FindActive() (Active, error) {
	res := Active{Batches: []ActiveBatch{}, Workers: []ActiveWorker{}}

	rows, err := db.JobsDB.Query(
		`SELECT batch_id, COUNT(*),
		        SUM(state = 'done'), SUM(state = 'queued'), SUM(state = 'running'), SUM(state = 'cancelled'), MIN(requested_at)
		 FROM job_recrawl_requests
		 GROUP BY batch_id
		 HAVING SUM(state IN ('queued', 'running')) > 0
		 ORDER BY MIN(requested_at) ASC, batch_id ASC`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var b ActiveBatch
		if err := rows.Scan(&b.BatchID, &b.Total, &b.Done, &b.Queued, &b.Running, &b.Cancelled, &b.StartedAt); err != nil {
			rows.Close()
			return res, err
		}
		b.Stopping = b.Queued == 0 && b.Running > 0 && b.Cancelled > 0
		res.Batches = append(res.Batches, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close() // one connection only: the next read starts after this result set is closed

	wrows, err := db.JobsDB.Query(
		`SELECT r.job_id, COALESCE(j.title, ''), COALESCE(j.source_url, ''),
		        COALESCE(p.id, ''), COALESCE(p.name, ''), COALESCE(pl.id, ''), COALESCE(pl.title, ''),
		        r.worker_slot, COALESCE(r.started_at, r.requested_at), COALESCE(r.phase, ''), COALESCE(r.phase_message, '')
		 FROM job_recrawl_requests r
		 JOIN jobs j ON j.id = r.job_id
		 LEFT JOIN portal_links pl ON pl.id = j.portal_link_id
		 LEFT JOIN portals p ON p.id = pl.portal_id
		 WHERE r.state = 'running'
		 ORDER BY COALESCE(r.started_at, r.requested_at) ASC, r.job_id ASC`)
	if err != nil {
		return res, err
	}
	defer wrows.Close()
	for wrows.Next() {
		var w ActiveWorker
		var slot sql.NullInt64
		if err := wrows.Scan(&w.JobID, &w.JobTitle, &w.URL, &w.PortalID, &w.PortalName, &w.PortalLinkID, &w.PortalLinkTitle, &slot, &w.StartedAt, &w.Phase, &w.Message); err != nil {
			return res, err
		}
		w.Slot = -1
		if slot.Valid {
			w.Slot = int(slot.Int64)
		}
		res.Workers = append(res.Workers, w)
	}
	return res, wrows.Err()
}

// ActiveHandler handles GET jobs/recrawl/active.
func ActiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := FindActive()
	if err != nil {
		http.Error(w, "failed to read the recrawl: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	db.WriteJSON(w, res)
}
