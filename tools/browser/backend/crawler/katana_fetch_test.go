package crawler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/projectdiscovery/katana/pkg/output"

	"browser-tool-backend/shared"
)

func requireKatanaChrome(t *testing.T) {
	t.Helper()
	requireRodChrome(t)
	t.Cleanup(StopKatanaEngine)
}

func fetchKatana(ctx context.Context, url string) (crawlResponse, error) {
	return fetchWithKatana(ctx, url, "", nil, nil, nil, 0, nil)
}

// --- dispatch (no browser needed) ---

func TestFetchWithEngineDispatchesToKatana(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	var calls []string
	orig := katanaFetch
	katanaFetch = func(_ context.Context, rawURL, requestID string, expected, remove, attrs []string, max int, ignore []string) (crawlResponse, error) {
		calls = append(calls, rawURL+"|"+requestID)
		if !reflect.DeepEqual(expected, []string{"#a"}) || !reflect.DeepEqual(remove, []string{".b"}) || max != 9 {
			t.Errorf("arguments altered: %v %v %d", expected, remove, max)
		}
		return crawlResponse{Title: "from-katana"}, nil
	}
	t.Cleanup(func() { katanaFetch = orig })

	setEngine(t, shared.FetchEngineKatana)
	resp, err := fetchWithEngine(context.Background(), "https://x.test/", "req", []string{"#a"}, []string{".b"}, nil, 9, nil)
	if err != nil || resp.Title != "from-katana" {
		t.Fatalf("katana: %+v %v", resp, err)
	}
	if !reflect.DeepEqual(calls, []string{"https://x.test/|req"}) {
		t.Errorf("calls = %v", calls)
	}
}

func TestKatanaIsStoppedWhenTheEngineIsSwitchedAwayAndOnlyThen(t *testing.T) {
	resetEngineTracking(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	var mu sync.Mutex
	var stops []string
	orig := engineStoppers
	engineStoppers = map[string]func(){
		fetchEngineRod:    func() { mu.Lock(); stops = append(stops, "rod"); mu.Unlock() },
		fetchEngineKatana: func() { mu.Lock(); stops = append(stops, "katana"); mu.Unlock() },
	}
	t.Cleanup(func() { engineStoppers = orig })
	origK, origR := katanaFetch, rodFetch
	katanaFetch = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		return crawlResponse{Title: "k"}, nil
	}
	rodFetch = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		return crawlResponse{Title: "r"}, nil
	}
	t.Cleanup(func() { katanaFetch, rodFetch = origK, origR })

	fetchOnce(t)
	fetchOnce(t)
	if len(stops) != 0 {
		t.Fatalf("an unchanged selection stops nothing: %v", stops)
	}
	setEngine(t, shared.FetchEngineRod)
	fetchOnce(t)
	if !reflect.DeepEqual(stops, []string{"katana"}) {
		t.Fatalf("switching away from katana must stop it once: %v", stops)
	}
}

func TestTheRealStopperTableKnowsKatana(t *testing.T) {
	if engineStoppers[fetchEngineKatana] == nil {
		t.Fatal("engineStoppers has no entry for katana: switching away would leave its Chrome running")
	}
}

// --- the options are locked down (no browser needed) ---

