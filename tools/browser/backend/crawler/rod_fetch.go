// rod_fetch.go (step 49) — fetchWithRod, the go-rod/rod counterpart of
// fetchWithPlaywright (playwright_engine.go): the PRIMARY single-page
// fetch for a domain with no known Cloudflare history. Same signature,
// same result (a crawlResponse), same escalation contract: a Cloudflare
// verdict is returned as the *crawlError the caller already knows how to
// escalate; ANY other failure is a plain error, which the caller treats
// as "this engine failed" and falls through to the unchanged chromedp
// primary tab — a rod failure is never a request's final answer.
//
// It is not wired into the fetch path yet (that is the engine-selection
// step); nothing calls it outside tests. See
// plan/ai/tools/browser/step-49-rod-fetch-engine.md.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// defaultRodMaxPages mirrors the chromedp worker-tab pool's default
// (shared.defaultMaxWorkerTabs): each page is a renderer process
// (~100–300 MB), so this is a memory bound as much as a concurrency one.
// BROWSER_TOOL_MAX_TABS overrides it, the same variable the tab pool reads.
const defaultRodMaxPages = 4

// The DOM-stable wait (waitForDOMToSettle): a page that fills its content
// with JavaScript AFTER the load event (a job list fetched over XHR, a
// framework rendering client-side) would otherwise be read half-empty.
// The page must go rodDOMQuiet without a single DOM mutation, and the wait
// is capped at rodDOMMaxWait so a page that never stops changing (a ticker,
// rotating ads) cannot stall a crawl. Vars, not consts, so a test can
// shrink them — this codebase's own convention (staleCrawlRunThreshold).
//
// The chromedp path settles with a fixed SettleDelay (1.5 s) after every
// navigation; this waits only as long as the page is actually changing, so
// a static page costs just rodDOMQuiet.
var (
	rodDOMQuiet   = 500 * time.Millisecond
	rodDOMMaxWait = 4 * time.Second
)

var (
	rodSlotsMu sync.Mutex
	rodSlots   chan struct{}
)

// rodPageSlots returns the semaphore bounding concurrently open rod pages,
// created on first use from BROWSER_TOOL_MAX_TABS.
func rodPageSlots() chan struct{} {
	rodSlotsMu.Lock()
	defer rodSlotsMu.Unlock()
	if rodSlots == nil {
		n := defaultRodMaxPages
		if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BROWSER_TOOL_MAX_TABS"))); err == nil && v > 0 {
			n = v
		}
		rodSlots = make(chan struct{}, n)
	}
	return rodSlots
}

// rodEvalExpression runs a JavaScript EXPRESSION in the page and returns
// its JSON value — the raw CDP Runtime.evaluate, exactly what chromedp's
// Evaluate does. Deliberately NOT rod's Page.Eval: that wraps the string
// as a function value and calls it, which double-invokes this package's
// already-invoked IIFE builders (RemoveElementsJS, removeAttributesJS,
// buildChallengeProbeJS) and throws (step 48's finding). Using this, every
// builder is reused unchanged.
func rodEvalExpression(p *rod.Page, expression string) ([]byte, error) {
	res, err := proto.RuntimeEvaluate{Expression: expression, ReturnByValue: true, AwaitPromise: true}.Call(p)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		msg := res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
			msg = res.ExceptionDetails.Exception.Description
		}
		return nil, fmt.Errorf("javascript error: %s", msg)
	}
	return json.Marshal(res.Result.Value)
}

