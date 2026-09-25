// portal_pacing.go implements a deliberate, randomized delay between
// consecutive page fetches sharing the same caller-supplied key —
// anti-detection pacing for Career's own deterministic crawl
// mechanisms ("Crawl now" / "Crawl job details now"), which visit
// many pages across possibly-different portal links but must never
// hit the SAME portal faster than a human plausibly would. This
// package has no concept of "portal" itself — key is opaque, supplied
// by the caller (Career passes its own portal id); the AI-driven
// crawl_paginated/extract_page_data/fetch_page_html path never sets
// one, so it is completely unaffected by this file.
//
// Since the browser-pool step, url-bearing crawls (Career's) run in
// their own worker tabs concurrently, so two fetches for the same key CAN
// now be in flight at the same instant. awaitPortalCrawlPacing therefore
// RESERVES each fetch's start time under portalPacingMu (the next slot
// is computed from the last reservation, not from whenever a fetch
// happened to finish) — otherwise two workers reading the same "last"
// timestamp would sleep the same amount and fire together. See
// plan/ai/tools/career/step-XX-portal-crawl-pacing.md.
package crawler

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// minPortalCrawlDelay/maxPortalCrawlDelay bound the randomized wait —
// plain constants, matching this codebase's own "constants over
// premature configurability" convention.
const (
	minPortalCrawlDelay = 10 * time.Second
	maxPortalCrawlDelay = 30 * time.Second
)

var (
	portalPacingMu   sync.Mutex
	portalPacingLast = map[string]time.Time{}
)

// awaitPortalCrawlPacing blocks until this fetch's reserved start time:
// at least a random [minPortalCrawlDelay, maxPortalCrawlDelay) after
// the previous reservation under the same key — a no-op when key is ""
// (every AI-driven call, which never sets rateLimitKey; see this file's
// own top comment). ctx-aware: a "Stop crawl" cancellation (Career's own
// crawl_cancel.go, one level up) interrupts the wait immediately; the
// reservation itself is kept, which only ever spaces later fetches
// further apart.
func awaitPortalCrawlPacing(ctx context.Context, key string) {
	if key == "" {
		return
	}
	portalPacingMu.Lock()
	start := time.Now()
	if last, ok := portalPacingLast[key]; ok {
		delay := minPortalCrawlDelay + time.Duration(rand.Int63n(int64(maxPortalCrawlDelay-minPortalCrawlDelay)))
		if next := last.Add(delay); next.After(start) {
			start = next
		}
	}
	portalPacingLast[key] = start
	portalPacingMu.Unlock()

	if wait := time.Until(start); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}
