package crawler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListLinksRequestIsChecked(t *testing.T) {
	ok := listLinksRequest{URL: "https://x.example.com/", Selectors: []string{"a.movie"}}
	if err := ok.check(); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", maxListSelectorLen+1)
	many := make([]string, maxListSelectors+1)
	for i := range many {
		many[i] = "a" + strings.Repeat("b", i)
	}
	for name, mut := range map[string]func(r *listLinksRequest){
		"no url":         func(r *listLinksRequest) { r.URL = " " },
		"no selectors":   func(r *listLinksRequest) { r.Selectors = nil },
		"too many":       func(r *listLinksRequest) { r.Selectors = many },
		"blank selector": func(r *listLinksRequest) { r.Selectors = []string{" "} },
		"long selector":  func(r *listLinksRequest) { r.Selectors = []string{long} },
		"control":        func(r *listLinksRequest) { r.Selectors = []string{"a\nb"} },
		"long next":      func(r *listLinksRequest) { r.NextSelector = long },
		"control next":   func(r *listLinksRequest) { r.NextSelector = "a\x00" },
		"negative wait":  func(r *listLinksRequest) { r.WaitMs = -1 },
	} {
		r := ok
		mut(&r)
		if err := r.check(); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	r := ok
	r.Selectors = many[:maxListSelectors]
	r.NextSelector = strings.Repeat("a", maxListSelectorLen)
	if err := r.check(); err != nil {
		t.Errorf("the caps are inclusive: %v", err)
	}
}

func TestListLinksScriptCarriesSelectorsAsDataNotCode(t *testing.T) {
	evil := `a"); alert(1); ("`
	js, err := listLinksJS([]string{evil}, `x'; steal()`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js, `alert(1); ("`) && !strings.Contains(js, `\"); alert(1); (\"`) {
		t.Errorf("the selector must be a JSON string literal: %s", js)
	}
	if strings.Count(js, "alert(1)") != 1 {
		t.Errorf("appears only inside the literal")
	}
}

func TestCleanListLinksResolvesFiltersDedupesAndCaps(t *testing.T) {
	got, trunc := cleanListLinks("http://a.example.com/dir/page", []string{
		"/m/1", "http://a.example.com/m/1#frag", "m/2", "javascript:alert(1)", "mailto:x@y.z", "data:text/html,x", "", "  ", "https://b.example.com/x", "ftp://x/y", "http://[bad",
	})
	want := []string{"http://a.example.com/m/1", "http://a.example.com/dir/m/2", "https://b.example.com/x"}
	if trunc || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%v %v", got, trunc)
	}
	many := make([]string, maxListLinks+5)
	for i := range many {
		many[i] = fmt.Sprintf("/m/%d", i)
	}
	got, trunc = cleanListLinks("http://a.example.com/", many)
	if len(got) != maxListLinks || !trunc {
		t.Errorf("%d %v", len(got), trunc)
	}
	exact, trunc := cleanListLinks("http://a.example.com/", many[:maxListLinks])
	if len(exact) != maxListLinks || trunc {
		t.Errorf("exactly the cap is not truncated: %d %v", len(exact), trunc)
	}
	if long, _ := cleanListLinks("http://a.example.com/", []string{"/" + strings.Repeat("a", 2000)}); len(long) != 0 {
		t.Error("an address over 2000 characters is dropped")
	}
}

