package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// pageCtx serves html from a local server and returns a chromedp context
// already navigated to it, in a private browser (not the shared session).
func pageCtx(t *testing.T, html string) context.Context {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	chrome, err := shared.ChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelCtx(); cancelAlloc() })
	if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL+"/")); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return ctx
}

func job(i int, class string) string {
	return fmt.Sprintf(`<article class="%s"><h2>Software Engineer number %d</h2><a href="/jobs/%d">Open job %d</a><p class="company">Acme Corporation %d, Hamburg</p></article>`, class, i, i, i, i)
}

func jobs(n int, class string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(job(i, class))
	}
	return sb.String()
}

func analyze(t *testing.T, ctx context.Context, minItems, maxCandidates int) findRepeatingScriptResult {
	t.Helper()
	res, err := runFindRepeatingScript(ctx, minItems, maxCandidates)
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	return res
}

func countMatches(t *testing.T, ctx context.Context, selector string) int {
	t.Helper()
	var n int
	js := "document.querySelectorAll(" + string(mustJSON(selector)) + ").length"
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &n)); err != nil {
		t.Fatalf("selector %q is not valid CSS: %v", selector, err)
	}
	return n
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestPicksTheJobsNotTheNavigationOrFooter(t *testing.T) {
	ctx := pageCtx(t, `<html><body>
	  <header><nav><ul><li><a href="/a">Home page</a></li><li><a href="/b">Jobs list</a></li><li><a href="/c">Companies</a></li><li><a href="/d">About us</a></li></ul></nav></header>
	  <main><div class="results">`+jobs(8, "job-card")+`</div></main>
	  <footer><ul><li><a href="/x">Imprint of the company</a></li><li><a href="/y">Privacy policy page</a></li><li><a href="/z">Terms and conditions</a></li></ul></footer>
	</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 1 {
		t.Fatalf("want exactly the job list, got %d candidates: %+v", len(res.Candidates), res.Candidates)
	}
	c := res.Candidates[0]
	if c.Count != 8 || !strings.Contains(c.Selector, "job-card") || c.MembersWithLink != 8 || c.DistinctLinks != 8 {
		t.Errorf("candidate: %+v", c)
	}
	if len(c.FirstLinks) != 3 || !strings.Contains(c.FirstLinks[0], "/jobs/1") {
		t.Errorf("firstLinks: %v", c.FirstLinks)
	}
	if !strings.Contains(c.SampleText, "Software Engineer number 1") || !strings.Contains(c.SampleHTML, "<h2>") {
		t.Errorf("sample: %q / %q", c.SampleText, c.SampleHTML)
	}
}

func TestObfuscatedButIdenticalClassesStillGroup(t *testing.T) {
	ctx := pageCtx(t, `<html><body><div class="res-c0zne4"><div class="res-zz91"><div class="res-1xno7gx">`+
		strings.NewReplacer("<article", "<div", "</article>", "</div>").Replace(jobs(6, "res-1xno7gx"))+`</div></div></div></body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) == 0 || res.Candidates[0].Count != 6 {
		t.Fatalf("candidates: %+v", res.Candidates)
	}
}

func TestASharedDataAttributeIsPreferredOverClasses(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&sb, `<li data-at="job-item" class="r%d"><a href="/j/%d">Backend Developer %d at Example GmbH in Hamburg</a></li>`, i, i, i)
	}
	ctx := pageCtx(t, `<html><body><ul>`+sb.String()+`</ul></body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) == 0 || res.Candidates[0].Selector != `li[data-at="job-item"]` {
		t.Fatalf("candidates: %+v", res.Candidates)
	}
}

func TestAHiddenLoadingSkeletonIsIgnoredEvenWhenLarger(t *testing.T) {
	var skel strings.Builder
	for i := 0; i < 12; i++ {
		skel.WriteString(`<div class="skeleton"><a href="/s/` + fmt.Sprint(i) + `">Loading placeholder text here</a></div>`)
	}
	ctx := pageCtx(t, `<html><body>
	  <div id="spinner" style="display: none"><div role="progressbar">`+skel.String()+`</div></div>
	  <div class="list">`+jobs(5, "real-job")+`</div></body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 1 || res.Candidates[0].Count != 5 || strings.Contains(res.Candidates[0].Selector, "skeleton") {
		t.Fatalf("the visible list must win and the hidden skeleton must not appear: %+v", res.Candidates)
	}
}

