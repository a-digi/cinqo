package crawl

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"career-tool-backend/jobs"
	"career-tool-backend/portal"
)

// NotPossibleError says a job cannot be recrawled at all, as opposed to a crawl that was tried and failed: it has no portal to
// read it with, or the portal's job detail crawl instructions are missing or invalid.
type NotPossibleError struct{ Reason string }

func (e *NotPossibleError) Error() string { return e.Reason }

// RecrawlOneJob crawls one job's own page with the job detail crawl instructions of its portal link and saves what it finds
// through jobs.SaveJobDetailExtraction, the very work "Crawl job details now" does for each job of a link (crawlOneJobDetail),
// but for a single job and without a crawl_runs row (step 148). token is a user session token for the Browser tool's proxy.
//
// saved is true when a description was found and saved; false when the page gave none (the job is then marked as a failed detail
// crawl and its old text is kept). An error is a crawl or save that failed. A *NotPossibleError says there was nothing to crawl it
// with.
func RecrawlOneJob(ctx context.Context, t jobs.RecrawlTarget, token string) (saved bool, err error) {
	if t.PortalLinkID == "" || t.SourceURL == "" {
		return false, &NotPossibleError{"no portal to crawl it from"}
	}
	req, err := portal.BuildJobDetailCrawlRequest(t.PortalLinkID)
	switch {
	case errors.Is(err, portal.ErrNoJobDetailCrawlInstructions):
		return false, &NotPossibleError{"this portal has no job detail crawl instructions"}
	case errors.Is(err, portal.ErrUnknownPortalLink):
		return false, &NotPossibleError{"the portal link no longer exists"}
	case errors.Is(err, portal.ErrInvalidCrawlInstructions):
		return false, &NotPossibleError{fmt.Sprintf("the portal's job detail crawl instructions are not valid: %v", err)}
	case err != nil:
		return false, err
	}
	portalID, err := portal.GetPortalIDForLink(t.PortalLinkID)
	if err != nil {
		return false, err
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		return false, err
	}
	// A request id of its own: the Browser tool tracks and can cancel a call by it. There is no crawl_runs row under it, so the
	// phase updates the poller writes find nothing to update.
	requestID := "recrawl-" + uuid.NewString()
	return crawlOneJobDetail(ctx, coreURL, portalID, requestID, token, req, jobs.JobDetailCrawlTarget{ID: t.ID, SourceURL: t.SourceURL, Title: t.Title})
}
