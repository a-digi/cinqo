// challenge_resolution.go decides whether a page is behind a Cloudflare
// challenge, and when that challenge has cleared — including a SILENT
// approval, where no person does anything.
//
// It trusts what Cloudflare itself sends, not what the page looks like.
// Every earlier version guessed from the page — the "Just a moment"
// title, challenge wording, a visible Turnstile widget, a thin body,
// HTML growth — and each guess failed on a real site: a job-listing
// page's own search-form widget was taken for a challenge (an AI call
// waited for a solve nobody needed, and timed out), and a German
// challenge page ("Nur einen Moment…") was returned to an AI as the
// page's real content.
//
// What Cloudflare sends, verified on the wire (duapune.com, de.indeed.com):
//
//   - A challenge response carries `cf-mitigated: challenge` (Cloudflare's
//     own documented marker; seen here on 429 responses). Only the MAIN
//     FRAME's document counts: duapune.com served its real page (200, no
//     marker) while 134–143 of that page's own assets got
//     `429 cf-mitigated: challenge` — rate limiting on assets, not a
//     challenge on the page.
//   - A silent approval is Cloudflare's own script setting `cf_clearance`
//     (observed within 0.8–1.5s of load on both sites), after which a
//     challenged page is reloaded into the real one — a new main-frame
//     document WITHOUT the marker.
//
// So (challengeTracker.state):
//
//   - challenge = the main frame's latest document carried
//     `cf-mitigated: challenge`, or — only when that response wasn't
//     observed (a page loaded before watching started) — the page
//     carries Cloudflare's own challenge-page markers
//     (window._cf_chl_opt, the "utm_source=challenge" footer link), which
//     exist in every language and never on a real page;
//   - blocked = `cf-mitigated: block`, or Cloudflare's block page;
//   - cleared = the challenge condition no longer holds on a fully
//     loaded page (a clean document replaced the challenged one), or the
//     caller's own expected content is visible. When `cf_clearance` is
//     issued but Cloudflare doesn't reload the page itself, the tracker
//     reloads it (clearanceReloadAfter).
//
// A visible Turnstile widget on a page is never, by itself, a challenge:
// sites use it to protect a form, not to hide the page.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// cloudflareAutoWait bounds the automatic (no human) wait after a
// challenge is detected. A non-interactive challenge typically clears in
// a few seconds. Callers additionally cap it by their own ctx deadline
// (autoWaitBudget).
const cloudflareAutoWait = 25 * time.Second

const (
	// autoPollInterval — every check is one cheap JS evaluation plus an
	// in-memory read of the network signals.
	autoPollInterval = 500 * time.Millisecond
	// clearanceSettleTimeout bounds settleAfterClearance's own wait for
	// the post-clearance page to finish loading.
	clearanceSettleTimeout = 10 * time.Second
	// clearStableFor — "no longer a challenge" must hold this long before
	// it's trusted, so a navigation's in-between state can't pass.
	clearStableFor = 1 * time.Second
	// clearanceReloadAfter — how long after a newly issued cf_clearance
	// the page may still show the challenge before the tracker reloads it
	// itself (Cloudflare normally reloads within a second).
	clearanceReloadAfter = 3 * time.Second
	// widgetStalledReloadAfter — how long a challenge page's widget may
	// sit without its interactive iframe before the human wait reloads
	// the page.
	widgetStalledReloadAfter = 20 * time.Second
	// challengeLogInterval throttles logChallengeDetectorHTML (one file
	// per call) — 30 minutes of 2s ticks would otherwise write ~900 files.
	challengeLogInterval = 15 * time.Second
)

// humanPollInterval — how often the human wait checks. var, not const,
// so a temporary test can shorten it.
var humanPollInterval = 2 * time.Second

// maxWidgetReloadAttempts bounds the human wait's stalled-widget reloads.
// var, not const, for the same testing reason as humanPollInterval.
var maxWidgetReloadAttempts = 2

type challengeMode int

const (
	challengeModeAuto challengeMode = iota
	challengeModeHuman
)

// challengeOutcome is the result of one await call. Exactly one of
// Cleared/HardBlocked is true, or neither when the budget ran out with
// the challenge still present.
type challengeOutcome struct {
	Cleared     bool
	ClearedBy   string
	HardBlocked bool
	Reason      string
}

