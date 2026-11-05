// linkcrawl_run.go (plan 185, step 7) — crawling a portal link with its link-crawler rule: the jobs of the link's page(s), found by the
// URL pattern of their links, without AI. A link whose crawl method is "links" crawls this way for "Crawl now" AND for auto-discovery;
// every other link crawls exactly as before.
//
// The run is a normal crawl run (same crawl_runs row, phases, log, Stop, outcome and event as the selector crawl), enforced here, not
// only by the screen: Katana must be the Browser tool's selected engine when the run STARTS and again when it RUNS, and the link must not
// need a login. When that is not the case the run FAILS with a clear message: there is never a silent fallback to the AI instructions.
package crawl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"career-tool-backend/domainevent"
	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// linkCrawlSelected reports whether a link is crawled with its link-crawler rule: its method is "links". A link on that method without a
// rule is an error (ErrLinkRulesNeeded), never a quiet fallback to the instructions.
func linkCrawlSelected(portalLinkID string) (bool, error) {
	settings, err := portal.GetLinkCrawlSettings(portalLinkID)
	if err != nil {
		return false, err
	}
	if settings.CrawlMethod != portal.CrawlMethodLinks {
		return false, nil
	}
	if settings.Rules == nil {
		return true, portal.ErrLinkRulesNeeded
	}
	return true, nil
}

// checkLinkCrawlStart is the check a link-crawler run passes before it may start (and again when it runs): not a login link, and Katana
// is the selected engine.
func checkLinkCrawlStart(ctx context.Context, coreURL, accessToken, portalLinkID string) error {
	needsLogin, _, err := portal.GetPortalLinkLogin(portalLinkID)
	if err != nil {
		return err
	}
	if needsLogin {
		return portal.ErrLinkRulesLogin
	}
	return requireKatanaSelected(ctx, coreURL, accessToken)
}

// linkCrawlStopText says, in a crawl summary's own words, why a link crawl ended.
func linkCrawlStopText(reason, detail string) string {
	switch reason {
	case linkrule.StopMaxPages:
		return "page limit reached"
	case linkrule.StopNoNextPage:
		return "no next page"
	case linkrule.StopNoNewJobs:
		return "the last page had no new jobs"
	case linkrule.StopRepeatedPage:
		return "the next page was already loaded"
	case linkrule.StopFetchFailed:
		return "stopped early: a page could not be loaded: " + detail
	case linkrule.StopLeftTheSite:
		return "stopped early: a page ended up on another site (" + detail + ")"
	default:
		return reason
	}
}

// linkFetchFailureText turns a failed page load into the sentence of the run's error: the Browser's message and, for a site that
// stopped the page, its advice.
func linkFetchFailureText(err error) string {
	var fe *linkFetchError
	if errors.As(err, &fe) {
		if fe.Advice != "" {
			return fe.Message + " " + fe.Advice
		}
		return fe.Message
	}
	return err.Error()
}

