package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

// --- pure functions (no browser) ---

func TestNormalizePageURL(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"https://Example.COM/Jobs?a=1#top", "https://example.com/Jobs?a=1", true},
		{"https://example.com", "https://example.com/", true},
		{"https://example.com/", "https://example.com/", true},
		{"HTTP://example.com:8080/x", "http://example.com:8080/x", true},
		{"  https://example.com/a  ", "https://example.com/a", true},
		{"https://example.com/a b", "https://example.com/a%20b", true},
		{"https://example.com/jobs/121-ü", "https://example.com/jobs/121-%C3%BC", true}, // always the encoded form, so equal URLs compare equal
		{"javascript:void(0)", "", false},
		{"mailto:a@b.c", "", false},
		{"tel:+123", "", false},
		{"data:text/html,x", "", false},
		{"ftp://example.com/x", "", false},
		{"//example.com/x", "", false},
		{"/relative", "", false},
		{"", "", false},
		{"   ", "", false},
		{"https://user:pw@example.com/x", "", false},
		{"https://user@example.com/x", "", false},
		{"https:///nohost", "", false},
		{"https://exa mple.com/", "", false},
	} {
		got, ok := normalizePageURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: got %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsPrivateLinkHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "app.localhost": true, "printer.local": true, "db.internal": true,
		"127.0.0.1": true, "10.0.0.5": true, "192.168.1.1": true, "172.16.0.1": true, "169.254.169.254": true,
		"::1": true, "[::1]": true, "fe80::1": true, "0.0.0.0": true,
		"example.com": false, "8.8.8.8": false, "93.184.216.34": false, "localhost.example.com": false, "notlocal": false,
	} {
		if got := isPrivateLinkHost(host); got != want {
			t.Errorf("%q: got %v, want %v", host, got, want)
		}
	}
}

func TestBuildLinkListMergesTextsAndKeepsPageOrder(t *testing.T) {
	raw := []rawPageLink{
		{Href: "https://x.test/jobs/1", Text: "Backend Developer", Kind: "a"},
		{Href: "https://x.test/jobs/2", Text: "DevOps Engineer", Title: "Job two", Kind: "a"},
		{Href: "https://x.test/jobs/1#apply", Text: "More", Kind: "a"},        // same URL (fragment), second text
		{Href: "https://x.test/jobs/1", Text: "Backend Developer", Kind: "a"}, // same text again: not repeated
		{Href: "https://x.test", Text: "Home", Kind: "a"},
		{Href: "https://x.test/", Text: "Start", Kind: "a"}, // bare host == with slash
		{Href: "https://x.test/list?page=2", Rel: "next", Kind: "link"},
		{Href: "https://x.test/list?page=2", Text: "2", Rel: "NEXT nofollow", Kind: "a"},
		{Href: "javascript:void(0)", Text: "js"},
		{Href: "mailto:a@b.c", Text: "mail"},
		{Href: "", Text: "empty"},
	}
	got, dropped, truncated := buildLinkList(raw, "x.test")
	want := []pageLink{
		{URL: "https://x.test/jobs/1", Texts: []string{"Backend Developer", "More"}},
		{URL: "https://x.test/jobs/2", Texts: []string{"DevOps Engineer"}, TitleAttr: "Job two"},
		{URL: "https://x.test/", Texts: []string{"Home", "Start"}},
		{URL: "https://x.test/list?page=2", Texts: []string{"2"}, Rel: "next nofollow"},
	}
	if !reflect.DeepEqual(got, want) || dropped != 0 || truncated {
		t.Errorf("got %+v dropped=%d truncated=%v\nwant %+v", got, dropped, truncated, want)
	}
}

