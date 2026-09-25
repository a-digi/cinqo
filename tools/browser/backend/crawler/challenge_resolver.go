// challenge_resolver.go routes a page that's behind a Cloudflare
// challenge somewhere it can clear WITHOUT occupying the primary tab
// (shared.Ctx, guarded by shared.Mu) that every stateful operation —
// fetch_page_html → extract_page_data, login, find_login_elements —
// depends on.
//
// Before this, the whole challenge wait ran inside the caller's own
// shared.Mu hold (headless), and the headed fallback held its own
// process-wide mutex for up to 31 minutes: one challenged site made
// every other operation in the tool wait. Now:
//
//  1. the primary tab only notices the challenge (primaryAutoWait),
//     leaves it (about:blank) and releases shared.Mu;
//  2. escalateChallenge retries in a headless WORKER tab with the full
//     automatic wait (same browser → the same cookie jar, so whatever
//     cf_clearance it earns applies to the primary tab too), and only
//     if that fails, in a HEADED tab — where a person can help — whose
//     cookies are then copied into the headless browser;
//  3. syncPrimaryTabAsync brings the primary tab back to the page (for
//     AI callers, whose next call may operate on "the current page").
//
// AI calls (MCP tool calls — no requestID) never reach step 2's headed
// tab: the agent host abandons a tool call after 45s (aiCallBudget), far
// less than a headed attempt, let alone a person solving a challenge,
// can take. They get one headless worker attempt within that budget and
// a clear cloudflare_challenge_unresolved error otherwise — instead of
// the host's own generic timeout, which the agent misreads as the site
// being unreachable.
//
// At most one resolution per domain runs at a time (lockDomain):
// requests for a domain that's already being resolved wait for it, then
// retry headless — which usually passes straight away on the cookie the
// first one earned — instead of each opening its own headed tab.
package crawler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// primaryAutoWait is how long the PRIMARY tab itself waits for a
// detected challenge to clear before handing it to escalateChallenge —
// just enough to absorb the fastest silent passes without a second
// navigation, short enough that shared.Mu is released almost at once.
const primaryAutoWait = 3 * time.Second

// primarySyncTimeout bounds syncPrimaryTabLocked's own navigation.
const primarySyncTimeout = 15 * time.Second

// aiCallBudget bounds one AI (MCP) tool call end to end, Cloudflare
// handling included. The agent host kills a tool call after 45s
// (invokeTimeout, api/src/tool/mcp/invoke.go — spawn + MCP handshake +
// the call itself); this leaves margin for the spawn/handshake and for
// the response to travel back. Past it the host reports only a generic
// timeout, which the agent reads as "the site is unreachable".
const aiCallBudget = 38 * time.Second

// minWorkerAttempt — below this much remaining budget a worker attempt
// can't navigate and wait meaningfully, so it isn't started at all.
const minWorkerAttempt = 8 * time.Second

// aiDeadline returns the end of an AI call's own budget, or the zero
// time for a call with a requestID (Career's own, which has no such
// host limit and may wait for a person).
func aiDeadline(requestID string, start time.Time) time.Time {
	if requestID != "" {
		return time.Time{}
	}
	return start.Add(aiCallBudget)
}

// capTimeout returns d, shortened to the time left until deadline (a
// zero deadline means "no cap").
func capTimeout(d time.Duration, deadline time.Time) time.Duration {
	if deadline.IsZero() {
		return d
	}
	if left := time.Until(deadline); left < d {
		return left
	}
	return d
}

// recordIfUnresolved records rawURL's domain as known-Cloudflare (step
// 28) when err is an unresolved challenge. Best-effort.
func recordIfUnresolved(rawURL string, err error) {
	var cfErr *crawlError
	if errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved {
		if domain, hostErr := hostnameOf(rawURL); hostErr == nil {
			_ = recordCloudflareDomain(domain, cfErr.Reason)
		}
	}
}

