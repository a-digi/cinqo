// cloudflare.go implements Cloudflare challenge-interstitial detection.
//
// The important distinction made here is:
//
//   1. Cloudflare assets being present does NOT necessarily mean a challenge
//      is currently being shown.
//   2. A challenge is considered active only when there is stronger evidence
//      that the user is actually looking at an interstitial.
//   3. Expected crawl content is treated as an authoritative "the real page
//      is here" signal.
//
// This is intentionally heuristic. Cloudflare can change its challenge DOM,
// titles, and wording at any time, so no individual selector/text check should
// be treated as a permanent API.
//
// The normal flow is:
//
//   navigate
//      |
//      +--> expected content already visible -> continue
//      |
//      +--> Cloudflare challenge detected
//                |
//                +--> wait briefly for automatic clearance
//                |
//                +--> cleared -> continue
//                |
//                +--> still present -> caller can hand the browser to user
//
// The fixed detector never uses caller-supplied arbitrary JavaScript.
// expectedSelectors are only used with querySelector(), consistent with the
// existing extraction trust boundary.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// cloudflarePollInterval/cloudflareMaxWait control the short automatic wait
// after a challenge is detected.
//
// This is intentionally separate from the longer human-resolution wait that
// a caller may perform after handing the browser to a user.
const (
	cloudflarePollInterval = 1 * time.Second
	cloudflareMaxWait      = 8 * time.Second
)

// cloudflareCheck is one point-in-time read of whether the currently loaded
// page looks like a Cloudflare challenge interstitial.
type cloudflareCheck struct {
	Detected bool   `json:"detected"`
	Reason   string `json:"reason"`
}

// isRenderedVisibleJS is shared by all DOM visibility checks.
//
// Presence in the DOM is deliberately not enough. Cloudflare/Turnstile may
// leave hidden iframes or elements in the DOM even after the challenge has
// been cleared.
const isRenderedVisibleJS = `
	function isRenderedVisible(el) {
		if (!el) return false;

		var style = window.getComputedStyle(el);

		if (
			style.display === "none" ||
			style.visibility === "hidden" ||
			parseFloat(style.opacity || "1") === 0
		) {
			return false;
		}

		var rect = el.getBoundingClientRect();

		return rect.width > 0 && rect.height > 0;
	}
`

