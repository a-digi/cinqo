// crawl_cancel.go tracks a live cancel func for one detached
// runCrawlNow goroutine, keyed by its own crawl_runs.id (runID) —
// mirrors tools/browser/backend/crawl_cancel.go's own shape exactly,
// one level up: that file lets Career tell Browser to interrupt its
// own in-flight chromedp work; this one lets crawlNowCancelHandler
// (crawl_now.go) tell THIS process's own goroutine to give up
// immediately (its outbound HTTP call to browser aborts —
// postWithBearer uses http.NewRequestWithContext — and its
// retry-backoff sleep wakes early).
//
// See plan/ai/tools/career/step-63-stop-crawling-now.md.
package main

import (
	"context"
	"sync"
)

var (
	crawlNowCancelMu sync.Mutex
	crawlNowCancels  = map[string]context.CancelFunc{}
)

// registerCrawlNowCancel records runID's own cancel func. Every
// runCrawlNow invocation has a real, non-empty runID (startCrawlRun's
// own generated uuid) — unlike browser's own registerCrawlCancel, this
// one never needs an empty-id no-op case.
func registerCrawlNowCancel(runID string, cancel context.CancelFunc) {
	crawlNowCancelMu.Lock()
	defer crawlNowCancelMu.Unlock()
	crawlNowCancels[runID] = cancel
}

// unregisterCrawlNowCancel removes runID's own entry — a no-op if none
// exists (already removed).
func unregisterCrawlNowCancel(runID string) {
	crawlNowCancelMu.Lock()
	defer crawlNowCancelMu.Unlock()
	delete(crawlNowCancels, runID)
}

// cancelCrawlNowRun fires runID's own registered cancel func and
// reports whether one was actually found — false means "already
// finished, or unknown."
func cancelCrawlNowRun(runID string) bool {
	crawlNowCancelMu.Lock()
	defer crawlNowCancelMu.Unlock()
	cancel, ok := crawlNowCancels[runID]
	if !ok {
		return false
	}
	cancel()
	return true
}
