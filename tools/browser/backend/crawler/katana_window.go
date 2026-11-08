// katana_window.go (plan 198) — the Katana window: ONE visible Chrome with its own persistent profile, in which a person clears a
// Cloudflare check and in which Katana then reads the site.
//
// Why it exists. The headless Chrome behind Katana (katana_engine.go) fails in about a second on a check only a person can clear. The
// escalation here opens a visible Chrome at the blocked address, waits for the person (the same probe the headless verdict uses),
// remembers the site, and re-reads through Katana ATTACHED TO THAT SAME CHROME with Options.HeadlessNoIncognito = true: Katana then uses
// the Chrome's DEFAULT browser context, the one the person's tab is in, so the cookies the person earned are the cookies its fetch carries.
//
// Guards (it opens a window on the person's computer): only for an `attended` read (work the person started: see withAttended), only
// when the human-assist setting is on, only for a check a person can clear (never a rate limit, a deny or another vendor's wall), one
// window, one wait per domain (lockDomain), the cooldown of human_assist.go after a give-up. Unattended reads (auto-discovery, the
// recrawl workers) never open the window; they read in it only while the person has it open. A link with a kept login never reads in
// it (the default context would mix two logins' cookies). Nothing here solves or evades the check: the person does.
//
// Katana is the only user. Rod, its AI tools and the old headed fallback (shared/headed.go) are not touched; this file shares nothing
// with them but the launcher flags and the challenge probe. Deleting this file and its three call sites returns today's behaviour.
package crawler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"browser-tool-backend/shared"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	outcomeHumanGaveUp   = "human_gave_up"
	katanaWindowIdleStop = 30 * time.Minute
	katanaWindowDirName  = "katana-window-profile"
)

// errWindowNotForLogin: a read with a kept login was asked to run in the window. It never does (see the file comment).
var errWindowNotForLogin = errors.New("katana window: not used for a link with a kept login")

// ---- attended ----

type attendedKey struct{}

// withAttended marks work a person started and is watching (a manual Crawl now, the picker and rule screens, a Generate they pressed).
// Only such work may open the Katana window.
func withAttended(ctx context.Context, attended bool) context.Context {
	if !attended {
		return ctx
	}
	return context.WithValue(ctx, attendedKey{}, true)
}

func isAttended(ctx context.Context) bool {
	v, _ := ctx.Value(attendedKey{}).(bool)
	return v
}

// ---- the window's Chrome ----

var (
	windowMu       sync.Mutex
	windowBrowser  *rod.Browser // our connection: liveness, and the tab the person clears the check in
	windowLauncher *launcher.Launcher
	windowWS       string
	windowIdle     *time.Timer
	// launchWindowChrome starts the visible Chrome. A var so tests can stand in for it.
	launchWindowChrome = realLaunchWindowChrome
)

func katanaWindowProfileDir() (string, error) {
	root := os.Getenv("TOOL_DB_DIR")
	if root == "" {
		return "", errors.New("katana window: TOOL_DB_DIR is not set")
	}
	dir := filepath.Join(root, katanaWindowDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("katana window: cannot make its profile folder: %w", err)
	}
	return dir, nil
}

// realLaunchWindowChrome: the shared flags of the other engines, but visible and with a persistent profile.
func realLaunchWindowChrome() (*rod.Browser, *launcher.Launcher, string, error) {
	path, err := rodChromePath() // the ONE resolver; never a download
	if err != nil {
		return nil, nil, "", err
	}
	dir, err := katanaWindowProfileDir()
	if err != nil {
		return nil, nil, "", err
	}
	l := newRodLauncher(path, "").Headless(false).UserDataDir(dir).Leakless(false)
	ws, err := l.Launch()
	if err != nil {
		l.Kill()
		return nil, nil, "", fmt.Errorf("katana window: failed to launch Chrome: %w", err)
	}
	b := rod.New().ControlURL(ws).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, nil, "", fmt.Errorf("katana window: failed to connect to Chrome: %w", err)
	}
	return b, l, ws, nil
}

// closeWindowChromeLocked ends the window's Chrome but never removes its profile (the person's cookies live there).
func closeWindowChromeLocked() {
	if windowIdle != nil {
		windowIdle.Stop()
		windowIdle = nil
	}
	if windowBrowser != nil {
		_ = windowBrowser.Close()
	}
	if windowLauncher != nil {
		windowLauncher.Kill()
	}
	windowBrowser, windowLauncher, windowWS = nil, nil, ""
}