var (
	domainLocksMu sync.Mutex
	domainLocks   = map[string]chan struct{}{}
)

// lockDomain serializes challenge resolution per domain. waited reports
// whether another resolution for the same domain was in progress when
// this call arrived — the caller should then retry headless first,
// since that resolution may already have earned a usable cookie.
// Cancellable via ctx, unlike a sync.Mutex.
func lockDomain(ctx context.Context, domain string) (unlock func(), waited bool, err error) {
	for {
		domainLocksMu.Lock()
		ch, busy := domainLocks[domain]
		if !busy {
			ch = make(chan struct{})
			domainLocks[domain] = ch
			domainLocksMu.Unlock()
			return func() {
				domainLocksMu.Lock()
				delete(domainLocks, domain)
				domainLocksMu.Unlock()
				close(ch)
			}, waited, nil
		}
		domainLocksMu.Unlock()
		waited = true
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, waited, ctx.Err()
		}
	}
}

// escalateChallenge resolves a challenged rawURL: headless worker first
// (when tryWorkerFirst, or when another resolution for the same domain
// was just in progress), then headed. worker/headed are the caller's own
// single-page or paginated attempts. A worker attempt ending in anything
// other than an unresolved challenge (success, a block page, a
// cancellation, any other error) is final; an unresolved one records the
// domain as known-Cloudflare (step 28) and moves on to headed.
func escalateChallenge[T any](reqCtx context.Context, rawURL, requestID string, tryWorkerFirst bool, worker, headed func() (T, error)) (T, error) {
	var zero T
	domain, _ := hostnameOf(rawURL)

	setCrawlPhase(requestID, phaseAwaitingDomain, "waiting for any other Cloudflare resolution on "+domain+" to finish")
	unlock, waited, err := lockDomain(reqCtx, domain)
	if err != nil {
		return zero, newCrawlCancelledError()
	}
	defer unlock()

	if tryWorkerFirst || waited {
		res, err := worker()
		var cfErr *crawlError
		if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
			return res, err
		}
		// step 28 — best-effort: a failed cache write must never turn an
		// otherwise-working fallback into a failure.
		if domain != "" {
			_ = recordCloudflareDomain(domain, cfErr.Reason)
		}
	}
	return headed()
}

// linkRequestCancel ties a request's own reqCtx (cancelCrawl, step 63)
// to ctx: the moment reqCtx is cancelled, ctx is cancelled too and the
// tab behind baseCtx is told to stop loading (stopBrowserLoad) — the
// same pattern every crawl entry point uses, extracted here now that
// worker and headed tabs need it too. Go's context package has no "done
// when either is done" combinator; this goroutine is the idiomatic
// substitute and exits with whichever finishes first.
func linkRequestCancel(reqCtx, ctx context.Context, cancel context.CancelFunc, baseCtx context.Context) {
	go func() {
		select {
		case <-reqCtx.Done():
			cancel()
			stopBrowserLoad(baseCtx)
		case <-ctx.Done():
		}
	}()
}

