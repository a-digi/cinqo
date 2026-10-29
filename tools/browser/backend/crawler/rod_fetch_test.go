package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/stealth"
)

// rodTestServer serves a few canned pages for the fetch tests.
func rodTestServer(t *testing.T, mux map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	m := http.NewServeMux()
	for path, h := range mux {
		m.HandleFunc(path, h)
	}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv
}

func htmlPage(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

func fetchRod(t *testing.T, ctx context.Context, url string) (crawlResponse, error) {
	t.Helper()
	return fetchWithRod(ctx, url, "", nil, nil, nil, 0, nil)
}

func TestFetchWithRodReadsAPageLikeTheOtherEngines(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/final": htmlPage(`<!doctype html><html><head><title>Jobs list</title><style>.x{color:red}</style></head>
			<body><div class="ad">buy</div><ul><li><a href="/job/1">Go developer</a></li></ul>
			<script>document.body.setAttribute("data-js", "ran")</script><svg><circle/></svg></body></html>`),
		"/start": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) },
	})

	resp, err := fetchWithRod(context.Background(), srv.URL+"/start", "", nil, []string{".ad"}, nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Jobs list" {
		t.Errorf("title = %q (it must be read before <head> is removed)", resp.Title)
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
	if resp.Truncated {
		t.Errorf("a small page must not be truncated")
	}
}

func TestFetchWithRodStripsLongAttributesButKeepsURLs(t *testing.T) {
	requireRodChrome(t)
	long := strings.Repeat("x", 300)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<body><a href="/` + long + `" data-blob="` + long + `" title="short">link</a></body>`),
	})
	resp, err := fetchWithRod(context.Background(), srv.URL, "", nil, nil, nil, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.HTML, "data-blob") {
		t.Errorf("a long attribute must be stripped: %s", resp.HTML)
	}
	if !strings.Contains(resp.HTML, long) || !strings.Contains(resp.HTML, `title="short"`) {
		t.Errorf("href is exempt from the length cap and short attributes stay: %s", resp.HTML)
	}
}

func TestFetchWithRodTruncatesOversizedPages(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<body><p>` + strings.Repeat("a", MaxHTMLBytes+5000) + `</p></body>`),
	})
	resp, err := fetchRod(t, context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated || len(resp.HTML) != MaxHTMLBytes {
		t.Errorf("truncated=%v len=%d, want true and %d", resp.Truncated, len(resp.HTML), MaxHTMLBytes)
	}
}

func TestFetchWithRodReportsCloudflareFromTheWire(t *testing.T) {
	requireRodChrome(t)
	cf := func(value string, status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("CF-Mitigated", value) // mixed case on purpose
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`<body>Just a moment...</body>`))
		}
	}
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/block":     cf("block", 403),
		"/challenge": cf("Challenge", 403),
	})

	_, err := fetchRod(t, context.Background(), srv.URL+"/block")
	var ce *crawlError
	if !errors.As(err, &ce) || ce.Code != codeCloudflareBlocked || !strings.Contains(ce.Reason, "HTTP 403") {
		t.Errorf("block: %v", err)
	}
	_, err = fetchRod(t, context.Background(), srv.URL+"/challenge")
	if !errors.As(err, &ce) || ce.Code != codeCloudflareUnresolved {
		t.Errorf("challenge: %v", err)
	}
}

func TestFetchWithRodDetectsChallengePagesByMarkers(t *testing.T) {
	requireRodChrome(t)
	challenge := `<html><head><title>Just a moment...</title></head><body>
		<script>window._cf_chl_opt = {cvId: "3"};</script>
		<a href="https://www.cloudflare.com/?utm_source=challenge">Cloudflare</a></body></html>`
	block := `<html><head><title>Attention Required! | Cloudflare</title></head><body>Sorry, you have been blocked</body></html>`
	// A real page that merely embeds the markers but shows the expected content.
	withContent := `<html><body><script>window._cf_chl_opt = {};</script><h1 id="jobs">Jobs</h1></body></html>`
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/challenge": htmlPage(challenge), "/block": htmlPage(block), "/content": htmlPage(withContent),
	})

	var ce *crawlError
	if _, err := fetchRod(t, context.Background(), srv.URL+"/challenge"); !errors.As(err, &ce) || ce.Code != codeCloudflareUnresolved {
		t.Errorf("challenge markers: %v", err)
	}
	if _, err := fetchRod(t, context.Background(), srv.URL+"/block"); !errors.As(err, &ce) || ce.Code != codeCloudflareBlocked {
		t.Errorf("block markers: %v", err)
	}
	// Expected content visible always wins over the markers.
	resp, err := fetchWithRod(context.Background(), srv.URL+"/content", "", []string{"#jobs"}, nil, nil, 0, nil)
	if err != nil || !strings.Contains(resp.HTML, "Jobs") {
		t.Errorf("expected content must win: %v %v", err, resp.HTML)
	}
}

func TestFetchWithRodSurvivesJavaScriptDialogs(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<body><script>alert("hello"); confirm("sure?"); document.title = "after dialogs";</script><p>done</p></body>`),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := fetchRod(t, ctx, srv.URL)
	if err != nil {
		t.Fatalf("a page with dialogs must still load: %v", err)
	}
	if resp.Title != "after dialogs" || !strings.Contains(resp.HTML, "done") {
		t.Errorf("the script after the dialogs must have run: %+v", resp)
	}
}

