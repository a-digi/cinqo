package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"browser-tool-backend/shared"
)

// windowRig stands in for the visible Chrome with a headless one (a test cannot show a window) and for the person with a script that
// clears the check in the tab the escalation opened: a server answers a check page (cf-mitigated: challenge) until the cookie
// "cleared=1" is set, which is what a cleared check does in a browser.
type windowRig struct {
	url      string
	jobURL   string
	launched atomic.Int32
	person   atomic.Bool // whether the scripted person clears the check
	hits     atomic.Int32
}

func newWindowRig(t *testing.T) *windowRig {
	t.Helper()
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	resetHumanRequests()
	humanPollInterval = 100 * time.Millisecond
	oldLaunch, oldPoll := launchWindowChrome, humanPollInterval
	rig := &windowRig{}
	rig.person.Store(true)
	gated := func(page string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rig.hits.Add(1)
			w.Header().Set("Content-Type", "text/html")
			if c, err := r.Cookie("cleared"); err != nil || c.Value != "1" {
				w.Header().Set("cf-mitigated", "challenge")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head><body>checking<script>window._cf_chl_opt = {}</script></body></html>"))
				return
			}
			_, _ = w.Write([]byte(page))
		}
	}
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/jobs":  gated(`<html><head><title>Jobs</title></head><body><a href="/job/1">Job one</a></body></html>`),
		"/job/1": gated(`<html><head><title>Job</title></head><body><h1>Job one</h1></body></html>`),
	})
	rig.url = srv.URL + "/jobs"
	rig.jobURL = srv.URL + "/job/1"
	launchWindowChrome = func() (*rod.Browser, *launcher.Launcher, string, error) {
		rig.launched.Add(1)
		b, l, ws, err := launchKatanaChrome(mustChromePath(t), "")
		if err == nil && rig.person.Load() {
			go func() { // the person: clear the check in any tab of the site (until the Chrome goes away)
				for {
					time.Sleep(150 * time.Millisecond)
					pages, perr := b.Pages()
					if perr != nil {
						return
					}
					for _, p := range pages {
						info, ierr := p.Info()
						if ierr != nil || !strings.Contains(info.URL, "/job") {
							continue
						}
						if v, eerr := p.Eval(`() => document.cookie.includes("cleared=1")`); eerr == nil && !v.Value.Bool() {
							_, _ = p.Eval(`() => { document.cookie = "cleared=1; path=/" }`)
							_ = p.Reload()
						}
					}
				}
			}()
		}
		return b, l, ws, err
	}
	t.Cleanup(func() {
		launchWindowChrome, humanPollInterval = oldLaunch, oldPoll
		StopKatanaWindow()
		resetHumanRequests()
	})
	return rig
}

func mustChromePath(t *testing.T) string {
	t.Helper()
	p, err := rodChromePath()
	if err != nil {
		t.Skip("no Chrome")
	}
	return p
}

func readLinks(ctx context.Context, url, requestID string) (fetchLinksResponse, error) {
	return fetchLinksWithKatana(ctx, url, requestID)
}

func TestAnAttendedReadOpensTheWindowAndKatanaReadsInIt(t *testing.T) {
	rig := newWindowRig(t)
	ctx := withAttended(context.Background(), true)
	res, err := readLinks(ctx, rig.url, "req-1")
	if err != nil {
		t.Fatalf("attended read: %v", err)
	}
	if len(res.Links) == 0 {
		t.Fatalf("no links in the window read: %+v", res)
	}
	if rig.launched.Load() != 1 {
		t.Errorf("windows opened = %d, want 1", rig.launched.Load())
	}
	host, _ := hostnameOf(rig.url)
	if !isWindowHost(host) {
		t.Error("the site must be remembered as a window site")
	}
	// the next read of the site uses the open window: no second window
	if _, err := readLinks(ctx, rig.url, "req-2"); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if rig.launched.Load() != 1 {
		t.Errorf("a second window was opened (%d)", rig.launched.Load())
	}
	// an UNattended read of the same site also reads in the open window
	if _, err := readLinks(context.Background(), rig.url, "req-3"); err != nil {
		t.Fatalf("unattended read while the window is open: %v", err)
	}
}

