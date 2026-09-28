// playwright_engine.go is the PRIMARY single-page fetch attempt for
// fetch_page_html — tried BEFORE the chromedp primary tab (crawlPage's
// own first attempt, crawl.go) on every call, and again as
// crawlOnWorkerTab's own first attempt (challenge_resolver.go) if that
// escalation path is ever reached. It is deliberately thin: one
// navigation, one wire-level Cloudflare check (the SAME cf-mitigated
// response header challenge_resolution.go's own challengeTracker trusts
// over any page-content heuristic — see that file's own top comment for
// why), and a normal read on a clean page. It does NOT reimplement that
// file's own automatic-wait/reload/settle state machine for an actual
// challenge — a challenge or block signal here is returned as the exact
// same *crawlError shape navigateAndReadWithCloudflareCheck already
// produces, so the caller's own EXISTING escalation
// (resolve/escalateChallenge) retries on chromedp and, if needed, hands
// off to a person exactly as it already does today. This file's only
// job is the fast, common, unchallenged case — which is also why it
// needs none of that system's own tab-pool/mutex machinery: a
// Playwright page is cheap and independent (browser.NewPage() below),
// so every call gets its own, closed when done, instead of sharing and
// serializing on one tab the way the chromedp worker-tab pool must.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"
)

var (
	pwOnce    sync.Once
	pwOnceErr error
	pwInst    *playwright.Playwright
	pwBrowser playwright.Browser
)

// ensurePlaywrightEngine lazily starts one shared Playwright driver and
// headless Chromium browser for this process's entire lifetime —
// mirrors shared.EnsureSharedSession's own lazy-start convention. Unlike
// that shared chromedp session, this holds no single shared page and
// needs no mutex around navigation: fetchWithPlaywright opens and closes
// its own page per call.
func ensurePlaywrightEngine() (playwright.Browser, error) {
	pwOnce.Do(func() {
		pwInst, pwOnceErr = playwright.Run()
		if pwOnceErr != nil {
			return
		}
		pwBrowser, pwOnceErr = pwInst.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true),
		})
	})
	return pwBrowser, pwOnceErr
}

// StopPlaywrightEngine shuts the shared Playwright browser/driver down —
// called once at process exit (main.go), mirroring
// shared.StopSharedSession. A no-op if the engine was never started
// (pwBrowser/pwInst nil, e.g. every crawl in this process happened to
// fail before ever reaching ensurePlaywrightEngine, or it was never
// called at all).
func StopPlaywrightEngine() {
	if pwBrowser != nil {
		_ = pwBrowser.Close()
	}
	if pwInst != nil {
		_ = pwInst.Stop()
	}
}