// Every field that could make Katana do more than load and read one page is
// pinned here, so a change of Katana's defaults or an edit of the options
// can never widen what it does without this test failing.
func TestKatanaOptionsAreLockedToOneReadOnlyPage(t *testing.T) {
	seed := "https://a.test/jobs?x=1&y=(2)"
	o := katanaOptions(context.Background(), "ws://127.0.0.1:1/devtools/browser/x", seed, func(output.Result) {})

	// attached to OUR Chrome: nothing launched, found or downloaded by Katana
	if o.ChromeWSUrl != "ws://127.0.0.1:1/devtools/browser/x" || !o.HeadlessHybrid || o.UseInstalledChrome || o.SystemChromePath != "" || o.ShowBrowser {
		t.Errorf("Chrome options: %+v", o)
	}
	if o.ChromeDataDir == "" {
		t.Error("ChromeDataDir must be non-empty so Katana creates and removes no temp folder")
	}
	if o.Proxy != "" || len(o.CustomHeaders) != 0 || len(o.HeadlessOptionalArguments) != 0 {
		t.Errorf("no proxy, headers or Chrome arguments: %q %v %v", o.Proxy, o.CustomHeaders, o.HeadlessOptionalArguments)
	}
	// one page, nothing it links to
	if o.MaxDepth != 1 || o.Concurrency != 1 || o.Parallelism != 1 || o.NoScope || len(o.OutOfScope) != 0 {
		t.Errorf("scope/depth: %+v", o)
	}
	if len(o.Scope) != 1 {
		t.Fatalf("scope: %v", o.Scope)
	}
	re := regexp.MustCompile(o.Scope[0])
	if !re.MatchString(seed) {
		t.Errorf("the page itself must be in scope: %q", o.Scope[0])
	}
	for _, other := range []string{seed + "&z=1", "https://a.test/jobs", "https://a.test/other", "https://evil.test/?u=" + seed, seed + "/x"} {
		if re.MatchString(other) {
			t.Errorf("%q must be out of scope", other)
		}
	}
	// read-only
	if o.AutomaticFormFill || o.FormExtraction || o.KnownFiles != "" || o.ScrapeJSResponses || o.ScrapeJSLuiceResponses || o.XhrExtraction {
		t.Errorf("forms, known files or script crawling are on: %+v", o)
	}
	if o.MaxOnclickLinks >= 0 {
		// 0 is NOT "none": NewCrawlerOptions turns it into Katana's default of 10 clicks.
		t.Errorf("Katana must never click links on the page (MaxOnclickLinks = %d, must be negative)", o.MaxOnclickLinks)
	}
	if o.StoreResponse || o.StoreFields != "" || o.StoreFieldDir != "" || o.OutputFile != "" || o.TechDetect || o.TlsImpersonate || o.HealthCheck || !o.DisableUpdateCheck {
		t.Errorf("storing/detection/update options: %+v", o)
	}
	if o.PageLoadStrategy != "none" || o.Timeout != int(crawlTimeout/time.Second) || o.Context == nil || o.OnResult == nil {
		t.Errorf("load/timeout/context: %+v", o)
	}
}

// Katana's engine Chrome is the ONE resolver's: with a bogus path it is
// unavailable and the error is a plain one (the caller falls through).
func TestKatanaWithoutChromeIsAPlainEngineError(t *testing.T) {
	t.Setenv(shared.ChromePathEnv, "/definitely/not/chrome")
	_, err := fetchKatana(context.Background(), "http://127.0.0.1:1/")
	var ce *crawlError
	if err == nil || errors.As(err, &ce) {
		t.Fatalf("want a plain error, got %v", err)
	}
	if katanaRunning() {
		t.Error("nothing may be running after a failed start")
	}
}

// --- real browser ---

