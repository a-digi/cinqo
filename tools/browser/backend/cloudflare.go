// cloudflare.go implements Cloudflare challenge-interstitial detection —
// a real, common failure mode for any headless-Chrome crawler:
// crawlPage's own fixed settleDelay (crawl.go) is often not long enough
// for Cloudflare's own JS challenge ("Just a moment...") to resolve and
// redirect to the real page, so fetch_page_html silently returns
// challenge-page HTML with no signal anywhere that Cloudflare,
// specifically, is why extraction downstream finds nothing. See
// plan/ai/tools/browser/step-21-cloudflare-challenge-detection.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// cloudflarePollInterval/cloudflareMaxWait — after the existing
// settleDelay (crawl.go), if detectCloudflareChallenge reports Detected,
// poll again every cloudflarePollInterval until either it clears or
// cloudflareMaxWait is exhausted. Cloudflare's own JS challenge
// typically resolves in ~4-5s; 8s of extra budget (up to 9 checks) gives
// real margin without materially risking crawlTimeout (20s total —
// 1.5s settle + up to 8s of this wait leaves ~10.5s of navigation/read
// margin, unchanged from today's already-comfortable slack). Untested
// against a real Cloudflare-gated site from inside this development
// environment — a starting estimate, flagged as open question 4 in this
// step's own design doc, worth tuning once cloudflareDetected/
// cloudflareReason (crawl.go/paginate.go) provide real feedback.
const (
	cloudflarePollInterval = 1 * time.Second
	cloudflareMaxWait      = 8 * time.Second
)

// cloudflareCheck is one point-in-time read of whether the currently
// loaded page looks like a Cloudflare challenge interstitial.
type cloudflareCheck struct {
	Detected bool
	Reason   string // "title" | "challenge-element" | "turnstile-script"
}

// cloudflareDetectJS is a small, fixed (never AI-supplied) script —
// strictly less powerful than extract.go's own arbitrary caller-supplied
// CSS selectors, which this tool already evaluates today. Checks, in
// order: document.title === "Just a moment..." (the stable,
// version-independent title Cloudflare's own JS challenge page has used
// for years); a known challenge element id/class — both of these are
// genuinely removed/replaced once a challenge is solved (Cloudflare
// swaps the whole interstitial DOM out or navigates away), so presence
// alone is reliable evidence a challenge is CURRENTLY showing.
//
// A <script src="*challenges.cloudflare.com*"> or
// "*/cdn-cgi/challenge-platform/*"> tag, and the global
// window.__CF$cv$params, are a DIFFERENT kind of signal — verified
// directly (not assumed), and a real bug fixed here: a <script> tag is
// never removed from the DOM once the browser has executed it, and
// window.__CF$cv$params is set once at page load and never unset —
// neither is retroactively cleaned up just because a challenge was
// solved. A live user-reported false positive confirmed this exactly:
// the real page and URL were visibly correct in the browser after
// solving a challenge, yet this check kept reporting "still detected"
// indefinitely, because the zone's own Cloudflare bot-management
// script (present on every page it serves, cleared or not) never
// leaves the DOM. These two signals were originally added to catch
// Cloudflare's WAF "Request Blocked" interstitial — a harder,
// non-resolving block with no title/element markers of its own — so
// they can't simply be removed; instead, each now requires a SECOND,
// independent corroborating signal that a challenge is actually being
// shown right now: either a visibly rendered challenge widget element,
// or a suspiciously thin page body (a real block/challenge page has
// almost no real content; any genuine page — a job listing, an
// article — does). See
// plan/ai/tools/browser/step-33-fix-persistent-script-tag-false-positive.md.
//
// A second, real false positive found on the FIRST version of the
// widget check, live-reported with the exact reason it produced
// ("challenge-platform-script") on a page confirmed to show no
// challenge at all: `document.querySelector('iframe[src*="..."]')`
// only tests DOM *presence*, and Cloudflare's own Turnstile can run in
// "invisible" mode — a real `<iframe>` permanently present for
// background bot-scoring, deliberately kept hidden
// (`display:none`/zero-size) unless an interactive challenge is
// actually needed. isRenderedVisibleJS below checks genuine on-screen
// rendering (computed display/visibility/opacity plus a non-zero
// bounding box), not mere presence — the same "presence isn't the same
// as active" lesson `step-33` already applied to the script tag/global
// variable checks, applied here to the widget element check too. See
// plan/ai/tools/browser/step-35-fix-hidden-turnstile-widget-false-positive.md.
//
// isRenderedVisibleJS is a shared JS function body — both the forward
// override embedded in buildCloudflareDetectJS below and the standalone
// expectedContentVisible (the reverse-direction check,
// waitForHumanToClearCloudflare) depend on it, so both directions agree
// on exactly what "visible" means. See
// plan/ai/tools/browser/step-36-content-based-cloudflare-override.md.
const isRenderedVisibleJS = `
	function isRenderedVisible(el) {
		if (!el) return false;
		var style = window.getComputedStyle(el);
		if (style.display === "none" || style.visibility === "hidden" || parseFloat(style.opacity) === 0) {
			return false;
		}
		var rect = el.getBoundingClientRect();
		return rect.width > 0 && rect.height > 0;
	}
`

