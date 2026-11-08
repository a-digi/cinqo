// crawl_status.go tracks the live, in-flight phase of one /crawl or
// /crawl-paginated call, keyed by a caller-supplied requestId — the
// gap this closes: both those endpoints are a single, opaque, blocking
// HTTP call from any caller's own point of view, with no signal at all
// about whether the call is currently navigating, waiting on an
// automated Cloudflare check, waiting on a HUMAN to solve a challenge,
// or extracting fields, until the whole thing finally returns. Career's
// own deterministic "Crawl now" (tools/career/backend/crawl_now.go) is
// this feature's one real consumer — the AI's own MCP-driven tool
// calls never set a requestId, so this whole mechanism is a no-op for
// them, unchanged from today's behavior.
//
// Deliberately in-memory only, not persisted to SQLite — a phase is
// inherently tied to a live chromedp session that itself does not
// survive this process restarting (the whole headless/headed Chrome
// process and its session state are gone too), so persisting phase
// history past that point would describe a session that no longer
// exists. See plan/ai/tools/browser/step-31-crawl-phase-status-endpoint.md.
package crawler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type crawlPhase string

const (
	phaseNavigating             crawlPhase = "navigating"
	phaseCheckingCloudflare     crawlPhase = "checking_cloudflare"
	phaseAwaitingHumanChallenge crawlPhase = "awaiting_human_challenge"
	phaseExtracting             crawlPhase = "extracting"
	phaseCompleted              crawlPhase = "completed"
	phaseFailed                 crawlPhase = "failed"
)

// crawlStatusTTL bounds how long a requestId's own entry survives with
// no update — comfortably longer than normalSessionPaginatedCrawlTimeout's
// own 32-minute ceiling (paginate.go), so no caller actively polling a
// real, still-running operation could ever see its own entry evicted
// out from under it. Purely a memory-hygiene bound for a long-running
// process, not a behavioral timeout of the crawl itself.
const crawlStatusTTL = 1 * time.Hour

type crawlStatusEntry struct {
	Phase     crawlPhase
	Message   string
	UpdatedAt time.Time
}

var (
	crawlStatusMu sync.Mutex
	crawlStatuses = map[string]crawlStatusEntry{}
)

// setCrawlPhase records requestId's own current phase — a no-op when
// requestId is empty, which is what makes this whole feature opt-in:
// the AI's own MCP-driven calls (shared.CallSibling, no requestId of their
// own) never populate crawlStatuses at all. Also opportunistically
// sweeps any entry older than crawlStatusTTL on every call, so this
// map can never grow unboundedly across the life of this process — a
// plain, inline sweep rather than a separate goroutine/ticker, sized
// for this tool's own realistic concurrency (at most a small number of
// simultaneous crawls, matching shared.Mu/shared.NormalSessionMu's own
// single-shared-session model).
func setCrawlPhase(requestID string, phase crawlPhase, message string) {
	if requestID == "" {
		return
	}
	crawlStatusMu.Lock()
	defer crawlStatusMu.Unlock()

	crawlStatuses[requestID] = crawlStatusEntry{Phase: phase, Message: message, UpdatedAt: time.Now()}

	cutoff := time.Now().Add(-crawlStatusTTL)
	for id, entry := range crawlStatuses {
		if entry.UpdatedAt.Before(cutoff) {
			delete(crawlStatuses, id)
		}
	}
}

func getCrawlStatus(requestID string) (crawlStatusEntry, bool) {
	crawlStatusMu.Lock()
	defer crawlStatusMu.Unlock()
	entry, ok := crawlStatuses[requestID]
	return entry, ok
}

type crawlStatusResponse struct {
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	UpdatedAt string `json:"updatedAt"`
}

// CrawlStatusHandler handles GET /crawl-status?requestId=... — a
// read-only view of setCrawlPhase's own in-memory state. 404 for an
// unknown requestId (never started, already swept by crawlStatusTTL,
// or this process restarted since it was set) rather than a zero-value
// 200, so a poller can distinguish "nothing to report yet" from "this
// requestId was never tracked at all."
func CrawlStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requestID := r.URL.Query().Get("requestId")
	if requestID == "" {
		http.Error(w, "requestId query parameter is required", http.StatusBadRequest)
		return
	}
	entry, ok := getCrawlStatus(requestID)
	if !ok {
		http.Error(w, "no status found for this requestId", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(crawlStatusResponse{
		Phase:     string(entry.Phase),
		Message:   entry.Message,
		UpdatedAt: entry.UpdatedAt.UTC().Format(time.RFC3339),
	})
}