func TestFewerThanMinItemsFindsNothing(t *testing.T) {
	ctx := pageCtx(t, `<html><body>`+jobs(2, "job-card")+`</body></html>`)
	if res := analyze(t, ctx, 3, 5); len(res.Candidates) != 0 {
		t.Fatalf("2 items must not be a list at minItems 3: %+v", res.Candidates)
	}
	if res := analyze(t, ctx, 2, 5); len(res.Candidates) != 1 {
		t.Fatalf("minItems 2 should accept them: %+v", res.Candidates)
	}
}

func TestTwoListsAreRankedAndBounded(t *testing.T) {
	ctx := pageCtx(t, `<html><body>
	  <section class="main">`+jobs(10, "job-card")+`</section>
	  <section class="similar">`+jobs(4, "similar-card")+`</section></body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 2 || res.Candidates[0].Count != 10 || res.Candidates[1].Count != 4 {
		t.Fatalf("want the bigger list first: %+v", res.Candidates)
	}
	if one := analyze(t, ctx, 3, 1); len(one.Candidates) != 1 || one.Candidates[0].Count != 10 {
		t.Fatalf("maxCandidates 1: %+v", one.Candidates)
	}
}

// The returned selector must select exactly the reported items on the live
// page, even when another element elsewhere shares a class with them.
func TestTheSelectorIsVerifiedAgainstTheLivePage(t *testing.T) {
	ctx := pageCtx(t, `<html><body>
	  <div class="widget"><article class="card"><h2>Lone sidebar card with enough text</h2><a href="/side">Sidebar link</a></article></div>
	  <div class="grid">`+jobs(6, "card")+`</div></body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	for _, c := range res.Candidates {
		if got := countMatches(t, ctx, c.Selector); got != c.Count {
			t.Errorf("selector %q matches %d elements on the page, reported count %d", c.Selector, got, c.Count)
		}
	}
	if res.Candidates[0].Count != 6 {
		t.Errorf("the grid of 6 is the list: %+v", res.Candidates[0])
	}
}

func TestSamplesAreBounded(t *testing.T) {
	big := strings.Repeat("lorem ipsum dolor sit amet ", 400)
	var sb strings.Builder
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, `<article class="c" data-blob="%s"><h2>Title %d long enough</h2><a href="/p/%d">link</a><p>%s</p></article>`, big, i, i, big)
	}
	ctx := pageCtx(t, `<html><body>`+sb.String()+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 1 {
		t.Fatalf("%+v", res.Candidates)
	}
	c := res.Candidates[0]
	if n := len([]rune(c.SampleHTML)); n > 1501 {
		t.Errorf("sampleHtml is %d runes", n)
	}
	if n := len([]rune(c.SampleText)); n > 200 {
		t.Errorf("sampleText is %d runes", n)
	}
	if strings.Contains(c.SampleHTML, "data-blob") {
		t.Error("an over-long attribute must be removed from the sample")
	}
}

func TestShortTagListsInsideEachCardAreNotCandidates(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&sb, `<article class="job"><h2>Platform Engineer number %d</h2><a href="/x/%d">Read more</a><div class="tags"><a href="/t/1">go</a><a href="/t/2">k8s</a><a href="/t/3">sql</a><a href="/t/4">aws</a></div></article>`, i, i)
	}
	ctx := pageCtx(t, `<html><body>`+sb.String()+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 1 || res.Candidates[0].Count != 6 {
		t.Fatalf("only the 6 cards expected: %+v", res.Candidates)
	}
}

func TestAListRenderedAfterLoadIsFoundByTheRetry(t *testing.T) {
	ctx := pageCtx(t, `<html><body><div id="root"></div><script>
	  setTimeout(function () {
	    var h = ''; for (var i = 1; i <= 5; i++) h += '<div class="late"><h2>Late rendered job '+i+'</h2><a href="/l/'+i+'">open</a></div>';
	    document.getElementById('root').innerHTML = h;
	  }, 900);</script></body></html>`)
	resp, err := analyzeCurrentPage(ctx, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].Count != 5 || strings.Contains(resp.Note, "No repeating group") {
		t.Fatalf("the retry should find the late list: %+v", resp)
	}
}

func TestAnEmptyResultSaysSo(t *testing.T) {
	ctx := pageCtx(t, `<html><body><p>Nothing to see here.</p></body></html>`)
	resp, err := analyzeCurrentPage(ctx, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Candidates) != 0 || !strings.Contains(resp.Note, "No repeating group") {
		t.Fatalf("%+v", resp)
	}
	if resp.PageTitle == "" && resp.FinalURL == "" {
		t.Error("the response should carry the page's title/url")
	}
}