// decodeJSResult converts a page.Evaluate return value (Playwright's own
// JSON-shaped any — typically a map[string]any for a JS object) into a
// Go struct, via a JSON round-trip. Simpler and less error-prone than
// hand-walking the map, and correct for every shape this file's own
// probe script (buildChallengeProbeJS, challenge_resolution.go) returns.
func decodeJSResult(raw any, out any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// fetchWithPlaywright is crawlOnWorkerTab's own first attempt (wired in
// challenge_resolver.go) — see this file's own top comment for scope.
// requestID is used only for phase reporting (setCrawlPhase; a no-op for
// the AI's own calls, which never set one — crawl_status.go's own
// convention).
//
// Any error that is NOT a *crawlError (the shared browser failed to
// start, the page crashed, a navigation error unrelated to Cloudflare)
// is returned as a plain error — the caller falls back to the existing,
// fully-featured chromedp path for this one request; a Playwright
// engine failure is never allowed to become this crawl's own final
// answer.
//
// ctx is honored for cancellation only (cancelCrawl, step 63 — a user's
// own "Stop crawl" click, or the caller's own deadline): unlike
// chromedp, playwright-go's own API takes no context.Context at all, so
// there's no per-action deadline to thread through the way
// navigateWithHangGuard (crawl.go) does for chromedp. Instead, the one
// potentially slow call (page.Goto) runs in its own goroutine, raced
// against ctx.Done(); if ctx ends first, the page is closed to
// interrupt the in-flight navigation (Playwright surfaces that as a
// navigation error, discarded in favor of ctx's own error, same as
// classifyCancellation's own convention elsewhere in this package) and
// this function returns immediately rather than waiting out
// PageGotoOptions.Timeout regardless of why the caller stopped waiting.
func fetchWithPlaywright(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	browser, err := ensurePlaywrightEngine()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright engine unavailable: %w", err)
	}

	page, err := browser.NewPage()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: failed to open page: %w", err)
	}
	defer func() {
		_ = page.Close()
	}()

	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL+" (playwright)")
	timeoutMs := float64(crawlTimeout.Milliseconds())
	type gotoResult struct {
		resp playwright.Response
		err  error
	}
	gotoDone := make(chan gotoResult, 1)
	go func() {
		resp, err := page.Goto(rawURL, playwright.PageGotoOptions{Timeout: &timeoutMs})
		gotoDone <- gotoResult{resp, err}
	}()
	var resp playwright.Response
	select {
	case r := <-gotoDone:
		resp, err = r.resp, r.err
	case <-ctx.Done():
		_ = page.Close()
		return crawlResponse{}, ctx.Err()
	}
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: navigation failed: %w", err)
	}

	setCrawlPhase(requestID, phaseCheckingCloudflare, "checking for a Cloudflare challenge (playwright)")
	if resp != nil {
		headers, herr := resp.AllHeaders()
		if herr == nil {
			switch strings.ToLower(strings.TrimSpace(headers["cf-mitigated"])) {
			case "block":
				return crawlResponse{}, newCloudflareBlockedError(fmt.Sprintf("cf-mitigated: block (HTTP %d)", resp.Status()))
			case "challenge":
				return crawlResponse{}, newCloudflareUnresolvedError(fmt.Sprintf("cf-mitigated: challenge (HTTP %d)", resp.Status()), nil)
			}
		}
	}

	// No cf-mitigated verdict on the main response (header absent, or it
	// couldn't be read): fall back to the same page-marker probe
	// challenge_resolution.go's own tracker uses whenever it has no
	// observed wire signal either — a real challenge/block page must
	// still be caught here, never confidently returned as real content.
	probeJS, err := buildChallengeProbeJS(expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	raw, err := page.Evaluate(probeJS)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: challenge probe failed: %w", err)
	}
	var probe challengeProbe
	if err := decodeJSResult(raw, &probe); err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: challenge probe decode failed: %w", err)
	}
	if probe.BlockPage {
		return crawlResponse{}, newCloudflareBlockedError("cloudflare block page")
	}
	if probe.ChallengePage && !probe.Expected {
		return crawlResponse{}, newCloudflareUnresolvedError("cloudflare challenge page", nil)
	}

	return readCrawlResponseWithPlaywright(page, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
}

// readCrawlResponseWithPlaywright mirrors readCrawlResponse's (crawl.go)
// exact extraction sequence — title before element removal (removing
// <head> would otherwise blank it), then RemoveElementsJS, then the
// optional attribute-stripping pass, then the final URL/HTML read — so a
// caller sees identical behavior regardless of which engine actually
// served the page. Reuses RemoveElementsJS/removeAttributesJS/
// DefaultRemoveSelectors/defaultMaxLengthIgnoredAttributes/MaxHTMLBytes
// unchanged (same package, same pure JS-string builders — completely
// engine-agnostic already).
func readCrawlResponseWithPlaywright(page playwright.Page, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	title, err := page.Title()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: failed to read title: %w", err)
	}

	allRemoveSelectors := append(append([]string{}, DefaultRemoveSelectors...), removeSelectors...)
	script, err := RemoveElementsJS(allRemoveSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	if _, err := page.Evaluate(script); err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: element removal failed: %w", err)
	}

	if len(removeAttributes) > 0 || maxAttributeLength > 0 {
		allIgnoreForMaxLength := append(append([]string{}, defaultMaxLengthIgnoredAttributes...), ignoreAttributesForMaxLength...)
		attrScript, err := removeAttributesJS(removeAttributes, maxAttributeLength, allIgnoreForMaxLength)
		if err != nil {
			return crawlResponse{}, err
		}
		if _, err := page.Evaluate(attrScript); err != nil {
			return crawlResponse{}, fmt.Errorf("playwright: attribute removal failed: %w", err)
		}
	}

	finalURL := page.URL()
	html, err := page.Content()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("playwright: failed to read HTML: %w", err)
	}

	truncated := false
	if len(html) > MaxHTMLBytes {
		html = html[:MaxHTMLBytes]
		truncated = true
	}

	return crawlResponse{
		HTML:      html,
		Title:     title,
		FinalURL:  finalURL,
		Truncated: truncated,
	}, nil
}
