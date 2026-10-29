// human_assist.go (step 96) is how an AI call gets a person involved when a
// site shows a challenge only a person can clear. An agent's tool call cannot
// wait for a person (its host times out long before the up-to-30-minute human
// wait Career's own crawls use), so this does not block it: the call returns at
// once saying a window was opened for the user, and a background task keeps the
// window and waits. When the person clears it, the clearance is kept
// (shared/clearance.go) and the agent's retry passes. Career's own crawls are
// untouched: they already wait for a person in the same window.
//
// Guards, since this opens a window on the user's computer: one request per
// domain at a time, a cooldown after one ends without a clearance, one headed
// window at a time (the existing headed pool), and a setting to turn it off.
// Nothing here solves or evades the challenge: the person does. See
// plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.
package crawler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"browser-tool-backend/shared"
)

const (
	outcomeHumanRequested = "human_requested"
	codeHumanRequested    = "human_check_requested"
)

// humanRequestCooldown is how long, after a request ended without a clearance,
// before another window may be opened for the same domain.
var humanRequestCooldown = 10 * time.Minute

type humanRequestState string

const (
	humanWaiting humanRequestState = "waiting"
	humanCleared humanRequestState = "cleared"
	humanGaveUp  humanRequestState = "gave_up" // the wait ended with the challenge still there
	humanFailed  humanRequestState = "failed"  // no window could be opened, or another error
)

type humanRequest struct {
	state      humanRequestState
	requestID  string
	startedAt  time.Time
	finishedAt time.Time
	detail     string
}

var (
	humanAssistMu sync.Mutex
	humanRequests = map[string]*humanRequest{}
	humanNow      = time.Now
	// launchHumanClearance opens the window and waits for the person. A var so
	// tests can stand in for the person.
	launchHumanClearance = realLaunchHumanClearance
)

// humanAssistEnabled reads the setting fresh (a live toggle).
func humanAssistEnabled() bool {
	if shared.DB == nil {
		return false
	}
	s, err := shared.LoadBrowserSettings()
	return err == nil && s.HumanAssist
}

func adviceHumanRequested(waiting bool) string {
	if waiting {
		return "The user was already asked to clear this site and has not done it yet; a browser window is waiting for them. Do not retry now. Tell the user that this site needs their check, and retry later."
	}
	return "A browser window was opened on the computer where Cinqo runs so the user can clear this site. Do not retry for about two minutes; once the user has cleared it, calls to this site work again. If it still fails after that, tell the user."
}

func newHumanRequestedError(reason string, waiting bool) *crawlError {
	return &crawlError{
		Code:    codeHumanRequested,
		Message: "A person has been asked to clear this site's check",
		Reason:  reason,
		Outcome: outcomeHumanRequested,
		Advice:  adviceHumanRequested(waiting),
	}
}

// humanAssistedError is the hook for AI calls (an empty requestID). Given the
// error a call ended with, it returns it unchanged unless it is an unresolved
// Cloudflare challenge on a domain where a person can be asked: then it starts
// (or reports) the request and returns the human-requested error instead.
func humanAssistedError(rawURL, requestID string, err error) error {
	var cfErr *crawlError
	if requestID != "" || err == nil || !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
		return err
	}
	if !humanAssistEnabled() {
		return err
	}
	domain, derr := hostnameOf(rawURL)
	if derr != nil || domain == "" {
		return err
	}

	humanAssistMu.Lock()
	req := humanRequests[domain]
	now := humanNow()
	switch {
	case req != nil && req.state == humanWaiting:
		humanAssistMu.Unlock()
		return newHumanRequestedError(cfErr.Reason, true)
	case req != nil && now.Sub(req.finishedAt) < humanRequestCooldown:
		humanAssistMu.Unlock()
		// A window was offered a moment ago and it ended without a clearance:
		// do not open another; say so, keeping the original code.
		out := *cfErr
		out.Advice = "The user was asked to clear this site a moment ago and it did not work out. Do not retry; tell the user, or use another source."
		return &out
	}
	beginHumanRequestLocked(domain, rawURL, now)
	humanAssistMu.Unlock()
	return newHumanRequestedError(cfErr.Reason, false)
}

// beginHumanRequestLocked registers a new request for domain and starts it in
// the background. The caller holds humanAssistMu and has already decided that
// a request may start. Returns the request id.
func beginHumanRequestLocked(domain, rawURL string, now time.Time) string {
	reqID := fmt.Sprintf("human-%s-%d", domain, now.UnixNano())
	humanRequests[domain] = &humanRequest{state: humanWaiting, requestID: reqID, startedAt: now}
	// Registered before the goroutine starts, so a cancel arriving right away
	// (cancelCrawl on this request id) always finds it.
	ctx, cancel := context.WithTimeout(context.Background(), maxHumanSolveDuration+2*time.Minute)
	registerCrawlCancel(reqID, cancel)
	go runHumanRequest(ctx, cancel, domain, rawURL, reqID)
	return reqID
}

// startHumanRequestNow is the person's own "Clear now" (the Domains page): it
// ignores the setting and the cooldown, which exist to stop an agent from
// opening windows, not the user. A request already waiting for this domain is
// reused. Returns the request id and whether one was already waiting.
func startHumanRequestNow(domain, rawURL string) (string, bool) {
	humanAssistMu.Lock()
	defer humanAssistMu.Unlock()
	if r := humanRequests[domain]; r != nil && r.state == humanWaiting {
		return r.requestID, true
	}
	return beginHumanRequestLocked(domain, rawURL, humanNow()), false
}

// runHumanRequest waits for the person in the background and records how it
// ended.
func runHumanRequest(ctx context.Context, cancel context.CancelFunc, domain, rawURL, requestID string) {
	defer cancel()
	defer unregisterCrawlCancel(requestID)

	err := launchHumanClearance(ctx, rawURL, requestID)

	state, detail := humanCleared, ""
	var cfErr *crawlError
	switch {
	case err == nil:
	case errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved:
		state, detail = humanGaveUp, "the challenge was still there when the wait ended"
	default:
		state, detail = humanFailed, err.Error()
	}
	humanAssistMu.Lock()
	if r := humanRequests[domain]; r != nil && r.requestID == requestID {
		r.state, r.finishedAt, r.detail = state, humanNow(), detail
	}
	humanAssistMu.Unlock()
}

// realLaunchHumanClearance is the headed window with the human wait, exactly
// the one Career's own crawls use, serialized with them per domain. Its
// clearance is kept by importHeadedCookies on success.
func realLaunchHumanClearance(ctx context.Context, rawURL, requestID string) error {
	domain, _ := hostnameOf(rawURL)
	unlock, _, err := lockDomain(ctx, domain)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = crawlWithNormalSession(ctx, rawURL, requestID, nil, nil, nil, 0, nil)
	return err
}

// resetHumanRequests clears the bookkeeping (tests).
func resetHumanRequests() {
	humanAssistMu.Lock()
	defer humanAssistMu.Unlock()
	humanRequests = map[string]*humanRequest{}
}

// humanRequestSummary describes a domain's request for diagnostics.
func humanRequestSummary(domain string) string {
	humanAssistMu.Lock()
	defer humanAssistMu.Unlock()
	r := humanRequests[strings.ToLower(domain)]
	if r == nil {
		return ""
	}
	return string(r.state)
}
