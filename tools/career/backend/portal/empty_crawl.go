// empty_crawl.go (step 90) answers, per portal link, "did the latest
// listing crawl find any jobs, and if not, what is known about why?" from
// the structured outcome crawl_runs now stores. The UI turns this into the
// orange "Crawled 0 jobs" flag and its diagnosis. See
// plan/ai/tools/career/step-90-zero-jobs-crawl-flag.md.
package portal

import (
	"encoding/json"
	"time"

	"career-tool-backend/db"
)

// LastListingCrawl is one link's latest COMPLETED listing crawl that has
// structured data, plus the context needed to interpret a zero-job
// result. Absent from the list response for a link whose latest completed
// listing run has none (never crawled, or only runs from before step 90).
type LastListingCrawl struct {
	FinishedAt     string   `json:"finishedAt"`
	TriggeredBy    string   `json:"triggeredBy"`
	JobsSaved      int      `json:"jobsSaved"`
	JobsUpdated    int      `json:"jobsUpdated"`
	JobsSkipped    int      `json:"jobsSkipped"`
	PagesVisited   int      `json:"pagesVisited"`
	StoppedReason  string   `json:"stoppedReason"`
	NotFoundFields []string `json:"notFoundFields"`
	// ZeroStreak is how many consecutive latest completed listing runs
	// (with data) saved or updated nothing; 0 when the latest run found
	// jobs.
	ZeroStreak int `json:"zeroStreak"`
	// LastJobsAt is when the most recent run that did save or update a
	// job finished; empty when no run ever did.
	LastJobsAt string `json:"lastJobsAt,omitempty"`
	// JobsFromThisLink is how many jobs this link has ever produced
	// (jobs.portal_link_id) — the best available evidence that a link
	// which now returns nothing used to work.
	JobsFromThisLink int `json:"jobsFromThisLink"`
	// AcknowledgedAt is when the user marked the current zero streak as
	// expected; the UI treats it as still in force only while it is later
	// than LastJobsAt.
	AcknowledgedAt string `json:"acknowledgedAt,omitempty"`
}

// LastListingCrawlByLink returns the summary for every link that has one,
// keyed by portal link id, in a fixed number of queries (never one per
// link). Runs are ordered by rowid — insert order — which is also finish
// order, since a link never has two runs going at once.
func LastListingCrawlByLink() (map[string]LastListingCrawl, error) {
	out := map[string]LastListingCrawl{}

	// 1. The latest completed listing run per link, joined with the
	// link's acknowledgement.
	rows, err := db.JobsDB.Query(
		`SELECT r.portal_link_id, COALESCE(r.finished_at, ''), r.triggered_by,
		        r.jobs_saved, r.jobs_updated, r.jobs_skipped, r.pages_visited,
		        COALESCE(r.stopped_reason, ''), COALESCE(r.not_found_fields, '[]'),
		        COALESCE(l.empty_crawl_acknowledged_at, '')
		 FROM crawl_runs r
		 JOIN portal_links l ON l.id = r.portal_link_id
		 WHERE r.rowid = (
		   SELECT MAX(x.rowid) FROM crawl_runs x
		   WHERE x.portal_link_id = r.portal_link_id AND x.kind = 'listing' AND x.status = 'completed')`,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var linkID string
		var c LastListingCrawl
		var saved, updated, skipped, pages *int
		var notFound string
		if err := rows.Scan(&linkID, &c.FinishedAt, &c.TriggeredBy, &saved, &updated, &skipped, &pages, &c.StoppedReason, &notFound, &c.AcknowledgedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if saved == nil { // a run from before step 90: no data, so no entry
			continue
		}
		c.JobsSaved, c.JobsUpdated = *saved, valueOrZero(updated)
		c.JobsSkipped, c.PagesVisited = valueOrZero(skipped), valueOrZero(pages)
		if err := json.Unmarshal([]byte(notFound), &c.NotFoundFields); err != nil || c.NotFoundFields == nil {
			c.NotFoundFields = []string{}
		}
		out[linkID] = c
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close() // jobs.db is a single connection: close before the next query

	if len(out) == 0 {
		return out, nil
	}

	// 2. When did a run last find a job, and how many zero runs followed
	// it. Both only consider runs that have data.
	lastJobs, err := scanStringByLink(
		`SELECT r.portal_link_id, COALESCE(r.finished_at, '')
		 FROM crawl_runs r
		 WHERE r.kind = 'listing' AND r.status = 'completed' AND r.jobs_saved IS NOT NULL
		   AND (r.jobs_saved + COALESCE(r.jobs_updated, 0)) > 0
		   AND r.rowid = (
		     SELECT MAX(x.rowid) FROM crawl_runs x
		     WHERE x.portal_link_id = r.portal_link_id AND x.kind = 'listing' AND x.status = 'completed'
		       AND x.jobs_saved IS NOT NULL AND (x.jobs_saved + COALESCE(x.jobs_updated, 0)) > 0)`,
	)
	if err != nil {
		return nil, err
	}
	streaks, err := scanIntByLink(
		`SELECT z.portal_link_id, COUNT(*)
		 FROM crawl_runs z
		 WHERE z.kind = 'listing' AND z.status = 'completed' AND z.jobs_saved IS NOT NULL
		   AND (z.jobs_saved + COALESCE(z.jobs_updated, 0)) = 0
		   AND z.rowid > COALESCE((
		     SELECT MAX(x.rowid) FROM crawl_runs x
		     WHERE x.portal_link_id = z.portal_link_id AND x.kind = 'listing' AND x.status = 'completed'
		       AND x.jobs_saved IS NOT NULL AND (x.jobs_saved + COALESCE(x.jobs_updated, 0)) > 0), 0)
		 GROUP BY z.portal_link_id`,
	)
	if err != nil {
		return nil, err
	}
	jobCounts, err := scanIntByLink(`SELECT portal_link_id, COUNT(*) FROM jobs WHERE portal_link_id IS NOT NULL GROUP BY portal_link_id`)
	if err != nil {
		return nil, err
	}

	for id, c := range out {
		c.LastJobsAt = lastJobs[id]
		c.ZeroStreak = streaks[id]
		c.JobsFromThisLink = jobCounts[id]
		out[id] = c
	}
	return out, nil
}

func valueOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func scanStringByLink(query string) (map[string]string, error) {
	rows, err := db.JobsDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, v string
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, rows.Err()
}

func scanIntByLink(query string) (map[string]int, error) {
	rows, err := db.JobsDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var v int
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, rows.Err()
}

// SetEmptyCrawlAcknowledged marks (true) or clears (false) the link's
// current run of zero-job crawls as "expected, this page has no jobs".
// The timestamp is RFC3339 UTC, the same format crawl_runs.finished_at
// uses, so the two can be compared directly.
func SetEmptyCrawlAcknowledged(id string, acknowledged bool) error {
	if err := RequirePortalLinkExists(id); err != nil {
		return err
	}
	if !acknowledged {
		_, err := db.JobsDB.Exec(`UPDATE portal_links SET empty_crawl_acknowledged_at = NULL WHERE id = ?`, id)
		return err
	}
	_, err := db.JobsDB.Exec(`UPDATE portal_links SET empty_crawl_acknowledged_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}