func TestFetchWithRodHonoursCancellation(t *testing.T) {
	requireRodChrome(t)
	release := make(chan struct{})
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/slow": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		},
	})
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := fetchRod(t, ctx, srv.URL+"/slow")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("cancellation took %s — it must interrupt the navigation, not wait out crawlTimeout", elapsed)
	}
	var ce *crawlError
	if errors.As(err, &ce) {
		t.Errorf("a cancellation must not be a Cloudflare verdict")
	}
}

func TestFetchWithRodReturnsAPlainErrorWhenThePageCannotLoad(t *testing.T) {
	requireRodChrome(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening any more

	_, err := fetchRod(t, context.Background(), url)
	if err == nil {
		t.Fatal("an unreachable page must fail")
	}
	var ce *crawlError
	if errors.As(err, &ce) {
		t.Errorf("a connection failure must be a plain error (so the caller falls back to chromedp), got %v", err)
	}
}

func TestFetchWithRodClosesItsPages(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage(`<body>x</body>`)})
	browser, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	before, err := browser.Pages()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := fetchRod(t, context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	// Closing is asynchronous in Chrome: allow it a moment to settle.
	deadline := time.Now().Add(5 * time.Second)
	for {
		after, err := browser.Pages()
		if err != nil {
			t.Fatal(err)
		}
		if len(after) <= len(before) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pages leaked: %d before, %d after", len(before), len(after))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestFetchWithRodBoundsConcurrentPages(t *testing.T) {
	requireRodChrome(t)

	rodSlotsMu.Lock()
	saved := rodSlots
	rodSlots = make(chan struct{}, 2)
	rodSlotsMu.Unlock()
	t.Cleanup(func() {
		rodSlotsMu.Lock()
		rodSlots = saved
		rodSlotsMu.Unlock()
	})

	var inFlight, peak atomic.Int32
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" { // Chrome also asks for /favicon.ico: not a page load
				http.NotFound(w, r)
				return
			}
			n := inFlight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(400 * time.Millisecond)
			inFlight.Add(-1)
			htmlPage(`<body>ok</body>`)(w, r)
		},
	})

	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fetchRod(t, context.Background(), srv.URL); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a queued fetch failed: %v", err)
	}
	if peak.Load() > 2 {
		t.Errorf("at most 2 pages may load at once, saw %d", peak.Load())
	}
}