// ensureKatanaWindow returns the DevTools address of the visible Chrome, starting it on first use (or after the person closed it).
func ensureKatanaWindow() (string, error) {
	windowMu.Lock()
	defer windowMu.Unlock()
	if windowBrowser != nil {
		if _, err := rodBrowserVersion(windowBrowser); err == nil {
			touchWindowLocked()
			ensureWindowTabLocked()
			return windowWS, nil
		}
		closeWindowChromeLocked() // the person closed it
	}
	b, l, ws, err := launchWindowChrome()
	if err != nil {
		return "", err
	}
	windowBrowser, windowLauncher, windowWS = b, l, ws
	touchWindowLocked()
	ensureWindowTabLocked()
	go applyPendingCookieForgetting() // sites forgotten while the window was closed lose their cookies now
	return ws, nil
}

// ensureWindowTabLocked makes sure the Chrome has a tab: a Chrome whose last tab the person closed keeps running with no window, and opening
// a new tab in it then fails with "no browser is open". A blank tab brings the window back.
func ensureWindowTabLocked() {
	if windowBrowser == nil {
		return
	}
	if pages, err := windowBrowser.Pages(); err == nil && len(pages) == 0 {
		_, _ = windowBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	}
}

func touchWindowLocked() {
	if windowIdle != nil {
		windowIdle.Stop()
	}
	windowIdle = time.AfterFunc(katanaWindowIdleStop, StopKatanaWindow)
}

// StopKatanaWindow ends the visible Chrome (idle time, shutdown, the engine switched away). A later attended read opens it again.
func StopKatanaWindow() {
	windowMu.Lock()
	defer windowMu.Unlock()
	closeWindowChromeLocked()
}

// katanaWindowOpen reports whether the window's Chrome is up and answering. Unattended reads use the window only while this is true.
func katanaWindowOpen() bool {
	windowMu.Lock()
	b := windowBrowser
	windowMu.Unlock()
	if b == nil {
		return false
	}
	_, err := rodBrowserVersion(b)
	return err == nil
}

// ---- the sites the window serves ----

var (
	windowHostsMu sync.Mutex
	windowHostsDB *sql.DB // the handle the table was created on
)

func windowHostsTable() *sql.DB {
	if shared.DB == nil {
		return nil
	}
	windowHostsMu.Lock()
	defer windowHostsMu.Unlock()
	if windowHostsDB != shared.DB {
		if _, err := shared.DB.Exec(`CREATE TABLE IF NOT EXISTS katana_window_hosts (
			host TEXT PRIMARY KEY, created_at TEXT NOT NULL, last_cleared_at TEXT)`); err != nil {
			log.Printf("katana window: hosts table: %v", err)
		}
		windowHostsDB = shared.DB
	}
	return shared.DB
}

var (
	windowHostsMem   = map[string]bool{} // used when there is no database (tests)
	windowHostsMemMu sync.Mutex
)

// isWindowHost: reads of this site go through the window while it is open.
func isWindowHost(host string) bool {
	host = strings.ToLower(host)
	if db := windowHostsTable(); db != nil {
		var n int
		return db.QueryRow(`SELECT COUNT(1) FROM katana_window_hosts WHERE host = ?`, host).Scan(&n) == nil && n > 0
	}
	windowHostsMemMu.Lock()
	defer windowHostsMemMu.Unlock()
	return windowHostsMem[host]
}

// markWindowHost remembers that a person cleared this site's check in the window.
func markWindowHost(host string) {
	host = strings.ToLower(host)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if db := windowHostsTable(); db != nil {
		_, _ = db.Exec(`INSERT INTO katana_window_hosts (host, created_at, last_cleared_at) VALUES (?, ?, ?)
			ON CONFLICT(host) DO UPDATE SET last_cleared_at = excluded.last_cleared_at`, host, now, now)
		return
	}
	windowHostsMemMu.Lock()
	windowHostsMem[host] = true
	windowHostsMemMu.Unlock()
}

// ---- the person's "I cleared it" ----

