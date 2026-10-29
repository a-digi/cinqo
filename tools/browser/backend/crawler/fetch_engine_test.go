package crawler

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

// useBrowserSettings opens a real settings database in a temp directory
// (the real schema, through shared.InitDB) and stores engine as the
// current choice. Closes the database afterwards.
func useBrowserSettings(t *testing.T, engine string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("TOOL_DB_DIR", filepath.Join(root, "db"))
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := shared.InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { shared.DB.Close() })
	setEngine(t, engine)
}

// setEngine changes the stored engine the way the Engine page's PUT does.
func setEngine(t *testing.T, engine string) {
	t.Helper()
	s, err := shared.LoadBrowserSettings()
	if err != nil {
		t.Fatal(err)
	}
	s.FetchEngine = engine
	if err := shared.SaveBrowserSettings(s); err != nil {
		t.Fatal(err)
	}
}

// resetEngineTracking gives a test a clean slate of the switch bookkeeping
// and restores it afterwards.
func resetEngineTracking(t *testing.T) {
	t.Helper()
	engineMu.Lock()
	savedCurrent, savedInFlight := engineCurrent, engineInFlight
	engineCurrent, engineInFlight = "", map[string]int{}
	engineMu.Unlock()
	t.Cleanup(func() {
		engineMu.Lock()
		engineCurrent, engineInFlight = savedCurrent, savedInFlight
		engineMu.Unlock()
	})
}

// recordStops replaces the engines' stop functions with recorders.
func recordStops(t *testing.T) *[]string {
	t.Helper()
	var mu sync.Mutex
	stops := &[]string{}
	orig := engineStoppers
	engineStoppers = map[string]func(){
		fetchEngineRod:        func() { mu.Lock(); *stops = append(*stops, "rod"); mu.Unlock() },
		fetchEnginePlaywright: func() { mu.Lock(); *stops = append(*stops, "playwright"); mu.Unlock() },
	}
	t.Cleanup(func() { engineStoppers = orig })
	return stops
}

func swapFetchEngines(t *testing.T) (playwrightCalls, rodCalls *[]string) {
	t.Helper()
	pw, rd := &[]string{}, &[]string{}
	origPW, origRod := playwrightFetch, rodFetch
	playwrightFetch = func(_ context.Context, rawURL, requestID string, _, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		*pw = append(*pw, rawURL+"|"+requestID)
		return crawlResponse{Title: "from-playwright"}, nil
	}
	rodFetch = func(_ context.Context, rawURL, requestID string, _, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		*rd = append(*rd, rawURL+"|"+requestID)
		return crawlResponse{Title: "from-rod"}, nil
	}
	t.Cleanup(func() { playwrightFetch, rodFetch = origPW, origRod })
	return pw, rd
}

func fetchOnce(t *testing.T) crawlResponse {
	t.Helper()
	resp, err := fetchWithEngine(context.Background(), "https://x.test/", "req", nil, nil, nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestSelectedEngineFollowsTheStoredSettingLive(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineRod)
	if got := selectedFetchEngine(); got != fetchEngineRod {
		t.Fatalf("stored rod: got %q", got)
	}
	// Flip it the way the Engine page does — no restart, no rebuild.
	setEngine(t, shared.FetchEnginePlaywright)
	if got := selectedFetchEngine(); got != fetchEnginePlaywright {
		t.Fatalf("after switching to playwright: got %q", got)
	}
	setEngine(t, shared.FetchEngineRod)
	if got := selectedFetchEngine(); got != fetchEngineRod {
		t.Fatalf("after switching back: got %q", got)
	}
	if defaultFetchEngine != fetchEngineRod {
		t.Errorf("the default engine must be rod")
	}
}

func TestEnvironmentVariableNoLongerSelectsTheEngine(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineRod)
	t.Setenv("BROWSER_TOOL_FETCH_ENGINE", "playwright") // retired in step 50
	if got := selectedFetchEngine(); got != fetchEngineRod {
		t.Errorf("the stored setting is the only source of truth, got %q", got)
	}
}