func TestNormalizeFindRepeating(t *testing.T) {
	for name, c := range map[string]struct{ in, want findRepeatingRequest }{
		"defaults":      {findRepeatingRequest{}, findRepeatingRequest{MinItems: 3, MaxCandidates: 5}},
		"min below 2":   {findRepeatingRequest{MinItems: 1, MaxCandidates: 2}, findRepeatingRequest{MinItems: 2, MaxCandidates: 2}},
		"max above 8":   {findRepeatingRequest{MinItems: 4, MaxCandidates: 50}, findRepeatingRequest{MinItems: 4, MaxCandidates: 8}},
		"left as given": {findRepeatingRequest{MinItems: 6, MaxCandidates: 3}, findRepeatingRequest{MinItems: 6, MaxCandidates: 3}},
	} {
		got := c.in
		normalizeFindRepeating(&got)
		if got.MinItems != c.want.MinItems || got.MaxCandidates != c.want.MaxCandidates {
			t.Errorf("%s: got %+v, want %+v", name, got, c.want)
		}
	}
}

func TestFindRepeatingHandlerRejectsBadRequestsBeforeAnyBrowserWork(t *testing.T) {
	post := func(body string) int {
		rec := httptest.NewRecorder()
		FindRepeatingItemsHandler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		return rec.Code
	}
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"not JSON":     {`nope`, http.StatusBadRequest},
		"no url":       {`{}`, http.StatusBadRequest},
		"ftp scheme":   {`{"url":"ftp://example.com/"}`, http.StatusBadRequest},
		"loopback":     {`{"url":"http://127.0.0.1:1/"}`, http.StatusBadRequest},
		"negative min": {`{"url":"https://example.com/","minItems":-1}`, http.StatusBadRequest},
		"negative max": {`{"url":"https://example.com/","maxCandidates":-1}`, http.StatusBadRequest},
	} {
		if got := post(c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", name, got, c.want)
		}
	}
	rec := httptest.NewRecorder()
	FindRepeatingItemsHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestManifestDeclaresTheFindRepeatingRouteAndTool(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"path_suffix": "find-repeating-items"`, `"name": "find_repeating_items"`} {
		if !strings.Contains(text, want) {
			t.Errorf("manifest.json lacks %s", want)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
}

// TestLiveFindRepeatingItems runs the real tool path (worker tab, stealth
// session) against a live URL, for a human to read the result. Skipped unless
// FIND_REPEATING_LIVE_URL is set; it is evidence-gathering, not a regression
// test, because a live site changes.
func TestLiveFindRepeatingItems(t *testing.T) {
	url := os.Getenv("FIND_REPEATING_LIVE_URL")
	if url == "" {
		t.Skip("set FIND_REPEATING_LIVE_URL to run against a live page")
	}
	if _, err := shared.ChromePath(); err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	t.Cleanup(func() { shared.StopSharedSession() })
	resp, err := performFindRepeating(context.Background(), findRepeatingRequest{URL: url, MinItems: 3, MaxCandidates: 5})
	if err != nil {
		t.Fatalf("live run: %v", err)
	}
	out, _ := json.MarshalIndent(resp, "", "  ")
	t.Logf("%s", out)
}

// A card's first link is often a logo or company link shared between cards;
// the job's own link is the one with the most text.
func TestThePrimaryLinkIsTheTitleLinkNotTheSharedCompanyLink(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&sb, `<article class="c"><a href="/company/acme"><img alt="" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"></a><a href="/jobs/%d">Senior Backend Engineer %d (m/w/d)</a><p>Acme GmbH, Hamburg, full-time position</p></article>`, i, i)
	}
	ctx := pageCtx(t, `<html><body>`+sb.String()+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) != 1 {
		t.Fatalf("%+v", res.Candidates)
	}
	c := res.Candidates[0]
	if c.DistinctLinks != 5 {
		t.Errorf("distinctLinks = %d, want 5 (the shared company link must not count as the item's link)", c.DistinctLinks)
	}
	if len(c.FirstLinks) != 3 || !strings.Contains(c.FirstLinks[0], "/jobs/1") {
		t.Errorf("firstLinks should be the job links: %v", c.FirstLinks)
	}
	if strings.Contains(c.SampleHTML, "base64") || strings.Contains(c.SampleHTML, "<img") {
		t.Errorf("inline images must not eat the sample: %q", c.SampleHTML)
	}
}

