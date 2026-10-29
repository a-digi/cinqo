package crawler

import (
	"browser-tool-backend/shared"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"
)

// requireRodChrome skips a test that needs a real browser when none is
// installed (or under -short), and makes sure the shared engine is torn
// down again afterwards so tests never leak a Chrome process.
func requireRodChrome(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	if _, err := rodChromePath(); err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	t.Cleanup(StopRodEngine)
}

func TestDeriveRodUserAgentAndMajorVersion(t *testing.T) {
	native := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/153.0.0.0 Safari/537.36"
	want := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"
	if got := deriveRodUserAgent(native); got != want {
		t.Errorf("derive = %q, want %q", got, want)
	}
	if got := deriveRodUserAgent(want); got != want {
		t.Errorf("a UA with no headless token must be unchanged, got %q", got)
	}

	for in, want := range map[string]string{
		"Chrome/153.0.8010.53":            "153",
		"HeadlessChrome/153.0.0.0":        "153",
		"Mozilla/5.0 Chrome/9.1.2 Safari": "9",
		"Firefox/120.0":                   "",
		"":                                "",
	} {
		if got := rodMajorVersion(in); got != want {
			t.Errorf("rodMajorVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRodMissingChromeIsAPlainError(t *testing.T) {
	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")
	orig := shared.LookChrome
	shared.LookChrome = func() (string, bool) { return "", false }
	t.Cleanup(func() { shared.LookChrome = orig })

	_, err := ensureRodEngine()
	if !errors.Is(err, errRodNoChrome) {
		t.Fatalf("want errRodNoChrome, got %v", err)
	}
	// Never a *crawlError: the caller must treat it as "engine unavailable"
	// and fall back to chromedp, not as a Cloudflare verdict.
	var ce *crawlError
	if errors.As(err, &ce) {
		t.Errorf("a missing browser must not be a *crawlError: %v", err)
	}
}

func TestRodExplicitChromePathMustExist(t *testing.T) {
	t.Setenv("BROWSER_TOOL_CHROME_PATH", "/definitely/not/a/browser")
	if _, err := rodChromePath(); err == nil || !strings.Contains(err.Error(), "BROWSER_TOOL_CHROME_PATH") {
		t.Fatalf("an unusable explicit path must be reported, got %v", err)
	}
}

func TestRodEngineStartsStopsAndRestarts(t *testing.T) {
	requireRodChrome(t)

	StopRodEngine() // a no-op before any start
	StopRodEngine()

	b1, err := ensureRodEngine()
	if err != nil || b1 == nil {
		t.Fatalf("first start: %v", err)
	}
	if v, err := rodBrowserVersion(b1); err != nil || v.Product == "" {
		t.Fatalf("a started browser must answer CDP: %v %+v", err, v)
	}
	// The same live browser is reused.
	if b2, err := ensureRodEngine(); err != nil || b2 != b1 {
		t.Fatalf("a live browser must be reused: %v same=%v", err, b2 == b1)
	}

	StopRodEngine()
	StopRodEngine() // idempotent
	b3, err := ensureRodEngine()
	if err != nil || b3 == nil || b3 == b1 {
		t.Fatalf("after Stop a fresh browser must start: %v", err)
	}
}

func TestRodEngineRestartsAfterTheBrowserDiesUnderneath(t *testing.T) {
	requireRodChrome(t)

	b1, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	// Kill the Chrome process behind the engine's back (a crash, or an
	// OOM kill) — the browser's own Close() asks it to exit politely and
	// can still answer for a moment, which is not what a crash looks like.
	rodMu.Lock()
	rodLauncher.Kill()
	rodMu.Unlock()

	b2, err := ensureRodEngine()
	if err != nil {
		t.Fatalf("restart after a dead browser: %v", err)
	}
	if b2 == b1 {
		t.Fatal("a dead browser must be replaced, not handed out again")
	}
	if _, err := rodBrowserVersion(b2); err != nil {
		t.Fatalf("the replacement must be alive: %v", err)
	}
}

// fingerprintServer serves a page that reports what the browser looks
// like from inside, and records the request headers the server saw.
type fingerprintServer struct {
	*httptest.Server
	mu      sync.Mutex
	headers http.Header
}

func newFingerprintServer(t *testing.T) *fingerprintServer {
	t.Helper()
	fs := &fingerprintServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.headers = r.Header.Clone()
		fs.mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><title>fp</title><body>ok</body>`))
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fingerprintServer) seen(name string) string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.headers.Get(name)
}

func evalString(t *testing.T, p *rod.Page, js string) string {
	t.Helper()
	res, err := p.Eval(js)
	if err != nil {
		t.Fatalf("eval %q: %v", js, err)
	}
	return res.Value.String()
}

func TestRodBrowserPresentsAHumanFingerprint(t *testing.T) {
	requireRodChrome(t)
	browser, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	fs := newFingerprintServer(t)

	page, err := stealth.Page(browser)
	if err != nil {
		t.Fatalf("stealth page: %v", err)
	}
	defer page.Close()
	if err := page.Navigate(fs.URL); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitLoad(); err != nil {
		t.Fatal(err)
	}

	// The UA on the wire and in JS: no "HeadlessChrome", and identical.
	wireUA := fs.seen("User-Agent")
	jsUA := evalString(t, page, `() => navigator.userAgent`)
	if wireUA == "" || strings.Contains(wireUA, "HeadlessChrome") {
		t.Errorf("User-Agent header must not reveal headless: %q", wireUA)
	}
	if jsUA != wireUA {
		t.Errorf("navigator.userAgent %q differs from the header %q", jsUA, wireUA)
	}
	if got := rodCurrentUserAgent(); got != wireUA {
		t.Errorf("the cached derived UA %q must be what the browser sends %q", got, wireUA)
	}

	// Stealth: the classic automation marker is not set.
	if wd := evalString(t, page, `() => String(navigator.webdriver)`); wd == "true" {
		t.Errorf("navigator.webdriver must not be true, got %q", wd)
	}
	// Language: the Accept-Language header and navigator.languages must
	// agree (a mismatch is a classic bot signal). Which language it is
	// depends on the OS — Chrome on macOS ignores --lang — so only
	// consistency is asserted, not a specific value.
	acceptLang := fs.seen("Accept-Language")
	jsLang := evalString(t, page, `() => navigator.languages[0]`)
	if acceptLang == "" || !strings.HasPrefix(acceptLang, jsLang) {
		t.Errorf("Accept-Language %q must start with navigator.languages[0] %q", acceptLang, jsLang)
	}

	// Client hints must agree with the UA's Chrome major version
	// (the exact mismatch a UA *override* used to cause).
	if hint := fs.seen("Sec-Ch-Ua"); hint != "" {
		major := rodMajorVersion(wireUA)
		if major == "" || !strings.Contains(hint, `"`+major+`"`) {
			t.Errorf("Sec-CH-UA %q must carry the UA's Chrome major %q", hint, major)
		}
	}
}

func TestRodBrowserNeverHasTheAutomationBannerFlag(t *testing.T) {
	requireRodChrome(t)
	browser, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	page, err := stealth.Page(browser)
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()
	if err := page.Navigate("about:blank"); err != nil {
		t.Fatal(err)
	}
	// enable-automation, rod's default, sets navigator.webdriver = true at
	// the browser level; the launcher must have removed it.
	if got := evalString(t, page, `() => String(navigator.webdriver)`); got == "true" {
		t.Errorf("navigator.webdriver = true: enable-automation is still on")
	}
}