func TestFetchWithRodWaitingForASlotIsCancellable(t *testing.T) {
	// No browser needed: the slot wait happens before it is ever started.
	rodSlotsMu.Lock()
	saved := rodSlots
	rodSlots = make(chan struct{}, 1)
	rodSlots <- struct{}{} // the only slot is taken
	rodSlotsMu.Unlock()
	t.Cleanup(func() {
		rodSlotsMu.Lock()
		rodSlots = saved
		rodSlotsMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := fetchWithRod(ctx, "http://127.0.0.1:1/", "", nil, nil, nil, 0, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a caller waiting for a slot must be released by its context, got %v", err)
	}
}

// --- DOM-stable wait ---

func TestDOMSettleJSFallsBackOnNonPositiveArguments(t *testing.T) {
	js := domSettleJS(0, -5)
	quiet := int(rodDOMQuiet / time.Millisecond)
	max := int(rodDOMMaxWait / time.Millisecond)
	if !strings.Contains(js, fmt.Sprintf("})(%d, %d)", quiet, max)) {
		t.Errorf("bad arguments must fall back to the defaults (%d, %d): %s", quiet, max, js[len(js)-30:])
	}
	if js2 := domSettleJS(300, 1000); !strings.Contains(js2, "})(300, 1000)") {
		t.Errorf("explicit arguments must be used: %s", js2[len(js2)-30:])
	}
}

// buildUpPage appends one element every stepMs, count times, with ids
// step-1 … step-N — a page that keeps rendering after the load event.
func buildUpPage(count, stepMs int) http.HandlerFunc {
	return htmlPage(fmt.Sprintf(`<body><div id="root"></div><script>
		var n = 0;
		var timer = setInterval(function() {
			n++;
			var el = document.createElement("p");
			el.id = "step-" + n;
			el.textContent = "item " + n;
			document.getElementById("root").appendChild(el);
			if (n >= %d) clearInterval(timer);
		}, %d);
	</script></body>`, count, stepMs))
}

func TestFetchWithRodWaitsForAPageThatKeepsRenderingAfterLoad(t *testing.T) {
	requireRodChrome(t)
	// One element every 250 ms for 1.5 s: no quiet gap reaches the 500 ms
	// window until the very last one, so reading right after load would
	// miss most of them.
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": buildUpPage(6, 250)})

	resp, err := fetchRod(t, context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"step-1", "step-3", "step-6"} {
		if !strings.Contains(resp.HTML, `id="`+id+`"`) {
			t.Errorf("%s is missing — the page was read before it finished rendering: %s", id, resp.HTML)
		}
	}
}

func TestDOMSettleReportsStableAndTimeout(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/static": htmlPage(`<body><p>nothing changes</p></body>`),
		// Mutates every 50 ms forever.
		"/churn": htmlPage(`<body><div id="r"></div><script>setInterval(function(){
			document.getElementById("r").textContent = String(Date.now());}, 50);</script></body>`),
	})
	browser, err := ensureRodEngine()
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/static": "stable", "/churn": "timeout"} {
		page, err := stealth.Page(browser)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.Navigate(srv.URL + path); err != nil {
			t.Fatal(err)
		}
		if err := page.WaitLoad(); err != nil {
			t.Fatal(err)
		}
		got, err := rodEvalString(page, domSettleJS(300, 1200))
		_ = page.Close()
		if err != nil || got != want {
			t.Errorf("%s: got %q (%v), want %q", path, got, err, want)
		}
	}
}

func TestFetchWithRodNeverWaitsPastTheCapOnAPageThatNeverSettles(t *testing.T) {
	requireRodChrome(t)
	origQuiet, origMax := rodDOMQuiet, rodDOMMaxWait
	rodDOMQuiet, rodDOMMaxWait = 300*time.Millisecond, 1200*time.Millisecond
	t.Cleanup(func() { rodDOMQuiet, rodDOMMaxWait = origQuiet, origMax })

	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<html><head><title>Ticker</title></head><body><div id="r">start</div><script>
			setInterval(function(){ document.getElementById("r").textContent = String(Date.now()); }, 40);</script></body></html>`),
	})
	start := time.Now()
	resp, err := fetchRod(t, context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("a page that never settles must still be returned, not failed: %v", err)
	}
	if resp.Title != "Ticker" || !strings.Contains(resp.HTML, `id="r"`) {
		t.Errorf("content missing: %+v", resp)
	}
	// Load + cap + read, with generous slack for a slow machine.
	if elapsed := time.Since(start); elapsed > 12*time.Second {
		t.Errorf("took %s — the wait must be capped at %s", elapsed, rodDOMMaxWait)
	}
}

func TestFetchWithRodAStaticPageOnlyPaysTheQuietWindow(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage(`<body><p>static</p></body>`)})
	// Warm the browser up first so only the fetch itself is timed.
	if _, err := fetchRod(t, context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := fetchRod(t, context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > rodDOMMaxWait {
		t.Errorf("a static page took %s — it must settle after the quiet window (%s), not run into the cap (%s)", elapsed, rodDOMQuiet, rodDOMMaxWait)
	}
}

func TestFetchWithRodCancellationDuringTheSettleWait(t *testing.T) {
	requireRodChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/": htmlPage(`<body><div id="r"></div><script>setInterval(function(){
			document.getElementById("r").textContent = String(Date.now());}, 30);</script></body>`),
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1200 * time.Millisecond) // after load, inside the settle wait
		cancel()
	}()
	start := time.Now()
	_, err := fetchRod(t, ctx, srv.URL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > rodDOMMaxWait+3*time.Second {
		t.Errorf("cancellation took %s — it must interrupt the settle wait", elapsed)
	}
}