func rodEvalString(p *rod.Page, expression string) (string, error) {
	raw, err := rodEvalExpression(p, expression)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

// mainDocumentResponse records the last main-frame document response seen
// (the last, so a redirect chain ends on the final one).
type mainDocumentResponse struct {
	mu      sync.Mutex
	status  int
	headers map[string]string // lower-cased names
	seen    bool
}

func (m *mainDocumentResponse) record(status int, headers proto.NetworkHeaders) {
	h := make(map[string]string, len(headers))
	for name, value := range headers {
		h[strings.ToLower(name)] = value.String()
	}
	m.mu.Lock()
	m.status, m.headers, m.seen = status, h, true
	m.mu.Unlock()
}

func (m *mainDocumentResponse) header(name string) (value string, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.TrimSpace(m.headers[strings.ToLower(name)]), m.status
}

// fetchWithRod — see the file comment. requestID is used only for phase
// reporting (setCrawlPhase is a no-op for the AI's own calls, which never
// set one).
//
// ctx is honored for cancellation: every rod call below runs on a page
// clone bound to ctx, so a caller that stops waiting (a user's "Stop
// crawl", the caller's own deadline) interrupts the in-flight navigation
// and this returns ctx's error rather than waiting out crawlTimeout.
func fetchWithRod(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	// Bound the number of simultaneously open pages.
	slots := rodPageSlots()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return crawlResponse{}, ctx.Err()
	}

	browser, err := ensureRodEngine()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("rod engine unavailable: %w", err)
	}

	// stealth.Page: a new page with stealth.JS injected on every new
	// document — the same script the chromedp tabs get (shared/tabs.go).
	// It sets no user agent; the browser was launched with NoDefaultDevice
	// for exactly that reason (see launchRodBrowser).
	page, err := stealth.Page(browser)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("rod: failed to open page: %w", err)
	}
	defer func() { _ = page.Close() }() // the ORIGINAL page, never a cancelled clone

	// One clone bound to ctx with the whole-fetch budget. Rod's Timeout()
	// returns a NEW page (step 48's trap): everything below uses `p`, and
	// `page` itself stays on the background context so it can still be
	// closed after p's deadline or ctx's cancellation.
	p := page.Context(ctx).Timeout(crawlTimeout)

	// Dismiss every JavaScript dialog: an unhandled alert()/confirm()
	// blocks the page's load forever (the chromedp path has
	// installDialogAutoDismiss for the same reason).
	evCtx, stopEvents := context.WithCancel(ctx)
	defer stopEvents()
	main := &mainDocumentResponse{}
	mainFrame := proto.PageFrameID(page.TargetID)
	// The subscription is made here, synchronously, before navigating;
	// only the returned wait function runs in the goroutine.
	go page.Context(evCtx).EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false}.Call(page)
		},
		func(e *proto.NetworkResponseReceived) {
			if e.Type == proto.NetworkResourceTypeDocument && e.FrameID == mainFrame && e.Response != nil {
				main.record(e.Response.Status, e.Response.Headers)
			}
		},
	)()

	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL+" (rod)")
	if err := p.Navigate(rawURL); err != nil {
		return crawlResponse{}, rodNavigationError(ctx, err)
	}
	if err := p.WaitLoad(); err != nil {
		return crawlResponse{}, rodNavigationError(ctx, err)
	}

	setCrawlPhase(requestID, phaseCheckingCloudflare, "checking for a Cloudflare challenge (rod)")
	// The wire verdict first: the same cf-mitigated response header the
	// challenge tracker trusts over any page-content heuristic.
	if value, status := main.header("cf-mitigated"); value != "" {
		switch strings.ToLower(value) {
		case "block":
			return crawlResponse{}, newCloudflareBlockedError(fmt.Sprintf("cf-mitigated: block (HTTP %d)", status))
		case "challenge":
			return crawlResponse{}, newCloudflareUnresolvedError(fmt.Sprintf("cf-mitigated: challenge (HTTP %d)", status), nil)
		}
	}

	// No wire verdict: the same page-marker probe the tracker uses, so a
	// real challenge/block page is never returned as real content.
	probeJS, err := buildChallengeProbeJS(expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	raw, err := rodEvalExpression(p, probeJS)
	if err != nil {
		return crawlResponse{}, rodCtxOr(ctx, fmt.Errorf("rod: challenge probe failed: %w", err))
	}
	var probe challengeProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return crawlResponse{}, fmt.Errorf("rod: challenge probe decode failed: %w", err)
	}
	if probe.BlockPage {
		return crawlResponse{}, newCloudflareBlockedError("cloudflare block page")
	}
	if probe.ChallengePage && !probe.Expected {
		return crawlResponse{}, newCloudflareUnresolvedError("cloudflare challenge page", nil)
	}

	// Only now — after every Cloudflare verdict — let the page finish
	// rendering, so the wait never delays a challenge or block report.
	if err := waitForDOMToSettle(ctx, p); err != nil {
		return crawlResponse{}, err
	}

	resp, err := readCrawlResponseWithRod(p, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	return resp, rodCtxOr(ctx, err)
}