func TestSampleTextLeavesOutScriptContent(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, `<div class="row"><script>window.__PRELOADED__ = {"secret": "script text %d"};</script><h2>Visible heading number %d</h2><a href="/v/%d">visible link text</a></div>`, i, i, i)
	}
	ctx := pageCtx(t, `<html><body>`+sb.String()+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	c := res.Candidates[0]
	if strings.Contains(c.SampleText, "PRELOADED") || strings.Contains(c.SampleHTML, "PRELOADED") {
		t.Errorf("script content must be left out: %q / %q", c.SampleText, c.SampleHTML)
	}
	if !strings.Contains(c.SampleText, "Visible heading number 1") {
		t.Errorf("visible text expected: %q", c.SampleText)
	}
}

func TestPresentationAttributesAreStrippedFromTheSample(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&sb, `<article class="c" role="button" tabindex="0" style="color:red" aria-label="x" data-id="k%d"><h2>Heading text %d here</h2><a href="/p/%d" target="_blank" rel="noopener">Open posting number %d</a></article>`, i, i, i, i)
	}
	ctx := pageCtx(t, `<html><body>`+sb.String()+`</body></html>`)
	c := analyze(t, ctx, 3, 5).Candidates[0]
	for _, noise := range []string{"role=", "tabindex", "style=", "aria-", "target=", "rel="} {
		if strings.Contains(c.SampleHTML, noise) {
			t.Errorf("sample still has %s: %s", noise, c.SampleHTML)
		}
	}
	if !strings.Contains(c.SampleHTML, `class="c"`) || !strings.Contains(c.SampleHTML, `data-id=`) || !strings.Contains(c.SampleHTML, "href=") {
		t.Errorf("class, data-* and href must stay (they are what selectors use): %s", c.SampleHTML)
	}
}

// --- next-page control candidates (step 95) -----------------------------------

func pageWithJobs(extraHead, extraBody string) string {
	return `<html><head>` + extraHead + `</head><body><main>` + jobs(5, "job-card") + `</main>` + extraBody + `</body></html>`
}