func TestBuildLinkListDropsPrivateLinksOfOtherHostsButKeepsTheOwnHost(t *testing.T) {
	raw := []rawPageLink{
		{Href: "https://public.test/a", Text: "ok"},
		{Href: "http://127.0.0.1:9/admin", Text: "loopback"},
		{Href: "http://169.254.169.254/latest/meta-data", Text: "metadata"},
		{Href: "http://localhost/x", Text: "local name"},
		{Href: "http://10.1.2.3/", Text: "lan"},
		{Href: "http://[::1]/", Text: "v6"},
	}
	got, dropped, _ := buildLinkList(raw, "public.test")
	if len(got) != 1 || got[0].URL != "https://public.test/a" || dropped != 5 {
		t.Errorf("got %+v dropped=%d", got, dropped)
	}
	// A page that itself lives on a private host (a dev/test page the caller was allowed to load) keeps its own links only.
	raw = []rawPageLink{{Href: "http://127.0.0.1:9/job/1", Text: "own"}, {Href: "http://10.0.0.1/x", Text: "other private"}}
	got, dropped, _ = buildLinkList(raw, "127.0.0.1")
	if len(got) != 1 || got[0].URL != "http://127.0.0.1:9/job/1" || dropped != 1 {
		t.Errorf("own-host links: got %+v dropped=%d", got, dropped)
	}
}

func TestBuildLinkListCapsDistinctURLsButStillMergesKnownOnes(t *testing.T) {
	var raw []rawPageLink
	for i := 0; i < maxPageLinks+50; i++ {
		raw = append(raw, rawPageLink{Href: fmt.Sprintf("https://x.test/j/%d", i), Text: "t"})
	}
	raw = append(raw, rawPageLink{Href: "https://x.test/j/0", Text: "late text"}) // a known URL after the cap still merges
	got, _, truncated := buildLinkList(raw, "x.test")
	if len(got) != maxPageLinks || !truncated {
		t.Fatalf("len=%d truncated=%v", len(got), truncated)
	}
	if !reflect.DeepEqual(got[0].Texts, []string{"t", "late text"}) {
		t.Errorf("first link texts: %v", got[0].Texts)
	}
}

func TestBuildLinkListBoundsTextsAndTheirLength(t *testing.T) {
	var raw []rawPageLink
	for i := 0; i < 10; i++ {
		raw = append(raw, rawPageLink{Href: "https://x.test/a", Text: fmt.Sprintf("text %d", i)})
	}
	raw = append(raw, rawPageLink{Href: "https://x.test/b", Text: strings.Repeat("ä", 500), Title: "  spaced \n  out  "})
	got, _, _ := buildLinkList(raw, "x.test")
	if len(got[0].Texts) != maxTextsPerLink {
		t.Errorf("texts per link: %d", len(got[0].Texts))
	}
	if n := len([]rune(got[1].Texts[0])); n != maxLinkTextRunes {
		t.Errorf("text length: %d runes", n)
	}
	if got[1].TitleAttr != "spaced out" {
		t.Errorf("title attr: %q", got[1].TitleAttr)
	}
}

func TestFetchLinksIsAListedRouteOfTheManifest(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct{ Method, PathSuffix, RequiredScope string } `json:"routes"`
	}
	// field names in the manifest are snake_case
	var generic struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	_ = m
	found := false
	for _, r := range generic.Routes {
		if r["path_suffix"] == "fetch-links" {
			found = true
			if r["method"] != "POST" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %v", r)
			}
		}
	}
	if !found {
		t.Error("fetch-links is not a route in manifest.json: the core would not forward it")
	}
}

// --- the handler (stubbed fetch, real settings database) ---

func postFetchLinks(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	FetchLinksHandler(rec, httptest.NewRequest(http.MethodPost, "/fetch-links", strings.NewReader(body)))
	return rec
}

func stubFetchLinks(t *testing.T, fn func(ctx context.Context, rawURL, requestID string) (fetchLinksResponse, error)) *int {
	t.Helper()
	calls := 0
	var mu sync.Mutex
	orig := fetchLinksFn
	fetchLinksFn = func(ctx context.Context, rawURL, requestID string) (fetchLinksResponse, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return fn(ctx, rawURL, requestID)
	}
	t.Cleanup(func() { fetchLinksFn = orig })
	return &calls
}

const publicTestURL = "http://93.184.216.34/jobs" // an IP literal: validateCrawlURL needs no DNS