// domSettleJS builds the expression behind the wait: a promise that
// resolves "stable" once the document has seen no DOM mutation (child
// list, attributes or text, anywhere in the tree) for quietMs, or
// "timeout" once maxMs have passed. Non-positive arguments fall back to
// the defaults so a bad value can never produce a wait that returns
// instantly or never.
func domSettleJS(quietMs, maxMs int) string {
	if quietMs <= 0 {
		quietMs = int(rodDOMQuiet / time.Millisecond)
	}
	if maxMs <= 0 {
		maxMs = int(rodDOMMaxWait / time.Millisecond)
	}
	return fmt.Sprintf(`(function(quiet, max) {
		return new Promise(function(resolve) {
			var done = false, quietTimer, capTimer, observer;
			function finish(reason) {
				if (done) return;
				done = true;
				clearTimeout(quietTimer);
				clearTimeout(capTimer);
				if (observer) observer.disconnect();
				resolve(reason);
			}
			function armQuiet() {
				clearTimeout(quietTimer);
				quietTimer = setTimeout(function() { finish("stable"); }, quiet);
			}
			capTimer = setTimeout(function() { finish("timeout"); }, max);
			if (typeof MutationObserver === "function" && document.documentElement) {
				observer = new MutationObserver(armQuiet);
				observer.observe(document.documentElement, {childList: true, subtree: true, attributes: true, characterData: true});
			}
			armQuiet();
		});
	})(%d, %d)`, quietMs, maxMs)
}

// waitForDOMToSettle runs the wait above on the page. It never fails the
// fetch by itself: a page that keeps changing simply hits the cap, and a
// page that navigates away mid-wait (a JS redirect destroying the
// execution context) is read as it is afterwards. Only the CALLER's own
// cancellation is returned, as ctx's error.
func waitForDOMToSettle(ctx context.Context, p *rod.Page) error {
	// The outer deadline is a safety net a little beyond the script's own
	// cap; p already carries the whole-fetch budget. Rod's Timeout returns
	// a NEW page (step 48's trap), used only for this one call.
	bounded := p.Timeout(rodDOMMaxWait + 2*time.Second)
	defer bounded.CancelTimeout() // releases the timer; bounded is not used afterwards
	_, err := rodEvalExpression(bounded, domSettleJS(int(rodDOMQuiet/time.Millisecond), int(rodDOMMaxWait/time.Millisecond)))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_ = err // see the doc comment: not a fetch failure
	return nil
}

// rodNavigationError turns a navigation failure into the right kind of
// error: the caller's own cancellation is reported as ctx's error (as
// classifyCancellation's convention elsewhere in this package expects);
// anything else is a plain, engine-specific error.
func rodNavigationError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("rod: navigation failed: %w", err)
}

// rodCtxOr prefers the caller's cancellation over whatever rod reported
// once ctx is done (an interrupted call surfaces as a vague rod error).
func rodCtxOr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
		return ctx.Err()
	}
	return err
}

// readCrawlResponseWithRod mirrors readCrawlResponse's (crawl.go) exact
// sequence so a caller sees identical behavior whichever engine served the
// page: title BEFORE element removal (removing <head> would blank it),
// then RemoveElementsJS, then the optional attribute pass, then the final
// URL and the <html> element's outer HTML (what chromedp's OuterHTML("html")
// returns), truncated at MaxHTMLBytes. The JS builders and constants are
// engine-agnostic and reused unchanged.
func readCrawlResponseWithRod(p *rod.Page, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	title, err := rodEvalString(p, `document.title`)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("rod: failed to read title: %w", err)
	}

	allRemoveSelectors := append(append([]string{}, DefaultRemoveSelectors...), removeSelectors...)
	script, err := RemoveElementsJS(allRemoveSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	if _, err := rodEvalExpression(p, script); err != nil {
		return crawlResponse{}, fmt.Errorf("rod: element removal failed: %w", err)
	}

	if len(removeAttributes) > 0 || maxAttributeLength > 0 {
		allIgnoreForMaxLength := append(append([]string{}, defaultMaxLengthIgnoredAttributes...), ignoreAttributesForMaxLength...)
		attrScript, err := removeAttributesJS(removeAttributes, maxAttributeLength, allIgnoreForMaxLength)
		if err != nil {
			return crawlResponse{}, err
		}
		if _, err := rodEvalExpression(p, attrScript); err != nil {
			return crawlResponse{}, fmt.Errorf("rod: attribute removal failed: %w", err)
		}
	}

	finalURL, err := rodEvalString(p, `document.location.toString()`)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("rod: failed to read the final URL: %w", err)
	}
	html, err := rodEvalString(p, `document.documentElement.outerHTML`)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("rod: failed to read HTML: %w", err)
	}

	truncated := false
	if len(html) > MaxHTMLBytes {
		html = html[:MaxHTMLBytes]
		truncated = true
	}
	return crawlResponse{HTML: html, Title: title, FinalURL: finalURL, Truncated: truncated}, nil
}