func TestListLinksHandlerRefusesBadRequests(t *testing.T) {
	withTestHosts(t)
	for name, body := range map[string]string{
		"not json": `{`, "unknown field": `{"url":"https://x.example.com/","selectors":["a"],"x":1}`, "no selectors": `{"url":"https://x.example.com/"}`,
		"loopback": `{"url":"http://127.0.0.1/","selectors":["a"]}`, "bad scheme": `{"url":"file:///etc/passwd","selectors":["a"]}`,
	} {
		rec := httptest.NewRecorder()
		ListLinksHandler(rec, httptest.NewRequest("POST", "/list-links", bytes.NewBufferString(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	ListLinksHandler(rec, httptest.NewRequest("GET", "/list-links", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func listingMock(t *testing.T) (port string) {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>
			<a class="movie" href="/m/1">One</a><div class="movie"><span><a href="/m/2">Two</a></span></div>
			<a class="movie" href="javascript:void(0)">bad</a><a class="movie" href="/m/1">dup</a><a class="ad" href="/ad">ad</a>
			<a class="other" href="http://b.localhost/x">cross</a><div class="movie">no link here</div>
			<a href="/m/3"><span class="inner">inside a link</span></a>
			<h1>  Two   words
			 here </h1><p id="long">`+strings.Repeat("x", 600)+`</p>
			<a class="next" href="/list?page=2">next</a></body></html>`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.Split(srv.Listener.Addr().String(), ":")[1]
}

func TestListLinksAppliesSelectorsAndFindsTheNextPageWithRealChrome(t *testing.T) {
	requireRodChrome(t)
	port := listingMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := "http://a.localhost:" + port
	res, err := listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"a.movie", "div.movie"}, NextSelector: "a.next", WaitMs: 200})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{base + "/m/1", base + "/m/2"}
	if strings.Join(res.Links, "|") != strings.Join(want, "|") || res.NextURL != base+"/list?page=2" || res.Matched != 5 || res.Truncated {
		t.Errorf("%+v", res)
	}
	// an ad link is not in the answer: only what the selectors name
	for _, l := range res.Links {
		if strings.Contains(l, "/ad") {
			t.Errorf("unselected link returned: %s", l)
		}
	}
	// no next selector → no next address; a selector that matches nothing → none either
	res, err = listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"a.other"}, WaitMs: 200})
	if err != nil || res.NextURL != "" || len(res.Links) != 1 || res.Links[0] != "http://b.localhost/x" {
		t.Errorf("%+v %v", res, err)
	}
	res, err = listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"a.nothing"}, NextSelector: "a.absent", WaitMs: 200})
	if err != nil || len(res.Links) != 0 || res.NextURL != "" || res.Matched != 0 {
		t.Errorf("%+v %v", res, err)
	}
	// a selector the page cannot parse is named, not swallowed
	_, err = listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"a[[["}, WaitMs: 200})
	var bad *badSelectorError
	if err == nil || !strings.Contains(err.Error(), "a[[[") || !asBad(err, &bad) {
		t.Errorf("bad selector: %v", err)
	}
	_, err = listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"a"}, NextSelector: "a[[[", WaitMs: 200})
	if !asBad(err, &bad) {
		t.Errorf("bad next selector: %v", err)
	}
}

func asBad(err error, target **badSelectorError) bool {
	for err != nil {
		if b, ok := err.(*badSelectorError); ok {
			*target = b
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestAnElementInsideALinkGivesTheLinksAddressWithRealChrome(t *testing.T) {
	requireRodChrome(t)
	port := listingMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := "http://a.localhost:" + port
	res, err := listLinks(ctx, listLinksRequest{URL: base + "/list", Selectors: []string{"span.inner"}, WaitMs: 200})
	if err != nil || len(res.Links) != 1 || res.Links[0] != base+"/m/3" {
		t.Errorf("%+v %v", res, err)
	}
}

func TestSelectedTextIsCollapsedAndCappedWithRealChrome(t *testing.T) {
	requireRodChrome(t)
	port := listingMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := "http://a.localhost:" + port + "/list"
	res, err := fetchFrames(ctx, fetchFramesRequest{URL: base, WaitMs: 200, MaxDepth: 1, TextSelector: "h1"})
	if err != nil || res.SelectedText != "Two words here" {
		t.Errorf("white space collapses to single spaces: %q %v", res.SelectedText, err)
	}
	res, err = fetchFrames(ctx, fetchFramesRequest{URL: base, WaitMs: 200, MaxDepth: 1, TextSelector: "#long"})
	if err != nil || len([]rune(res.SelectedText)) != maxSelectedText {
		t.Errorf("capped at %d: %d %v", maxSelectedText, len([]rune(res.SelectedText)), err)
	}
}

func TestTheCapsOfListLinksAndSelectedTextAreTheDocumentedOnes(t *testing.T) {
	if maxListSelectors != 20 || maxListSelectorLen != 200 || maxListLinks != 500 || maxSelectedText != 500 {
		t.Errorf("%d %d %d %d", maxListSelectors, maxListSelectorLen, maxListLinks, maxSelectedText)
	}
}

func TestTheHandlersRefuseUnknownFieldsAndBadSelectorsBeforeOpeningAnything(t *testing.T) {
	requireRodChrome(t)
	withTestHosts(t, "a.localhost") // the address itself is fine: only the body is wrong, so only the body check can say no
	for name, c := range map[string]struct {
		h    http.HandlerFunc
		body string
	}{
		"list-links unknown field":  {ListLinksHandler, `{"url":"http://a.localhost:1/","selectors":["a"],"x":1}`},
		"fetch-frames bad selector": {FetchFramesHandler, `{"url":"http://a.localhost:1/","textSelector":"a\u0000b"}`},
	} {
		rec := httptest.NewRecorder()
		c.h(rec, httptest.NewRequest("POST", "/x", bytes.NewBufferString(c.body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}
