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
// No separate mutex/gate is needed for correctness: this tool's own
// single shared browser tab (shared.Mu) already fully serializes every
// page fetch, of any portal, at the process level — a multi-page
// crawl already holds shared.Mu for its ENTIRE sequence (see
// performPaginatedCrawl's own doc comment), so two fetches for the
// same key can never actually be in flight at the same instant. The
// only gap this closes is TIMING: without it, two back-to-back fetches
// for the same key would run essentially immediately one after another
// the moment shared.Mu frees up, with zero deliberate spacing between
// them. Deliberately consulted (and slept against) WHILE shared.Mu is
// still held by its own callers — consistent with, not a new departure
// from, the existing "hold Mu for the whole sequence" trade-off already
// accepted here (e.g. the Cloudflare headed-fallback wait). See
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

// awaitPortalCrawlPacing blocks until at least a random
// [minPortalCrawlDelay, maxPortalCrawlDelay) duration has elapsed
// since the last page fetched under the same key — a no-op when key
// is "" (every AI-driven call, which never sets rateLimitKey; see this
// file's own top comment). ctx-aware: a "Stop crawl" cancellation
// (Career's own crawl_cancel.go, one level up) interrupts the wait
// immediately, the same select-on-ctx.Done pattern Career's own
// session-retry backoff (crawl_now.go) already uses.
func awaitPortalCrawlPacing(ctx context.Context, key string) {
	if key == "" {
		return
	}
	portalPacingMu.Lock()
	last, ok := portalPacingLast[key]
	portalPacingMu.Unlock()

	if ok {
		need := minPortalCrawlDelay + time.Duration(rand.Int63n(int64(maxPortalCrawlDelay-minPortalCrawlDelay)))
		if wait := need - time.Since(last); wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
		}
	}

	portalPacingMu.Lock()
	portalPacingLast[key] = time.Now()
	portalPacingMu.Unlock()
}