// challengeSignals records, for one tab, what Cloudflare said on the
// wire: the `cf-mitigated` value of the main frame's LATEST document
// response, and how many responses have set `cf_clearance`. Fed by a
// network listener registered BEFORE navigation (watchChallengeSignals)
// — the only way to see a challenge response's own headers. Written from
// chromedp's event loop, read from the wait loop, hence mu.
type challengeSignals struct {
	mu          sync.Mutex
	mainFrameID string

	docSeen      bool
	docMitigated string
	docStatus    int64

	clearanceIssued int
}

// wireState is one consistent read of challengeSignals.
type wireState struct {
	docSeen         bool
	mitigated       string
	status          int64
	clearanceIssued int
}

// watchChallengeSignals starts recording ctx's own tab for the lifetime
// of ctx (chromedp removes the listener once ctx is done). Call it before
// navigating; a tracker without an observed document falls back to
// Cloudflare's page markers.
//
// The main frame is identified by the page target's own ID, which is what
// Chrome uses as its top-level frame ID — needed because iframes' (e.g.
// Turnstile's) and assets' responses arrive on this same listener.
func watchChallengeSignals(ctx context.Context) *challengeSignals {
	s := &challengeSignals{}
	if c := chromedp.FromContext(ctx); c != nil && c.Target != nil {
		s.mainFrameID = string(c.Target.TargetID)
	}
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if e.Type != network.ResourceTypeDocument || e.Response == nil {
				return
			}
			if s.mainFrameID != "" && string(e.FrameID) != s.mainFrameID {
				return
			}
			s.mu.Lock()
			s.docSeen = true
			s.docMitigated = strings.ToLower(strings.TrimSpace(headerValue(e.Response.Headers, "cf-mitigated")))
			s.docStatus = e.Response.Status
			s.mu.Unlock()
		case *network.EventResponseReceivedExtraInfo:
			// Raw headers — the only place Set-Cookie is visible.
			if strings.Contains(headerValue(e.Headers, "set-cookie"), "cf_clearance=") {
				s.mu.Lock()
				s.clearanceIssued++
				s.mu.Unlock()
			}
		}
	})
	return s
}

