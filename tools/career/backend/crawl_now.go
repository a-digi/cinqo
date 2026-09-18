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
	"net/url"
	"os"
	"strings"
	"time"
)

// errBrowserSessionInterrupted is what callBrowserProxy returns
// (wrapped, so errors.Is still matches) when browser's own response
// carries its crawlError "browser_session_interrupted" Code —
// browser's own shared session was torn down mid-crawl by its own
// SIGTERM handler (a restart/stop/crash-recovery of that subprocess),
// not a genuine navigation/extraction failure. runCrawlNow's own
// retry loop below is the only place this is ever checked. See
// plan/ai/tools/browser/step-38-session-interrupted-retry.md.
var errBrowserSessionInterrupted = errors.New("browser tool session was interrupted")

// errBrowserSessionWedged (step 47.3) is what callBrowserProxy returns
// (wrapped, so errors.Is still matches) when browser's own response
// carries its crawlError "browser_session_wedged" Code — the shared
// session's own process is fine, but its one tab was left unresponsive
// by an earlier, unrelated caller (an unhandled dialog, a wedged
// renderer, an interrupted navigation) and has already been
// automatically replaced with a fresh one before this response was
// even sent. Distinct from errBrowserSessionInterrupted (a whole-
// process restart) but handled identically by runCrawlNow's own retry
// loop below — both are transient, self-healing failures where a
// retry is expected to land on a healthy session. See
// plan/ai/tools/browser/step-47-shared-tab-wedge-and-stuck-crawl-fix.md.
var errBrowserSessionWedged = errors.New("browser tool session's tab was wedged")

// errCrawlCancelledByUser (step 63.4) is what callBrowserProxy returns
// (wrapped, so errors.Is still matches) when browser's own response
// carries its crawlError "crawl_cancelled_by_user" Code — the user
// clicked "Stop crawl" (crawlNowCancelHandler, below) and browser
// already interrupted its own in-flight chromedp work for this exact
// runID. Deliberately NOT treated as transient by runCrawlNow's own
// retry loop below — the opposite of a retry-worthy failure, since
// retrying would silently un-cancel a crawl the user explicitly
// stopped. See plan/ai/tools/career/step-63-stop-crawling-now.md.
var errCrawlCancelledByUser = errors.New("crawl was cancelled by the user")

// crawlNowSessionRetryInterval/maxCrawlNowSessionRetries bound
// runCrawlNow's own retry of a single errBrowserSessionInterrupted
// failure — a short, fixed-count budget, deliberately NOT this
// codebase's own wall-clock-deadline convention
// (waitForHumanToClearCloudflare's 30-minute human-solve wait,
// browser tool): that convention exists because a human's own
// reaction time is unbounded and unpredictable, whereas a subprocess
// restart is a short, mechanical operation (manager.go's own process
// relaunch plus startSharedSession() typically completes in low
// single-digit seconds). Three attempts across ~10-15s rides out a
// normal restart; anything beyond that is more likely a genuine
// crash-loop, which should surface as a real failure (matching
// manager.go's own maxCrashRestarts — bounded, not infinite, retry)
// rather than retry silently forever. var, not const, for the same
// testing reason browser's own humanSolveRetryInterval is a var.
//
// This is deliberately NOT the whole-sequence retry step 35 removed
// (plan/ai/tools/career/step-35-crawl-now-single-long-lived-attempt.md):
// that retry re-ran the entire navigate+crawl sequence every 15s,
// which kept closing and reopening the headed Cloudflare-fallback
// window mid-wait — a real, named regression. This retry only ever
// fires once browser's own previous session (headless or headed) is
// already gone (the subprocess that owned it was killed), so there is
// no "window still trying to solve a challenge" to preserve — a fresh
// /crawl-paginated call is the only way to make further progress at
// all.
var (
	crawlNowSessionRetryInterval = 5 * time.Second
	maxCrawlNowSessionRetries    = 2 // 3 total attempts
)

