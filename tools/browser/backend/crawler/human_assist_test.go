package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-rod/rod/lib/proto"

	"browser-tool-backend/shared"
)

// humanStub stands in for the person: it records each launch and lets the test
// decide when and how the "person" finishes.
type humanStub struct {
	mu       sync.Mutex
	launches []string // the URLs a window was opened for
	release  chan error
}

func newHumanStub(t *testing.T) *humanStub {
	t.Helper()
	useBrowserSettings(t, shared.FetchEngineRod)
	resetHumanRequests()
	s := &humanStub{release: make(chan error, 8)}
	oldLaunch, oldCool, oldNow := launchHumanClearance, humanRequestCooldown, humanNow
	launchHumanClearance = func(ctx context.Context, rawURL, requestID string) error {
		s.mu.Lock()
		s.launches = append(s.launches, rawURL)
		s.mu.Unlock()
		select {
		case err := <-s.release:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.Cleanup(func() {
		launchHumanClearance, humanRequestCooldown, humanNow = oldLaunch, oldCool, oldNow
		resetHumanRequests()
	})
	return s
}

func (s *humanStub) launched() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.launches)
}

// waitLaunches waits for n windows to have been opened (the launch runs in the
// background).
func (s *humanStub) waitLaunches(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.launched() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("launches = %d, want %d", s.launched(), n)
}