// buildCloudflareDetectJS creates the fixed Cloudflare detection script.
//
// Detection intentionally follows this order:
//
//  1. The page title is a strong signal.
//  2. Known challenge DOM elements, but only when actually rendered.
//  3. Known challenge wording combined with Cloudflare evidence.
//  4. A visible Cloudflare/Turnstile widget combined with Cloudflare evidence.
//  5. A Cloudflare challenge-platform script combined with a very thin body.
//
// Cloudflare scripts and __CF$cv$params are NOT sufficient on their own.
//
// expectedSelectors provide an important override:
//
//	if the user's actual target content is visibly present, the page is not
//	considered blocked, even if a generic Cloudflare heuristic happens to
//	match.
func buildCloudflareDetectJS(expectedSelectors []string) (string, error) {
	selectorsJSON, err := json.Marshal(expectedSelectors)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`(function() {
		%s

		var detected = false;
		var reason = "";

		var title = (document.title || "").trim().toLowerCase();

		var bodyText = (
			(document.body && document.body.innerText) || ""
		).trim();

		var bodyTextLower = bodyText.toLowerCase();

		// ------------------------------------------------------------
		// 1. Strong title signals
		// ------------------------------------------------------------

		if (
			title === "just a moment..." ||
			title === "just a moment"
		) {
			detected = true;
			reason = "title";
		}

		// ------------------------------------------------------------
		// 2. Known Cloudflare challenge DOM
		//
		// These are deliberately checked for visibility rather than mere
		// presence. DOM remnants can survive after the challenge clears.
		// ------------------------------------------------------------

		if (!detected) {
			var challengeCandidates = document.querySelectorAll(
				[
					"#cf-challenge-running",
					".cf-browser-verification",
					"#challenge-stage",
					"#challenge-error-text"
				].join(",")
			);

			for (var i = 0; i < challengeCandidates.length; i++) {
				if (isRenderedVisible(challengeCandidates[i])) {
					detected = true;
					reason = "challenge-element";
					break;
				}
			}
		}

		// ------------------------------------------------------------
		// 3. Establish whether Cloudflare is actually involved.
		//
		// These signals are NOT themselves enough to declare a challenge.
		// They are corroborating evidence for the weaker checks below.
		// ------------------------------------------------------------

		var hasChallengeScript =
			!!document.querySelector(
				'script[src*="challenges.cloudflare.com"]'
			) ||
			!!document.querySelector(
				'script[src*="/cdn-cgi/challenge-platform/"]'
			);

		var hasCloudflareURL =
			(window.location.pathname || "").indexOf(
				"/cdn-cgi/challenge-platform/"
			) !== -1;

		var hasCloudflareEvidence =
			hasChallengeScript || hasCloudflareURL;

		// ------------------------------------------------------------
		// 4. Challenge wording.
		//
		// Text alone is not sufficient because normal sites can mention
		// Cloudflare/security verification in their own content.
		// Require Cloudflare evidence as well.
		// ------------------------------------------------------------

		if (!detected && hasCloudflareEvidence) {
			var challengeText =
				bodyTextLower.indexOf("checking your browser") !== -1 ||
				bodyTextLower.indexOf("checking if the site connection is secure") !== -1 ||
				bodyTextLower.indexOf("verify you are human") !== -1 ||
				bodyTextLower.indexOf("performing security verification") !== -1 ||
				bodyTextLower.indexOf("security verification") !== -1 ||
				bodyTextLower.indexOf("ray id") !== -1;

			if (challengeText) {
				detected = true;
				reason = "challenge-text";
			}
		}

		// ------------------------------------------------------------
		// 5. Visible Turnstile/challenge widget.
		//
		// Invisible Turnstile must NOT trigger detection merely because
		// its iframe exists.
		// ------------------------------------------------------------

		if (!detected && hasCloudflareEvidence) {
			var widgetCandidates = document.querySelectorAll(
				[
					'iframe[src*="challenges.cloudflare.com"]',
					'iframe[src*="turnstile"]',
					".cf-turnstile",
					"[class*='cf-turnstile']",
					"#challenge-stage"
				].join(",")
			);

			var visibleWidget = false;

			for (var j = 0; j < widgetCandidates.length; j++) {
				if (isRenderedVisible(widgetCandidates[j])) {
					visibleWidget = true;
					break;
				}
			}

			if (visibleWidget) {
				detected = true;
				reason = "visible-widget";
			}
		}

		// ------------------------------------------------------------
		// 6. Very small Cloudflare page.
		//
		// This is intentionally conservative. A challenge/block page often
		// has almost no text. A real application page normally has much more.
		//
		// Cloudflare evidence is required.
		// ------------------------------------------------------------

		if (!detected && hasCloudflareEvidence) {
			if (bodyText.length > 0 && bodyText.length < 200) {
				detected = true;
				reason = "thin-cloudflare-page";
			}
		}

		// ------------------------------------------------------------
		// Expected-content override.
		//
		// If the crawl's own target content is visibly present, consider
		// the page successfully loaded. This prevents generic Cloudflare
		// remnants from blocking an otherwise usable page.
		// ------------------------------------------------------------

		if (detected) {
			var expected = %s;

			if (expected && expected.length > 0) {
				for (var k = 0; k < expected.length; k++) {
					var el;

					try {
						el = document.querySelector(expected[k]);
					} catch (e) {
						continue;
					}

					if (el && isRenderedVisible(el)) {
						return {
							detected: false,
							reason: "expected-content-found"
						};
					}
				}
			}
		}

		return {
			detected: detected,
			reason: reason
		};
	})()`, isRenderedVisibleJS, string(selectorsJSON)), nil
}