// crawlOnWorkerTab runs one full single-page crawl — navigate, full
// automatic Cloudflare wait, read — in its own headless worker tab,
// finishing by deadline (zero: no cap beyond crawlTimeout).
func crawlOnWorkerTab(reqCtx context.Context, deadline time.Time, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	timeout := capTimeout(crawlTimeout, deadline)
	if timeout < minWorkerAttempt {
		return crawlResponse{}, newCloudflareUnresolvedError("time budget exhausted before a headless retry", nil)
	}
	setCrawlPhase(requestID, phaseQueuedForTab, "waiting for a free browser tab")
	tabCtx, release, err := shared.AcquireWorkerTab(reqCtx)
	if err != nil {
		if reqCtx.Err() != nil {
			return crawlResponse{}, newCrawlCancelledError()
		}
		return crawlResponse{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(tabCtx, capTimeout(timeout, deadline))
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	resp, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, release, cloudflareAutoWait, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	return resp, classifyCancellation(err, reqCtx, tabCtx)
}

// syncPrimaryTabAsync points the primary tab at rawURL after the page
// was read somewhere else (a worker tab), so an AI caller's NEXT call —
// extract_page_data, find_login_elements, login, crawl_paginated
// without a url all operate on "the current page" — sees that page, not
// the blank tab crawlPage left behind.
//
// Asynchronous so it never delays the response (an AI call is already
// up against aiCallBudget), yet ordered before that next call: when
// shared.Mu is free it's taken HERE, synchronously, and handed to the
// goroutine (a Go mutex may be unlocked by a different goroutine), so
// the next Mu-guarded call necessarily waits for the sync to finish.
// Only when Mu is busy (another primary-tab operation in progress) is
// that ordering best-effort.
func syncPrimaryTabAsync(rawURL string, resp crawlResponse, skipCache bool) {
	if shared.Mu.TryLock() {
		if shared.Ctx != nil && shared.Ctx.Err() == nil {
			go func() {
				defer shared.Mu.Unlock()
				syncPrimaryTabLocked(rawURL, resp, skipCache)
			}()
			return
		}
		shared.Mu.Unlock()
	}
	go func() {
		if err := shared.EnsureSharedSession(); err != nil {
			return
		}
		shared.Mu.Lock()
		defer shared.Mu.Unlock()
		syncPrimaryTabLocked(rawURL, resp, skipCache)
	}()
}

// syncPrimaryTabLocked does syncPrimaryTabAsync's navigation. Assumes
// shared.Mu is held. Best-effort, quick (primaryAutoWait only): if the
// cookie earned in the worker tab isn't accepted here, the primary tab
// is left blank rather than on a challenge page. When it does land
// cleanly, it's recorded as showing rawURL and resp is cached for it —
// exactly the state crawlPage itself leaves after an unchallenged crawl.
func syncPrimaryTabLocked(rawURL string, resp crawlResponse, skipCache bool) {
	if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
		_ = shared.RecreateSharedSessionLocked()
		return
	}

	ctx, cancel := context.WithTimeout(shared.Ctx, primarySyncTimeout)
	defer cancel()
	recreate := func() { _ = shared.RecreateSharedSessionLocked() }

	signals := watchChallengeSignals(ctx)
	if err := navigateWithHangGuard(ctx, recreate, chromedp.Navigate(rawURL), chromedp.Sleep(SettleDelay)); err != nil {
		return
	}
	tracker, err := newChallengeTracker(ctx, signals, nil)
	if err != nil {
		return
	}
	outcome, err := tracker.checkAndAwaitAutoClearance(ctx, "", primaryAutoWait, 0)
	if err != nil || !outcome.Cleared {
		leavePrimaryTabLocked(ctx, recreate)
		return
	}
	shared.LastHeadlessFetchURL = rawURL
	if !skipCache {
		_ = fetchCacheStore(rawURL, resp)
	}
}

// leavePrimaryTabLocked navigates the primary tab to about:blank so a
// challenge page stops running its scripts in it, and so no stateful
// follow-up call ever operates on challenge DOM. Assumes shared.Mu is
// held. Best-effort, bounded by the hang guard.
func leavePrimaryTabLocked(ctx context.Context, recreate func()) {
	shared.LastHeadlessFetchURL = ""
	blankCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = navigateWithHangGuard(blankCtx, recreate, chromedp.Navigate("about:blank"))
}

// importHeadedCookies copies the cookies the headed tab holds for
// rawURL (cf_clearance included, once a challenge cleared there) into
// the headless browser — see shared.ImportCookiesToHeadless. Best-effort.
func importHeadedCookies(ctx context.Context, rawURL string) {
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().WithURLs([]string{rawURL}).Do(ctx)
		return err
	})); err != nil {
		return
	}
	_ = shared.ImportCookiesToHeadless(cookies)
}