// The automatic probe cannot always tell that a check was cleared (Cloudflare can leave its own marker on the page after the check), so the
// person says so: the Career screen asks "was the check cleared?" and its Yes arrives here. It ends a wait for this site, and remembers the
// site, so the next read goes through the window.
var (
	windowConfirmMu sync.Mutex
	windowConfirmed = map[string]bool{}
)

func confirmWindowCleared(host string) {
	host = strings.ToLower(host)
	windowConfirmMu.Lock()
	windowConfirmed[host] = true
	windowConfirmMu.Unlock()
	markWindowHost(host)
}

func takeWindowConfirmation(host string) bool {
	host = strings.ToLower(host)
	windowConfirmMu.Lock()
	defer windowConfirmMu.Unlock()
	was := windowConfirmed[host]
	delete(windowConfirmed, host)
	return was
}

// KatanaWindowClearedHandler handles POST /katana-window-cleared {"url": "..."}: the person says the check of this address' site was cleared in
// the Katana window.
func KatanaWindowClearedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		http.Error(w, "url is required (and only url is accepted)", http.StatusBadRequest)
		return
	}
	if err := validateCrawlURL(body.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	host, err := hostnameOf(body.URL)
	if err != nil || host == "" {
		http.Error(w, "the address has no host", http.StatusBadRequest)
		return
	}
	confirmWindowCleared(host)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ---- the escalation ----

// isClearableCheck: the error is the Katana verdict "a check only a person can clear". Not a rate limit (429), a deny, nor another wall.
func isClearableCheck(err error) (*crawlError, bool) {
	var cfErr *crawlError
	if errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved && cfErr.Outcome == outcomeInteractiveRequired {
		return cfErr, true
	}
	return nil, false
}

// waitForPersonInWindow opens a tab at rawURL in the visible Chrome and waits until the check no longer shows (the same probe the
// headless verdict uses), the person closes the tab, or the human-wait limit passes. It returns nil once cleared. The tab is left
// open: it keeps the window alive and shows the person the site they cleared.
func waitForPersonInWindow(ctx context.Context, rawURL, requestID string) error {
	domain, _ := hostnameOf(rawURL)
	unlock, _, err := lockDomain(ctx, domain)
	if err != nil {
		return err
	}
	defer unlock()
	takeWindowConfirmation(domain) // an old answer must not end this wait
	if _, err := ensureKatanaWindow(); err != nil {
		return err
	}
	windowMu.Lock()
	b := windowBrowser
	windowMu.Unlock()
	if b == nil {
		return errors.New("katana window: closed")
	}
	page, err := b.Page(proto.TargetCreateTarget{URL: rawURL})
	if err != nil {
		return fmt.Errorf("katana window: could not open the page: %w", err)
	}
	// The probe only means something once the tab has loaded: an empty, still-loading tab shows no check and must not read as "cleared".
	_ = page.Context(ctx).Timeout(30 * time.Second).WaitLoad()
	setCrawlPhase(requestID, phaseAwaitingHumanChallenge, "a browser window opened: clear the check there, the read goes on in it (katana)")
	probeJS, err := buildChallengeProbeJS(nil)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(maxHumanSolveDuration)
	for {
		if err := sleepCtx(ctx, humanPollInterval); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return newCloudflareUnresolvedError("the check was still there when the wait ended", nil)
		}
		if takeWindowConfirmation(domain) { // the person said the check is cleared
			return nil
		}
		raw, perr := rodEvalExpression(page.Context(ctx), probeJS)
		if perr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return newCloudflareUnresolvedError("the window was closed before the check was cleared", nil)
		}
		var probe challengeProbe
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		if !probe.ChallengePage || probe.Expected {
			return nil
		}
	}
}