// detectCloudflareChallenge runs the detection script against whatever page
// the supplied browser context currently has loaded.
//
// expectedSelectors may be nil/empty. When supplied, the selectors are only
// used to determine whether the crawl's expected target content is already
// visible.
func detectCloudflareChallenge(
	ctx context.Context,
	expectedSelectors []string,
) (cloudflareCheck, error) {
	script, err := buildCloudflareDetectJS(expectedSelectors)
	if err != nil {
		return cloudflareCheck{}, err
	}

	var result cloudflareCheck

	if err := chromedp.Run(
		ctx,
		chromedp.Evaluate(script, &result),
	); err != nil {
		return cloudflareCheck{}, err
	}

	return result, nil
}

// expectedContentVisible checks whether ANY expected selector currently
// matches a visibly rendered element.
//
// This is intentionally separate from detectCloudflareChallenge because it
// is useful after handing a challenge page to a human: the success condition
// is the application's real content appearing, not merely Cloudflare's
// challenge DOM disappearing.
func expectedContentVisible(
	ctx context.Context,
	expectedSelectors []string,
) (bool, error) {
	if len(expectedSelectors) == 0 {
		return false, nil
	}

	selectorsJSON, err := json.Marshal(expectedSelectors)
	if err != nil {
		return false, err
	}

	script := fmt.Sprintf(`(function(selectors) {
		%s

		for (var i = 0; i < selectors.length; i++) {
			var el;

			try {
				el = document.querySelector(selectors[i]);
			} catch (e) {
				continue;
			}

			if (el && isRenderedVisible(el)) {
				return true;
			}
		}

		return false;
	})(%s)`, isRenderedVisibleJS, string(selectorsJSON))

	var found bool

	if err := chromedp.Run(
		ctx,
		chromedp.Evaluate(script, &found),
	); err != nil {
		return false, err
	}

	return found, nil
}

// expectedSelectorsFromFields derives the "expected content" selector list
// from a crawl instruction's container/fields.
//
// When a container is present, it is the strongest single signal that the
// real page has loaded. Otherwise use each field's selector.
func expectedSelectorsFromFields(
	container string,
	fields []extractField,
) []string {
	if container != "" {
		return []string{container}
	}

	selectors := make([]string, 0, len(fields))

	for _, f := range fields {
		if strings.TrimSpace(f.Selector) != "" {
			selectors = append(selectors, f.Selector)
		}
	}

	return selectors
}

// waitForCloudflareClearance performs a short automatic wait.
//
// It checks immediately. For normal pages this therefore adds essentially
// zero latency.
//
// If a Cloudflare challenge is detected, it polls until:
//
//   - the challenge disappears, or
//   - cloudflareMaxWait is exhausted.
//
// A cleared challenge returns an empty cloudflareCheck. A still-active
// challenge is returned to the caller so it can hand the browser to a human
// or return a structured error.
func waitForCloudflareClearance(
	ctx context.Context,
	expectedSelectors []string,
) (cloudflareCheck, error) {
	check, err := detectCloudflareChallenge(ctx, expectedSelectors)
	if err != nil {
		return cloudflareCheck{}, err
	}

	if !check.Detected {
		return cloudflareCheck{}, nil
	}

	deadline := time.Now().Add(cloudflareMaxWait)

	for time.Now().Before(deadline) {
		if err := chromedp.Run(
			ctx,
			chromedp.Sleep(cloudflarePollInterval),
		); err != nil {
			return cloudflareCheck{}, err
		}

		check, err = detectCloudflareChallenge(
			ctx,
			expectedSelectors,
		)
		if err != nil {
			return cloudflareCheck{}, err
		}

		if !check.Detected {
			return cloudflareCheck{}, nil
		}
	}

	return check, nil
}

// crawlError is a distinct typed crawl failure.
//
// Today the Cloudflare-specific code is:
//
//	cloudflare_challenge_unresolved
//
// This allows callers to branch on Code rather than string-matching an error
// message.
type crawlError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

func (e *crawlError) Error() string {
	return e.Message
}

// newCloudflareUnresolvedError is returned when a Cloudflare challenge is
// still present after the automatic wait budget.
//
// The caller can use this signal to expose the existing browser to a user
// for manual completion.
func newCloudflareUnresolvedError(reason string) *crawlError {
	return &crawlError{
		Code:    "cloudflare_challenge_unresolved",
		Message: "Cloudflare challenge could not be cleared",
		Reason:  reason,
	}
}

