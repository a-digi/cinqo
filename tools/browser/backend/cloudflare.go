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
// for years); a known challenge element id/class; a
// <script src="*challenges.cloudflare.com*"> tag (covers the
// Turnstile-based challenge even when the page's own title/wrapper
// markup varies); a <script src="*/cdn-cgi/challenge-platform/*"> tag
// and a global window.__CF$cv$params (both cover Cloudflare's WAF
// "Request Blocked" interstitial — a harder, non-resolving block
// distinct from the resolvable JS challenge the first three signatures
// target, but still "a Cloudflare challenge the browser could not
// overcome" from a caller's point of view). DOM/title signatures only —
// does not inspect HTTP response headers (cf-mitigated, server:
// cloudflare), which would require chromedp's Network domain event
// interception; see this step's own open question 3 for what that gap
// means in practice.
const cloudflareDetectJS = `(function() {
	if (document.title === "Just a moment...") {
		return {detected: true, reason: "title"};
	}
	if (document.getElementById("cf-challenge-running") ||
		document.querySelector(".cf-browser-verification") ||
		document.getElementById("cf-wrapper")) {
		return {detected: true, reason: "challenge-element"};
	}
	if (document.querySelector('script[src*="challenges.cloudflare.com"]')) {
		return {detected: true, reason: "turnstile-script"};
	}
	if (document.querySelector('script[src*="/cdn-cgi/challenge-platform/"]')) {
		return {detected: true, reason: "challenge-platform-script"};
	}
	if (typeof window.__CF$cv$params !== "undefined") {
		return {detected: true, reason: "waf-block-params"};
	}
	return {detected: false, reason: ""};
})()`

// detectCloudflareChallenge runs cloudflareDetectJS against whatever
// page ctx's own session currently has loaded.
func detectCloudflareChallenge(ctx context.Context) (cloudflareCheck, error) {
	var result cloudflareCheck
	if err := chromedp.Run(ctx, chromedp.Evaluate(cloudflareDetectJS, &result)); err != nil {
		return cloudflareCheck{}, err
	}
	return result, nil
}

// waitForCloudflareClearance checks once immediately; if clear, returns
// {false, "", nil} right away — zero added latency for the overwhelming
// majority of (non-Cloudflare) pages. If detected, polls up to
// cloudflareMaxWait; returns {false, "", nil} the moment it clears
// (proceed normally — the real content is now loaded), or
// {true, reason, nil} if still present once the budget is exhausted
// (give up; caller proceeds anyway with whatever HTML/DOM exists, now
// correctly flagged).
func waitForCloudflareClearance(ctx context.Context) (cloudflareCheck, error) {
	check, err := detectCloudflareChallenge(ctx)
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
		check, err = detectCloudflareChallenge(ctx)
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