func TestAnUnattendedReadNeverOpensTheWindow(t *testing.T) {
	rig := newWindowRig(t)
	_, err := readLinks(context.Background(), rig.url, "req-1")
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Code != codeCloudflareUnresolved {
		t.Fatalf("want the check verdict, got %v", err)
	}
	if rig.launched.Load() != 0 {
		t.Errorf("a window was opened for an unattended read")
	}
}

func TestARateLimitAndADenyNeverOpenTheWindow(t *testing.T) {
	rig := newWindowRig(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/limited": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("<html><body>slow down</body></html>"))
		},
		"/block": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("cf-mitigated", "block")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><body>blocked</body></html>"))
		},
	})
	ctx := withAttended(context.Background(), true)
	for _, p := range []string{"/limited", "/block"} {
		_, err := readLinks(ctx, srv.URL+p, "r")
		var ce *crawlError
		want := map[string]string{"/limited": codeRateLimited, "/block": codeCloudflareBlocked}[p]
		if !errors.As(err, &ce) || ce.Code != want {
			t.Errorf("%s: got %v, want code %s", p, err, want)
		}
	}
	if rig.launched.Load() != 0 {
		t.Errorf("a window was opened for a rate limit or a deny")
	}
}

func TestWhenThePersonDoesNotClearItTheReadGivesTheCheckBack(t *testing.T) {
	rig := newWindowRig(t)
	rig.person.Store(false)
	old := maxHumanSolveDuration
	maxHumanSolveDuration = 1500 * time.Millisecond
	t.Cleanup(func() { maxHumanSolveDuration = old })
	_, err := readLinks(withAttended(context.Background(), true), rig.url, "req-1")
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Outcome != outcomeHumanGaveUp {
		t.Fatalf("want outcome %s, got %v", outcomeHumanGaveUp, err)
	}
	host, _ := hostnameOf(rig.url)
	if isWindowHost(host) {
		t.Error("a site nobody cleared must not become a window site")
	}
}

func TestAnAICallDoesNotWaitAndTheWindowOpensInTheBackground(t *testing.T) {
	rig := newWindowRig(t)
	_, err := readLinks(withAttended(context.Background(), true), rig.url, "") // no request id: an AI call
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Code != codeHumanRequested {
		t.Fatalf("want %s at once, got %v", codeHumanRequested, err)
	}
	host, _ := hostnameOf(rig.url)
	deadline := time.Now().Add(30 * time.Second)
	for !isWindowHost(host) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !isWindowHost(host) {
		t.Fatal("the background request never cleared")
	}
	if _, err := readLinks(withAttended(context.Background(), true), rig.url, "r2"); err != nil {
		t.Fatalf("the next call must read in the window: %v", err)
	}
	if rig.launched.Load() != 1 {
		t.Errorf("windows = %d", rig.launched.Load())
	}
}

func TestTheOptionsOfTheTwoVariantsDifferInExactlyTwoFields(t *testing.T) {
	a := katanaOptionsFor(context.Background(), "ws://a", "https://x.test/", nil, false)
	b := katanaOptionsFor(context.Background(), "ws://b", "https://x.test/", nil, true)
	if a.HeadlessNoIncognito || !b.HeadlessNoIncognito {
		t.Errorf("incognito flags: headless=%v window=%v", a.HeadlessNoIncognito, b.HeadlessNoIncognito)
	}
	if a.ChromeWSUrl != "ws://a" || b.ChromeWSUrl != "ws://b" {
		t.Error("each variant attaches to its own Chrome")
	}
	b.HeadlessNoIncognito, b.ChromeWSUrl = false, "ws://a"
	a.OnResult, b.OnResult = nil, nil
	if a.MaxDepth != b.MaxDepth || a.Scope[0] != b.Scope[0] || a.MaxOnclickLinks != b.MaxOnclickLinks || a.ShowBrowser != b.ShowBrowser ||
		a.AutomaticFormFill != b.AutomaticFormFill || a.Concurrency != b.Concurrency || a.PageLoadStrategy != b.PageLoadStrategy {
		t.Error("the window variant may differ from the headless one only in the two fields")
	}
}

func TestAnUnattendedReadDoesNotReopenAWindowThePersonClosed(t *testing.T) {
	rig := newWindowRig(t)
	host, _ := hostnameOf(rig.url)
	markWindowHost(host) // cleared earlier, then the window was closed
	_, err := readLinks(context.Background(), rig.url, "r")
	if code := errCode(err); code != codeCloudflareUnresolved {
		t.Fatalf("want the check verdict, got %v", err)
	}
	if rig.launched.Load() != 0 {
		t.Errorf("an unattended read opened the window again (%d)", rig.launched.Load())
	}
}

