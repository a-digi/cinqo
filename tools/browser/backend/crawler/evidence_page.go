// evidence_page.go (Namako step 1) — the one way the evidence routes (/fetch-frames, /capture-network, /page-archive) open a page: a Rod page
// from the shared Rod engine, with the Network domain on BEFORE the navigation so no request is missed, the same Cloudflare verdicts as
// fetchWithRod (a challenge or block page is reported, never returned as the real page), and everything closed again when the call ends.
// The routes are read-only: nothing is typed, submitted or clicked except the one fixed play script of /capture-network.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// evidencePage is an open, loaded page. Close it exactly once.
type evidencePage struct {
	P       *rod.Page // bound to the call's context and budget
	page    *rod.Page // the original page (closed at the end, never a cancelled clone)
	main    *mainDocumentResponse
	release func()
	stop    context.CancelFunc
}

func (e *evidencePage) Close() {
	e.stop()
	_ = e.page.Close()
	e.release()
}

// openEvidencePage opens rawURL and waits for it to load and settle. handlers are extra CDP event handlers (func(*proto.NetworkRequestWillBeSent) …)
// that are subscribed before the navigation. The caller has already validated the URL with validateCrawlURL.
func openEvidencePage(ctx context.Context, rawURL, requestID string, handlers ...any) (*evidencePage, error) {
	return openEvidencePageFrom(ctx, rawURL, requestID, "", handlers...)
}

// openEvidencePageFrom is openEvidencePage that navigates with a referrer (the page the address was found on): an embed player often plays only when its parent page
// is the referrer. An empty referrer is a plain navigation.
func openEvidencePageFrom(ctx context.Context, rawURL, requestID, referer string, handlers ...any) (*evidencePage, error) {
	slots := rodPageSlots()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-slots }
	fail := func(err error) (*evidencePage, error) {
		release()
		return nil, err
	}

	browser, err := ensureRodEngine()
	if err != nil {
		return fail(fmt.Errorf("rod engine unavailable: %w", err))
	}
	page, err := stealth.Page(browser)
	if err != nil {
		return fail(fmt.Errorf("rod: failed to open page: %w", err))
	}
	p := page.Context(ctx).Timeout(crawlTimeout)

	evCtx, stopEvents := context.WithCancel(ctx)
	closeAll := func() {
		stopEvents()
		_ = page.Close()
		release()
	}
	main := &mainDocumentResponse{}
	mainFrame := proto.PageFrameID(page.TargetID)
	all := []any{
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false}.Call(page)
		},
		func(e *proto.NetworkResponseReceived) {
			if e.Type == proto.NetworkResourceTypeDocument && e.FrameID == mainFrame && e.Response != nil {
				main.record(e.Response.Status, e.Response.Headers)
			}
		},
	}
	all = append(all, handlers...)
	// Subscribed here, synchronously, before navigating; only the returned wait function runs in the goroutine.
	go page.Context(evCtx).EachEvent(all...)()
	if err := (proto.NetworkEnable{}).Call(p); err != nil {
		closeAll()
		return nil, fmt.Errorf("rod: could not watch the network: %w", err)
	}

	seedStoredClearanceRod(p, rawURL)
	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL+" (evidence)")
	if err := navigateWithReferrer(p, rawURL, referer); err != nil {
		closeAll()
		return nil, rodNavigationError(ctx, err)
	}
	if err := p.WaitLoad(); err != nil && loadWaitIsFatal(ctx, err) {
		closeAll()
		return nil, rodNavigationError(ctx, err)
	}
	if err := evidenceCloudflareVerdict(ctx, p, main); err != nil {
		closeAll()
		return nil, err
	}
	if err := waitForDOMToSettle(ctx, p); err != nil {
		closeAll()
		return nil, err
	}
	return &evidencePage{P: p, page: page, main: main, release: release, stop: stopEvents}, nil
}

// evidenceCloudflareVerdict is the same verdict fetchWithRod gives, so a challenge page is never taken for the page.
func evidenceCloudflareVerdict(ctx context.Context, p *rod.Page, main *mainDocumentResponse) error {
	if value, status := main.header("cf-mitigated"); value != "" {
		switch strings.ToLower(value) {
		case "block":
			return newCloudflareBlockedError(fmt.Sprintf("cf-mitigated: block (HTTP %d)", status))
		case "challenge":
			return newCloudflareUnresolvedError(fmt.Sprintf("cf-mitigated: challenge (HTTP %d)", status), nil)
		}
	}
	probeJS, err := buildChallengeProbeJS(nil)
	if err != nil {
		return err
	}
	raw, err := rodEvalExpression(p, probeJS)
	if err != nil {
		return rodCtxOr(ctx, fmt.Errorf("rod: challenge probe failed: %w", err))
	}
	var probe challengeProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("rod: challenge probe decode failed: %w", err)
	}
	if probe.BlockPage {
		return newCloudflareBlockedError("cloudflare block page")
	}
	if probe.ChallengePage && !probe.Expected {
		return newCloudflareUnresolvedError("cloudflare challenge page", nil)
	}
	status, headers := main.statusAndHeaders()
	// accessErrorFromSingleShot returns a *crawlError: a nil one must not become a non-nil error value.
	if cfErr := accessErrorFromSingleShot(status, headers, probe); cfErr != nil {
		return cfErr
	}
	return nil
}

// loadWaitIsFatal says whether a failed "wait for the load event" ends the visit. Some sites make Chrome answer the wait's own script with "Object reference chain is
// too long" (the script's result cannot be serialised on that page): the page was navigated to and is there, so it is read as it is, like a page that never fires
// its load event. Anything else (the caller's cancellation, a dead tab) still ends the visit.
func loadWaitIsFatal(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return !strings.Contains(err.Error(), "Object reference chain is too long")
}

// navigateWithReferrer is rod's Navigate that can send a referrer. The referrer is the address the page being opened was found on, never a header a caller makes up.
func navigateWithReferrer(p *rod.Page, rawURL, referer string) error {
	if referer == "" {
		return p.Navigate(rawURL)
	}
	res, err := proto.PageNavigate{URL: rawURL, Referrer: referer}.Call(p)
	if err != nil {
		return err
	}
	if res.ErrorText != "" {
		return &rod.NavigationError{Reason: res.ErrorText}
	}
	return nil
}
