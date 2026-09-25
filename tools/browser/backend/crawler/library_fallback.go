// library_fallback.go is a Go port of krist-18/cloudflare-web-scraper
// (https://github.com/krist-18/cloudflare-web-scraper, commit 44ce8bb,
// src/scraper/cloudflare_handler.py), wired in as a FALLBACK: it runs only
// when this tool's own headless attempt ends in an unresolved Cloudflare
// challenge (see crawlPage). Added at the user's request, to try it out
// against real sites.
//
// Ported as-is from the library:
//
//   - a fresh browser per attempt (fresh profile, no stealth script),
//     closed afterwards — CloudflareHandler._new_context;
//   - its settings.json values: headless, Chrome/120 Windows user agent,
//     1366×768 viewport, 45s navigation timeout, 1.5s js_wait;
//   - navigate until DOMContentLoaded, then wait for network idle (no
//     request in flight for 500ms, Playwright's "networkidle"), continuing
//     on timeout — CloudflareHandler.fetch;
//   - _wait_cloudflare: sleep 1s, and while the page HTML contains one of
//     its four phrases, poll every 1s for up to 20s;
//   - sleep js_wait_ms, then take the page HTML;
//   - up to 3 attempts with exponential backoff of 1–10s on errors
//     (retry_handler.py's tenacity policy);
//   - round-robin proxies (ProxyManager) — BROWSER_TOOL_FALLBACK_PROXIES,
//     comma-separated, e.g. "http://user:pass@host:port,socks5://host:1080".
//
// Deliberately different from the library:
//
//   - The result is checked with this tool's own challenge detection
//     (challengeTracker — `cf-mitigated`, Cloudflare's page markers). The
//     library returns whatever HTML it ends up with, challenge page
//     included; here a page that is still a challenge is reported as
//     cloudflare_challenge_unresolved, never returned as content.
//   - No caller-supplied JavaScript (the library's js_script): this tool's
//     API has no such input and never evaluates caller JS.
//   - The output is read the same way as every other crawl
//     (readCrawlResponse), and on success the fallback browser's cookies
//     for the URL are copied into the shared headless browser, so the
//     primary tab can follow the AI to that page.
//
// BROWSER_TOOL_LIBRARY_FALLBACK=0 (or "false") disables it.
package crawler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// The library's own src/config/settings.json values.
const (
	libraryNavigationTimeout = 45 * time.Second
	libraryJSWait            = 1500 * time.Millisecond
	libraryMaxAttempts       = 3
	libraryUserAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	libraryViewportWidth     = 1366
	libraryViewportHeight    = 768
)

// libraryChallengePhrases — CloudflareHandler._wait_cloudflare's own
// text_candidates, matched case-insensitively against the page HTML.
var libraryChallengePhrases = []string{
	"checking your browser",
	"verifying you are human",
	"just a moment",
	"security check",
}

// libraryVerifyReserve — how much of an attempt's own time is kept for
// the final challenge check and reading the page.
const libraryVerifyReserve = 3 * time.Second

// libraryFallbackReserve — how much of an AI call's aiCallBudget the
// tool's own headless attempt leaves for this fallback (crawlPage).
const libraryFallbackReserve = 18 * time.Second

// minLibraryAttempt — below this much remaining budget an attempt can't
// launch a browser, navigate and wait meaningfully.
const minLibraryAttempt = 8 * time.Second

// libraryFallbackEnabled reports whether the fallback may run (default on).
func libraryFallbackEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("BROWSER_TOOL_LIBRARY_FALLBACK")))
	return v != "0" && v != "false" && v != "off"
}

// libraryProxies is ProxyManager: a round-robin over the configured list.
var (
	libraryProxyMu   sync.Mutex
	libraryProxyNext int
)