func TestKatanaReadsAPageLikeTheOtherEngines(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/final": htmlPage(`<!doctype html><html><head><title>Jobs list</title><style>.x{color:red}</style></head>
			<body><div class="ad">buy</div><ul><li><a href="/job/1">Go developer</a></li></ul>
			<script>document.body.setAttribute("data-js", "ran")</script><svg><circle/></svg></body></html>`),
		"/start": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) },
	})
	resp, err := fetchWithKatana(context.Background(), srv.URL+"/start", "", nil, []string{".ad"}, nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Jobs list" {
		t.Errorf("title = %q", resp.Title)
	}
	if resp.FinalURL != srv.URL+"/final" {
		t.Errorf("final URL = %q, want the redirect target", resp.FinalURL)
	}
	if !strings.Contains(resp.HTML, "Go developer") || !strings.Contains(resp.HTML, `href="/job/1"`) {
		t.Errorf("content missing: %s", resp.HTML)
	}
	for _, gone := range []string{"<script", "<style", "<svg", "<head", `class="ad"`, "buy"} {
		if strings.Contains(resp.HTML, gone) {
			t.Errorf("%q must have been removed: %s", gone, resp.HTML)
		}
	}
	if !strings.Contains(resp.HTML, `data-js="ran"`) {
		t.Errorf("page JavaScript must have run before the read: %s", resp.HTML)
	}
}

// The strongest parity check: the same page, the same arguments, through rod
// and through Katana, must give the identical HTML, title and final URL.
func TestKatanaAndRodReturnTheSameHTML(t *testing.T) {
	requireKatanaChrome(t)
	long := strings.Repeat("y", 300)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/p": htmlPage(`<!doctype html><html><head><title>T</title></head><body>
			<nav class="menu">menu</nav><main id="m"><h1 style="color:red" data-big="` + long + `">Hello</h1>
			<a href="/a?` + long + `" title="x">link</a><img src="/i.png" alt="pic"></main>
			<script>setTimeout(function(){document.getElementById("m").insertAdjacentHTML("beforeend","<p>late</p>")},200)</script>
			</body></html>`),
	})
	args := func(f fetchFunc) (crawlResponse, error) {
		return f(context.Background(), srv.URL+"/p", "", nil, []string{".menu"}, []string{"style"}, 40, []string{"title"})
	}
	k, err := args(fetchWithKatana)
	if err != nil {
		t.Fatalf("katana: %v", err)
	}
	r, err := args(fetchWithRod)
	if err != nil {
		t.Fatalf("rod: %v", err)
	}
	if k.HTML != r.HTML || k.Title != r.Title || k.FinalURL != r.FinalURL || k.Truncated != r.Truncated {
		t.Errorf("engines differ:\nkatana: %+v\nrod:    %+v", k, r)
	}
	if !strings.Contains(k.HTML, "late") {
		t.Errorf("the late script content must be waited for: %s", k.HTML)
	}
}

// One page, read-only: a link on the page, an onclick link, a form, robots.txt
// and sitemap.xml are never requested, and exactly one document is fetched.
func TestKatanaFetchesExactlyOnePageAndTouchesNothingElse(t *testing.T) {
	requireKatanaChrome(t)
	var mu sync.Mutex
	hits := map[string]int{}
	count := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.Method+" "+r.URL.Path]++
			mu.Unlock()
			h(w, r)
		}
	}
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": count(htmlPage(`<!doctype html><html><head><title>One</title></head><body>
			<a href="/linked">a link</a><a href="#" onclick="location.href='/clicked'">onclick link</a>
			<a onclick="fetch('/danger',{method:'POST'})">delete everything</a>
			<form action="/submit" method="post"><input name="q" value="x"><button>go</button></form>
			<script src="/app.js"></script></body></html>`)),
		"/linked":      count(htmlPage("linked")),
		"/clicked":     count(htmlPage("clicked")),
		"/danger":      count(htmlPage("danger")),
		"/submit":      count(htmlPage("submit")),
		"/robots.txt":  count(htmlPage("User-agent: *")),
		"/sitemap.xml": count(htmlPage("<urlset/>")),
		"/app.js": count(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(`var u="/from-js";`))
		}),
	})
	resp, err := fetchKatana(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.HTML, "a link") {
		t.Errorf("page not read: %s", resp.HTML)
	}
	time.Sleep(2 * time.Second) // a stray follow-up request would show up by now
	mu.Lock()
	defer mu.Unlock()
	for _, never := range []string{"GET /linked", "GET /clicked", "POST /danger", "GET /danger", "POST /submit", "GET /submit", "GET /robots.txt", "GET /sitemap.xml", "GET /from-js"} {
		if hits[never] != 0 {
			t.Errorf("%s was requested %d times; Katana may only load the one page", never, hits[never])
		}
	}
	if hits["GET /"] != 1 {
		t.Errorf("the page itself was requested %d times, want 1", hits["GET /"])
	}
}

// Katana's tab must present the same browser as rod's: no stale device
// user agent, no HeadlessChrome, webdriver hidden by the stealth script.
func TestKatanaTabPresentsAHumanFingerprint(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<!doctype html><html><head><title>fp</title></head><body><pre id="o"></pre>
			<script>document.getElementById("o").textContent = JSON.stringify({ua: navigator.userAgent, wd: navigator.webdriver,
			langs: navigator.languages, w: window.innerWidth, touch: navigator.maxTouchPoints,
			brands: (navigator.userAgentData && navigator.userAgentData.brands || []).map(function(b){return b.brand}).join("|")})</script></body></html>`),
	})
	resp, err := fetchKatana(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`<pre id="o">(.*?)</pre>`).FindStringSubmatch(resp.HTML)
	if m == nil {
		t.Fatalf("no probe output: %s", resp.HTML)
	}
	out := strings.ReplaceAll(m[1], "&quot;", `"`)
	if strings.Contains(out, "HeadlessChrome") || strings.Contains(out, "Chrome/87") || strings.Contains(out, "Mac OS X 11_0_0") {
		t.Errorf("the tab presents a bot or stale device user agent: %s", out)
	}
	if !strings.Contains(out, `"wd":false`) && !strings.Contains(out, `"wd":null`) {
		t.Errorf("navigator.webdriver must be hidden: %s", out)
	}
	if !strings.Contains(out, `"touch":0`) {
		t.Errorf("touch emulation must be cleared: %s", out)
	}
	if !strings.Contains(out, "Chrom") || strings.Contains(out, `"brands":""`) {
		t.Errorf("client hints (brands) are missing, which a user-agent override would cause: %s", out)
	}
}