func waitState(t *testing.T, domain string, want humanRequestState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if humanRequestSummary(domain) == string(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("request for %s is %q, want %q", domain, humanRequestSummary(domain), want)
}

func unresolved() error {
	return newCloudflareUnresolvedError("cf-mitigated: challenge (HTTP 403)", nil)
}

func TestAnAgentsUnresolvedChallengeAsksAPersonAndReturnsAtOnce(t *testing.T) {
	stub := newHumanStub(t)
	err := humanAssistedError("https://example.com/jobs", "", unresolved())
	ce := asCrawlError(t, err)
	if ce.Code != codeHumanRequested || ce.Outcome != outcomeHumanRequested || !strings.Contains(ce.Advice, "A browser window was opened") {
		t.Fatalf("%+v", ce)
	}
	waitState(t, "example.com", humanWaiting)
	stub.waitLaunches(t, 1)
	// The same domain while the person is still at it: no second window.
	again := asCrawlError(t, humanAssistedError("https://example.com/other", "", unresolved()))
	if again.Code != codeHumanRequested || !strings.Contains(again.Advice, "already asked") {
		t.Errorf("second call: %+v", again)
	}
	if stub.launched() != 1 {
		t.Errorf("a second window was opened: %d launches", stub.launched())
	}
	stub.release <- nil
	waitState(t, "example.com", humanCleared)
}

func TestOtherDomainsAreIndependent(t *testing.T) {
	stub := newHumanStub(t)
	humanAssistedError("https://a.example/", "", unresolved())
	humanAssistedError("https://b.example/", "", unresolved())
	waitState(t, "a.example", humanWaiting)
	waitState(t, "b.example", humanWaiting)
	stub.waitLaunches(t, 2)
	if stub.launched() != 2 {
		t.Errorf("each domain asks for itself: %d launches", stub.launched())
	}
	stub.release <- nil
	stub.release <- nil
}

func TestOnlyAnUnresolvedChallengeOnAnAIGetsAPerson(t *testing.T) {
	stub := newHumanStub(t)
	for name, c := range map[string]struct {
		requestID string
		err       error
	}{
		"Career's own crawl":    {"req-1", unresolved()},
		"a hard block":          {"", newCloudflareBlockedError("cloudflare-block")},
		"a rate limit":          {"", newAccessError(accessVerdict{Kind: outcomeRateLimited, Reason: "HTTP 429"})},
		"another vendor's wall": {"", newAccessError(accessVerdict{Kind: outcomeOtherChallenge, Vendor: "datadome"})},
		"a plain error":         {"", errors.New("boom")},
	} {
		got := humanAssistedError("https://example.com/", c.requestID, c.err)
		if got != c.err {
			t.Errorf("%s must pass through unchanged, got %v", name, got)
		}
	}
	if humanAssistedError("https://example.com/", "", nil) != nil {
		t.Error("no error stays no error")
	}
	if stub.launched() != 0 {
		t.Errorf("no window must have been opened, %d were", stub.launched())
	}
}

func TestTheSettingTurnsItOff(t *testing.T) {
	stub := newHumanStub(t)
	s, err := shared.LoadBrowserSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !s.HumanAssist {
		t.Fatal("on by default")
	}
	s.HumanAssist = false
	if err := shared.SaveBrowserSettings(s); err != nil {
		t.Fatal(err)
	}
	in := unresolved()
	if got := humanAssistedError("https://example.com/", "", in); got != in {
		t.Errorf("with the setting off the original error must come back: %v", got)
	}
	if stub.launched() != 0 {
		t.Error("no window with the setting off")
	}
}

func TestACooldownFollowsARequestThatEndedWithoutAClearance(t *testing.T) {
	stub := newHumanStub(t)
	now := time.Now()
	humanNow = func() time.Time { return now }
	humanRequestCooldown = 10 * time.Minute

	humanAssistedError("https://example.com/", "", unresolved())
	waitState(t, "example.com", humanWaiting)
	stub.waitLaunches(t, 1)
	stub.release <- &crawlError{Code: codeCloudflareUnresolved} // the wait ran out
	waitState(t, "example.com", humanGaveUp)

	got := asCrawlError(t, humanAssistedError("https://example.com/", "", unresolved()))
	if got.Code != codeCloudflareUnresolved || !strings.Contains(got.Advice, "did not work out") {
		t.Errorf("within the cooldown: %+v", got)
	}
	if stub.launched() != 1 {
		t.Errorf("no new window within the cooldown: %d", stub.launched())
	}

	now = now.Add(11 * time.Minute)
	if again := asCrawlError(t, humanAssistedError("https://example.com/", "", unresolved())); again.Code != codeHumanRequested {
		t.Errorf("after the cooldown a person is asked again: %+v", again)
	}
	waitState(t, "example.com", humanWaiting)
	stub.waitLaunches(t, 2)
	stub.release <- nil
}

func TestAWindowThatCannotBeOpenedIsRecordedAndNotRetriedAtOnce(t *testing.T) {
	stub := newHumanStub(t)
	humanAssistedError("https://example.com/", "", unresolved())
	waitState(t, "example.com", humanWaiting)
	stub.waitLaunches(t, 1)
	stub.release <- errors.New("normal-session fallback: failed to start: no display")
	waitState(t, "example.com", humanFailed)
	if got := asCrawlError(t, humanAssistedError("https://example.com/", "", unresolved())); got.Code != codeCloudflareUnresolved {
		t.Errorf("after a failed launch the agent gets the plain outcome: %+v", got)
	}
	if stub.launched() != 1 {
		t.Errorf("launches = %d", stub.launched())
	}
}

func TestARequestCanBeCancelledLikeAnyCrawl(t *testing.T) {
	stub := newHumanStub(t)
	humanAssistedError("https://example.com/", "", unresolved())
	waitState(t, "example.com", humanWaiting)
	humanAssistMu.Lock()
	reqID := humanRequests["example.com"].requestID
	humanAssistMu.Unlock()
	if !strings.HasPrefix(reqID, "human-example.com-") {
		t.Fatalf("request id %q", reqID)
	}
	stub.waitLaunches(t, 1)
	if !cancelCrawl(reqID) {
		t.Fatal("the request must be cancellable by its id")
	}
	waitState(t, "example.com", humanFailed) // cancelled: not a clearance
}

func TestTheAdviceNeverTellsAnAgentToWaitHalfAnHour(t *testing.T) {
	for _, waiting := range []bool{false, true} {
		a := adviceHumanRequested(waiting)
		if !strings.Contains(a, "Do not retry") || strings.ContainsAny(a, "<>{}") {
			t.Errorf("advice (waiting=%v): %q", waiting, a)
		}
	}
}

// --- find_repeating_items asks a person too -------------------------------------

func TestAChallengeOnTheListingToolAsksAPerson(t *testing.T) {
	stub := newHumanStub(t)
	t.Cleanup(func() { shared.StopSharedSession() })
	url := wallSite(t, 403, map[string]string{"cf-mitigated": "challenge"}, `<html><body>Just a moment...</body></html>`)
	_, err := performFindRepeating(context.Background(), findRepeatingRequest{URL: url, MinItems: 3, MaxCandidates: 5})
	ce := asCrawlError(t, err)
	if ce.Code != codeHumanRequested {
		t.Fatalf("%+v", ce)
	}
	waitState(t, "127.0.0.1", humanWaiting)
	stub.release <- nil
}

// --- the kept clearance, in a real browser ----------------------------------------

// clearanceSite serves a challenge unless the request carries cf_clearance=GOOD.
func clearanceSite(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if c, err := r.Cookie("cf_clearance"); err == nil && c.Value == "GOOD" {
			_, _ = w.Write([]byte(longPage + `</main></body></html>`))
			return
		}
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><body>Just a moment...</body></html>`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func keepClearance(t *testing.T, ua string, expires time.Time) {
	t.Helper()
	err := shared.StoreClearance(shared.Clearance{Domain: "127.0.0.1", Name: shared.ClearanceCookieName, Value: "GOOD", Path: "/", ExpiresAt: expires, UserAgent: ua, Source: "human"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAKeptClearanceLetsTheTabPathThrough(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineRod)
	url := clearanceSite(t)
	ctx := browserOnly(t)
	var ua string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua)); err != nil {
		t.Fatal(err)
	}

	// Nothing kept: the challenge stands.
	if _, err := navigateAndReadWithCloudflareCheck(ctx, url, "", func() {}, 2*time.Second, nil, nil, nil, 0, nil); asCrawlError(t, err).Code != codeCloudflareUnresolved {
		t.Fatalf("without a clearance the site must challenge: %v", err)
	}

	// A clearance kept under ANOTHER user agent is never used.
	keepClearance(t, "some other browser", time.Now().Add(time.Hour))
	ctx2 := browserOnly(t)
	if _, err := navigateAndReadWithCloudflareCheck(ctx2, url, "", func() {}, 2*time.Second, nil, nil, nil, 0, nil); err == nil {
		t.Fatal("a clearance earned under another user agent must not be used")
	}

	// Under this browser's own user agent it passes.
	keepClearance(t, ua, time.Now().Add(time.Hour))
	ctx3 := browserOnly(t)
	resp, err := navigateAndReadWithCloudflareCheck(ctx3, url, "", func() {}, 2*time.Second, nil, nil, nil, 0, nil)
	if err != nil || !strings.Contains(resp.HTML, "ordinary page") {
		t.Fatalf("a kept clearance must let the page through: %v", err)
	}
}

func TestAKeptClearanceLetsRodThrough(t *testing.T) {
	requireRodChrome(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	url := clearanceSite(t)
	b, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	pg, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := pg.Eval(`() => navigator.userAgent`)
	if err != nil {
		t.Fatal(err)
	}
	ua := res.Value.String()
	_ = pg.Close()

	if _, err := fetchWithRod(context.Background(), url, "", nil, nil, nil, 0, nil); asCrawlError(t, err).Code != codeCloudflareUnresolved {
		t.Fatalf("without a clearance: %v", err)
	}
	keepClearance(t, "some other browser", time.Now().Add(time.Hour))
	if _, err := fetchWithRod(context.Background(), url, "", nil, nil, nil, 0, nil); err == nil {
		t.Fatal("another user agent's clearance must not be used")
	}
	keepClearance(t, ua, time.Now().Add(time.Hour))
	resp, err := fetchWithRod(context.Background(), url, "", nil, nil, nil, 0, nil)
	if err != nil || !strings.Contains(resp.HTML, "ordinary page") {
		t.Fatalf("rod with a kept clearance: %v", err)
	}
}

func TestAnExpiredClearanceIsNotUsed(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineRod)
	url := clearanceSite(t)
	ctx := browserOnly(t)
	var ua string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua))
	keepClearance(t, ua, time.Now().Add(1500*time.Millisecond))
	time.Sleep(2 * time.Second)
	if _, err := navigateAndReadWithCloudflareCheck(browserOnly(t), url, "", func() {}, 2*time.Second, nil, nil, nil, 0, nil); err == nil {
		t.Fatal("an expired clearance must not let the page through")
	}
}

// A person's clearance is kept: only cf_clearance, only a persistent one, with
// the browser's user agent.
func TestAHumanClearanceIsKeptAndNothingElseIs(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineRod)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "cf_clearance=ABC123; Max-Age=3600; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "session=SECRET; Max-Age=3600; Path=/")
		w.Header().Add("Set-Cookie", "cf_bm=other; Max-Age=3600; Path=/")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer srv.Close()
	ctx := browserOnly(t)
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/")); err != nil {
		t.Fatal(err)
	}
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().WithURLs([]string{srv.URL + "/"}).Do(ctx)
		return err
	})); err != nil {
		t.Fatal(err)
	}
	var ua string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua))

	storeHumanClearance(ctx, srv.URL+"/", cookies)

	got, err := shared.ClearancesFor("127.0.0.1", time.Now())
	if err != nil || len(got) != 1 {
		t.Fatalf("exactly the clearance must be kept: %v %+v", err, got)
	}
	c := got[0]
	if c.Name != "cf_clearance" || c.Value != "ABC123" || c.UserAgent != ua || c.Source != "human" || !c.HTTPOnly || !c.ExpiresAt.After(time.Now().Add(50*time.Minute)) {
		t.Errorf("%+v", c)
	}
	var n int
	if err := shared.DB.QueryRow(`SELECT COUNT(*) FROM domain_clearance WHERE name <> 'cf_clearance'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("no other cookie may be kept: %d (%v)", n, err)
	}
}

func TestSeedDomain(t *testing.T) {
	for in, want := range map[string]string{"example.com": ".example.com", "127.0.0.1": "127.0.0.1", "localhost": "localhost", "a.b.example.co.uk": ".a.b.example.co.uk"} {
		if got := seedDomain(in); got != want {
			t.Errorf("seedDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