func (s *challengeSignals) snapshot() wireState {
	if s == nil {
		return wireState{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return wireState{docSeen: s.docSeen, mitigated: s.docMitigated, status: s.docStatus, clearanceIssued: s.clearanceIssued}
}

// headerValue is a case-insensitive lookup — HTTP/2 delivers header
// names lowercased, HTTP/1.1 as the server sent them.
func headerValue(h network.Headers, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// challengeTracker evaluates one page in one tab across the automatic
// and human phases: the tab's wire signals plus a small page probe.
// Threaded from detection to the human wait through crawlError.tracker.
type challengeTracker struct {
	signals *challengeSignals
	probeJS string

	// baselineClearance is how many cf_clearance cookies had been issued
	// when the challenge was first detected — a later one is new.
	baselineClearance int
	baselineCaptured  bool
}

// newChallengeTracker builds a tracker for ctx's own current tab.
// signals nil (a caller that couldn't watch from before navigation)
// starts watching now; until a new document arrives, only the page
// markers can tell.
func newChallengeTracker(ctx context.Context, signals *challengeSignals, expectedSelectors []string) (*challengeTracker, error) {
	if signals == nil {
		signals = watchChallengeSignals(ctx)
	}
	probeJS, err := buildChallengeProbeJS(expectedSelectors)
	if err != nil {
		return nil, err
	}
	return &challengeTracker{signals: signals, probeJS: probeJS}, nil
}

// captureBaseline records the clearance count at first detection.
func (t *challengeTracker) captureBaseline() {
	if t.baselineCaptured {
		return
	}
	t.baselineCaptured = true
	t.baselineClearance = t.signals.snapshot().clearanceIssued
}

// challengeProbe is the page side of one check — one JS evaluation.
type challengeProbe struct {
	// ChallengePage — Cloudflare's own challenge-page markers, present in
	// every language: the inline window._cf_chl_opt configuration and the
	// footer's "utm_source=challenge" link.
	ChallengePage bool `json:"challengePage"`
	// BlockPage — Cloudflare's block page (fallback for when no
	// `cf-mitigated: block` response was observed).
	BlockPage bool `json:"blockPage"`
	// Expected — one of the caller's own expected selectors is visible.
	Expected      bool   `json:"expected"`
	WidgetVisible bool   `json:"widgetVisible"`
	WidgetStalled bool   `json:"widgetStalled"`
	ReadyState    string `json:"readyState"`
	Href          string `json:"href"`
}

// pageState is the tracker's verdict for one check.
type pageState struct {
	challenge bool
	blocked   bool
	reason    string
	probe     challengeProbe
	wire      wireState
}

func buildChallengeProbeJS(expectedSelectors []string) (string, error) {
	selectorsJSON, err := json.Marshal(expectedSelectors)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(function() {
		%s

		function anyVisible(selector) {
			var els = document.querySelectorAll(selector);
			for (var i = 0; i < els.length; i++) {
				if (isRenderedVisible(els[i])) return true;
			}
			return false;
		}

		var title = (document.title || "").trim().toLowerCase();
		var body = ((document.body && document.body.innerText) || "").toLowerCase();

		var expected = false;
		var sels = %s || [];
		for (var k = 0; k < sels.length && !expected; k++) {
			try {
				var el = document.querySelector(sels[k]);
				expected = !!el && isRenderedVisible(el);
			} catch (e) {}
		}

		var widgetStalled = false;
		var containers = document.querySelectorAll(%s);
		for (var c = 0; c < containers.length; c++) {
			if (isRenderedVisible(containers[c]) && !containers[c].querySelector("iframe")) {
				widgetStalled = true;
				break;
			}
		}

		return {
			challengePage:
				typeof window._cf_chl_opt !== "undefined" ||
				!!document.querySelector('a[href*="utm_source=challenge"]'),
			blockPage:
				(title.indexOf("attention required") !== -1 && title.indexOf("cloudflare") !== -1 && body.indexOf("blocked") !== -1) ||
				title.indexOf("used cloudflare to restrict access") !== -1,
			expected: expected,
			widgetVisible: anyVisible(%s),
			widgetStalled: widgetStalled,
			readyState: document.readyState,
			href: window.location.href || ""
		};
	})()`, isRenderedVisibleJS, string(selectorsJSON), challengeContainerSelectorsJS, challengeWidgetSelectorsJS), nil
}

// state evaluates the page now: the wire decides when a document was
// observed; the page markers decide otherwise, and also catch a
// challenge the wire didn't label. Expected content visible always wins.
func (t *challengeTracker) state(ctx context.Context) (pageState, error) {
	var p challengeProbe
	if err := chromedp.Run(ctx, chromedp.Evaluate(t.probeJS, &p)); err != nil {
		return pageState{}, err
	}
	w := t.signals.snapshot()
	st := pageState{probe: p, wire: w}

	switch {
	case w.docSeen && w.mitigated == "block", p.BlockPage:
		st.blocked, st.reason = true, "cloudflare-block"
	case p.Expected:
		// the caller's own content is on the page
	case w.docSeen && w.mitigated == "challenge":
		st.challenge, st.reason = true, fmt.Sprintf("cf-mitigated: challenge (HTTP %d)", w.status)
	case p.ChallengePage:
		st.challenge, st.reason = true, "cloudflare challenge page"
	}
	return st, nil
}

// stateWithRetry tolerates the "Execution context was destroyed" class of
// error a check hits when it lands mid-navigation.
func (t *challengeTracker) stateWithRetry(ctx context.Context) (pageState, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		st, err := t.state(ctx)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return pageState{}, ctx.Err()
		}
		if err := sleepCtx(ctx, autoPollInterval); err != nil {
			return pageState{}, err
		}
	}
	return pageState{}, lastErr
}

// checkAndAwaitAutoClearance is the full automatic path for a freshly
// navigated page: one immediate check (a normal page returns right away),
// and only when a challenge is actually detected, the automatic wait —
// capped at maxWait and at ctx's own deadline minus reserve (time the
// caller still needs afterwards to read/extract the page).
func (t *challengeTracker) checkAndAwaitAutoClearance(ctx context.Context, requestID string, maxWait, reserve time.Duration) (challengeOutcome, error) {
	st, err := t.stateWithRetry(ctx)
	if err != nil {
		return challengeOutcome{}, err
	}
	if st.blocked {
		return challengeOutcome{HardBlocked: true, Reason: st.reason}, nil
	}
	if !st.challenge {
		return challengeOutcome{Cleared: true, ClearedBy: "not-detected"}, nil
	}
	t.captureBaseline()
	return t.await(ctx, challengeModeAuto, autoWaitBudget(ctx, maxWait, reserve), requestID, st.reason)
}

// autoWaitBudget caps maxWait by ctx's own remaining time minus
// reserve; never negative (0 still performs one check).
func autoWaitBudget(ctx context.Context, maxWait, reserve time.Duration) time.Duration {
	budget := maxWait
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline) - reserve; remaining < budget {
			budget = remaining
		}
	}
	if budget < 0 {
		budget = 0
	}
	return budget
}

// await polls until the challenge clears (confirmed by
// settleAfterClearance), a block appears, or budget is spent. Check
// errors count as "not cleared yet" (a navigation in progress is the
// common cause) — only ctx itself ending aborts with an error.
//
// When a new cf_clearance was issued but the page still shows the
// challenge after clearanceReloadAfter, the page is reloaded once per new
// cookie. Human mode additionally reloads a stalled widget (bounded by
// maxWidgetReloadAttempts), throttled-logs the page HTML when the
// DebugLogChallenge setting is on, and reports progress every tick.
func (t *challengeTracker) await(ctx context.Context, mode challengeMode, budget time.Duration, requestID, initialReason string) (challengeOutcome, error) {
	interval := autoPollInterval
	phase := phaseCheckingCloudflare
	logChallengeHTML := false
	if mode == challengeModeHuman {
		interval = humanPollInterval
		phase = phaseAwaitingHumanChallenge
		// Read once, up front. A read failure is treated as "off" — pure
		// debug observability, never allowed to interrupt the wait.
		settings, _ := shared.LoadBrowserSettings()
		logChallengeHTML = settings.DebugEnabled && settings.DebugLogChallenge
	}
	t.captureBaseline()

	var clearSince, stalledSince, lastLog, clearanceSeenAt time.Time
	clearanceHandled := t.baselineClearance
	reloads := 0
	reason := initialReason
	start := time.Now()
	deadline := start.Add(budget)

	for attempt := 1; ; attempt++ {
		st, err := t.state(ctx)
		if err != nil && ctx.Err() != nil {
			return challengeOutcome{}, ctx.Err()
		}
		if err == nil {
			if st.reason != "" {
				reason = st.reason
			}
			if st.blocked {
				setCrawlPhase(requestID, phase, "Cloudflare block page — no automatic or human clearance possible")
				return challengeOutcome{HardBlocked: true, Reason: st.reason}, nil
			}

			if !st.challenge && st.probe.ReadyState == "complete" {
				if clearSince.IsZero() {
					clearSince = time.Now()
				}
				if st.probe.Expected || time.Since(clearSince) >= clearStableFor {
					by := "challenge-gone"
					if st.probe.Expected {
						by = "expected-content"
					} else if st.wire.docSeen {
						by = fmt.Sprintf("clean document (HTTP %d)", st.wire.status)
					}
					setCrawlPhase(requestID, phase, fmt.Sprintf("Cloudflare challenge cleared (%s) — waiting for the page to settle", by))
					settled, err := t.settleAfterClearance(ctx)
					if err != nil {
						return challengeOutcome{}, err
					}
					if settled {
						return challengeOutcome{Cleared: true, ClearedBy: by, Reason: reason}, nil
					}
					clearSince = time.Time{}
				}
			} else {
				clearSince = time.Time{}
			}

			// A new cf_clearance while the challenge is still showing:
			// Cloudflare normally reloads the page itself; if it hasn't
			// after clearanceReloadAfter, reload it here.
			if st.challenge && st.wire.clearanceIssued > clearanceHandled {
				if clearanceSeenAt.IsZero() {
					clearanceSeenAt = time.Now()
				}
				if time.Since(clearanceSeenAt) >= clearanceReloadAfter {
					clearanceHandled = st.wire.clearanceIssued
					clearanceSeenAt = time.Time{}
					setCrawlPhase(requestID, phase, "cf_clearance issued but the challenge page is still showing — reloading the page")
					if err := chromedp.Run(ctx, chromedp.Reload(), chromedp.Sleep(SettleDelay)); err != nil && ctx.Err() != nil {
						return challengeOutcome{}, ctx.Err()
					}
					continue
				}
			}

			if mode == challengeModeHuman {
				if logChallengeHTML && time.Since(lastLog) >= challengeLogInterval {
					var html string
					if chromedp.Run(ctx, chromedp.OuterHTML("html", &html)) == nil {
						logChallengeDetectorHTML(requestID, attempt, html)
					}
					lastLog = time.Now()
				}

				// A challenge page whose widget never got its interactive
				// iframe has nothing a person could click, however long this
				// waits. A reload gives it a fresh initialization attempt.
				if st.challenge && st.probe.WidgetVisible && st.probe.WidgetStalled && !turnstileFrameAttached(ctx) {
					if stalledSince.IsZero() {
						stalledSince = time.Now()
					}
					if time.Since(stalledSince) >= widgetStalledReloadAfter && reloads < maxWidgetReloadAttempts {
						reloads++
						stalledSince = time.Time{}
						setCrawlPhase(requestID, phase, fmt.Sprintf(
							"challenge widget stalled (no interactive iframe rendered) — reloading the page (attempt %d of %d)",
							reloads, maxWidgetReloadAttempts,
						))
						if err := chromedp.Run(ctx, chromedp.Reload(), chromedp.Sleep(SettleDelay)); err != nil {
							return challengeOutcome{}, err
						}
						continue
					}
				} else {
					stalledSince = time.Time{}
				}
			}
		}

		if !time.Now().Before(deadline) {
			return challengeOutcome{Reason: reason}, nil
		}

		if mode == challengeModeHuman {
			setCrawlPhase(requestID, phase, fmt.Sprintf(
				"not solved yet (%s) — check #%d, waiting for a person to solve it in the open browser window",
				reason, attempt,
			))
		} else {
			setCrawlPhase(requestID, phase, fmt.Sprintf(
				"Cloudflare challenge detected (%s) — waiting for it to clear automatically (%ds of %ds)",
				reason, int(time.Since(start).Seconds()), int(budget.Seconds()),
			))
		}

		if err := sleepCtx(ctx, interval); err != nil {
			return challengeOutcome{}, err
		}
	}
}

// settleAfterClearance waits for the post-clearance page to finish
// loading, then checks again: true when it's still not a challenge.
// false (not an error) sends the wait loop back to polling.
func (t *challengeTracker) settleAfterClearance(ctx context.Context) (bool, error) {
	deadline := time.Now().Add(clearanceSettleTimeout)
	for time.Now().Before(deadline) {
		var readyState string
		if chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &readyState)) == nil && readyState == "complete" {
			break
		}
		if err := sleepCtx(ctx, 250*time.Millisecond); err != nil {
			return false, err
		}
	}
	if err := sleepCtx(ctx, 500*time.Millisecond); err != nil {
		return false, err
	}
	st, err := t.state(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	return !st.challenge && !st.blocked, nil
}

// clearanceCookie returns the cf_clearance value the browser would send
// to href ("" if none) — GetCookies with a URL includes cookies set on
// parent domains, which is where Cloudflare usually scopes it.
func clearanceCookie(ctx context.Context, href string) (string, error) {
	var value string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().WithURLs([]string{href}).Do(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookies {
			if c.Name == "cf_clearance" {
				value = c.Value
				return nil
			}
		}
		return nil
	}))
	return value, err
}

// turnstileFrameAttached reports whether ctx's own page has a Cloudflare
// challenge iframe — the check challengeProbe.WidgetStalled can't do from
// inside the page: Turnstile renders its iframe inside a CLOSED shadow
// root, so `container.querySelector("iframe")` finds nothing even on a
// fully rendered widget, and every live Turnstile widget looked
// "stalled" (a real, reproduced false positive that reloaded an
// already-loaded page). Chrome itself still lists that iframe: as an
// "iframe" target whose ParentID is this page's own target when it runs
// out of process (verified on a live page), or in the page's own frame
// tree when it runs in process. An unreadable answer counts as
// "attached" — never reload on an unreliable read.
func turnstileFrameAttached(ctx context.Context) bool {
	c := chromedp.FromContext(ctx)
	if c == nil || c.Target == nil {
		return true
	}
	pageID := c.Target.TargetID
	attached := false
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		infos, err := target.GetTargets().Do(ctx)
		if err != nil {
			return err
		}
		for _, ti := range infos {
			if ti.Type == "iframe" && ti.ParentID == pageID && strings.Contains(ti.URL, "challenges.cloudflare.com") {
				attached = true
				return nil
			}
		}
		tree, err := page.GetFrameTree().Do(ctx)
		if err != nil {
			return err
		}
		attached = frameTreeHasURL(tree, "challenges.cloudflare.com")
		return nil
	}))
	return attached || err != nil
}

func frameTreeHasURL(tree *page.FrameTree, substr string) bool {
	if tree == nil {
		return false
	}
	if tree.Frame != nil && strings.Contains(tree.Frame.URL, substr) {
		return true
	}
	for _, child := range tree.ChildFrames {
		if frameTreeHasURL(child, substr) {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