func TestFetchLinksRefusesAnotherMethod(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		FetchLinksHandler(rec, httptest.NewRequest(m, "/fetch-links", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
}

func TestFetchLinksRefusesAnythingButUrlRequestIdAndRateLimitKey(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	calls := stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) {
		return fetchLinksResponse{}, nil
	})
	for _, body := range []string{
		``, `not json`, `{}`, `{"url":""}`, `{"url":"   "}`,
		`{"url":"` + publicTestURL + `","useLoginWindow":true,"requestId":"r"}`,
		`{"url":"` + publicTestURL + `","aiContextId":"c"}`,
		`{"url":"` + publicTestURL + `","removeSelectors":["a"]}`,
		`{"url":"` + publicTestURL + `","cookies":"x"}`,
	} {
		if rec := postFetchLinks(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, rec.Code)
		}
	}
	if *calls != 0 {
		t.Errorf("a refused request reached the engine %d times", *calls)
	}
}

func TestFetchLinksIsRefusedUnlessKatanaIsSelected(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	calls := stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) {
		return fetchLinksResponse{}, nil
	})
	for _, engine := range []string{shared.FetchEngineRod} {
		setEngine(t, engine)
		rec := postFetchLinks(`{"url":"` + publicTestURL + `"}`)
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusConflict || body["code"] != codeEngineNotKatana {
			t.Errorf("%s: %d %s", engine, rec.Code, rec.Body.String())
		}
	}
	if *calls != 0 {
		t.Errorf("with another engine selected the fetch ran %d times", *calls)
	}
	// the gate reads the setting fresh on every request
	setEngine(t, shared.FetchEngineKatana)
	if rec := postFetchLinks(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusOK {
		t.Errorf("katana selected: %d %s", rec.Code, rec.Body.String())
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1", *calls)
	}
}

func TestFetchLinksWithoutAReadableSettingIsRefusedAsNotKatana(t *testing.T) {
	// no database at all: the default engine (rod) applies, so the gate is closed
	orig := shared.DB
	shared.DB = nil
	t.Cleanup(func() { shared.DB = orig })
	calls := stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) { return fetchLinksResponse{}, nil })
	if rec := postFetchLinks(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusConflict || *calls != 0 {
		t.Errorf("%d calls=%d", rec.Code, *calls)
	}
}

