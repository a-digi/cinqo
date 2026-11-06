package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// ---- the route ----

func TestFetchDetailCandidatesIsAListedRouteOfTheManifest(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var generic struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range generic.Routes {
		if r["path_suffix"] == "fetch-detail-candidates" {
			found = true
			if r["method"] != "POST" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %v", r)
			}
		}
	}
	if !found {
		t.Error("fetch-detail-candidates is not a route in manifest.json: the core would not forward it")
	}
}

func postDetail(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	FetchDetailCandidatesHandler(rec, httptest.NewRequest(http.MethodPost, "/fetch-detail-candidates", strings.NewReader(body)))
	return rec
}

func stubDetail(t *testing.T, fn func(ctx context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error)) *int32 {
	t.Helper()
	var calls int32
	orig := fetchDetailCandidatesFn
	fetchDetailCandidatesFn = func(ctx context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error) {
		atomic.AddInt32(&calls, 1)
		return fn(ctx, rawURL, requestID)
	}
	t.Cleanup(func() { fetchDetailCandidatesFn = orig })
	return &calls
}

func TestFetchDetailCandidatesRefusesAnotherMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	FetchDetailCandidatesHandler(rec, httptest.NewRequest(http.MethodGet, "/fetch-detail-candidates", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestFetchDetailCandidatesRefusesAnythingButUrlRequestIdAndRateLimitKey(t *testing.T) {
	calls := stubDetail(t, func(context.Context, string, string) (fetchDetailCandidatesResponse, error) {
		return fetchDetailCandidatesResponse{}, nil
	})
	for name, body := range map[string]string{
		"empty":            ``,
		"no url":           `{}`,
		"blank url":        `{"url":"  "}`,
		"login window":     `{"url":"` + publicTestURL + `","useLoginWindow":true}`,
		"ai context":       `{"url":"` + publicTestURL + `","aiContextId":"x"}`,
		"cookies":          `{"url":"` + publicTestURL + `","cookies":"a=b"}`,
		"unknown selector": `{"url":"` + publicTestURL + `","selector":"h1"}`,
		"not json":         `url=x`,
	} {
		if rec := postDetail(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if *calls != 0 {
		t.Errorf("a refused body reached the fetch %d time(s)", *calls)
	}
}

func TestFetchDetailCandidatesRefusesPrivateAddressesBeforeAnyFetch(t *testing.T) {
	calls := stubDetail(t, func(context.Context, string, string) (fetchDetailCandidatesResponse, error) {
		return fetchDetailCandidatesResponse{}, nil
	})
	for _, u := range []string{"http://localhost/x", "http://127.0.0.1/x", "http://169.254.169.254/latest", "http://10.0.0.5/x", "file:///etc/passwd", "ftp://example.com/x"} {
		if rec := postDetail(`{"url":"` + u + `"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", u, rec.Code)
		}
	}
	if *calls != 0 {
		t.Errorf("a private address reached the fetch %d time(s)", *calls)
	}
}

// Decision D1: no Katana gate — the route answers whichever engine is selected.
func TestFetchDetailCandidatesIsNotGatedOnTheSelectedEngine(t *testing.T) {
	for _, engine := range []string{shared.FetchEngineRod, shared.FetchEngineKatana} {
		resetEngineTracking(t)
		recordStops(t)
		useBrowserSettings(t, engine)
		stubDetail(t, func(_ context.Context, rawURL string, _ string) (fetchDetailCandidatesResponse, error) {
			return fetchDetailCandidatesResponse{FinalURL: rawURL, Title: "T"}, nil
		})
		if rec := postDetail(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusOK {
			t.Errorf("engine %s: status %d (%s)", engine, rec.Code, rec.Body.String())
		}
	}
}

func TestFetchDetailCandidatesReturnsTheAnalysisAndPassesTheArguments(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	var gotURL, gotReq string
	stubDetail(t, func(_ context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error) {
		gotURL, gotReq = rawURL, requestID
		return fetchDetailCandidatesResponse{FinalURL: rawURL, Title: "T", Candidates: detailCandidateSet{Title: []detailCandidate{{Selector: "h1", Unique: true, Value: "Engineer", Source: "dom"}}}}, nil
	})
	rec := postDetail(`{"url":"` + publicTestURL + `","requestId":"run-1","rateLimitKey":"portal-1"}`)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got fetchDetailCandidatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if gotURL != publicTestURL || gotReq != "run-1" || got.Title != "T" || len(got.Candidates.Title) != 1 || got.Candidates.Title[0].Value != "Engineer" {
		t.Errorf("url=%q req=%q body=%s", gotURL, gotReq, rec.Body.String())
	}
}

func TestFetchDetailCandidatesVerdictsAndFailuresAre502WithTheCodeIntact(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	verdict := &crawlError{Code: codeCloudflareUnresolved, Reason: "challenge"}
	stubDetail(t, func(context.Context, string, string) (fetchDetailCandidatesResponse, error) {
		return fetchDetailCandidatesResponse{}, verdict
	})
	rec := postDetail(`{"url":"` + publicTestURL + `"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), codeCloudflareUnresolved) {
		t.Errorf("verdict: %d %s", rec.Code, rec.Body.String())
	}
	stubDetail(t, func(context.Context, string, string) (fetchDetailCandidatesResponse, error) {
		return fetchDetailCandidatesResponse{}, context.DeadlineExceeded
	})
	if rec := postDetail(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("failure: %d", rec.Code)
	}
}

func TestFetchDetailCandidatesCanBeCancelledByRequestID(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	started := make(chan struct{})
	stubDetail(t, func(ctx context.Context, _, _ string) (fetchDetailCandidatesResponse, error) {
		close(started)
		<-ctx.Done()
		return fetchDetailCandidatesResponse{}, ctx.Err()
	})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- postDetail(`{"url":"` + publicTestURL + `","requestId":"cancel-detail"}`) }()
	<-started
	cancelCrawl("cancel-detail")
	select {
	case rec := <-done:
		if rec.Code != http.StatusBadGateway {
			t.Errorf("cancelled fetch answered %d", rec.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("crawl-cancel did not stop the analysis")
	}
}

// The Katana Chrome this route uses must be stopped after its idle time like for every fetch: the route goes through the
// engine bookkeeping, which arms the idle shutdown.
func TestFetchDetailCandidatesArmsTheIdleShutdownOfItsBrowser(t *testing.T) {
	resetEngineTracking(t)
	var mu sync.Mutex
	stopped := 0
	orig := engineStoppers
	engineStoppers = map[string]func(){fetchEngineKatana: func() { mu.Lock(); stopped++; mu.Unlock() }}
	t.Cleanup(func() { engineStoppers = orig })
	shortenIdleStop(t, 30*time.Millisecond)
	stubDetail(t, func(context.Context, string, string) (fetchDetailCandidatesResponse, error) {
		return fetchDetailCandidatesResponse{}, nil
	})
	if rec := postDetail(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := stopped
		mu.Unlock()
		if n >= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the browser was never stopped after the analysis")
}

// ---- bounding what a hostile page returns ----

func TestClampDetailCandidatesBoundsEverything(t *testing.T) {
	long := strings.Repeat("x", 5000)
	var many []detailCandidate
	for i := 0; i < 50; i++ {
		many = append(many, detailCandidate{Selector: "div.a", Value: long, Reason: long, Tag: long, Source: "jsonld", Matches: -4, Chars: -9, ShareOfPage: 7})
	}
	many = append(many, detailCandidate{Selector: strings.Repeat("a", maxDetailSelector+5)}, detailCandidate{Selector: ""})
	var els []detailElement
	for i := 0; i < 1000; i++ {
		els = append(els, detailElement{Selector: "p", Text: long, Chars: -1})
	}
	in := fetchDetailCandidatesResponse{
		Title: "T\x00\x07itle\nx",
		Candidates: detailCandidateSet{Title: many, Description: []detailCandidate{{Selector: "div", Furniture: func() []detailFurniture {
			var f []detailFurniture
			for i := 0; i < 40; i++ {
				f = append(f, detailFurniture{Selector: "nav", Value: long, Kind: long})
			}
			return f
		}(), LineSignals: make([]string, 50)}}},
		Structured:    detailStructured{JSONLD: make([]detailJobPosting, 10), Meta: detailMeta{OgTitle: long}, Microdata: -3},
		Elements:      els,
		PageTextChars: -5,
	}
	out := clampDetailCandidates(in)
	if len(out.Candidates.Title) != maxDetailCandidates {
		t.Errorf("title candidates %d", len(out.Candidates.Title))
	}
	for _, c := range out.Candidates.Title {
		if len([]rune(c.Value)) > maxDetailText || len([]rune(c.Reason)) > 60 || len([]rune(c.Tag)) > 20 || c.Source != "dom" || c.Matches != 0 || c.Chars != 0 || c.ShareOfPage != 0 {
			t.Fatalf("not bounded: %+v", c)
		}
	}
	if len(out.Candidates.Description[0].Furniture) != maxDetailCandidates || len(out.Candidates.Description[0].LineSignals) != 10 {
		t.Errorf("inner lists not bounded: %d %d", len(out.Candidates.Description[0].Furniture), len(out.Candidates.Description[0].LineSignals))
	}
	if len(out.Elements) != maxDetailElements || len(out.Structured.JSONLD) != maxDetailJobs || out.Structured.Microdata != 0 || out.PageTextChars != 0 {
		t.Errorf("elements %d jobs %d micro %d", len(out.Elements), len(out.Structured.JSONLD), out.Structured.Microdata)
	}
	if len([]rune(out.Structured.Meta.OgTitle)) != maxDetailText || out.Title != "Title x" {
		t.Errorf("text not cleaned: %q", out.Title)
	}
	if !out.Truncated {
		t.Error("hitting a cap must set truncated")
	}
}

func TestClampDetailCandidatesDropsSelectorsThatCannotBeStored(t *testing.T) {
	in := fetchDetailCandidatesResponse{Candidates: detailCandidateSet{Title: []detailCandidate{{Selector: strings.Repeat("a", maxDetailSelector+1)}, {Selector: "  "}, {Selector: "h1"}}}}
	out := clampDetailCandidates(in)
	if len(out.Candidates.Title) != 1 || out.Candidates.Title[0].Selector != "h1" {
		t.Fatalf("got %+v", out.Candidates.Title)
	}
}

func TestDecodeDetailCandidatesRefusesNonsenseAndHugeAnswers(t *testing.T) {
	if _, err := decodeDetailCandidates(`not json`); err == nil {
		t.Error("nonsense accepted")
	}
	if _, err := decodeDetailCandidates(`{"candidates":{}}` + strings.Repeat(" ", maxDetailJSONBytes)); err == nil {
		t.Error("an oversized answer was accepted")
	}
	got, err := decodeDetailCandidates(`{"candidates":{"title":[{"selector":"h1","unique":true}]},"truncated":false}`)
	if err != nil || len(got.Candidates.Title) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}

// ---- reading pages (real browser) ----

const detailFixture = `<!doctype html><html><head><title>Backend Engineer at Acme</title>
<meta property="og:title" content="Backend Engineer - Acme"><meta property="og:site_name" content="JobsRUs">
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"JobsRUs"},{"@type":"JobPosting","title":"Backend Engineer","hiringOrganization":{"@type":"Organization","name":"Acme GmbH"},"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Berlin","addressCountry":"DE"}},"datePosted":"2026-10-28T09:00:00+00:00","description":"<p>Build <b>APIs</b> all day long.</p>"}]}</script>
<script type="application/ld+json">{ this is not json</script>
</head><body>
<header><nav><a href="/">Home</a></nav></header>
<main>
 <h1 class="css-1x2y3z job-title">Backend Engineer</h1>
 <div class="company-name">Acme GmbH</div>
 <span class="job-location">Berlin, Germany</span>
 <time datetime="2026-10-28">28 October 2026</time>
 <div id="job-description" class="description">
  <h2>Tasks</h2><p>` + "Build and run the services behind our product, together with a small team. " + `</p>
  <ul><li>Go</li><li>SQL</li></ul>
  <p>` + "More text about the role so that the box is long enough to count as a description candidate on its own merits. " + `</p>
  <div class="share-buttons">Share this job</div>
  <a class="apply-now">Apply now</a>
  <h2>Profile</h2><p>` + "You know how databases work and you like to ship small, careful changes that stay easy to reason about later on. " + `</p>
 </div>
</main>
<div class="cookie-banner" id="consent-content" role="dialog"><p>` + "We use cookies to improve your experience on this site. You can accept all cookies or manage your settings at any time. We use cookies to improve your experience on this site. You can accept all cookies or manage your settings at any time. We use cookies to improve your experience on this site. You can accept all cookies or manage your settings at any time. " + `</p><p>` + "More about cookies, partners and purposes, including a very long list of vendors that is not part of the job at all. " + `</p></div>
<footer>© Acme</footer></body></html>`

func evalDetail(t *testing.T, ctx context.Context, html string) fetchDetailCandidatesResponse {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(detailCandidatesJS, &raw)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	res, err := decodeDetailCandidates(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return res
}

func TestDetailCandidatesScript(t *testing.T) {
	ctx := pageCtx(t, "<body></body>")
	cases := []struct {
		name string
		run  func(*testing.T, context.Context)
	}{
		{"FindsTheFieldsOfAJobPage", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			if len(res.Candidates.Title) == 0 || res.Candidates.Title[0].Value != "Backend Engineer" || res.Candidates.Title[0].Tag != "h1" || !res.Candidates.Title[0].InMain {
				t.Fatalf("title: %+v", res.Candidates.Title)
			}
			find := func(list []detailCandidate, v string) *detailCandidate {
				for i := range list {
					if strings.Contains(list[i].Value, v) {
						return &list[i]
					}
				}
				return nil
			}
			if find(res.Candidates.Company, "Acme GmbH") == nil {
				t.Errorf("company: %+v", res.Candidates.Company)
			}
			if find(res.Candidates.Location, "Berlin, Germany") == nil {
				t.Errorf("location: %+v", res.Candidates.Location)
			}
			d := find(res.Candidates.PostedAt, "2026-10-28")
			if d == nil || d.Attribute != "datetime" || d.Value != "2026-10-28" {
				t.Errorf("a machine date must be read from the datetime attribute: %+v", res.Candidates.PostedAt)
			}
		}},
		{"TheDescriptionBoxIsFoundWithItsFurniture", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			if len(res.Candidates.Description) == 0 {
				t.Fatal("no description candidate")
			}
			// The raw list is by size (main, which holds the box, comes first); choosing is the ranking's job in Career.
			var top detailCandidate
			for _, c := range res.Candidates.Description {
				if c.Selector == "#job-description" {
					top = c
				}
			}
			if top.Selector == "" || !top.Unique || top.Headings != 2 || top.ListItems != 2 || !top.ByAttr {
				t.Fatalf("the description box is missing or wrong: %+v", res.Candidates.Description)
			}
			kinds := map[string]bool{}
			for _, f := range top.Furniture {
				kinds[f.Kind] = true
			}
			if !kinds["share"] || !kinds["apply"] {
				t.Errorf("furniture suggestions: %+v", top.Furniture)
			}
			if len(top.LineSignals) == 0 {
				t.Errorf("line signals missing: %+v", top)
			}
		}},
		{"ACookieDialogIsNeverADescriptionCandidate", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			for _, c := range res.Candidates.Description {
				if strings.Contains(c.Selector, "cookie") || strings.Contains(c.Value, "We use cookies") {
					t.Fatalf("consent dialog offered as a description: %+v", c)
				}
			}
			for _, e := range res.Elements {
				if strings.Contains(e.Text, "We use cookies") {
					t.Fatalf("consent text listed among the elements: %+v", e)
				}
			}
		}},
		{"StructuredDataIsReadAndBrokenBlocksIgnored", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			if len(res.Structured.JSONLD) != 1 {
				t.Fatalf("jsonld: %+v", res.Structured.JSONLD)
			}
			j := res.Structured.JSONLD[0]
			if j.Title != "Backend Engineer" || j.Company != "Acme GmbH" || j.Location != "Berlin, DE" || j.DatePosted != "2026-10-28T09:00:00+00:00" || j.DescriptionChars != len("Build APIs all day long.") {
				t.Errorf("job posting: %+v", j)
			}
			if res.Structured.Meta.OgTitle != "Backend Engineer - Acme" || res.Structured.Meta.OgSiteName != "JobsRUs" {
				t.Errorf("meta: %+v", res.Structured.Meta)
			}
		}},
		{"EverySelectorThatSaysUniqueSelectsExactlyThatElement", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			var all []string
			for _, l := range [][]detailCandidate{res.Candidates.Title, res.Candidates.Company, res.Candidates.Location, res.Candidates.PostedAt, res.Candidates.Description} {
				for _, c := range l {
					if c.Unique {
						all = append(all, c.Selector)
					}
					for _, f := range c.Furniture {
						if f.Unique {
							all = append(all, f.Selector)
						}
					}
				}
			}
			for _, e := range res.Elements {
				if e.Unique {
					all = append(all, e.Selector)
				}
			}
			if len(all) < 8 {
				t.Fatalf("too few unique selectors to mean anything: %d", len(all))
			}
			for _, s := range all {
				var n int
				if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll(`+jsString(s)+`).length`, &n)); err != nil || n != 1 {
					t.Errorf("selector %q matches %d elements (err %v)", s, n, err)
				}
			}
		}},
		{"GeneratedClassNamesAreNotUsedInSelectors", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, detailFixture)
			for _, c := range res.Candidates.Title {
				if strings.Contains(c.Selector, "css-1x2y3z") {
					t.Fatalf("a generated class token is in a selector: %q", c.Selector)
				}
			}
		}},
		{"OnlyTheInnermostElementOfANestedMatchIsACandidate", func(t *testing.T, ctx context.Context) {
			html := `<body><main><h1>T</h1><div class="company-box"><span class="company-name">Acme</span></div></main></body>`
			res := evalDetail(t, ctx, html)
			if len(res.Candidates.Company) != 1 || res.Candidates.Company[0].Tag != "span" {
				t.Fatalf("company candidates: %+v", res.Candidates.Company)
			}
		}},
		{"TheElementListIsCappedAndFlaggedTruncated", func(t *testing.T, ctx context.Context) {
			html := `<body><main>` + strings.Repeat(`<p>some text here</p>`, 1000) + `</main></body>`
			res := evalDetail(t, ctx, html)
			if len(res.Elements) != maxDetailElements || !res.Truncated {
				t.Fatalf("elements=%d truncated=%v", len(res.Elements), res.Truncated)
			}
		}},
		{"AHostilePageWithAHugeDOMIsBoundedAndFast", func(t *testing.T, ctx context.Context) {
			html := `<body>` + strings.Repeat(`<div><span>x</span></div>`, 12000) + `</body>`
			start := time.Now()
			res := evalDetail(t, ctx, html)
			if !res.Truncated {
				t.Error("a page with 24,000 elements must be flagged truncated")
			}
			if time.Since(start) > 20*time.Second {
				t.Errorf("analysis took %v", time.Since(start))
			}
		}},
		{"ThePageIsLeftAloneAndNoHandlerRuns", func(t *testing.T, ctx context.Context) {
			html := `<body><main><h1 onclick="window.__c=1" id="h">Title</h1><div class="company" onmouseover="window.__c=1">Acme</div></main></body>`
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
				t.Fatal(err)
			}
			var before, after string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &before))
			var raw string
			if err := chromedp.Run(ctx, chromedp.Evaluate(detailCandidatesJS, &raw)); err != nil {
				t.Fatal(err)
			}
			var ran bool
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &after), chromedp.Evaluate(`window.__c === 1`, &ran))
			if before != after || ran {
				t.Fatalf("the page was changed or a handler ran (changed=%v ran=%v)", before != after, ran)
			}
		}},
		{"APageWithNoJobDataGivesEmptyListsNotNull", func(t *testing.T, ctx context.Context) {
			res := evalDetail(t, ctx, `<body><p>hi</p></body>`)
			raw, _ := json.Marshal(res)
			if strings.Contains(string(raw), `":null`) {
				t.Errorf("null in the answer: %s", raw)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, ctx) })
	}
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Through the real route's reader: the live page is analysed once, with nothing else requested.
func TestKatanaDetailCandidatesReadsALivePageAndRequestsOnlyIt(t *testing.T) {
	requireKatanaChrome(t)
	var hits int32
	var other int32
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/job": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(detailFixture + `<a href="/other">other job</a>`))
		},
		"/other": func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&other, 1) },
	})
	res, err := fetchDetailCandidatesWithKatana(context.Background(), srv.URL+"/job", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Backend Engineer at Acme" || res.FinalURL != srv.URL+"/job" {
		t.Errorf("title/final: %q %q", res.Title, res.FinalURL)
	}
	if len(res.Structured.JSONLD) != 1 || len(res.Candidates.Title) == 0 || len(res.Candidates.Description) == 0 {
		t.Errorf("analysis incomplete: %+v", res.Structured)
	}
	if atomic.LoadInt32(&other) != 0 || atomic.LoadInt32(&hits) != 1 {
		t.Errorf("requests: page %d, other %d (want 1 and 0)", hits, other)
	}
}