// runKatanaPage is the one door for every Katana read (see runKatanaPageIn for the read itself): headless by default; in the visible
// window while the site is a window site and the window is open; and, for an attended read that meets a check a person can clear,
// the escalation (open the window, wait for the person, read there).
func runKatanaPage[T any](ctx context.Context, rawURL, requestID string, read katanaPageReader[T]) (T, error) {
	var zero T
	host, _ := hostnameOf(rawURL)
	if host != "" && loginSessionFrom(ctx) == "" && katanaWindowOpen() && isWindowHost(host) {
		v, err := runKatanaPageIn(ctx, rawURL, requestID, read, true)
		if err == nil {
			return v, nil
		}
		if _, clearable := isClearableCheck(err); !clearable && !errors.Is(err, errWindowNotForLogin) {
			return zero, err
		}
		// the check came back, or this link keeps a login: go on below, as for a site not yet in the window
	}
	v, err := runKatanaPageIn(ctx, rawURL, requestID, read, false)
	if err == nil {
		return v, nil
	}
	cfErr, clearable := isClearableCheck(err)
	if !clearable || host == "" || loginSessionFrom(ctx) != "" || !isAttended(ctx) || !humanAssistEnabled() { // a kept login never uses the window (plan 198, D4)
		return zero, err
	}
	if requestID == "" { // an AI call cannot wait for a person: open the window in the background and say so at once
		return zero, windowRequestInBackground(host, rawURL, cfErr)
	}
	if werr := waitForPersonInWindow(ctx, rawURL, requestID); werr != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		out := *cfErr
		out.Outcome = outcomeHumanGaveUp
		out.Advice = "The browser window was opened but the check was not cleared. Open the link again and clear it, or use Rod for this link."
		return zero, &out
	}
	v, err = runKatanaPageIn(ctx, rawURL, requestID, read, true)
	if errors.Is(err, errWindowNotForLogin) {
		return zero, cfErr
	}
	if again, still := isClearableCheck(err); still { // the window read still meets the check: it was not cleared
		out := *again
		out.Outcome = outcomeHumanGaveUp
		out.Advice = "The browser window was opened but the site still shows its check. Open the link again and clear it, or use Rod for this link."
		return zero, &out
	}
	if err == nil {
		markWindowHost(host) // only a site whose read passed in the window is remembered
	}
	return v, err
}

// beginWindowRequest registers a wait for the person in the Katana window for this site and starts it in the background: one request per
// domain at a time (the state of human_assist.go). It returns whether a request was already waiting; cooldown is the caller's business.
func beginWindowRequest(domain, rawURL string) (alreadyWaiting bool) {
	humanAssistMu.Lock()
	if req := humanRequests[domain]; req != nil && req.state == humanWaiting {
		humanAssistMu.Unlock()
		return true
	}
	now := humanNow()
	reqID := fmt.Sprintf("katana-window-%s-%d", domain, now.UnixNano())
	humanRequests[domain] = &humanRequest{state: humanWaiting, requestID: reqID, startedAt: now}
	ctx, cancel := context.WithTimeout(context.Background(), maxHumanSolveDuration+2*time.Minute)
	registerCrawlCancel(reqID, cancel)
	humanAssistMu.Unlock()
	go func() {
		defer cancel()
		defer unregisterCrawlCancel(reqID)
		err := waitForPersonInWindow(ctx, rawURL, reqID)
		state, detail := humanCleared, ""
		if err != nil {
			state, detail = humanGaveUp, err.Error()
		} else {
			markWindowHost(domain)
		}
		humanAssistMu.Lock()
		if r := humanRequests[domain]; r != nil && r.requestID == reqID {
			r.state, r.finishedAt, r.detail = state, humanNow(), detail
		}
		humanAssistMu.Unlock()
	}()
	return false
}

// windowRequestInBackground is the AI-call and screen path: the call is answered at once with the human-requested error while the window
// opens in the background. A give-up starts a cooldown (a cleared request does not).
func windowRequestInBackground(domain, rawURL string, cfErr *crawlError) error {
	humanAssistMu.Lock()
	req := humanRequests[domain]
	now := humanNow()
	if req != nil && req.state != humanWaiting && req.state != humanCleared && now.Sub(req.finishedAt) < humanRequestCooldown {
		humanAssistMu.Unlock()
		out := *cfErr
		out.Advice = "The user was asked to clear this site a moment ago and it did not work out. Do not retry; tell the user, or use another source."
		return &out
	}
	humanAssistMu.Unlock()
	waiting := beginWindowRequest(domain, rawURL)
	out := newHumanRequestedError(cfErr.Reason, waiting)
	if waiting {
		out.Advice = "The browser window is already open and waiting for you to clear the check. Clear it there, then read this link again."
	} else {
		out.Advice = "A browser window opened on the computer where Cinqo runs. Clear the check there; once it is cleared, read this link again and Katana reads in that same window. Do not retry before that."
	}
	return out
}