// A page that redirects to a private/internal address is refused, as a final
// verdict (not an engine failure that would fall through to another tab).
func TestKatanaRefusesARedirectToAPrivateAddress(t *testing.T) {
	requireKatanaChrome(t)
	var target *http.ServeMux
	_ = target
	var port string
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/go": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://localhost:"+port+"/secret", http.StatusFound)
		},
		"/secret": htmlPage("<html><body>internal secret</body></html>"),
	})
	port = srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	// asked as 127.0.0.1, redirected to "localhost": another host name, loopback address
	_, err := fetchKatana(context.Background(), srv.URL+"/go")
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Code != codeRedirectRefused {
		t.Fatalf("want a redirect_refused verdict, got %v", err)
	}
	if strings.Contains(err.Error(), "internal secret") {
		t.Error("the refused page's content leaked into the error")
	}
}

func TestRefuseRedirectToPrivateRules(t *testing.T) {
	for _, c := range []struct {
		asked, final string
		refused      bool
	}{
		{"https://example.com/a", "https://example.com/b", false},
		{"https://example.com/a", "https://EXAMPLE.com/b", false},
		{"http://127.0.0.1:9/a", "http://127.0.0.1:9/b", false}, // same host: the caller's own check applies
		{"https://example.com/a", "http://127.0.0.1/admin", true},
		{"https://example.com/a", "http://localhost/admin", true},
		{"https://example.com/a", "http://169.254.169.254/latest/meta-data", true},
		{"https://example.com/a", "http://10.0.0.5/", true},
		{"https://example.com/a", "http://[::1]/", true},
		{"https://example.com/a", "", false},
	} {
		got := refuseRedirectToPrivate(c.asked, c.final) != nil
		if got != c.refused {
			t.Errorf("%s -> %s: refused=%v, want %v", c.asked, c.final, got, c.refused)
		}
	}
}

// A Cloudflare challenge/denial is returned as the verdict the caller
// escalates (human clearance), exactly as the rod engine does.
func TestKatanaReportsChallengesAndDenialsAsVerdicts(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/challenge": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("cf-mitigated", "challenge")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head><body>checking</body></html>"))
		},
		"/block": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("cf-mitigated", "block")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><body>blocked</body></html>"))
		},
		"/limited": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("<html><body>slow down</body></html>"))
		},
	})
	for path, want := range map[string]string{"/challenge": codeCloudflareUnresolved, "/block": codeCloudflareBlocked, "/limited": codeRateLimited} {
		_, err := fetchKatana(context.Background(), srv.URL+path)
		var ce *crawlError
		if !errors.As(err, &ce) || ce.Code != want {
			t.Errorf("%s: got %v, want code %s", path, err, want)
		}
	}
}

// The caller's cancellation interrupts a slow page and is reported as ctx's
// error; nothing stays open afterwards.
func TestKatanaFetchHonorsCancellationAndLeavesNothingOpen(t *testing.T) {
	requireKatanaChrome(t)
	var started atomic.Bool
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/slow": func(w http.ResponseWriter, r *http.Request) {
			started.Store(true)
			select {
			case <-time.After(20 * time.Second):
			case <-r.Context().Done():
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for !started.Load() {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	begin := time.Now()
	_, err := fetchKatana(ctx, srv.URL+"/slow")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if time.Since(begin) > 12*time.Second {
		t.Errorf("cancellation took %s", time.Since(begin))
	}
	assertNoKatanaLeftovers(t)
}

// After a normal fetch the engine's Chrome has no page and no browser
// context of Katana's left, and goroutines return to where they were.
func TestKatanaFetchLeavesNoPagesContextsOrGoroutines(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage("<html><head><title>x</title></head><body>hi</body></html>")})
	if _, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil { // warm up: Chrome started
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	before := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		if _, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil {
			t.Fatal(err)
		}
	}
	assertNoKatanaLeftovers(t)
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines grew from %d to %d over three fetches", before, after)
	}
}