func TestUnreadableSettingsFallBackToRodNeverPlaywright(t *testing.T) {
	// No database at all (a process that has not opened one).
	saved := shared.DB
	shared.DB = nil
	if got := selectedFetchEngine(); got != fetchEngineRod {
		t.Errorf("no database: got %q, want rod", got)
	}
	shared.DB = saved

	// A database that fails on read (closed underneath).
	useBrowserSettings(t, shared.FetchEnginePlaywright)
	shared.DB.Close()
	if got := selectedFetchEngine(); got != fetchEngineRod {
		t.Errorf("a read error must fall back to rod, not to the stored/previous choice or Playwright: got %q", got)
	}
}

func TestFetchWithEngineDispatchesOnTheStoredSetting(t *testing.T) {
	pw, rd := swapFetchEngines(t)
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)

	if resp := fetchOnce(t); resp.Title != "from-rod" {
		t.Fatalf("default: %+v", resp)
	}
	setEngine(t, shared.FetchEnginePlaywright)
	if resp := fetchOnce(t); resp.Title != "from-playwright" {
		t.Fatalf("playwright: %+v", resp)
	}
	if len(*rd) != 1 || len(*pw) != 1 {
		t.Errorf("rod calls %d, playwright calls %d, want 1 and 1", len(*rd), len(*pw))
	}
	if (*rd)[0] != "https://x.test/|req" {
		t.Errorf("arguments must be passed through untouched, got %q", (*rd)[0])
	}
}

func TestFetchWithEnginePassesEveryArgumentThrough(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	var gotExpected, gotRemove, gotAttrs, gotIgnore []string
	var gotMax int
	orig := rodFetch
	rodFetch = func(_ context.Context, _, _ string, expected, remove, attrs []string, max int, ignore []string) (crawlResponse, error) {
		gotExpected, gotRemove, gotAttrs, gotMax, gotIgnore = expected, remove, attrs, max, ignore
		return crawlResponse{}, nil
	}
	t.Cleanup(func() { rodFetch = orig })

	_, _ = fetchWithEngine(context.Background(), "u", "", []string{"#a"}, []string{".b"}, []string{"style"}, 77, []string{"data-x"})
	if !reflect.DeepEqual(gotExpected, []string{"#a"}) || !reflect.DeepEqual(gotRemove, []string{".b"}) ||
		!reflect.DeepEqual(gotAttrs, []string{"style"}) || gotMax != 77 || !reflect.DeepEqual(gotIgnore, []string{"data-x"}) {
		t.Errorf("arguments were altered: %v %v %v %d %v", gotExpected, gotRemove, gotAttrs, gotMax, gotIgnore)
	}
}

func TestSwitchingStopsTheUnselectedEngineOnlyOnAChange(t *testing.T) {
	swapFetchEngines(t)
	resetEngineTracking(t)
	stops := recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)

	fetchOnce(t)
	fetchOnce(t)
	if len(*stops) != 0 {
		t.Fatalf("with an unchanged selection nothing may be stopped (and the other engine never started): %v", *stops)
	}

	setEngine(t, shared.FetchEnginePlaywright)
	fetchOnce(t)
	if !reflect.DeepEqual(*stops, []string{"rod"}) {
		t.Fatalf("switching to playwright must stop rod once: %v", *stops)
	}
	fetchOnce(t)
	if len(*stops) != 1 {
		t.Fatalf("a second fetch on the same engine must not stop anything again: %v", *stops)
	}

	setEngine(t, shared.FetchEngineRod)
	fetchOnce(t)
	if !reflect.DeepEqual(*stops, []string{"rod", "playwright"}) {
		t.Fatalf("switching back must stop playwright: %v", *stops)
	}
}