func TestAReadWithAKeptLoginNeverOpensOrUsesTheWindow(t *testing.T) {
	rig := newWindowRig(t)
	site := newLoginSite(t, "alice")
	info := site.saveSession(t, "alice")
	host, _ := hostnameOf(rig.url)
	markWindowHost(host)
	// the check page is on the rig's server; the kept login is for 127.0.0.1 (the same host name in this test)
	ctx := withAttended(withLoginSession(context.Background(), info.ID), true)
	_, err := readLinks(ctx, rig.url, "r")
	if err == nil {
		t.Fatal("want an error: the site shows its check and a kept login never uses the window")
	}
	if rig.launched.Load() != 0 {
		t.Errorf("a kept-login read opened or used the window (%d)", rig.launched.Load())
	}
}

// The probe cannot always see a cleared check (Cloudflare can leave its marker on the page), so the person's "I cleared it" ends the wait.
func TestThePersonsAnswerEndsTheWaitAndRemembersTheSite(t *testing.T) {
	rig := newWindowRig(t)
	rig.person.Store(false)
	host, _ := hostnameOf(rig.url)
	go func() {
		time.Sleep(700 * time.Millisecond)
		confirmWindowCleared(host) // what the route does for a public address (the route itself refuses this local test address)
	}()
	if err := waitForPersonInWindow(context.Background(), rig.url, "req-1"); err != nil {
		t.Fatalf("the wait must end with the person's answer: %v", err)
	}
	if !isWindowHost(host) {
		t.Error("the person's answer must remember the site")
	}
	// an answer given while nobody waits must not end a later wait at once
	confirmWindowCleared(host)
	takeWindowConfirmation(host)
	old := maxHumanSolveDuration
	maxHumanSolveDuration = 1200 * time.Millisecond
	t.Cleanup(func() { maxHumanSolveDuration = old })
	if err := waitForPersonInWindow(context.Background(), rig.url, "req-2"); err == nil {
		t.Error("a stale answer ended the next wait")
	}
}