// buildCloudflareDetectJS renders the fixed detection script plus one
// caller-scoped addition (step 36): when expectedSelectors is
// non-empty and the fixed heuristic below would otherwise report
// Detected, first check whether the crawl's OWN target content is
// already visibly present — if so, override to not-detected. Computed
// AFTER the full heuristic result (not short-circuited per-signal), so
// this applies uniformly regardless of which of the five signals
// fired, matching this step's own "if the real target content is
// there, it does not matter why the generic heuristic still fires"
// reasoning. expectedSelectors is marshaled to JSON and embedded
// directly — caller-supplied (ultimately an AI-authored, already
// shape-validated crawl instruction) but only ever used as a
// querySelector argument, the same trust boundary extract.go's own
// arbitrary selectors already cross.
func buildCloudflareDetectJS(expectedSelectors []string) (string, error) {
	selectorsJSON, err := json.Marshal(expectedSelectors)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(function() {
	%s
	var detected = false;
	var reason = "";

	if (document.title === "Just a moment...") {
		detected = true;
		reason = "title";
	} else if (document.getElementById("cf-challenge-running") ||
		document.querySelector(".cf-browser-verification") ||
		document.getElementById("cf-wrapper")) {
		detected = true;
		reason = "challenge-element";
	} else {
		var hasChallengeScript = !!document.querySelector('script[src*="challenges.cloudflare.com"]') ||
			!!document.querySelector('script[src*="/cdn-cgi/challenge-platform/"]');
		var hasWafParams = typeof window.__CF$cv$params !== "undefined";
		if (hasChallengeScript || hasWafParams) {
			var widgetCandidates = document.querySelectorAll(
				'iframe[src*="challenges.cloudflare.com"], #challenge-stage, [class*="cf-turnstile"]'
			);
			var visibleWidget = false;
			for (var i = 0; i < widgetCandidates.length; i++) {
				if (isRenderedVisible(widgetCandidates[i])) {
					visibleWidget = true;
					break;
				}
			}
			var bodyText = ((document.body && document.body.innerText) || '').trim();
			if (visibleWidget || bodyText.length < 200) {
				detected = true;
				reason = hasWafParams ? "waf-block-params" : "challenge-platform-script";
			}
		}
	}

	if (detected) {
		var expected = %s;
		if (expected && expected.length > 0) {
			for (var j = 0; j < expected.length; j++) {
				var el;
				try { el = document.querySelector(expected[j]); } catch (e) { continue; }
				if (el && isRenderedVisible(el)) {
					return {detected: false, reason: "expected-content-found"};
				}
			}
		}
	}

	return {detected: detected, reason: reason};
})()`, isRenderedVisibleJS, string(selectorsJSON)), nil
}

// detectCloudflareChallenge runs the detection script (with
// expectedSelectors' own forward override baked in, step 36) against
// whatever page ctx's own session currently has loaded. expectedSelectors
// may be nil/empty — every AI-driven caller (fetch_page_html,
// crawl_paginated with no container/fields context yet) passes nothing,
// identical to this function's own pre-step-36 behavior.
func detectCloudflareChallenge(ctx context.Context, expectedSelectors []string) (cloudflareCheck, error) {
	script, err := buildCloudflareDetectJS(expectedSelectors)
	if err != nil {
		return cloudflareCheck{}, err
	}
	var result cloudflareCheck
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
		return cloudflareCheck{}, err
	}
	return result, nil
}

// expectedContentVisible checks whether ANY of expectedSelectors
// currently matches a visibly rendered element — the reverse-direction
// check (step 36), used only by waitForHumanToClearCloudflare's own
// retry loop when the heuristic itself reports clear but the crawl's
// own target content still isn't confirmed present. Shares
// isRenderedVisibleJS with the forward override so both directions
// agree on what "visible" means.
func expectedContentVisible(ctx context.Context, expectedSelectors []string) (bool, error) {
	selectorsJSON, err := json.Marshal(expectedSelectors)
	if err != nil {
		return false, err
	}
	script := fmt.Sprintf(`(function(selectors) {
	%s
	for (var i = 0; i < selectors.length; i++) {
		var el;
		try { el = document.querySelector(selectors[i]); } catch (e) { continue; }
		if (el && isRenderedVisible(el)) return true;
	}
	return false;
})(%s)`, isRenderedVisibleJS, string(selectorsJSON))

	var found bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &found)); err != nil {
		return false, err
	}
	return found, nil
}

// expectedSelectorsFromFields derives the "expected content" selector
// list (step 36) from a crawl instruction's own container/fields — a
// single strong "does the repeating item wrapper exist" signal when a
// container is set (step 18), rather than several narrower selectors
// only meaningful nested inside it; otherwise every field's own
// selector. Shared by both this tool's own /crawl-paginated path
// (paginate.go, which already has container/fields in scope at every
// relevant call site) and Career's own crawl_now.go (which derives the
// same list from its own, independently-declared crawlRequest shape).
func expectedSelectorsFromFields(container string, fields []extractField) []string {
	if container != "" {
		return []string{container}
	}
	selectors := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Selector != "" {
			selectors = append(selectors, f.Selector)
		}
	}
	return selectors
}

// waitForCloudflareClearance checks once immediately; if clear, returns
// {false, "", nil} right away — zero added latency for the overwhelming
// majority of (non-Cloudflare) pages. If detected, polls up to
// cloudflareMaxWait; returns {false, "", nil} the moment it clears
// (proceed normally — the real content is now loaded), or
// {true, reason, nil} if still present once the budget is exhausted
// (give up; caller proceeds anyway with whatever HTML/DOM exists, now
// correctly flagged). expectedSelectors (step 36) only ever feeds the
// forward override inside detectCloudflareChallenge here — this
// function's own short automated wait deliberately does NOT apply the
// reverse direction (see waitForHumanToClearCloudflare's own doc
// comment for why that's scoped narrower).
func waitForCloudflareClearance(ctx context.Context, expectedSelectors []string) (cloudflareCheck, error) {
	check, err := detectCloudflareChallenge(ctx, expectedSelectors)
	if err != nil {
		return cloudflareCheck{}, err
	}
	if !check.Detected {
		return cloudflareCheck{}, nil
	}

	deadline := time.Now().Add(cloudflareMaxWait)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(ctx, chromedp.Sleep(cloudflarePollInterval)); err != nil {
			return cloudflareCheck{}, err
		}
		check, err = detectCloudflareChallenge(ctx, expectedSelectors)
		if err != nil {
			return cloudflareCheck{}, err
		}
		if !check.Detected {
			return cloudflareCheck{}, nil
		}
	}

	return check, nil
}

// crawlError is a distinct, typed crawl failure — today only "the
// Cloudflare challenge could not be cleared" — that crawlHandler and
// paginatedCrawlHandler (crawl.go, paginate.go) surface as structured
// JSON rather than the plain-text 502 body every other crawl failure
// still gets, so a caller (Career's own deterministic "Crawl now") can
// branch on Code instead of string-matching an error message.
type crawlError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

func (e *crawlError) Error() string { return e.Message }

// newCloudflareUnresolvedError is returned when a Cloudflare challenge
// was still present after waitForCloudflareClearance's own wait budget
// ran out (or, on the shorter per-page pagination path, detected at
// all — see paginate.go) — i.e. the browser could not overcome it, as
// opposed to step 21's original "detected but cleared in time" case,
// which is not an error at all.
func newCloudflareUnresolvedError(reason string) *crawlError {
	return &crawlError{
		Code:    "cloudflare_challenge_unresolved",
		Message: "Cloudflare challenge could not be cleared",
		Reason:  reason,
	}
}

// newSessionInterruptedError is returned when the shared browser
// session's own root context (sessionCtx, main.go) was canceled out
// from under an in-flight crawl — i.e. this process's own SIGTERM
// handler ran (stopSharedSession), because the manager restarted,
// stopped, or crash-recovered this tool's subprocess mid-crawl. A
// distinct Code lets a caller (Career's own "Crawl now") tell this
// apart from every other failure and retry automatically, since the
// underlying cause is almost always transient (the tool relaunches
// within seconds) — unlike a genuine navigation/extraction failure,
// which retrying would not fix. See
// plan/ai/tools/browser/step-38-session-interrupted-retry.md.
func newSessionInterruptedError(cause error) *crawlError {
	return &crawlError{
		Code:    "browser_session_interrupted",
		Message: "The browser tool's session was interrupted (it may have restarted) while this crawl was running",
		Reason:  cause.Error(),
	}
}

// wrapIfSessionInterrupted turns a raw context.Canceled — the shared
// session's own root context (sessionCtx) was torn down mid-crawl,
// never anything else on this specific derived ctx — into the
// distinct, structured crawlError above, so callers (crawlHandler,
// paginatedCrawlHandler) return it as JSON like any other crawlError
// instead of a generic plain-text 502. A no-op for every other error,
// including context.DeadlineExceeded (an ordinary, already-handled
// budget expiry, not a torn-down session).
func wrapIfSessionInterrupted(err error) error {
	if err != nil && errors.Is(err, context.Canceled) {
		return newSessionInterruptedError(err)
	}
	return err
}