func TestARelNextLinkIsTheTopCandidateAndGivesTheNextPageURL(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs(`<link rel="next" href="/jobs?page=2">`, `<nav class="pagination"><a href="/jobs?page=1">1</a><a rel="next" href="/jobs?page=2">Next</a></nav>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) == 0 {
		t.Fatalf("no next candidates: %+v", res)
	}
	n := res.NextCandidates[0]
	if !strings.Contains(n.Selector, `rel~="next"`) || n.Reason != "rel=next" || !strings.HasSuffix(n.Href, "/jobs?page=2") {
		t.Errorf("top candidate: %+v", n)
	}
	if !strings.HasSuffix(res.NextPageURL, "/jobs?page=2") {
		t.Errorf("nextPageUrl = %q", res.NextPageURL)
	}
}

// The real Stepstone shape: a generated class and an aria-label. The selector
// must be the stable label, not the class.
func TestTheAriaLabelIsPreferredOverAGeneratedClass(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<nav aria-label="pagination" id="p:1"><ul><li><a class="res-9e3779" aria-label="Nächste" href="/jobs?page=2"></a></li></ul></nav>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) == 0 || res.NextCandidates[0].Selector != `a[aria-label="Nächste"]` {
		t.Fatalf("candidates: %+v", res.NextCandidates)
	}
	if res.NextCandidates[0].Label != "Nächste" || res.NextCandidates[0].Matches != 1 {
		t.Errorf("%+v", res.NextCandidates[0])
	}
}

func TestTheControlAfterTheCurrentPageNumberIsACandidate(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<ul class="pager"><li><a href="/j?p=1">1</a></li><li class="active"><a href="/j?p=2" aria-current="page">2</a></li><li><a href="/j?p=3">3</a></li></ul>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) == 0 {
		t.Fatalf("no candidates: %+v", res)
	}
	found := false
	for _, n := range res.NextCandidates {
		if n.Reason == "follows the current page number" && strings.HasSuffix(n.Href, "/j?p=3") {
			found = true
		}
	}
	if !found {
		t.Errorf("want the link after page 2: %+v", res.NextCandidates)
	}
}

func TestAnArrowInAPaginationBarIsFound(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<div class="pagination"><a href="/j?p=1">‹</a><a href="/j?p=3">›</a></div>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) != 1 || res.NextCandidates[0].Text != "›" || !strings.HasSuffix(res.NextCandidates[0].Href, "/j?p=3") {
		t.Fatalf("only the right arrow is next: %+v", res.NextCandidates)
	}
}

func TestADisabledOrHiddenNextIsNotACandidate(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<div class="pagination">
	  <a class="next disabled" href="/j?p=9">Next</a>
	  <button aria-disabled="true" aria-label="Next page">Next</button>
	  <a style="display: none" href="/j?p=2" rel="next">Next</a></div>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) != 0 {
		t.Fatalf("disabled and hidden controls must not be offered: %+v", res.NextCandidates)
	}
}

func TestAReadMoreLinkInsideAJobCardIsNotANextControl(t *testing.T) {
	ctx := pageCtx(t, `<html><body>`+strings.ReplaceAll(jobs(5, "job-card"), "Open job", "Weiter lesen")+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) != 0 {
		t.Fatalf("a card's own link is not pagination: %+v", res.NextCandidates)
	}
}

func TestAPaginationBarShownTwiceIsOneCandidateThatMatchesTwice(t *testing.T) {
	bar := `<nav class="pagination"><a rel="next" href="/j?p=2">Next</a></nav>`
	ctx := pageCtx(t, `<html><body>`+bar+`<main>`+jobs(5, "job-card")+`</main>`+bar+`</body></html>`)
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) != 1 || res.NextCandidates[0].Matches != 2 {
		t.Fatalf("top and bottom bars are one selector matching twice: %+v", res.NextCandidates)
	}
}

func TestEveryNextSelectorMatchesExactlyWhatItReportsOnTheLivePage(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<a class="link" href="/x/1">Other link one</a><a class="link" href="/x/2">Other link two</a>
	  <nav class="pagination"><a class="link" aria-label="Next" href="/j?p=2">go</a></nav>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) == 0 {
		t.Fatalf("no candidates: %+v", res)
	}
	for _, n := range res.NextCandidates {
		if got := countMatches(t, ctx, n.Selector); got != n.Matches {
			t.Errorf("selector %q matches %d elements, reported %d", n.Selector, got, n.Matches)
		}
	}
}

func TestNoPaginationGivesNoCandidatesAndANoteWhenItemsWereFound(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", ""))
	resp, err := analyzeCurrentPage(ctx, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Candidates) == 0 || len(resp.NextCandidates) != 0 || !strings.Contains(resp.Note, "No next-page control was found") {
		t.Fatalf("%+v", resp)
	}
	if resp.NextPageURL != "" {
		t.Errorf("nextPageUrl = %q", resp.NextPageURL)
	}
}

func TestThePaginationNoteIsAddedToAnExistingNoteNotInstead(t *testing.T) {
	// No items and no pagination: the "no list" note stands alone, with no
	// pagination remark (there is nothing to paginate).
	ctx := pageCtx(t, `<html><body><p>Nothing here.</p></body></html>`)
	resp, err := analyzeCurrentPage(ctx, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Note, "next-page") || !strings.Contains(resp.Note, "No repeating group") {
		t.Errorf("note: %q", resp.Note)
	}
}

// "The control after the current page" must keep meaning that as pages advance:
// move the current-page marker and the same selector must follow it.
func TestTheFollowsCurrentSelectorMovesWithTheCurrentPage(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<ul class="pager"><li><a href="/j?p=1">1</a></li><li class="active"><a href="/j?p=2">2</a></li><li><a href="/j?p=3">3</a></li><li><a href="/j?p=4">4</a></li></ul>`))
	var selector string
	for _, n := range analyze(t, ctx, 3, 5).NextCandidates {
		if n.Reason == "follows the current page number" {
			selector = n.Selector
		}
	}
	if selector == "" {
		t.Fatal("no follows-current candidate")
	}
	var href string
	js := `(function(){document.querySelector('.pager li.active').classList.remove('active');document.querySelectorAll('.pager li')[2].classList.add('active');var a=document.querySelector(` + string(mustJSON(selector)) + `);return a?a.getAttribute('href'):'none'})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &href)); err != nil {
		t.Fatal(err)
	}
	if href != "/j?p=4" {
		t.Errorf("with page 3 current the selector %q should reach page 4, got %q", selector, href)
	}
}

func TestBareArrowsInListItemsAreToldApartByPosition(t *testing.T) {
	ctx := pageCtx(t, pageWithJobs("", `<ul class="pagination"><li><a href="/j?p=1">‹</a></li><li><a href="/j?p=3">›</a></li></ul>`))
	res := analyze(t, ctx, 3, 5)
	if len(res.NextCandidates) != 1 || !strings.HasSuffix(res.NextCandidates[0].Href, "/j?p=3") {
		t.Fatalf("only the right arrow: %+v", res.NextCandidates)
	}
	if got := countMatches(t, ctx, res.NextCandidates[0].Selector); got != 1 {
		t.Errorf("selector %q matches %d", res.NextCandidates[0].Selector, got)
	}
}
