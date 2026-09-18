// crawl_job_details_now.go is "Crawl now"'s own sibling for the
// SEPARATE job-detail-crawl-instructions document (portals.go):
// deterministic (no AI), backend-orchestrated (survives the initiating
// tab closing, crawl_now.go's own step-37 reasoning applies
// identically here), and tracked via the exact same crawl_runs table —
// just under kind='job_detail' instead of the default 'listing'
// (db.go's own crawl_runs.kind doc comment). Reuses crawl_now.go's own
// coreAPIURL/callBrowserProxy/postWithBearer/pollBrowserPhase/
// crawlNowHTTPClient and crawl_cancel.go's own registerCrawlNowCancel/
// cancelCrawlNowRun entirely unchanged — those are already generic
// over "one browser-proxy call tracked under one crawl_runs id," not
// specific to the listing crawl. crawlNowActiveHandler/
// crawlNowCancelHandler (crawl_now.go) are similarly already generic
// per portal link, regardless of kind — this file only ever needs its
// own START handler and goroutine body.
//
// The one real difference from runCrawlNow: this loop visits MANY
// pages (one per already-saved job), not one paginated sequence, and a
// single job's own failure is deliberately NOT fatal to the whole
// run — logged and skipped, continuing to the next job — unlike
// runCrawlNow's own single-shot fail-fast model. Only cancellation (or
// a request-building failure before the loop even starts) aborts the
// run early. See plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// crawlJobDetailsNowHandler handles POST /portal-links/crawl-job-details-now
// — starts a crawl_runs row (kind='job_detail') and launches the
// detached goroutine, responding immediately. Mirrors crawlNowHandler
// (crawl_now.go) exactly in shape.
func crawlJobDetailsNowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		PortalLinkID string `json:"portalLinkId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}

	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}

	// Fail fast — before creating a crawl_runs row at all — on missing
	// job detail crawl instructions, or on no jobs to crawl. Mirrors
	// crawlNowHandler's own buildCrawlRequest error mapping.
	if _, err := buildJobDetailCrawlRequest(body.PortalLinkID); err != nil {
		writePortalLinkAwareError(w, "build job detail crawl request", err)
		return
	}
	targets, err := listJobsForDetailCrawl(body.PortalLinkID)
	if err != nil {
		http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(targets) == 0 {
		http.Error(w, "this portal link has no saved jobs to crawl detail pages for — run a listing crawl first", http.StatusBadRequest)
		return
	}

	run, err := startCrawlRun(body.PortalLinkID, "job_detail")
	if err != nil {
		switch {
		case errors.Is(err, errCrawlAlreadyRunning):
			http.Error(w, "a crawl is already running for this link", http.StatusConflict)
		case errors.Is(err, errUnknownPortalLink):
			http.Error(w, "unknown portal link id", http.StatusBadRequest)
		default:
			http.Error(w, "failed to start crawl: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	registerCrawlNowCancel(run.ID, runCancel)
	go func() {
		defer unregisterCrawlNowCancel(run.ID)
		runJobDetailCrawlNow(runCtx, run.ID, body.PortalLinkID, targets, accessCookie.Value)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"crawlRunId": run.ID,
		"status":     run.Status,
		"startedAt":  run.StartedAt,
	})
}

// jobDetailExtractResult mirrors browser's own single-page
// paginatedCrawlResponse shape closely enough for this file's own
// purposes — only Pages[0] is ever read (EffectiveMaxPages is always
// 1, see buildJobDetailCrawlRequest), so this only declares the one
// field actually used, unlike crawlNowPaginatedResult (crawl_now.go)
// which needs the full shape for its own multi-page result.
type jobDetailExtractResult struct {
	Pages []crawlResultPage `json:"pages"`
}

// runJobDetailCrawlNow is the detached goroutine body — rooted in
// context.Background() by its caller (crawlJobDetailsNowHandler), same
// reasoning as runCrawlNow (crawl_now.go). Visits each target
// sequentially (this tool's own single shared browser tab has no room
// for real concurrency here — same constraint extract_from_url's own
// design, tools/browser/backend/crawler/extract_from_url.go, exists to
// make safe for the AI/sub-agent path); a job whose own extraction
// fails is logged and skipped, not fatal to the run — only
// cancellation stops the whole loop early.
func runJobDetailCrawlNow(ctx context.Context, runID, portalLinkID string, targets []jobDetailCrawlTarget, accessToken string) {
	fail := func(err error) {
		msg := err.Error()
		_ = appendCrawlRunLog(runID, "failed: "+msg)
		_ = finishCrawlRun(runID, "failed", nil, &msg)
	}

	coreURL, err := coreAPIURL()
	if err != nil {
		fail(err)
		return
	}

	_ = setCrawlRunPhase(runID, "building_request", "building job detail crawl request")
	req, err := buildJobDetailCrawlRequest(portalLinkID)
	if err != nil {
		fail(err)
		return
	}
	// portal_pacing.go — the owning portal's own id, not the link's:
	// browser's own rateLimitKey paces page fetches (here, one per job)
	// across EVERY link belonging to this portal, not just this link's
	// own jobs.
	portalID, err := getPortalIDForLink(portalLinkID)
	if err != nil {
		fail(err)
		return
	}

	updated, skipped := 0, 0
	for i, target := range targets {
		if ctx.Err() != nil {
			_ = appendCrawlRunLog(runID, "cancelled")
			return
		}

		label := target.Title
		if label == "" {
			label = target.SourceURL
		}
		_ = setCrawlRunPhase(runID, "extracting", fmt.Sprintf("job %d of %d: %s", i+1, len(targets), label))

		didUpdate, err := crawlOneJobDetail(ctx, coreURL, portalID, runID, accessToken, req, target)
		if err != nil {
			if errors.Is(err, errCrawlCancelledByUser) {
				_ = appendCrawlRunLog(runID, err.Error())
				return
			}
			if ctx.Err() != nil {
				_ = appendCrawlRunLog(runID, "cancelled")
				return
			}
			skipped++
			_ = appendCrawlRunLog(runID, fmt.Sprintf("skipped %q: %v", label, err))
			continue
		}
		if didUpdate {
			updated++
		} else {
			skipped++
			_ = appendCrawlRunLog(runID, fmt.Sprintf("skipped %q: no description extracted", label))
		}
	}

	summary := fmt.Sprintf("Updated %d of %d job(s); skipped %d.", updated, len(targets), skipped)
	_ = appendCrawlRunLog(runID, "done: "+summary)
	_ = finishCrawlRun(runID, "completed", &summary, nil)
}

// errNoPageExtracted is crawlOneJobDetail's own sentinel for "the
// browser call succeeded but produced no usable page" — distinct from
// a transport/session-level failure, but treated identically by every
// caller (mark the job's own detail-crawl attempt failed, skip it).
var errNoPageExtracted = errors.New("no page extracted")

// crawlOneJobDetail runs ONE job's own detail-page crawl attempt — the
// exact per-target work runJobDetailCrawlNow's own loop (above) needs,
// factored out so the deterministic "Match now" feature (job_match.go)
// can reuse it for a single job (its own crawl-if-missing step)
// without duplicating this retry/error-handling logic. Marks the job's
// own detail_crawl_status='failed' itself on any non-cancellation
// error (matching every skip path this function replaces) — the
// caller only needs to decide whether ITS OWN broader operation should
// keep going (any ordinary error) or stop entirely
// (errCrawlCancelledByUser, or ctx already cancelled).
func crawlOneJobDetail(ctx context.Context, coreURL, portalID, runID, accessToken string, req crawlRequest, target jobDetailCrawlTarget) (updated bool, err error) {
	pagRequest := struct {
		crawlRequest
		URL          string `json:"url"`
		RequestID    string `json:"requestId"`
		RateLimitKey string `json:"rateLimitKey"`
	}{crawlRequest: req, URL: target.SourceURL, RequestID: runID, RateLimitKey: portalID}
	pagBody, err := json.Marshal(pagRequest)
	if err != nil {
		_ = markJobDetailCrawlFailed(target.ID)
		return false, err
	}

	respBody, err := callBrowserProxyWithJobDetailRetry(ctx, coreURL, pagBody, accessToken, runID)
	if err != nil {
		if errors.Is(err, errCrawlCancelledByUser) || ctx.Err() != nil {
			return false, err
		}
		// A harder failure than "extraction ran but came back empty"
		// (saveJobDetailExtraction's own case, below) — the page fetch
		// itself never succeeded, so nothing else here will ever
		// record an attempt for this job. Marked 'failed' directly so
		// the Eye icon still distinguishes this from "never crawled."
		_ = markJobDetailCrawlFailed(target.ID)
		return false, err
	}

	var result jobDetailExtractResult
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Pages) == 0 {
		_ = markJobDetailCrawlFailed(target.ID)
		return false, errNoPageExtracted
	}

	values := make(map[string]string, len(result.Pages[0].Results))
	for k, v := range extractResultValues(result.Pages[0]) {
		values[k] = v
	}
	didUpdate, err := saveJobDetailExtraction(target.ID, values)
	if err != nil {
		_ = markJobDetailCrawlFailed(target.ID)
		return false, err
	}
	return didUpdate, nil
}

// extractResultValues flattens one page's own Results (label ->
// string, per extractResultStrings' own single-value convention for a
// non-multiple field — job detail fields are never declared multiple:
// true in practice, but a multiple field's own first matched value is
// still used rather than silently dropped) into a plain
// map[string]string, the shape saveJobDetailExtraction (jobs.go)
// expects.
func extractResultValues(page crawlResultPage) map[string]string {
	out := make(map[string]string, len(page.Results))
	for label, raw := range page.Results {
		if v := at(extractResultStrings(raw), 0); v != "" {
			out[label] = v
		}
	}
	return out
}

// callBrowserProxyWithJobDetailRetry mirrors runCrawlNow's own inline
// retry loop (crawl_now.go) — same transient-failure retry policy
// (errBrowserSessionInterrupted/errBrowserSessionWedged, up to
// maxCrawlNowSessionRetries) — but as a small, self-contained helper
// returning its outcome instead of directly calling this file's own
// fail()/return, since a single job's own non-transient failure here
// must only skip that job (runJobDetailCrawlNow's own caller loop),
// never abort the whole run the way runCrawlNow's own identical-looking
// inline version deliberately does for ITS single paginated sequence.
// Deliberately a separate copy, not a shared extraction of runCrawlNow's
// own loop — that existing, already-verified code is left completely
// untouched rather than risking a regression in it for this reuse.
func callBrowserProxyWithJobDetailRetry(ctx context.Context, coreURL string, body []byte, accessToken, runID string) ([]byte, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		stopPoll := pollBrowserPhase(ctx, coreURL, runID, accessToken)
		respBody, err := callBrowserProxy(ctx, coreURL, "/api/v1/tools/browser/proxy/crawl-paginated", body, accessToken)
		stopPoll()
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		if errors.Is(err, errCrawlCancelledByUser) {
			return nil, err
		}
		transient := errors.Is(err, errBrowserSessionInterrupted) || errors.Is(err, errBrowserSessionWedged)
		if !transient || attempt >= maxCrawlNowSessionRetries {
			return nil, lastErr
		}
		select {
		case <-time.After(crawlNowSessionRetryInterval):
		case <-ctx.Done():
			return nil, lastErr
		}
	}
}
