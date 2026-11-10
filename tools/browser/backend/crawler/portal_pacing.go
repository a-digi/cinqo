// portal_pacing.go implements a deliberate delay between
// consecutive page fetches sharing the same caller-supplied key —
// politeness pacing for Career's own deterministic crawl
// mechanisms ("Crawl now" / "Crawl job details now"), which visit
// many pages across possibly-different portal links but must not
// hit the SAME portal faster than a fixed small gap, and slower while the site is rate limiting them (plan 230). This
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
	"errors"
	"sync"
	"time"
)

// The gap between two reads of one key (plan 230, replacing the random 10 to 30 s and the doubling gap of plan 197): normalPortalGap, and
// alarmPortalGap while the key's alarm is raised. The alarm is raised by a read that ended rate limited (HTTP 429, Cloudflare error 1015, a
// cf-mitigated 429) and removed by the first read of the key that ends without a rate limit. Any other error changes nothing. In memory only:
// the rule is the site's, a restart starts without an alarm.
const (
	normalPortalGap = 3 * time.Second
	alarmPortalGap  = 20 * time.Second
)

var (
	portalPacingMu    sync.Mutex
	portalPacingLast  = map[string]time.Time{}
	portalPacingAlarm = map[string]bool{}
)

// notePacingOutcome records how a paced read of key ended: a rate limit raises the key's alarm, a read without one removes it, any other
// error changes nothing. A no-op for key "" (every AI call).
func notePacingOutcome(key string, err error) {
	if key == "" {
		return
	}
	portalPacingMu.Lock()
	defer portalPacingMu.Unlock()
	if err == nil {
		delete(portalPacingAlarm, key)
		return
	}
	var ce *crawlError
	if errors.As(err, &ce) && ce.Code == codeRateLimited {
		portalPacingAlarm[key] = true
	}
}

// pacingAlarmRaised says whether key's alarm is raised.
func pacingAlarmRaised(key string) bool {
	portalPacingMu.Lock()
	defer portalPacingMu.Unlock()
	return portalPacingAlarm[key]
}

// withPacingRetry fills RetryAfterSeconds of a rate-limit error with the alarm gap when the site gave none.
func withPacingRetry(key string, err error) {
	var ce *crawlError
	if key != "" && errors.As(err, &ce) && ce.Code == codeRateLimited && ce.RetryAfterSeconds == 0 && pacingAlarmRaised(key) {
		ce.RetryAfterSeconds = int(alarmPortalGap.Seconds())
	}
}

// reservePacingSlot reserves and returns the start time of a fetch for key requested at now: the first read of a key starts at once, the
// next at least the key's current gap after the previous reservation (not after whenever that fetch finished, so parallel workers cannot
// fire together). Pure apart from the reservation: awaitPortalCrawlPacing does the waiting.
func reservePacingSlot(key string, now time.Time) time.Time {
	portalPacingMu.Lock()
	defer portalPacingMu.Unlock()
	start := now
	if last, ok := portalPacingLast[key]; ok {
		gap := normalPortalGap
		if portalPacingAlarm[key] {
			gap = alarmPortalGap
		}
		if next := last.Add(gap); next.After(start) {
			start = next
		}
	}
	portalPacingLast[key] = start
	return start
}

// awaitPortalCrawlPacing blocks until this fetch's reserved start time (see reservePacingSlot): a no-op when key is "" (every AI-driven
// call, which never sets rateLimitKey; see this file's own top comment). ctx-aware: a "Stop crawl" cancellation (Career's own
// crawl_cancel.go, one level up) interrupts the wait immediately; the reservation itself is kept, which only ever spaces later fetches
// further apart.
func awaitPortalCrawlPacing(ctx context.Context, key string) {
	if key == "" {
		return
	}
	start := reservePacingSlot(key, time.Now())
	if wait := time.Until(start); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}