func TestFetchLinksRefusesPrivateAddressesBeforeAnyFetch(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	calls := stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) { return fetchLinksResponse{}, nil })
	for _, u := range []string{"http://127.0.0.1/", "http://localhost:8080/", "http://169.254.169.254/latest", "http://10.0.0.1/", "ftp://93.184.216.34/", "file:///etc/passwd", "javascript:alert(1)"} {
		if rec := postFetchLinks(`{"url":"` + u + `"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", u, rec.Code)
		}
	}
	if *calls != 0 {
		t.Errorf("a refused URL reached the engine %d times", *calls)
	}
}

func TestFetchLinksReturnsTheListAndPassesTheArguments(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	var gotURL, gotReq string
	stubFetchLinks(t, func(_ context.Context, rawURL, requestID string) (fetchLinksResponse, error) {
		gotURL, gotReq = rawURL, requestID
		return fetchLinksResponse{FinalURL: rawURL, Title: "T", Links: []pageLink{{URL: "http://93.184.216.34/j/1", Texts: []string{"One"}}}, DroppedPrivate: 2}, nil
	})
	rec := postFetchLinks(`{"url":"` + publicTestURL + `","requestId":"run-1","rateLimitKey":"portal-1"}`)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got fetchLinksResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if gotURL != publicTestURL || gotReq != "run-1" || got.Title != "T" || len(got.Links) != 1 || got.Links[0].Texts[0] != "One" || got.DroppedPrivate != 2 {
		t.Errorf("url=%q req=%q body=%s", gotURL, gotReq, rec.Body.String())
	}
}

func TestFetchLinksVerdictsAndFailuresAre502WithTheCodeIntact(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	for name, fetchErr := range map[string]error{
		"cloudflare": newCloudflareUnresolvedError("cloudflare challenge page", nil),
		"blocked":    newCloudflareBlockedError("cloudflare block page"),
		"redirect":   &crawlError{Code: codeRedirectRefused, Message: "refused", Reason: "r"},
	} {
		stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) {
			return fetchLinksResponse{}, fetchErr
		})
		rec := postFetchLinks(`{"url":"` + publicTestURL + `"}`)
		var body crawlError
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusBadGateway || body.Code != fetchErr.(*crawlError).Code {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) {
		return fetchLinksResponse{}, errors.New("katana engine unavailable: no Chrome")
	})
	if rec := postFetchLinks(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "fetch links failed") {
		t.Errorf("plain failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFetchLinksCanBeCancelledByRequestID(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	started := make(chan struct{})
	stubFetchLinks(t, func(ctx context.Context, _, _ string) (fetchLinksResponse, error) {
		close(started)
		<-ctx.Done()
		return fetchLinksResponse{}, ctx.Err()
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- postFetchLinks(`{"url":"` + publicTestURL + `","requestId":"cancel-me"}`) }()
	<-started
	cancelCrawl("cancel-me")
	select {
	case rec := <-done:
		if rec.Code != http.StatusBadGateway {
			t.Errorf("cancelled fetch answered %d", rec.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("crawl-cancel did not stop the link fetch")
	}
}

func TestFetchLinksKeepsTheEngineSwitchBookkeeping(t *testing.T) {
	resetEngineTracking(t)
	stops := recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	stubFetchLinks(t, func(context.Context, string, string) (fetchLinksResponse, error) { return fetchLinksResponse{}, nil })
	postFetchLinks(`{"url":"` + publicTestURL + `"}`)
	engineMu.Lock()
	current := engineCurrent
	engineMu.Unlock()
	if current != fetchEngineKatana || len(*stops) != 0 {
		t.Errorf("current=%q stops=%v", current, *stops)
	}
}

// --- real browser ---

const linksFixture = `<!doctype html><html><head><title>Listing</title>
	<link rel="next" href="/list?page=2"><link rel="canonical" href="/list"><link rel="stylesheet" href="/style.css"></head><body>
	<a href="/job/1">Backend Developer</a><a href="/job/1">More</a><a href="/job/2" title="Job two">  </a>
	<div id="late"></div><script>setTimeout(function(){document.getElementById("late").innerHTML='<a href="/job/3">Late job</a>'},150)</script>
	<span onclick="location.href='/job/4'">onclick span</span><div data-href="/job/5">data-href</div>
	<div id="host"></div><script>document.getElementById("host").attachShadow({mode:"open"}).innerHTML='<a href="/job/6">Shadow job</a>'</script>
	<img src="/img/a.png"><iframe src="/frame"></iframe><form action="/search"><input></form>
	<a href="mailto:a@b.c">mail</a><a href="javascript:void(0)">js</a><a href="#top">top</a>
	<a href="http://169.254.169.254/latest">metadata</a><a href="http://localhost:9/x">local</a></body></html>`

func TestKatanaLinksReadsTheLinksOfALivePage(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/list": htmlPage(linksFixture)})
	resp, err := fetchLinksWithKatana(context.Background(), srv.URL+"/list", "")
	if err != nil {
		t.Fatal(err)
	}
	byURL := map[string]pageLink{}
	var order []string
	for _, l := range resp.Links {
		byURL[strings.TrimPrefix(l.URL, srv.URL)] = l
		order = append(order, strings.TrimPrefix(l.URL, srv.URL))
	}
	if resp.Title != "Listing" || resp.FinalURL != srv.URL+"/list" {
		t.Errorf("title/final: %q %q", resp.Title, resp.FinalURL)
	}
	if got := byURL["/job/1"].Texts; !reflect.DeepEqual(got, []string{"Backend Developer", "More"}) {
		t.Errorf("job 1 texts: %v", got)
	}
	if l := byURL["/job/2"]; l.TitleAttr != "Job two" || len(l.Texts) != 0 {
		t.Errorf("job 2: %+v", l)
	}
	for _, want := range []string{"/job/3", "/job/6"} { // added by script after load / inside an open shadow root
		if _, ok := byURL[want]; !ok {
			t.Errorf("%s is missing from %v", want, order)
		}
	}
	for _, never := range []string{"/job/4", "/job/5", "/style.css", "/img/a.png", "/frame", "/search"} {
		if _, ok := byURL[never]; ok {
			t.Errorf("%s must not be listed (not an anchor or rel link): %v", never, order)
		}
	}
	if l := byURL["/list?page=2"]; l.Rel != "next" {
		t.Errorf("rel=next link: %+v", l)
	}
	if l := byURL["/list"]; l.Rel != "canonical" {
		t.Errorf("rel=canonical link: %+v", l)
	}
	for u := range byURL {
		if strings.HasPrefix(u, "mailto") || strings.HasPrefix(u, "javascript") || strings.Contains(u, "#") {
			t.Errorf("unsupported scheme or fragment kept: %s", u)
		}
	}
	if resp.DroppedPrivate != 2 { // 169.254.169.254 and localhost
		t.Errorf("droppedPrivate = %d, want 2 (list %v)", resp.DroppedPrivate, order)
	}
	if resp.Truncated {
		t.Error("a small page must not be truncated")
	}
}

// Reading the links requests nothing else: not the linked pages, not a form
// target, not robots.txt, and the page itself exactly once.
func TestKatanaLinksRequestsOnlyThePageItself(t *testing.T) {
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
		"/":           count(htmlPage(`<html><head><title>One</title><link rel="next" href="/p2"></head><body><a href="/job/1">One</a><a onclick="fetch('/danger',{method:'POST'})">x</a><form action="/submit" method="post"><button>go</button></form></body></html>`)),
		"/job/1":      count(htmlPage("job")),
		"/p2":         count(htmlPage("p2")),
		"/danger":     count(htmlPage("d")),
		"/submit":     count(htmlPage("s")),
		"/robots.txt": count(htmlPage("r")),
	})
	if _, err := fetchLinksWithKatana(context.Background(), srv.URL+"/", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for k, n := range hits {
		if k == "GET /favicon.ico" { // Chrome's own request for the tab icon, not a link
			continue
		}
		if k != "GET /" && n > 0 {
			t.Errorf("%s was requested %d times; only the page itself may be loaded", k, n)
		}
	}
	if hits["GET /"] != 1 {
		t.Errorf("the page was requested %d times, want 1", hits["GET /"])
	}
}

func TestKatanaLinksReportsChallengesRedirectsAndDenials(t *testing.T) {
	requireKatanaChrome(t)
	var port string
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/challenge": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("cf-mitigated", "challenge")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><head><title>Just a moment...</title></head><body><a href='/x'>x</a></body></html>"))
		},
		"/markers": htmlPage(`<html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt = {cvId: "3"};</script><a href="https://www.cloudflare.com/?utm_source=challenge">Cloudflare</a></body></html>`),
		"/limited": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("<html><body>slow down</body></html>"))
		},
		"/go": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://localhost:"+port+"/secret", http.StatusFound)
		},
		"/secret": htmlPage(`<html><body><a href="/internal">internal</a></body></html>`),
	})
	port = srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	for path, want := range map[string]string{"/challenge": codeCloudflareUnresolved, "/markers": codeCloudflareUnresolved, "/limited": codeRateLimited, "/go": codeRedirectRefused} {
		_, err := fetchLinksWithKatana(context.Background(), srv.URL+path, "")
		var ce *crawlError
		if !errors.As(err, &ce) || ce.Code != want {
			t.Errorf("%s: got %v, want code %s", path, err, want)
		}
	}
}

func TestKatanaLinksCancellationLeavesNothingOpen(t *testing.T) {
	requireKatanaChrome(t)
	started := make(chan struct{}, 1)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/slow": func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-time.After(20 * time.Second):
			case <-r.Context().Done():
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	begin := time.Now()
	_, err := fetchLinksWithKatana(ctx, srv.URL+"/slow", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if time.Since(begin) > 12*time.Second {
		t.Errorf("cancellation took %s", time.Since(begin))
	}
	assertNoKatanaLeftovers(t)
}

func TestKatanaLinksLeaveNoPagesContextsOrGoroutines(t *testing.T) {
	requireKatanaChrome(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/": htmlPage(`<html><head><title>x</title></head><body><a href="/a">a</a></body></html>`)})
	if _, err := fetchLinksWithKatana(context.Background(), srv.URL+"/", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	before := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		if _, err := fetchLinksWithKatana(context.Background(), srv.URL+"/", ""); err != nil {
			t.Fatal(err)
		}
	}
	assertNoKatanaLeftovers(t)
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines grew from %d to %d", before, after)
	}
}
