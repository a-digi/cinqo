package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"career-tool-backend/portal"
)

// inherit_trial.go runs the trial of "a new link first tries the portal's existing
// instructions" (step 103): the candidates the portal package picks are tried, one
// after the other, against the NEW link's page and one or two of its jobs, through the
// same Browser proxy routes "Crawl now" uses and with the caller's own session token.
// It only reads and reports: nothing is saved (no jobs, no instructions, no crawl run),
// so a trial that fails leaves no trace. The caller (the endpoint) saves what the result
// says to copy. See plan/ai/tools/career/step-103-inherit-portal-instructions-for-new-links.md.

// Statuses of the list part of one candidate's trial (fixed words, also in the API).
const (
	ListingOK     = "ok"
	ListingFailed = "failed" // the instructions did not work on this page
	ListingError  = "error"  // the trial could not run (browser, Cloudflare, session, network)
)

// inheritTrialTimeout bounds the whole trial. A var, so a test can shorten it.
var inheritTrialTimeout = 3 * time.Minute

// maxReasonRunes keeps a reason short enough to show in a list.
const maxReasonRunes = 200

// CandidateOutcome is what happened with one candidate.
type CandidateOutcome struct {
	LinkID  string `json:"linkId"`
	Title   string `json:"title"`
	Listing string `json:"listing"`
	// JobDetail is one of the portal.Detail* words; empty when the list did not pass
	// (the details are only tried after a list that worked).
	JobDetail string `json:"jobDetail,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// InheritTrialResult is the outcome of a whole trial.
type InheritTrialResult struct {
	// Tried lists the candidates that were tried, in order (empty when there were none).
	Tried []CandidateOutcome
	// Winner is the candidate whose list worked; nil when none did.
	Winner *portal.InheritCandidate
	// CopyListing and CopyJobDetail say which of the winner's documents to save on the new link.
	CopyListing   bool
	CopyJobDetail bool
}

// errTrialNoToken is returned when there is no session token to call the browser with.
var errTrialNoToken = errors.New("no session token to run the trial with")

func shortReason(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > maxReasonRunes {
		return string(r[:maxReasonRunes]) + "…"
	}
	return s
}

// trialPaginatedCall sends one crawl-paginated request for url and returns the raw response.
// It retries a transient browser failure (the session restarted or its tab was replaced)
// the way "Crawl now" does, and nothing else.
func trialPaginatedCall(ctx context.Context, coreURL, accessToken, portalID, url string, req portal.CrawlRequest) ([]byte, error) {
	body, err := json.Marshal(struct {
		portal.CrawlRequest
		URL          string `json:"url"`
		RequestID    string `json:"requestId"`
		RateLimitKey string `json:"rateLimitKey"`
	}{CrawlRequest: req, URL: url, RequestID: uuid.NewString(), RateLimitKey: portalID})
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		respBody, err := callBrowserProxy(ctx, coreURL, "/api/v1/tools/browser/proxy/crawl-paginated", body, accessToken)
		if err == nil {
			return respBody, nil
		}
		transient := errors.Is(err, errBrowserSessionInterrupted) || errors.Is(err, errBrowserSessionWedged)
		if !transient || attempt >= maxCrawlNowSessionRetries {
			return nil, err
		}
		select {
		case <-time.After(crawlNowSessionRetryInterval):
		case <-ctx.Done():
			return nil, err
		}
	}
}

// TrialInherit tries the portal's other instructions on linkID (see the file comment).
// It returns the context's error when the trial was cut short, with the result so far.
func TrialInherit(ctx context.Context, linkID, accessToken string) (InheritTrialResult, error) {
	var result InheritTrialResult
	if accessToken == "" {
		return result, errTrialNoToken
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		return result, err
	}
	linkURL, err := portal.GetPortalLinkURL(linkID)
	if err != nil {
		return result, err
	}
	portalID, err := portal.GetPortalIDForLink(linkID)
	if err != nil {
		return result, err
	}
	candidates, err := portal.InheritCandidatesFor(linkID)
	if err != nil {
		return result, err
	}

	ctx, cancel := context.WithTimeout(ctx, inheritTrialTimeout)
	defer cancel()

	for _, c := range candidates {
		outcome := CandidateOutcome{LinkID: c.LinkID, Title: c.Title}

		listingReq, err := portal.BuildCrawlRequestFromYAML(c.Listing)
		if err != nil {
			outcome.Listing, outcome.Reason = ListingError, shortReason(err.Error())
			result.Tried = append(result.Tried, outcome)
			continue
		}
		respBody, err := trialPaginatedCall(ctx, coreURL, accessToken, portalID, linkURL, portal.LimitToPages(listingReq, 1))
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			// The browser, the session or the site is the problem, not the instructions:
			// the next candidate would meet the same thing, so the trial stops here.
			outcome.Listing, outcome.Reason = ListingError, shortReason(err.Error())
			result.Tried = append(result.Tried, outcome)
			return result, nil
		}
		var crawled crawlNowPaginatedResult
		if err := json.Unmarshal(respBody, &crawled); err != nil {
			outcome.Listing, outcome.Reason = ListingError, "the browser's answer could not be read"
			result.Tried = append(result.Tried, outcome)
			return result, nil
		}
		if crawled.StoppedReason == "cloudflare_blocked" {
			outcome.Listing, outcome.Reason = ListingError, "blocked by Cloudflare"
			result.Tried = append(result.Tried, outcome)
			return result, nil
		}

		report := portal.ClassifyTrialPages(crawled.Pages)
		if !portal.ListingTrialPasses(report) {
			outcome.Listing, outcome.Reason = ListingFailed, portal.ListingTrialFailureReason(report)
			result.Tried = append(result.Tried, outcome)
			continue
		}
		outcome.Listing = ListingOK

		outcome.JobDetail, outcome.Reason, err = trialJobDetails(ctx, coreURL, accessToken, portalID, c, portal.TrialJobURLs(crawled.Pages, portal.MaxDetailTrialJobs))
		if err != nil {
			return result, err
		}
		winner := c
		result.Winner = &winner
		result.CopyListing, result.CopyJobDetail = portal.InheritCopies(true, outcome.JobDetail)
		result.Tried = append(result.Tried, outcome)
		return result, nil
	}
	return result, nil
}

// trialJobDetails opens the trial jobs with the candidate's job-detail instructions and
// says how that went (a portal.Detail* word and, when it did not work, why). Only the
// context's own error is returned as an error.
func trialJobDetails(ctx context.Context, coreURL, accessToken, portalID string, c portal.InheritCandidate, urls []string) (status, reason string, err error) {
	if c.JobDetail == "" {
		return portal.DetailNoInstructions, "", nil
	}
	if len(urls) == 0 {
		return portal.DetailNotTried, "the list gave no job to open", nil
	}
	detailReq, buildErr := portal.BuildJobDetailCrawlRequestFromYAML(c.JobDetail)
	if buildErr != nil {
		return portal.DetailFailed, shortReason(buildErr.Error()), nil
	}
	values := make([]map[string]string, 0, len(urls))
	for _, jobURL := range urls {
		respBody, callErr := trialPaginatedCall(ctx, coreURL, accessToken, portalID, jobURL, detailReq)
		if callErr != nil {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			return portal.DetailFailed, shortReason(callErr.Error()), nil
		}
		var extracted jobDetailExtractResult
		if json.Unmarshal(respBody, &extracted) != nil || len(extracted.Pages) == 0 {
			return portal.DetailFailed, "no page was extracted", nil
		}
		values = append(values, extractResultValues(extracted.Pages[0]))
	}
	if !portal.DetailTrialsPass(values) {
		return portal.DetailFailed, fmt.Sprintf("the description was missing or too short on %d of %d job(s)", countShort(values), len(values)), nil
	}
	return portal.DetailOK, "", nil
}

// countShort counts the jobs whose description did not pass on its own.
func countShort(values []map[string]string) int {
	n := 0
	for _, v := range values {
		if !portal.DetailTrialsPass([]map[string]string{v}) {
			n++
		}
	}
	return n
}
