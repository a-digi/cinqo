// crawl_now.go is the detached, backend-orchestrated replacement for
// the frontend's own "Crawl now" promise chain (browserApi.ts's
// fetchCrawlRequest -> navigateTo -> crawlPaginated -> ingestCrawlResults,
// step 27) — moved server-side so the sequence survives the initiating
// browser tab closing mid-crawl, tracked via crawl_runs.go's own
// crawl_runs table.
//
// The one genuinely new thing this file does: call another tool's
// (browser's) own HTTP proxy routes from inside a detached goroutine,
// with no live user request to ride along on. Resolved as Option A
// (plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md):
// this handler reads the caller's own access_token cookie directly off
// ITS OWN inbound request (which already carries it, since CORE's
// reverse proxy forwards the original Cookie header to this tool's own
// backend unmodified), and the launched goroutine presents that same
// value again on each of its own outbound calls to browser's proxy
// routes, as an Authorization: Bearer header (proxy_handler.go's own
// tokenFromRequest accepts either a cookie or a Bearer header).
//
// refresh_token is deliberately never captured or used — verified
// directly, not a simplification of convenience: both
// api/src/auth/handler/callback.go and renew.go set that cookie with
// Path: "/api/v1/auth/renew", so a browser never attaches it to a
// request against /api/v1/tools/career/proxy/..., and it can never
// reach this handler in the first place. If access_token expires
// mid-run (a real possibility given browser's own Cloudflare fallback
// can hold a call open for up to ~30 minutes), the run simply ends in
// error_message "session expired mid-crawl — please retry" — an
// honest failure, not a silent one, and not something this file can
// self-heal given the cookie's own scope. See step-37's own
// "Correction: no self-renewal" section for the full reasoning.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// crawlNowHTTPClient's own Timeout must exceed browser's own longest
// possible single-call duration on either /crawl or /crawl-paginated —
// both can internally run the up-to-~30-minute headed-Chrome/human-wait
// Cloudflare fallback (tools/browser/backend/crawl.go's
// normalSessionCrawlTimeout=31min, paginate.go's
// normalSessionPaginatedCrawlTimeout=32min) before returning. 35 minutes
// leaves margin on top of the larger of the two. See this step's own
// Open Question 2.
var crawlNowHTTPClient = &http.Client{Timeout: 35 * time.Minute}

// coreAPIURL reads CORE_API_URL — set for every tool subprocess
// (api/src/tool/manager/manager.go's ToolEnvVars) but, until this file,
// never actually used by this tool's own backend.
func coreAPIURL() (string, error) {
	v := os.Getenv("CORE_API_URL")
	if v == "" {
		return "", errors.New("CORE_API_URL is not set")
	}
	return v, nil
}

// crawlNowHandler handles POST /portal-links/crawl-now — starts a
// crawl_runs row and launches the detached goroutine, responding
// immediately rather than waiting on it.
func crawlNowHandler(w http.ResponseWriter, r *http.Request) {
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

	// The credential travels via the request's own Cookie header,
	// already present on every proxied call — never read from the JSON
	// body (access_token is HttpOnly; no frontend JS could put it there
	// in the first place). Defensive only: the proxy's own scope check
	// already requires a valid session cookie to reach this handler at
	// all, so a missing access_token cookie here should be unreachable
	// in practice.
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}

	// Fail fast on missing/invalid crawl instructions before creating a
	// crawl_runs row at all — mirrors crawlRequestHandler's own
	// buildCrawlRequest error mapping exactly.
	if _, err := buildCrawlRequest(body.PortalLinkID); err != nil {
		writePortalLinkAwareError(w, "build crawl request", err)
		return
	}

	run, err := startCrawlRun(body.PortalLinkID)
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

	go runCrawlNow(context.Background(), run.ID, body.PortalLinkID, accessCookie.Value)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"crawlRunId": run.ID,
		"status":     run.Status,
		"startedAt":  run.StartedAt,
	})
}

// crawlNowActiveHandler handles GET /portal-links/crawl-now/active?portalLinkId=...
// — returns the most recent crawl_runs row for the link, running or
// terminal, so a poller reliably observes a just-finished run instead
// of racing a 404 (mirrors GetActiveTurnHandler's own contract,
// api/src/conversation/handler/turn_handler.go).
func crawlNowActiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	portalLinkID := r.URL.Query().Get("portalLinkId")
	if portalLinkID == "" {
		http.Error(w, "portalLinkId query parameter is required", http.StatusBadRequest)
		return
	}
	run, err := findMostRecentCrawlRun(portalLinkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "no crawl run found for this link", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load crawl run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toCrawlRunResponse(run))
}