// runLinkCrawl is runCrawlNow for a link whose method is "links". Like runCrawlNow it builds progress in the run's log as it goes, always
// ends in a terminal finishCrawlRun/finishListingCrawlRun call, and publishes career.portal_link.jobs_crawled only when jobs were saved
// by a completed run; a Stop (ctx cancelled) leaves the terminal row to CrawlNowCancelHandler, which already wrote it.
func runLinkCrawl(ctx context.Context, runID, portalLinkID, accessToken, coreURL string) {
	fail := func(err error) {
		msg := linkFetchFailureText(err)
		_ = appendCrawlRunLog(runID, "failed: "+msg)
		_ = finishCrawlRun(runID, "failed", nil, &msg)
	}
	cancelled := func() bool {
		if ctx.Err() == nil {
			return false
		}
		_ = appendCrawlRunLog(runID, "cancelled")
		return true
	}

	_ = setCrawlRunPhase(runID, "building_request", "reading the link crawler rule")
	settings, err := portal.GetLinkCrawlSettings(portalLinkID)
	if err != nil {
		fail(err)
		return
	}
	if settings.CrawlMethod != portal.CrawlMethodLinks || settings.Rules == nil {
		fail(portal.ErrLinkRulesNeeded)
		return
	}
	// The setting can have changed since the run was accepted (and the auto-discovery path has no screen at all): checked again here.
	if err := checkLinkCrawlStart(ctx, coreURL, accessToken, portalLinkID); err != nil {
		if cancelled() {
			return
		}
		fail(err)
		return
	}
	linkURL, err := portal.GetPortalLinkURL(portalLinkID)
	if err != nil {
		fail(err)
		return
	}
	portalID, err := portal.GetPortalIDForLink(portalLinkID)
	if err != nil {
		fail(err)
		return
	}

	pageNo := 0
	fetch := func(fctx context.Context, pageURL string) (linkrule.Page, error) {
		pageNo++
		// the same address check a test run applies to every page: the link's own site, nothing else
		if _, err := ownSitePage(linkURL, pageURL); err != nil {
			return linkrule.Page{}, err
		}
		_ = setCrawlRunPhase(runID, "navigating", fmt.Sprintf("page %d: %s", pageNo, pageURL))
		stopPoll := pollBrowserPhase(fctx, coreURL, runID, accessToken)
		res, err := fetchPageLinks(fctx, coreURL, accessToken, pageURL, portalID, runID)
		stopPoll()
		if err != nil {
			return linkrule.Page{}, err
		}
		_ = appendCrawlRunLog(runID, fmt.Sprintf("page %d loaded: %d link(s) on it", pageNo, len(res.Links)))
		return linkrule.Page{FinalURL: res.FinalURL, Title: res.Title, Links: res.Links}, nil
	}

	result, err := linkrule.RunPages(ctx, *settings.Rules, linkURL, fetch, 0)
	if err != nil {
		if cancelled() {
			return
		}
		fail(err)
		return
	}
	if result.StopReason == linkrule.StopCancelled || cancelled() {
		_ = appendCrawlRunLog(runID, "cancelled")
		return
	}
	for i, p := range result.Pages {
		_ = appendCrawlRunLog(runID, fmt.Sprintf("page %d: %d job link(s), %d new", i+1, p.Matched, p.NewJobs))
	}
	if result.SkippedNoTitle > 0 {
		_ = appendCrawlRunLog(runID, fmt.Sprintf("%d matching link(s) had no usable text and were left out", result.SkippedNoTitle))
	}

	var ingested portal.IngestCrawlResultsResult
	if len(result.Items) > 0 {
		items := make([]map[string]any, 0, len(result.Items))
		for _, it := range result.Items {
			items = append(items, map[string]any{"title": it.Title, "url": it.URL})
		}
		_ = setCrawlRunPhase(runID, "ingesting_jobs", fmt.Sprintf("saving %d job(s)", len(items)))
		ingested, err = portal.IngestLinkCrawlResults(portalLinkID, []portal.CrawlResultPage{{URL: linkURL, Items: items}})
		if err != nil {
			fail(fmt.Errorf("failed to save crawled jobs: %w", err))
			return
		}
	}

	stopped := linkCrawlStopText(result.StopReason, result.StopDetail)
	summary := crawlSummary(ingested, len(result.Pages), stopped)
	if result.SkippedNoTitle > 0 {
		summary += fmt.Sprintf(" %d link(s) had no usable text.", result.SkippedNoTitle)
	}
	_ = appendCrawlRunLog(runID, "done: "+summary)
	outcome := listingOutcome{
		JobsSaved: ingested.JobsSaved, JobsUpdated: ingested.JobsUpdated, JobsSkipped: ingested.JobsSkipped + result.SkippedNoTitle,
		PagesVisited: len(result.Pages), StoppedReason: strings.TrimSpace(result.StopReason), NotFoundFields: []string{},
	}
	_ = finishListingCrawlRun(runID, summary, outcome)
	domainevent.Publish("career.portal_link.jobs_crawled", map[string]string{"portal_link_id": portalLinkID})
}