// crawlNowHTTPClient's own Timeout must exceed browser's own longest
// possible single-call duration on either /crawl or /crawl-paginated —
// both can internally run the headed-Chrome/human-wait Cloudflare
// fallback before returning (tools/browser/backend/crawl.go's
// normalSessionCrawlTimeout=31min; paginate.go's
// normalSessionPaginatedCrawlTimeout, the larger of the two, is now
// derived from maxHumanWaitRounds×maxHumanSolveDuration — currently
// ~96min, since that fallback can retry the human-wait itself up to
// maxHumanWaitRounds times, see that constant's own doc comment). 100
// minutes leaves margin on top of the larger of the two. A real,
// reproduced bug otherwise: raising browser's own ceiling to
// accommodate multiple wait rounds is pointless if this client's own
// shorter timeout aborts the connection first. See this step's own
// Open Question 2.
var crawlNowHTTPClient = &http.Client{Timeout: 100 * time.Minute}

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

	run, err := startCrawlRun(body.PortalLinkID, "listing")
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

	// step 63.4 — a cancellable ctx, registered before the goroutine
	// even starts, so a Stop click racing right after this response is
	// never lost. Unregistered once the goroutine itself returns —
	// crawlNowCancelHandler treats "not found" the same as "already
	// finished," so a slightly-too-late cancel attempt is always a
	// harmless no-op, never an error.
	runCtx, runCancel := context.WithCancel(context.Background())
	registerCrawlNowCancel(run.ID, runCancel)
	go func() {
		defer unregisterCrawlNowCancel(run.ID)
		runCrawlNow(runCtx, run.ID, body.PortalLinkID, accessCookie.Value)
	}()

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