// crawlRunResponse is the wire shape crawlNowActiveHandler returns —
// Log split into individual entries here (the data layer keeps the raw
// newline-delimited string, see crawl_runs.go's own doc comment).
type crawlRunResponse struct {
	CrawlRunID    string   `json:"crawlRunId"`
	Status        string   `json:"status"`
	StartedAt     string   `json:"startedAt"`
	FinishedAt    *string  `json:"finishedAt"`
	Log           []string `json:"log"`
	ResultSummary *string  `json:"resultSummary"`
	ErrorMessage  *string  `json:"errorMessage"`
}

func toCrawlRunResponse(r *crawlRun) crawlRunResponse {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(r.Log, "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	if lines == nil {
		lines = []string{}
	}
	return crawlRunResponse{
		CrawlRunID:    r.ID,
		Status:        r.Status,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
		Log:           lines,
		ResultSummary: r.ResultSummary,
		ErrorMessage:  r.ErrorMessage,
	}
}

// crawlNowPaginatedResult mirrors browser's own paginatedCrawlResponse
// (tools/browser/backend/paginate.go) — this tool has no dependency on
// that package, so the shape is independently declared here, kept in
// sync by hand, the same convention crawlRequest/crawlResultPage
// (portals.go) already established for this tool's other calls into
// browser's own JSON contracts. Pages uses this tool's own
// crawlResultPage (portals.go) directly — its fields are a strict
// subset of browser's pageExtractResult, so json.Unmarshal simply
// ignores the extra CloudflareDetected/CloudflareReason keys on each
// page.
type crawlNowPaginatedResult struct {
	Pages             []crawlResultPage `json:"pages"`
	StoppedReason     string            `json:"stoppedReason"`
	PagesVisited      int               `json:"pagesVisited"`
	RequestedMaxPages int               `json:"requestedMaxPages"`
	EffectiveMaxPages int               `json:"effectiveMaxPages"`
	BlockedReason     string            `json:"blockedReason,omitempty"`
}

// heartbeatInterval — how often startHeartbeat appends a "still
// waiting" line to crawl_runs.log while a call to browser's own /crawl
// or /crawl-paginated is in flight. A real, reported gap this closes:
// either call is a single opaque, blocking HTTP round trip from this
// goroutine's own perspective, and browser's own Cloudflare
// human-solve fallback can legitimately hold it open for up to ~30
// minutes (humanSolveRetryInterval/maxHumanSolveDuration,
// tools/browser/backend/crawl.go) — with nothing appended to the log
// for the whole wait, a perfectly normal, in-progress crawl reads as
// indistinguishable from a genuinely stuck one. See
// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md's
// own "Post-implementation" notes for the report that prompted this.
var heartbeatInterval = 20 * time.Second

// startHeartbeat appends one log line immediately (so a slow first
// tick doesn't leave a caller wondering) and then every
// heartbeatInterval, until the returned stop func is called — always
// call stop via defer immediately after starting one, on every exit
// path of the call it's wrapping.
func startHeartbeat(runID string) (stop func()) {
	startedAt := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(startedAt).Round(time.Second)
				_ = appendCrawlRunLog(runID, fmt.Sprintf(
					"still waiting on browser (%s elapsed) — this can take up to ~30 minutes if Cloudflare is blocking the page and a human hasn't cleared it yet",
					elapsed,
				))
			}
		}
	}()
	return func() { close(done) }
}