// newSessionInterruptedError is returned when the shared browser session's
// root context was canceled while a crawl was running.
//
// This is distinct from an ordinary crawl timeout/failure because the
// underlying browser session may have been restarted and the operation can
// generally be retried.
func newSessionInterruptedError(cause error) *crawlError {
	reason := "context canceled"

	if cause != nil {
		reason = cause.Error()
	}

	return &crawlError{
		Code:    "browser_session_interrupted",
		Message: "The browser tool's session was interrupted (it may have restarted) while this crawl was running",
		Reason:  reason,
	}
}

// newCrawlCancelledError (step 63.3) is returned when a request's own
// per-request context (reqCtx — crawl_cancel.go, registered by
// crawlPage/crawlWithNormalSession/performPaginatedCrawl/
// performPaginatedCrawlWithNormalSession) was canceled via
// cancelCrawl(requestID) — a deliberate, user-requested stop (Career's
// own "Stop crawl" button), never a transient failure worth retrying.
func newCrawlCancelledError() *crawlError {
	return &crawlError{
		Code:    "crawl_cancelled_by_user",
		Message: "Crawl was stopped by the user",
	}
}

// classifyCancellation replaces this file's own former
// wrapIfSessionInterrupted (step 38, removed by step 63.3 — this
// function is a strict superset: with reqCtx never canceled, its
// behavior for a session-interrupted context.Canceled is identical) —
// a plain context.Canceled can now mean one of TWO materially different
// things once a per-request context exists alongside the shared
// session's own sessionCtx, and conflating them would make a
// deliberate Stop click look like a transient, auto-retried failure
// (exactly backwards from what a Stop button must do). Checks
// sessionCtx FIRST: a torn-down session also cancels every in-flight
// request's own reqCtx as a side effect (reqCtx's own linked ctx —
// see crawlPage's own doc comment — is ultimately still rooted through
// sessionCtx), so if the whole session died, that explanation wins
// regardless of reqCtx's own state; the two are not mutually
// exclusive, so order matters here.
//
// err is returned unchanged when it isn't context.Canceled at all
// (e.g. context.DeadlineExceeded, or any ordinary navigation/
// extraction failure) — this function only ever disambiguates
// cancellation, never anything else.
func classifyCancellation(err error, reqCtx, sessionCtx context.Context) error {
	if err == nil || !errors.Is(err, context.Canceled) {
		return err
	}
	if sessionCtx.Err() != nil {
		return newSessionInterruptedError(err)
	}
	if reqCtx.Err() != nil {
		return newCrawlCancelledError()
	}
	return err
}

// newSessionWedgedError (step 47.3) is returned when probeSessionLiveness
// (step 47.2, main.go) found the shared session's own tab unresponsive —
// almost always leftover state from an earlier, unrelated caller (a
// wedged renderer, an unhandled dialog that predates step 47.1's fix,
// or an interrupted navigation) rather than anything about THIS call's
// own request. recreateErr is the result of this process's own
// immediate attempt to replace the wedged tab with a fresh one
// (recreateSharedSessionLocked, main.go) — folded into Reason so a
// human reading logs/Career's own crawl_runs log can tell "wedged, but
// already fixed for the next attempt" apart from "wedged, and the
// automatic recovery itself failed too" (materially worse — worth
// escalating, not just retrying blindly).
//
// A distinct Code from browser_session_interrupted (step 38, above) —
// that one means the whole process was restarted; this one means the
// process is fine but this one tab needed replacing. Callers like
// Career's own retry logic can reasonably treat both as "transient,
// worth one retry," but they are not the same underlying event.
func newSessionWedgedError(probeErr, recreateErr error) *crawlError {
	if recreateErr != nil {
		return &crawlError{
			Code:    "browser_session_wedged",
			Message: "The browser session's tab was unresponsive and the automatic attempt to replace it also failed",
			Reason:  fmt.Sprintf("probe: %v; recreate: %v", probeErr, recreateErr),
		}
	}
	return &crawlError{
		Code:    "browser_session_wedged",
		Message: "The browser session's tab was unresponsive; it has been automatically replaced with a fresh one for the next attempt",
		Reason:  probeErr.Error(),
	}
}
