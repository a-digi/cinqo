package service

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"career-tool-backend/crawl"
	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// The crawl operation (plan 192, the user's request: the job list AND the job details are instructed by the AI in ONE conversation). The AI
// saved the rule; the job details can only be picked on a crawled job; so the AI crawls the jobs itself and waits, then goes on in the same
// conversation. The crawl is the screen's "Crawl now" (the same function, the same checks), run in this long-lived process.

const (
	crawlWait     = 75 * time.Second // below the service's own timeout; a longer crawl is waited for by calling the tool again
	crawlPollEach = time.Second
)

type crawlBody struct {
	PortalLinkID string `json:"portalLinkId"`
	// Again crawls even when the link already has jobs.
	Again bool `json:"again,omitempty"`
}

type crawlAnswer struct {
	Status  string `json:"status"` // completed | failed | running | already_crawled
	Jobs    int    `json:"jobs"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
	Next    string `json:"next"`
}

func jobsOfLink(linkID string) (int, error) {
	var n int
	err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE portal_link_id = ?`, linkID).Scan(&n)
	return n, err
}

// latestListingRun reads the newest listing crawl of the link.
func latestListingRun(linkID string) (status, summary, errText string, err error) {
	var sum, msg sql.NullString
	err = db.JobsDB.QueryRow(
		`SELECT status, result_summary, error_message FROM crawl_runs WHERE portal_link_id = ? AND kind = 'listing' ORDER BY started_at DESC, rowid DESC LIMIT 1`, linkID,
	).Scan(&status, &sum, &msg)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	return status, sum.String, msg.String, err
}

// CrawlJobs crawls the link's job list (unless it already has jobs) and waits for the crawl to end.
func CrawlJobs(ctx context.Context, b crawlBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	settings, err := portal.GetLinkCrawlSettings(s.linkID)
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	if settings.Rules == nil {
		return nil, refuse(http.StatusConflict, "no_rule", "this link has no job list rule yet: write and save it first (katana_save_link_rule)")
	}
	jobs, err := jobsOfLink(s.linkID)
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	sc := getScratch(s.linkID)
	status, _, _, _ := latestListingRun(s.linkID)
	if jobs > 0 && !b.Again && !sc.needCrawl && status != "running" {
		return crawlAnswer{Status: "already_crawled", Jobs: jobs, Next: "the jobs exist: go on with katana_get_job_page_candidates"}, nil
	}

	if status != "running" {
		done := make(chan error, 1)
		go func() {
			_, err := crawl.RunListingCrawl(s.linkID, s.token)
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil && !crawl.IsCrawlAlreadyRunning(err) {
				return nil, refuse(http.StatusConflict, "crawl_refused", "the crawl could not start: %s", clip(err.Error(), 200))
			}
		case <-time.After(crawlPollEach):
			// the crawl runs on; it is waited for below
		case <-ctx.Done():
			return nil, refuse(http.StatusGatewayTimeout, "cancelled", "the request was cancelled")
		}
	}

	deadline := time.Now().Add(crawlWait)
	for {
		st, sum, msg, err := latestListingRun(s.linkID)
		if err != nil {
			return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
		}
		if st != "running" {
			jobs, _ = jobsOfLink(s.linkID)
			// the crawl the AI asked for makes the event's own request for a crawl redundant
			_, _ = db.JobsDB.Exec(`UPDATE portal_links SET listing_crawl_requested = 0 WHERE id = ?`, s.linkID)
			switch st {
			case "completed":
				sc.needCrawl = false
				a := crawlAnswer{Status: "completed", Jobs: jobs, Summary: clip(sum, 200)}
				if jobs == 0 {
					a.Next = "the crawl found no jobs: the rule does not work. STOP and tell the person (they can use Start over); do not pick job details"
				} else {
					a.Next = "the jobs are crawled: go on with katana_get_job_page_candidates"
				}
				return a, nil
			default:
				return crawlAnswer{Status: "failed", Jobs: jobs, Error: clip(msg, 200), Next: "the crawl failed: STOP and tell the person what it said; do not pick job details"}, nil
			}
		}
		if time.Now().After(deadline) {
			return crawlAnswer{Status: "running", Jobs: jobs, Next: "the crawl is still running: call katana_crawl_jobs again to keep waiting (at most twice), do nothing else"}, nil
		}
		select {
		case <-time.After(crawlPollEach):
		case <-ctx.Done():
			return nil, refuse(http.StatusGatewayTimeout, "cancelled", "the request was cancelled")
		}
	}
}