func nextLibraryProxy() string {
	var proxies []string
	for _, p := range strings.Split(os.Getenv("BROWSER_TOOL_FALLBACK_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	if len(proxies) == 0 {
		return ""
	}
	libraryProxyMu.Lock()
	defer libraryProxyMu.Unlock()
	p := proxies[libraryProxyNext%len(proxies)]
	libraryProxyNext++
	return p
}

// libraryFallbackFetch is CloudflareHandler.fetch, retried like the
// library's retry_policy: attempts that fail with an error are retried
// (backoff 1s, 2s, 4s… capped at 10s); an attempt that completes is final
// — cleared, blocked, or still a challenge. deadline (zero: none) bounds
// the whole thing, for AI calls.
func libraryFallbackFetch(reqCtx context.Context, deadline time.Time, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	var lastErr error
	backoff := time.Second
	for attempt := 1; attempt <= libraryMaxAttempts; attempt++ {
		timeout := capTimeout(libraryNavigationTimeout, deadline)
		if timeout < minLibraryAttempt {
			break
		}
		setCrawlPhase(requestID, phaseCheckingCloudflare, fmt.Sprintf("library fallback (cloudflare-web-scraper port): attempt %d of %d", attempt, libraryMaxAttempts))
		start := time.Now()
		resp, err := libraryFetchOnce(reqCtx, timeout, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
		log.Printf("library fallback: %s attempt %d/%d took %s: %s", rawURL, attempt, libraryMaxAttempts, time.Since(start).Round(100*time.Millisecond), describeFallbackResult(err))

		var cfErr *crawlError
		if err == nil || errors.As(err, &cfErr) || reqCtx.Err() != nil {
			return resp, err
		}
		lastErr = err
		if attempt < libraryMaxAttempts {
			if capTimeout(backoff, deadline) < backoff || sleepCtx(reqCtx, backoff) != nil {
				break
			}
			if backoff *= 2; backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
		}
	}
	if lastErr == nil {
		lastErr = newCloudflareUnresolvedError("time budget exhausted before the library fallback could run", nil)
	}
	return crawlResponse{}, lastErr
}

func describeFallbackResult(err error) string {
	var cfErr *crawlError
	switch {
	case err == nil:
		return "OK — page returned"
	case errors.As(err, &cfErr):
		return cfErr.Code + " (" + cfErr.Reason + ")"
	default:
		return "error: " + err.Error()
	}
}

// libraryFetchOnce is one CloudflareHandler.fetch attempt in a fresh
// browser, closed before returning.
func libraryFetchOnce(reqCtx context.Context, timeout time.Duration, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	proxy := nextLibraryProxy()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(libraryUserAgent),
		chromedp.WindowSize(libraryViewportWidth, libraryViewportHeight),
	)
	if p := os.Getenv("BROWSER_TOOL_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	var proxyUser, proxyPass string
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil || u.Host == "" {
			return crawlResponse{}, fmt.Errorf("library fallback: invalid proxy %q", redactProxy(proxy))
		}
		if u.User != nil {
			proxyUser = u.User.Username()
			proxyPass, _ = u.User.Password()
		}
		// Chrome takes credentials separately (answered below via Fetch).
		opts = append(opts, chromedp.ProxyServer(u.Scheme+"://"+u.Host))
		log.Printf("library fallback: using proxy %s", redactProxy(proxy))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	// Start the browser before timing the page work.
	if err := chromedp.Run(browserCtx); err != nil {
		return crawlResponse{}, fmt.Errorf("library fallback: failed to start browser: %w", err)
	}

	ctx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, browserCtx)

	if proxyUser != "" {
		answerProxyAuth(ctx, proxyUser, proxyPass)
		if err := chromedp.Run(ctx, fetch.Enable().WithHandleAuthRequests(true)); err != nil {
			return crawlResponse{}, err
		}
	}

	signals := watchChallengeSignals(ctx)
	idle := watchNetworkIdle(ctx)

	// page.goto(url, wait_until="domcontentloaded")
	setCrawlPhase(requestID, phaseNavigating, "library fallback: navigating to "+rawURL)
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, errText, _, err := page.Navigate(rawURL).Do(ctx)
		if err == nil && errText != "" {
			err = errors.New(errText)
		}
		return err
	})); err != nil {
		return crawlResponse{}, classifyCancellation(fmt.Errorf("library fallback: navigation failed: %w", err), reqCtx, browserCtx)
	}
	// The library's waits run on waitCtx, which ends libraryVerifyReserve
	// before ctx does: when the budget runs out mid-wait, the final check
	// below still runs and reports the page's real state, instead of the
	// call ending on a bare "context deadline exceeded".
	waitCtx := ctx
	if dl, ok := ctx.Deadline(); ok {
		var cancelWait context.CancelFunc
		waitCtx, cancelWait = context.WithDeadline(ctx, dl.Add(-libraryVerifyReserve))
		defer cancelWait()
	}
	libraryWaits(waitCtx, idle)
	if reqCtx.Err() != nil {
		return crawlResponse{}, newCrawlCancelledError()
	}

	// Not in the library: never return a challenge page as content.
	tracker, err := newChallengeTracker(ctx, signals, expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	st, err := tracker.stateWithRetry(ctx)
	if err != nil {
		return crawlResponse{}, classifyCancellation(err, reqCtx, browserCtx)
	}
	if st.blocked {
		return crawlResponse{}, newCloudflareBlockedError(st.reason)
	}
	if st.challenge {
		return crawlResponse{}, newCloudflareUnresolvedError("library fallback: "+st.reason, nil)
	}

	resp, err := readCrawlResponse(ctx, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	if err != nil {
		return crawlResponse{}, classifyCancellation(err, reqCtx, browserCtx)
	}
	importLibraryCookies(ctx, rawURL)
	return resp, nil
}

// libraryWaits runs the library's waits in order — DOMContentLoaded,
// network idle (continuing on timeout), _wait_cloudflare, js_wait — and
// simply stops early when ctx ends.
func libraryWaits(ctx context.Context, idle *networkIdle) {
	if waitDOMContentLoaded(ctx) != nil {
		return
	}
	// page.wait_for_load_state("networkidle") — on timeout, continue.
	if !idle.wait(ctx, libraryNavigationTimeout) && ctx.Err() == nil {
		log.Printf("library fallback: timed out waiting for network idle; continuing with best-effort HTML")
	}
	// _wait_cloudflare
	if sleepCtx(ctx, time.Second) != nil {
		return
	}
	if libraryPageHasChallengeText(ctx) {
		log.Printf("library fallback: Cloudflare challenge text detected; waiting…")
		for i := 0; i < 20; i++ {
			if sleepCtx(ctx, time.Second) != nil {
				return
			}
			if !libraryPageHasChallengeText(ctx) {
				break
			}
		}
	}
	// asyncio.sleep(js_wait_ms)
	_ = sleepCtx(ctx, libraryJSWait)
}

// libraryPageHasChallengeText is _wait_cloudflare's own check: any of
// the four phrases anywhere in page.content(). A read error counts as
// "no text" — the library proceeds regardless on errors.
func libraryPageHasChallengeText(ctx context.Context) bool {
	var html string
	if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html)); err != nil {
		return false
	}
	lower := strings.ToLower(html)
	for _, phrase := range libraryChallengePhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// waitDOMContentLoaded polls document.readyState until it's past