// crawlNowCancelHandler handles POST /portal-links/crawl-now/cancel —
// a real "Stop crawl," not to be confused with the frontend's own
// pre-existing client-side-only "Stop watching" (which never reached
// this backend at all). Two cooperating halves, both best-effort in
// the sense that neither blocks the other:
//
//  1. Local: cancelCrawlNowRun unblocks runCrawlNow's own goroutine
//     immediately — its in-flight HTTP call to browser aborts
//     (postWithBearer uses http.NewRequestWithContext), or its
//     retry-backoff sleep wakes early.
//  2. Remote: a direct call to browser's own POST /crawl-cancel,
//     telling it to interrupt its in-flight chromedp work for this
//     exact runID too — without this, browser would keep the shared
//     session busy for up to its own full timeout regardless of what
//     Career just decided. A failure here (browser unreachable, or
//     the request had already finished naturally moments earlier)
//     must never block marking this row cancelled below.
//
// finishCrawlRun's own WHERE status='running' guard (crawl_runs.go)
// makes this handler's own write race-safe against runCrawlNow's own
// goroutine also reaching a terminal state at roughly the same
// moment — whichever writes first wins, the second is a silent no-op.
// See plan/ai/tools/career/step-63-stop-crawling-now.md.
func crawlNowCancelHandler(w http.ResponseWriter, r *http.Request) {
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

	run, err := findActiveCrawlRun(body.PortalLinkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "no active crawl to cancel for this link", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to look up crawl run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cancelCrawlNowRun(run.ID)

	if coreURL, err := coreAPIURL(); err == nil {
		if accessCookie, err := r.Cookie("access_token"); err == nil {
			cancelBody, _ := json.Marshal(map[string]string{"requestId": run.ID})
			_, _, _ = postWithBearer(context.Background(), coreURL+"/api/v1/tools/browser/proxy/crawl-cancel", cancelBody, accessCookie.Value)
		}
	}

	msg := "Cancelled by user"
	if err := finishCrawlRun(run.ID, "cancelled", nil, &msg); err != nil {
		http.Error(w, "failed to mark crawl cancelled: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendCrawlRunLog(run.ID, msg)
	writeJSON(w, map[string]string{"crawlRunId": run.ID, "status": "cancelled"})
}

// crawlRunResponse is the wire shape crawlNowActiveHandler returns —
// Log split into individual entries here (the data layer keeps the raw
// newline-delimited string, see crawl_runs.go's own doc comment).
type crawlRunResponse struct {
	CrawlRunID    string   `json:"crawlRunId"`
	Kind          string   `json:"kind"`
	Status        string   `json:"status"`
	StartedAt     string   `json:"startedAt"`
	FinishedAt    *string  `json:"finishedAt"`
	Log           []string `json:"log"`
	ResultSummary *string  `json:"resultSummary"`
	ErrorMessage  *string  `json:"errorMessage"`
	Phase         *string  `json:"phase"`
}

func toCrawlRunResponse(r *crawlRun) crawlRunResponse {
	return crawlRunResponse{
		CrawlRunID:    r.ID,
		Kind:          r.Kind,
		Status:        r.Status,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
		Log:           splitCrawlRunLog(r.Log),
		ResultSummary: r.ResultSummary,
		ErrorMessage:  r.ErrorMessage,
		Phase:         r.Phase,
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

// crawlStatusPollInterval — how often pollBrowserPhase asks browser's
// own GET /crawl-status while a call to /crawl or /crawl-paginated is
// in flight. Supersedes the earlier plain elapsed-time heartbeat
// (which could only prove a run was alive, never say what it was
// actually doing) now that browser exposes its own real phase — see
// plan/ai/tools/browser/step-31-crawl-phase-status-endpoint.md and
// plan/ai/tools/career/step-39-fine-grained-crawl-phases.md.
var crawlStatusPollInterval = 5 * time.Second

// pollBrowserPhase polls GET /crawl-status?requestId=<runID> on
// browser's own proxy every crawlStatusPollInterval while a call to
// /crawl or /crawl-paginated is in flight, forwarding every observed
// change into crawl_runs via setCrawlRunPhase — this turns browser's
// own internal state into something Career's own log/phase actually
// reflects, in near-real-time. Runs concurrently with the blocking
// POST call it accompanies; stop() must be called on every exit path
// of that call. A poll failure (network error, browser tool
// temporarily unreachable, a stale/expired token) is silently ignored
// — this is a secondary, best-effort observability channel that must
// never affect the real, authoritative call's own outcome.
//
// Dedupes on phase+message together, not phase alone — a real bug
// fixed here: browser's own awaiting_human_challenge phase now updates
// its own message every retry tick (which attempt number, which
// detection reason) while the phase VALUE itself stays
// "awaiting_human_challenge" for the whole wait; deduping on phase
// alone would have silently dropped every one of those per-tick
// updates, making a perfectly-alive wait look identically frozen in
// Career's own log too.
func pollBrowserPhase(ctx context.Context, coreURL, runID, accessToken string) (stop func()) {
	done := make(chan struct{})
	last := ""
	go func() {
		ticker := time.NewTicker(crawlStatusPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				phase, message, ok := fetchBrowserCrawlStatus(ctx, coreURL, runID, accessToken)
				if !ok {
					continue
				}
				current := phase + "\x00" + message
				if current == last {
					continue
				}
				last = current
				_ = setCrawlRunPhase(runID, phase, message)
			}
		}
	}()
	return func() { close(done) }
}

// browserCrawlStatusResponse mirrors browser's own crawlStatusResponse
// (tools/browser/backend/crawl_status.go) — independently declared,
// same "two separate Go modules" reason every other cross-tool JSON
// shape in this tool is duplicated by hand.
type browserCrawlStatusResponse struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// fetchBrowserCrawlStatus GETs browser's own /crawl-status for runID —
// (phase, message, true) on a 200, or ("", "", false) for anything
// else (404 — nothing reported yet — included), so pollBrowserPhase
// can treat every non-success outcome identically as "nothing new."
func fetchBrowserCrawlStatus(ctx context.Context, coreURL, runID, accessToken string) (phase, message string, ok bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coreURL+"/api/v1/tools/browser/proxy/crawl-status?requestId="+url.QueryEscape(runID), nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := crawlNowHTTPClient.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var body browserCrawlStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", false
	}
	return body.Phase, body.Message, true
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

	_ = setCrawlRunPhase(runID, "building_request", "building crawl request")
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
	// portal_pacing.go — the owning portal's own id, not the link's:
	// browser's own rateLimitKey paces page fetches across EVERY link
	// belonging to this portal, not just within this one run.
	portalID, err := getPortalIDForLink(portalLinkID)
	if err != nil {
		fail(err)
		return
	}

	// step 37 — navigate and extract are now ONE call, not two: a real,
	// confirmed race existed here before this fix. Career used to call
	// POST /crawl (navigate) and, moments later, POST /crawl-paginated
	// (extract) as two fully separate HTTP round trips — browser's own
	// shared-session lock (sessionMu) was released completely in
	// between, so a DIFFERENT concurrent "Crawl now" run's own navigate
	// call could land in that gap and silently redirect THIS run's own
	// extraction to the wrong link's own page. Sending url alongside
	// the paginated request means browser's own handler navigates
	// there under the SAME lock acquisition that immediately precedes
	// extraction — the two steps are now atomic. See
	// plan/ai/tools/browser/step-37-atomic-navigate-and-extract.md.
	//
	// requestId (step 39) — runID doubles as the id browser's own
	// GET /crawl-status tracks this operation under; ties Career's own
	// crawl_runs row directly to browser's own in-memory phase, no new
	// id generation needed. expectedSelectors (step 36) — already
	// exactly what req's own Container/Fields carry.
	pagRequest := struct {
		crawlRequest
		URL          string `json:"url"`
		RequestID    string `json:"requestId"`
		RateLimitKey string `json:"rateLimitKey"`
	}{crawlRequest: req, URL: linkURL, RequestID: runID, RateLimitKey: portalID}
	pagBody, err := json.Marshal(pagRequest)
	if err != nil {
		fail(err)
		return
	}
	// step 38/47.3 — retried up to maxCrawlNowSessionRetries times, but
	// only for errBrowserSessionInterrupted (the whole subprocess was
	// restarted) or errBrowserSessionWedged (just the one tab was
	// wedged and already replaced): every other failure (Cloudflare
	// blocked, session expired, a genuine navigation/extraction error)
	// still fails on the first attempt, exactly as before.
	var respBody []byte
	for attempt := 0; ; attempt++ {
		stopPoll := pollBrowserPhase(ctx, coreURL, runID, accessToken)
		respBody, err = callBrowserProxy(ctx, coreURL, "/api/v1/tools/browser/proxy/crawl-paginated", pagBody, accessToken)
		stopPoll()
		if err == nil {
			break
		}
		// step 63.4 — a deliberate user-requested stop is the OPPOSITE
		// of transient: never retried, and crawlNowCancelHandler (below)
		// has already written the terminal 'cancelled' row itself by the
		// time this is ever observed here — this goroutine's only
		// remaining job is to stop touching that row, not to also call
		// fail() (which would try to overwrite it with 'failed';
		// finishCrawlRun's own WHERE status='running' guard would make
		// that a harmless no-op regardless, but returning here directly
		// is the honest, intended behavior, not one relying on that
		// guard to paper over a wrong call).
		if errors.Is(err, errCrawlCancelledByUser) {
			_ = appendCrawlRunLog(runID, err.Error())
			return
		}
		transient := errors.Is(err, errBrowserSessionInterrupted) || errors.Is(err, errBrowserSessionWedged)
		if !transient || attempt >= maxCrawlNowSessionRetries {
			fail(err)
			return
		}
		_ = appendCrawlRunLog(runID, fmt.Sprintf(
			"%s — retrying (%d/%d) in %s",
			err.Error(), attempt+1, maxCrawlNowSessionRetries, crawlNowSessionRetryInterval,
		))
		// step 63.4 — ctx-aware: a Stop click arriving during this
		// backoff wait now takes effect immediately instead of waiting
		// out the full interval first. ctx.Err() here is always
		// context.Canceled (this goroutine's own cancelCrawlNowRun,
		// below — nothing else ever cancels it), not a genuine
		// "browser session died" case, so it's reported directly as a
		// plain failure rather than routed through the Cloudflare/
		// session-interrupted error-Code machinery that only exists on
		// browser's own side of this call.
		select {
		case <-time.After(crawlNowSessionRetryInterval):
		case <-ctx.Done():
			_ = appendCrawlRunLog(runID, "cancelled while waiting to retry")
			return
		}
	}

	var result crawlNowPaginatedResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		fail(fmt.Errorf("failed to parse crawl result: %w", err))
		return
	}

	_ = setCrawlRunPhase(runID, "ingesting_jobs", fmt.Sprintf("saving %d page(s) of results", len(result.Pages)))
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
			if cfErr.Code == "browser_session_interrupted" {
				return nil, fmt.Errorf("%w: %s", errBrowserSessionInterrupted, cfErr.Message)
			}
			if cfErr.Code == "browser_session_wedged" {
				return nil, fmt.Errorf("%w: %s", errBrowserSessionWedged, cfErr.Message)
			}
			if cfErr.Code == "crawl_cancelled_by_user" {
				return nil, fmt.Errorf("%w: %s", errCrawlCancelledByUser, cfErr.Message)
			}
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