// runCrawlNow is the detached goroutine body — rooted in
// context.Background() by its caller (crawlNowHandler), not the
// request's own context, so it keeps running after the handler has
// already responded and the initiating tab may have closed. Mirrors
// the AI conversation feature's own runDetachedTurn
// (api/src/conversation/runner.go) in spirit: build up progress in
// crawl_runs.log as it goes, always end in a terminal finishCrawlRun
// call, never leave a row stuck at "running" on any exit path.
func runCrawlNow(ctx context.Context, runID, portalLinkID, accessToken string) {
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

	_ = appendCrawlRunLog(runID, "building crawl request")
	req, err := buildCrawlRequest(portalLinkID)
	if err != nil {
		fail(err)
		return
	}
	linkURL, err := getPortalLinkURL(portalLinkID)
	if err != nil {
		fail(err)
		return
	}

	_ = appendCrawlRunLog(runID, "navigating to "+linkURL)
	navBody, err := json.Marshal(struct {
		URL string `json:"url"`
	}{URL: linkURL})
	if err != nil {
		fail(err)
		return
	}
	stopHeartbeat := startHeartbeat(runID)
	_, err = callBrowserProxy(ctx, coreURL, "/api/v1/tools/browser/proxy/crawl", navBody, accessToken)
	stopHeartbeat()
	if err != nil {
		fail(err)
		return
	}

	_ = appendCrawlRunLog(runID, "running crawl_paginated")
	pagBody, err := json.Marshal(req)
	if err != nil {
		fail(err)
		return
	}
	stopHeartbeat = startHeartbeat(runID)
	respBody, err := callBrowserProxy(ctx, coreURL, "/api/v1/tools/browser/proxy/crawl-paginated", pagBody, accessToken)
	stopHeartbeat()
	if err != nil {
		fail(err)
		return
	}

	var result crawlNowPaginatedResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		fail(fmt.Errorf("failed to parse crawl result: %w", err))
		return
	}

	_ = appendCrawlRunLog(runID, fmt.Sprintf("ingesting %d page(s)", len(result.Pages)))
	ingested, err := ingestCrawlResults(portalLinkID, result.Pages)
	if err != nil {
		fail(fmt.Errorf("failed to save crawled jobs: %w", err))
		return
	}

	// Exact same summary text handleCrawlNow's own frontend version
	// already built (step 27/35) — preserved verbatim so this change is
	// invisible to a user on the success path.
	summary := fmt.Sprintf("Saved %d new, updated %d, skipped %d — visited %d page(s) (%s).",
		ingested.JobsSaved, ingested.JobsUpdated, ingested.JobsSkipped, result.PagesVisited, result.StoppedReason)

	if result.StoppedReason == "cloudflare_blocked" {
		reasonSuffix := ""
		if result.BlockedReason != "" {
			reasonSuffix = fmt.Sprintf(" (%s)", result.BlockedReason)
		}
		errMsg := fmt.Sprintf("%s Crawl stopped early — Cloudflare blocked page %d%s.", summary, result.PagesVisited+1, reasonSuffix)
		_ = appendCrawlRunLog(runID, errMsg)
		_ = finishCrawlRun(runID, "failed", nil, &errMsg)
		return
	}

	_ = appendCrawlRunLog(runID, "done: "+summary)
	_ = finishCrawlRun(runID, "completed", &summary, nil)
}

// callBrowserProxy POSTs body to coreURL+path (one of browser's own
// /crawl or /crawl-paginated proxy routes) with accessToken attached as
// an Authorization: Bearer header — proxy_handler.go's own
// tokenFromRequest (api/src/tool/handler/proxy_handler.go) accepts
// either a cookie or a Bearer header, cookie checked first, so a Bearer
// header is the natural choice for a server-to-server caller that isn't
// a browser at all.
//
// A 401 here means access_token has expired mid-run — there is no
// retry: refresh_token (the only thing that could renew it) never
// reaches this backend at all, see this file's own header comment. A
// 502 carrying browser's own crawlError JSON shape ({code, message,
// reason} — tools/browser/backend/cloudflare.go) is translated into the
// exact "Crawl blocked by Cloudflare — ..." phrasing browserApi.ts's
// own CrawlBlockedError handling already produced on the
// frontend-orchestrated path, so this change is invisible to a user in
// the failure case too.
func callBrowserProxy(ctx context.Context, coreURL, path string, body []byte, accessToken string) ([]byte, error) {
	respBody, status, err := postWithBearer(ctx, coreURL+path, body, accessToken)
	if err != nil {
		return nil, err
	}

	if status == http.StatusUnauthorized {
		return nil, errors.New("session expired mid-crawl — please retry")
	}

	if status == http.StatusBadGateway {
		var cfErr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		}
		if jsonErr := json.Unmarshal(respBody, &cfErr); jsonErr == nil && cfErr.Code != "" {
			reasonSuffix := ""
			if cfErr.Reason != "" {
				reasonSuffix = fmt.Sprintf(" (%s)", cfErr.Reason)
			}
			return nil, fmt.Errorf("Crawl blocked by Cloudflare — %s%s.", cfErr.Message, reasonSuffix)
		}
		return nil, errors.New(strings.TrimSpace(string(respBody)))
	}

	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("request failed (%d): %s", status, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func postWithBearer(ctx context.Context, url string, body []byte, accessToken string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := crawlNowHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}