// "loading" — Playwright's wait_until="domcontentloaded".
func waitDOMContentLoaded(ctx context.Context) error {
	for {
		var state string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &state)); err == nil && state != "loading" && state != "" {
			return nil
		}
		if err := sleepCtx(ctx, 100*time.Millisecond); err != nil {
			return fmt.Errorf("library fallback: page never reached DOMContentLoaded: %w", err)
		}
	}
}

// networkIdle tracks in-flight requests for Playwright's "networkidle":
// no request in flight for at least 500ms.
type networkIdle struct {
	mu       sync.Mutex
	inflight map[network.RequestID]bool
	lastBusy time.Time
}

func watchNetworkIdle(ctx context.Context) *networkIdle {
	n := &networkIdle{inflight: map[network.RequestID]bool{}, lastBusy: time.Now()}
	chromedp.ListenTarget(ctx, func(ev any) {
		n.mu.Lock()
		defer n.mu.Unlock()
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			n.inflight[e.RequestID] = true
		case *network.EventLoadingFinished:
			delete(n.inflight, e.RequestID)
		case *network.EventLoadingFailed:
			delete(n.inflight, e.RequestID)
		default:
			return
		}
		n.lastBusy = time.Now()
	})
	return n
}

// wait returns true once the network has been idle for 500ms, false on
// timeout or ctx end.
func (n *networkIdle) wait(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		idle := len(n.inflight) == 0 && time.Since(n.lastBusy) >= 500*time.Millisecond
		n.mu.Unlock()
		if idle {
			return true
		}
		if sleepCtx(ctx, 100*time.Millisecond) != nil {
			return false
		}
	}
	return false
}

// answerProxyAuth supplies the proxy's credentials when Chrome asks —
// Chrome's --proxy-server flag can't carry them, unlike Playwright's
// proxy option. Every other paused request is let through unchanged.
func answerProxyAuth(ctx context.Context, user, pass string) {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
					Response: fetch.AuthChallengeResponseResponseProvideCredentials,
					Username: user,
					Password: pass,
				}))
			}()
		case *fetch.EventRequestPaused:
			go func() { _ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID)) }()
		}
	})
}

// importLibraryCookies copies the fallback browser's cookies for rawURL
// into the shared headless browser (see shared.ImportCookiesToHeadless).
// Best-effort.
func importLibraryCookies(ctx context.Context, rawURL string) {
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

// redactProxy hides a proxy URL's password for logs.
func redactProxy(p string) string {
	u, err := url.Parse(p)
	if err != nil || u.User == nil {
		return p
	}
	if _, has := u.User.Password(); has {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}
