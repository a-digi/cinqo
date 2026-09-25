// cloudflare.go holds the Cloudflare-related error types and the page
// helpers shared by challenge_resolution.go, which decides whether a page
// is behind a challenge from what Cloudflare sends (`cf-mitigated`,
// `cf_clearance`) — see that file's own top comment.
//
// expectedSelectors are only ever used with querySelector(), consistent
// with the existing extraction trust boundary.

package crawler

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

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

// challengeWidgetSelectorsJS lists Turnstile/challenge widget elements.
// Used only by the human wait's stalled-widget recovery on a page that
// is already known to be a challenge (challengeProbe.WidgetVisible) —
// never to decide whether a page IS a challenge.
const challengeWidgetSelectorsJS = `[
	'iframe[src*="challenges.cloudflare.com"]',
	'iframe[src*="turnstile"]',
	".cf-turnstile",
	"[class*='cf-turnstile']",
	"#challenge-stage"
].join(",")`

// challengeContainerSelectorsJS is the SUBSET of challengeWidgetSelectorsJS
// that identifies the widget's own CONTAINER element (the div Cloudflare's
// script injects its interactive iframe into) — deliberately excludes the
// two iframe-src selectors from that list, since a container is what
// challengeProbe.WidgetStalled (challenge_resolution.go) checks for a
// MISSING iframe child inside, and an iframe element can never itself be
// that container.
const challengeContainerSelectorsJS = `[
	".cf-turnstile",
	"[class*='cf-turnstile']",
	"#challenge-stage"
].join(",")`

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

// crawlError is a distinct typed crawl failure.
//
// The Cloudflare-specific codes are:
//
//	cloudflare_challenge_unresolved — still present after the wait budget
//	cloudflare_blocked              — a block page nobody can get past
//
// This allows callers to branch on Code rather than string-matching an error
// message.
type crawlError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
	// tracker (only meaningful for cloudflare_challenge_unresolved)
	// carries the detection-time baseline and network signals from the
	// automatic wait into the human wait for the SAME tab — see
	// challengeTracker (challenge_resolution.go). nil when the error
	// wasn't produced by a live detection (the known-Cloudflare-domain
	// shortcut); the human wait then starts a fresh tracker.
	tracker *challengeTracker
}

func (e *crawlError) Error() string {
	return e.Message
}

// newCloudflareUnresolvedError is returned when a Cloudflare challenge is
// still present after the automatic wait budget.
//
// The caller can use this signal to expose the existing browser to a user
// for manual completion.
func newCloudflareUnresolvedError(reason string, tracker *challengeTracker) *crawlError {
	return &crawlError{
		Code:    codeCloudflareUnresolved,
		Message: "Cloudflare challenge could not be cleared",
		Reason:  reason,
		tracker: tracker,
	}
}

const (
	codeCloudflareUnresolved = "cloudflare_challenge_unresolved"
	codeCloudflareBlocked    = "cloudflare_blocked"
)

// newCloudflareBlockedError is returned the moment a Cloudflare BLOCK
// page is recognized (challengeProbe.HardBlocked, or a `cf-mitigated:
// block` response) — distinct from an unresolved challenge because no
// wait, headed retry, or human can get past it: callers never route it
// to the headed fallback or the human wait.
func newCloudflareBlockedError(reason string) *crawlError {
	return &crawlError{
		Code:    codeCloudflareBlocked,
		Message: "Cloudflare blocked access to this page",
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
// session's own shared.Ctx, and conflating them would make a
// deliberate Stop click look like a transient, auto-retried failure
// (exactly backwards from what a Stop button must do). Checks
// shared.Ctx FIRST: a torn-down session also cancels every in-flight
// request's own reqCtx as a side effect (reqCtx's own linked ctx —
// see crawlPage's own doc comment — is ultimately still rooted through
// shared.Ctx), so if the whole session died, that explanation wins
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

// NewSessionWedgedError (step 47.3) is returned when shared.ProbeSessionLiveness
// (step 47.2, main.go) found the shared session's own tab unresponsive —
// almost always leftover state from an earlier, unrelated caller (a
// wedged renderer, an unhandled dialog that predates step 47.1's fix,
// or an interrupted navigation) rather than anything about THIS call's
// own request. recreateErr is the result of this process's own
// immediate attempt to replace the wedged tab with a fresh one
// (shared.RecreateSharedSessionLocked, main.go) — folded into Reason so a
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
func NewSessionWedgedError(probeErr, recreateErr error) *crawlError {
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