func TestTheClearedRouteRefusesBadRequests(t *testing.T) {
	for name, body := range map[string]string{"empty": `{}`, "unknown field": `{"url":"https://x.test/","x":1}`, "private": `{"url":"http://169.254.169.254/"}`, "garbage": `nope`} {
		rec := httptest.NewRecorder()
		KatanaWindowClearedHandler(rec, httptest.NewRequest(http.MethodPost, "/katana-window-cleared", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	KatanaWindowClearedHandler(rec, httptest.NewRequest(http.MethodGet, "/katana-window-cleared", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

// A request that ended "cleared" is not a give-up: if the site asks again (the clearance ran out) the person is asked again at once,
// not told "do not retry" for ten minutes.
func TestAClearedRequestDoesNotStartTheCooldown(t *testing.T) {
	newWindowRig(t)
	cf := newCloudflareUnresolvedError("check", nil)
	cf.Outcome = outcomeInteractiveRequired
	humanAssistMu.Lock()
	humanRequests["x.test"] = &humanRequest{state: humanCleared, finishedAt: time.Now()}
	humanRequests["y.test"] = &humanRequest{state: humanGaveUp, finishedAt: time.Now()}
	humanAssistMu.Unlock()
	if err := windowRequestInBackground("y.test", "https://y.test/", cf); errCodeOf(err) == codeHumanRequested {
		t.Error("a give-up must still start the cooldown")
	}
	err := windowRequestInBackground("x.test", "https://x.test/", cf)
	if errCodeOf(err) != codeHumanRequested {
		t.Errorf("after a cleared request the person must be asked again, got %v", err)
	}
	StopKatanaWindow()
}

func errCodeOf(err error) string {
	var ce *crawlError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func titleOf(resp paginatedCrawlResponse) string {
	if len(resp.Pages) == 0 {
		return ""
	}
	return fmt.Sprint(resp.Pages[0].Results["title"])
}

func jobArgs(requestID string) paginatedArgs {
	return paginatedArgs{fields: []extractField{{Label: "title", Selector: "h1"}}, requestedMaxPages: 1, effectiveMaxPages: 1, requestID: requestID}
}

func TestAJobPageOfAWindowSiteIsCrawledInTheWindow(t *testing.T) {
	rig := newWindowRig(t)
	// the person clears the check through a listing read (opens the window, remembers the site)
	if _, err := readLinks(withAttended(context.Background(), true), rig.url, "req-1"); err != nil {
		t.Fatalf("clearing read: %v", err)
	}
	resp, _, err := performPaginatedCrawlViaKatanaWindow(context.Background(), rig.jobURL, jobArgs("job-1"), false)
	if err != nil || titleOf(resp) != "Job one" {
		t.Fatalf("the job page in the window: %v %+v", err, resp.Pages)
	}
	if rig.launched.Load() != 1 {
		t.Errorf("windows = %d, want the one window only", rig.launched.Load())
	}
}

func TestAPersonsJobCrawlOnAWindowSiteAsksInTheWindowWhenTheCheckIsBack(t *testing.T) {
	rig := newWindowRig(t)
	host, _ := hostnameOf(rig.jobURL)
	markWindowHost(host) // cleared before; the window is closed and its cookies are gone (a fresh stand-in Chrome)
	old := maxHumanSolveDuration
	maxHumanSolveDuration = 90 * time.Second
	t.Cleanup(func() { maxHumanSolveDuration = old })
	resp, _, err := performPaginatedCrawlViaKatanaWindow(context.Background(), rig.jobURL, jobArgs("job-2"), true)
	if err != nil || titleOf(resp) != "Job one" {
		t.Fatalf("the attended job crawl: %v %+v", err, resp.Pages)
	}
	if rig.launched.Load() != 1 {
		t.Errorf("windows = %d", rig.launched.Load())
	}
}

func TestOnlyAKatanaRequestOnAWindowSiteIsReadInTheWindow(t *testing.T) {
	rig := newWindowRig(t)
	host, _ := hostnameOf(rig.jobURL)
	want := func(b paginatedCrawlRequest) bool { return katanaWindowCrawlWanted(&b) }
	base := paginatedCrawlRequest{URL: rig.jobURL, Engine: "katana", Attended: true}
	if want(base) {
		t.Error("a site nobody cleared in the window is not a window site")
	}
	markWindowHost(host)
	if !want(base) {
		t.Error("an attended Katana request on a window site is read in the window")
	}
	rod := base
	rod.Engine = ""
	if want(rod) {
		t.Error("a request without engine katana (a Rod link) must never go to the window")
	}
	login := base
	login.LoginSessionID = "abc"
	ai := base
	ai.AIContextID = "ctx"
	lw := base
	lw.UseLoginWindow = true
	noURL := base
	noURL.URL = ""
	for name, b := range map[string]paginatedCrawlRequest{"kept login": login, "ai context": ai, "login window": lw, "no url": noURL} {
		if want(b) {
			t.Errorf("%s must not use the window", name)
		}
	}
	unattended := base
	unattended.Attended = false
	if want(unattended) {
		t.Error("an unattended request must not open a closed window")
	}
	if _, err := ensureKatanaWindow(); err != nil {
		t.Fatal(err)
	}
	if !want(unattended) {
		t.Error("an unattended request uses the window while it is open")
	}
}

func TestAJobCrawlThatMeetsTheCheckAndGetsNoAnswerGivesTheCheckBack(t *testing.T) {
	rig := newWindowRig(t)
	rig.person.Store(false)
	host, _ := hostnameOf(rig.jobURL)
	markWindowHost(host)
	old := maxHumanSolveDuration
	maxHumanSolveDuration = 1500 * time.Millisecond
	t.Cleanup(func() { maxHumanSolveDuration = old })
	_, _, err := performPaginatedCrawlViaKatanaWindow(context.Background(), rig.jobURL, jobArgs("job-3"), true)
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Outcome != outcomeHumanGaveUp {
		t.Fatalf("want outcome %s, got %v", outcomeHumanGaveUp, err)
	}
	// an unattended crawl never asks: the check comes back as it is
	_, _, err = performPaginatedCrawlViaKatanaWindow(context.Background(), rig.jobURL, jobArgs("job-4"), false)
	if !errors.As(err, &ce) || ce.Outcome == outcomeHumanGaveUp || ce.Code != codeCloudflareUnresolved {
		t.Fatalf("unattended: want the plain check verdict, got %v", err)
	}
}

func TestForgettingASiteDropsItAndItsCookiesFromTheOpenWindow(t *testing.T) {
	rig := newWindowRig(t)
	if _, err := readLinks(withAttended(context.Background(), true), rig.url, "req-1"); err != nil {
		t.Fatalf("clearing read: %v", err)
	}
	host, _ := hostnameOf(rig.url)
	if !isWindowHost(host) || len(listWindowHosts()) != 1 {
		t.Fatalf("the site must be listed: %+v", listWindowHosts())
	}
	hasCookie := func() bool {
		windowMu.Lock()
		b := windowBrowser
		windowMu.Unlock()
		all, err := proto.StorageGetCookies{}.Call(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range all.Cookies {
			if c.Name == "cleared" {
				return true
			}
		}
		return false
	}
	if !hasCookie() {
		t.Fatal("the cleared cookie must be in the window before it is forgotten")
	}
	forgetWindowHost(host)
	if isWindowHost(host) || len(listWindowHosts()) != 0 {
		t.Error("the site must be gone")
	}
	if hasCookie() {
		t.Error("the site's cookies must be removed from the window")
	}
}

func TestASiteForgottenWhileTheWindowIsClosedLosesItsCookiesAtTheNextStart(t *testing.T) {
	newWindowRig(t)
	markWindowHost("x.test")
	forgetWindowHost("x.test")
	var n int
	if err := shared.DB.QueryRow(`SELECT COUNT(*) FROM katana_window_forget WHERE host = 'x.test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending forget: %d %v", n, err)
	}
	if _, err := ensureKatanaWindow(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_ = shared.DB.QueryRow(`SELECT COUNT(*) FROM katana_window_forget WHERE host = 'x.test'`).Scan(&n)
		if n == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("the pending forget was not applied at the window's start")
}

func TestTheWindowSitesRoutes(t *testing.T) {
	newWindowRig(t)
	markWindowHost("a.example.com")
	rec := httptest.NewRecorder()
	KatanaWindowHostsHandler(rec, httptest.NewRequest(http.MethodGet, "/katana-window-hosts", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"a.example.com"`) || !strings.Contains(rec.Body.String(), `"windowOpen":false`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body.String())
	}
	for name, q := range map[string]string{"empty": "", "path": "a/b", "space": "a b"} {
		rec = httptest.NewRecorder()
		KatanaWindowHostHandler(rec, httptest.NewRequest(http.MethodDelete, "/katana-window-host?host="+strings.ReplaceAll(q, " ", "%20"), nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("delete %s: %d", name, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	KatanaWindowHostHandler(rec, httptest.NewRequest(http.MethodDelete, "/katana-window-host?host=A.Example.com", nil))
	if rec.Code != 200 || isWindowHost("a.example.com") {
		t.Errorf("delete: %d, still a window host: %v", rec.Code, isWindowHost("a.example.com"))
	}
	for name, body := range map[string]string{"empty": `{}`, "private": `{"url":"http://169.254.169.254/"}`, "unknown": `{"url":"https://x.test/","y":1}`} {
		rec = httptest.NewRecorder()
		KatanaWindowOpenHandler(rec, httptest.NewRequest(http.MethodPost, "/katana-window-open", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("open %s: %d", name, rec.Code)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut} {
		rec = httptest.NewRecorder()
		KatanaWindowHostsHandler(rec, httptest.NewRequest(m, "/katana-window-hosts", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s list: %d", m, rec.Code)
		}
	}
}

func TestACookieDomainBelongsToTheSiteItWasSetFor(t *testing.T) {
	for _, c := range []struct {
		cookie, host string
		want         bool
	}{
		{"www.example.com", "www.example.com", true}, {".example.com", "www.example.com", true}, {"example.com", "www.example.com", true},
		{".example.com", "example.com", true}, {"other.com", "www.example.com", false}, {"ple.com", "www.example.com", false},
		{"www.example.com", "example.com", false},
	} {
		if got := cookieDomainIs(c.cookie, c.host); got != c.want {
			t.Errorf("%q for %q = %v, want %v", c.cookie, c.host, got, c.want)
		}
	}
}