// A fetch already running on the old engine is never interrupted by a
// switch: the old engine is stopped only after its last fetch finishes.
func TestSwitchNeverInterruptsAFetchInFlight(t *testing.T) {
	_, _ = swapFetchEngines(t)
	resetEngineTracking(t)
	stops := recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)

	started := make(chan struct{})
	release := make(chan struct{})
	origRod := rodFetch
	rodFetch = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		close(started)
		<-release
		return crawlResponse{Title: "slow-rod"}, nil
	}
	t.Cleanup(func() { rodFetch = origRod })

	done := make(chan crawlResponse, 1)
	go func() { done <- fetchOnce(t) }()
	<-started

	// Switch while that rod fetch is still running, then fetch on Playwright.
	setEngine(t, shared.FetchEnginePlaywright)
	if resp := fetchOnce(t); resp.Title != "from-playwright" {
		t.Fatalf("the new selection must be used for the new fetch: %+v", resp)
	}
	if len(*stops) != 0 {
		t.Fatalf("rod still has a fetch in flight and must not be stopped yet: %v", *stops)
	}

	close(release)
	if resp := <-done; resp.Title != "slow-rod" {
		t.Fatalf("the in-flight fetch must complete normally: %+v", resp)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(*stops) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !reflect.DeepEqual(*stops, []string{"rod"}) {
		t.Fatalf("rod must be stopped once its last fetch finished: %v", *stops)
	}
}

func TestSwitchingBackQuicklyNeverStopsTheEngineJustSelected(t *testing.T) {
	swapFetchEngines(t)
	resetEngineTracking(t)
	stops := recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)

	fetchOnce(t) // rod
	setEngine(t, shared.FetchEnginePlaywright)
	fetchOnce(t) // stops rod
	setEngine(t, shared.FetchEngineRod)
	fetchOnce(t) // stops playwright, rod is current again
	if !reflect.DeepEqual(*stops, []string{"rod", "playwright"}) {
		t.Fatalf("each switch stops only the engine left behind: %v", *stops)
	}
	engineMu.Lock()
	current := engineCurrent
	engineMu.Unlock()
	if current != fetchEngineRod {
		t.Errorf("the current engine must be rod, got %q", current)
	}
}

// With the default (rod) and no Chrome, the failure must be a plain error:
// crawl.go treats exactly that as "engine failed, fall through to the
// chromedp primary tab" — and the Playwright engine must stay unstarted.
func TestFetchWithEngineDefaultWithoutChromeFallsThroughAsAPlainError(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")
	origLook := rodLookPath
	rodLookPath = func() (string, bool) { return "", false }
	t.Cleanup(func() { rodLookPath = origLook })

	_, err := fetchWithEngine(context.Background(), "https://x.test/", "", nil, nil, nil, 0, nil)
	if !errors.Is(err, errRodNoChrome) {
		t.Fatalf("want errRodNoChrome, got %v", err)
	}
	var ce *crawlError
	if errors.As(err, &ce) {
		t.Errorf("must not be a *crawlError (it would be treated as a Cloudflare verdict): %v", err)
	}
	if pwInst != nil || pwBrowser != nil {
		t.Errorf("the Playwright engine must not have been started")
	}
}

// End to end through the selector with a real Chrome: content, and a
// Cloudflare verdict, exactly as crawl.go's switch expects them.
func TestFetchWithEngineRodEndToEnd(t *testing.T) {
	requireRodChrome(t)
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/ok": htmlPage(`<html><head><title>Hello</title></head><body><p>content</p></body></html>`),
		"/cf": func(w http.ResponseWriter, r *http.Request) {
			// A body, like Cloudflare's real interstitial: Chrome replaces
			// an EMPTY-bodied 4xx with its own error page, which fails the
			// navigation before any header can be read (a plain error, so
			// the caller would simply fall through to chromedp).
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<body>Just a moment...</body>`))
		},
	})

	resp, err := fetchWithEngine(context.Background(), srv.URL+"/ok", "", nil, nil, nil, 0, nil)
	if err != nil || resp.Title != "Hello" || !strings.Contains(resp.HTML, "content") {
		t.Fatalf("%+v %v", resp, err)
	}
	_, err = fetchWithEngine(context.Background(), srv.URL+"/cf", "", nil, nil, nil, 0, nil)
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Code != codeCloudflareUnresolved {
		t.Fatalf("a challenge must come back as the unresolved verdict crawl.go escalates: %v", err)
	}
	if pwInst != nil || pwBrowser != nil {
		t.Errorf("the Playwright engine must not have been started while rod served the fetches")
	}
}
