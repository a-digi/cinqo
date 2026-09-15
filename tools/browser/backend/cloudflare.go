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
// markup varies). DOM/title signatures only — does not inspect HTTP
// response headers (cf-mitigated, server: cloudflare), which would
// require chromedp's Network domain event interception; see this step's
// own open question 3 for what that gap means in practice.
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
