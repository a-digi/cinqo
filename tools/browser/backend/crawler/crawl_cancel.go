// crawl_cancel.go tracks a live cancel func for one in-flight /crawl or
// /crawl-paginated call, keyed by the same caller-supplied requestId
// crawl_status.go already tracks phase under (step 31) — deliberately
// mirroring that file's own shape (a plain, mutex-guarded map, opt-in:
// the AI's own MCP-driven calls never set a requestId, so this whole
// mechanism is a no-op for them). Career's own deterministic "Crawl
// now" (tools/career/backend/crawl_now.go) is this feature's one real
// consumer, same as crawl_status.go's own framing.
//
// Unlike crawl_status.go, no TTL sweep is needed here: every entry is
// removed via its own registering call's `defer unregisterCrawlCancel`
// the moment that call returns, so nothing can accumulate across the
// life of this process.
//
// See plan/ai/tools/career/step-63-stop-crawling-now.md.
package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var (
	crawlCancelMu sync.Mutex
	crawlCancels  = map[string]context.CancelFunc{}
)

// registerCrawlCancel records requestId's own cancel func — a no-op
// when requestId is empty, matching setCrawlPhase's own opt-in
// convention (crawl_status.go). Overwrites any previous entry for the
// same id without warning: a caller reusing the same requestId across
// sequential attempts (Career's own retry loop passes the same runID
// on every attempt) is expected, and by the time a second attempt
// registers, the first's own `defer unregisterCrawlCancel` has already
// run.
func registerCrawlCancel(requestID string, cancel context.CancelFunc) {
	if requestID == "" {
		return
	}
	crawlCancelMu.Lock()
	defer crawlCancelMu.Unlock()
	crawlCancels[requestID] = cancel
}

// unregisterCrawlCancel removes requestId's own entry — a no-op if
// none exists (already removed, or requestId was empty to begin
// with).
func unregisterCrawlCancel(requestID string) {
	if requestID == "" {
		return
	}
	crawlCancelMu.Lock()
	defer crawlCancelMu.Unlock()
	delete(crawlCancels, requestID)
}

// cancelCrawl fires requestId's own registered cancel func and reports
// whether one was actually found — false means "already finished,
// never started, or unknown," all indistinguishable from this
// function's own point of view (matching crawl-status's own 404
// framing for the same three cases).
func cancelCrawl(requestID string) bool {
	crawlCancelMu.Lock()
	defer crawlCancelMu.Unlock()
	cancel, ok := crawlCancels[requestID]
	if !ok {
		return false
	}
	cancel()
	return true
}

// stopBrowserLoad issues a best-effort CDP Page.stopLoading against
// baseCtx — the ACTUAL underlying browser session a request's own
// per-request ctx was derived from (shared.Ctx for the shared headless
// singleton, or the ephemeral headed session's own base ctx for the
// normal-session fallback), never the per-request ctx that was just
// canceled.
//
// Canceling a request's own Go-side ctx only stops THIS PROCESS from
// waiting on Chrome's own in-flight navigation/render — it does not
// tell Chrome itself to abandon that load. Without this, the tab can
// remain busy (still actually loading the page the user asked to
// stop) for a beat after cancelCrawl already returned — caught
// directly, not assumed: a disposable test observed
// shared.ProbeSessionLiveness still timing out immediately after a
// "successful" cancellation before this fix existed.
//
// Best-effort and bounded by its own short timeout, independent of
// whatever budget the canceled request's own ctx had: an error here
// (the tab already gone, nothing was actually loading, the session
// itself is mid-teardown) is never worth surfacing anywhere — this is
// strictly an optimization to free the tab sooner, not a correctness
// requirement on its own (shared.ProbeSessionLiveness's own short, bounded
// timeout is what actually protects every future caller either way,
// with or without this).
func stopBrowserLoad(baseCtx context.Context) {
	stopCtx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	_ = chromedp.Run(stopCtx, page.StopLoading())
}

// CrawlCancelHandler handles POST /crawl-cancel — Career's own "Stop
// crawl" button's real, browser-side counterpart (crawl_now.go's
// crawlNowCancelHandler is the caller). Deliberately NOT registered as
// an MCP tool (see runMCPServer, main.go) — mirrors crawl-status's own
// "Career's own consumer only" framing: the AI has no reason to stop a
// crawl it itself is driving one tool call at a time.
func CrawlCancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RequestID == "" {
		http.Error(w, "requestId is required", http.StatusBadRequest)
		return
	}

	cancelled := cancelCrawl(body.RequestID)
	w.Header().Set("Content-Type", "application/json")
	if !cancelled {
		w.WriteHeader(http.StatusNotFound)
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"cancelled": cancelled})
}