func assertNoKatanaLeftovers(t *testing.T) {
	t.Helper()
	katanaMu.Lock()
	b := katanaBrowser
	katanaMu.Unlock()
	if b == nil {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctxs, err := proto.TargetGetBrowserContexts{}.Call(b)
		pages, perr := b.Pages()
		if err == nil && perr == nil && len(ctxs.BrowserContextIDs) == 0 && len(pages) <= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("leftovers in the engine's Chrome: contexts=%v (%v) pages=%d (%v)", ctxs, err, len(pages), perr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// StopKatanaEngine ends the Chrome process; a later fetch starts a fresh one.
func TestKatanaChromeIsStoppedAndRestarted(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage("<html><head><title>x</title></head><body>hi</body></html>")})
	if _, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if !katanaRunning() {
		t.Fatal("the engine's Chrome should be running after a fetch")
	}
	katanaMu.Lock()
	pid := katanaLauncher.PID()
	katanaMu.Unlock()
	StopKatanaEngine()
	StopKatanaEngine() // idempotent
	if katanaRunning() {
		t.Fatal("still running after Stop")
	}
	if p, err := os.FindProcess(pid); err == nil {
		if err := p.Signal(syscall.Signal(0)); err == nil {
			time.Sleep(time.Second)
			if err := p.Signal(syscall.Signal(0)); err == nil {
				t.Errorf("Chrome process %d is still alive after Stop", pid)
			}
		}
	}
	if resp, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil || !strings.Contains(resp.HTML, "hi") {
		t.Fatalf("restart: %+v %v", resp, err)
	}
}

// If the engine's Chrome dies, the next fetch notices and starts a new one.
func TestKatanaRecoversWhenItsChromeDied(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage("<html><head><title>x</title></head><body>alive</body></html>")})
	if _, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	katanaMu.Lock()
	katanaLauncher.Kill()
	katanaMu.Unlock()
	time.Sleep(500 * time.Millisecond)
	if resp, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil || !strings.Contains(resp.HTML, "alive") {
		t.Fatalf("after the Chrome died: %+v %v", resp, err)
	}
}

// Katana prints every result it finds to standard output by default. Nothing
// of it may reach this process's output (the tool's stdout is its own).
func TestKatanaWritesNothingToStdoutOrStderr(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage("<html><head><title>q</title></head><body>quiet</body></html>")})
	if _, err := fetchKatana(context.Background(), srv.URL+"/"); err != nil { // start Chrome first
		t.Fatal(err)
	}
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	_, ferr := fetchKatana(context.Background(), srv.URL+"/")
	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	gotOut, _ := io.ReadAll(outR)
	gotErr, _ := io.ReadAll(errR)
	if ferr != nil {
		t.Fatal(ferr)
	}
	if len(gotOut) != 0 || len(gotErr) != 0 {
		t.Errorf("Katana wrote to the process output: stdout=%q stderr=%q", gotOut, gotErr)
	}
}

// A challenge or block recognised only by the page's own markers (no
// cf-mitigated header): a verdict, and expected content on the page wins.
func TestKatanaDetectsChallengePagesByMarkers(t *testing.T) {
	requireKatanaChrome(t)
	challenge := `<html><head><title>Just a moment...</title></head><body>
		<script>window._cf_chl_opt = {cvId: "3"};</script>
		<a href="https://www.cloudflare.com/?utm_source=challenge">Cloudflare</a></body></html>`
	block := `<html><head><title>Attention Required! | Cloudflare</title></head><body>Sorry, you have been blocked</body></html>`
	withContent := `<html><body><script>window._cf_chl_opt = {};</script><h1 id="jobs">Jobs</h1></body></html>`
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/challenge": htmlPage(challenge), "/block": htmlPage(block), "/content": htmlPage(withContent),
	})
	var ce *crawlError
	if _, err := fetchKatana(context.Background(), srv.URL+"/challenge"); !errors.As(err, &ce) || ce.Code != codeCloudflareUnresolved {
		t.Errorf("challenge markers: %v", err)
	}
	if _, err := fetchKatana(context.Background(), srv.URL+"/block"); !errors.As(err, &ce) || ce.Code != codeCloudflareBlocked {
		t.Errorf("block markers: %v", err)
	}
	resp, err := fetchWithKatana(context.Background(), srv.URL+"/content", "", []string{"#jobs"}, nil, nil, 0, nil)
	if err != nil || !strings.Contains(resp.HTML, "Jobs") {
		t.Errorf("expected content must win: %v %v", err, resp.HTML)
	}
}

// An alert()/confirm() would block a page's load forever; Katana's tab
// dismisses them like the other engines' tabs do.
func TestKatanaSurvivesJavaScriptDialogs(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<body><script>alert("hello"); confirm("sure?"); document.title = "after dialogs";</script><p>done</p></body>`),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	resp, err := fetchKatana(ctx, srv.URL)
	if err != nil {
		t.Fatalf("a page with dialogs must still load: %v", err)
	}
	if resp.Title != "after dialogs" || !strings.Contains(resp.HTML, "done") {
		t.Errorf("the script after the dialogs must have run: %+v", resp)
	}
}
