package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

const overlayHTML = `<div style="position:fixed;top:0;left:0;right:0;bottom:0;z-index:99;background:rgba(0,0,0,.2)"><p class="cc-message-text">We use cookies</p></div>`

// pagedSite serves three pages of five jobs each at /?p=N. next(p) renders the
// next-page control for page p ("" on the last page); overlay covers the page.
func pagedSite(t *testing.T, overlay bool, next func(p int) string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	if _, err := shared.ChromePath(); err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := strconv.Atoi(r.URL.Query().Get("p"))
		if p < 1 {
			p = 1
		}
		var sb strings.Builder
		sb.WriteString(`<html><body>`)
		if overlay {
			sb.WriteString(overlayHTML)
		}
		sb.WriteString(`<main>`)
		for i := 1; i <= 5; i++ {
			fmt.Fprintf(&sb, `<article><h2>page-%d-job-%d</h2></article>`, p, i)
		}
		sb.WriteString(`</main>`)
		if p < 3 {
			sb.WriteString(next(p))
		}
		sb.WriteString(`</body></html>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sb.String()))
	}))
	t.Cleanup(srv.Close)
	// The URL guard refuses loopback in production; allow this test server only.
	old := validateAdvanceURL
	validateAdvanceURL = func(raw string) error {
		if u, err := url.Parse(raw); err == nil && u.Hostname() == "127.0.0.1" {
			return nil
		}
		return old(raw)
	}
	t.Cleanup(func() { validateAdvanceURL = old })
	return srv.URL + "/?p=1"
}

func crawlSite(t *testing.T, startURL string, maxPages int) paginatedCrawlResponse {
	t.Helper()
	useBrowserSettings(t, shared.FetchEngineRod)
	t.Cleanup(func() { shared.StopSharedSession(); StopRodEngine() })
	fields := []extractField{{Label: "title", Selector: "h2"}}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, _, err := performPaginatedCrawl(ctx, startURL, "article", fields, nil, "a.next", maxPages, maxPages, false, "", "")
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	return resp
}

func firstTitles(resp paginatedCrawlResponse) []string {
	var out []string
	for _, p := range resp.Pages {
		if len(p.Items) > 0 {
			out = append(out, fmt.Sprint(p.Items[0]["title"]))
		}
	}
	return out
}

func TestAnOverlayOverTheNextButtonIsGotPastByTheScriptClick(t *testing.T) {
	start := pagedSite(t, true, func(p int) string {
		return fmt.Sprintf(`<a class="next" href="/?p=%d">Next</a>`, p+1)
	})
	resp := crawlSite(t, start, 3)
	if got := strings.Join(firstTitles(resp), ","); got != "page-1-job-1,page-2-job-1,page-3-job-1" {
		t.Fatalf("pages visited: %s (stopped: %s)", got, resp.StoppedReason)
	}
	if strings.Join(resp.AdvancedBy, ",") != "js_click,js_click" {
		t.Errorf("advancedBy = %v, want the script click twice", resp.AdvancedBy)
	}
}

func TestWithoutAnOverlayAPlainClickIsUsedAndNothingIsReported(t *testing.T) {
	start := pagedSite(t, false, func(p int) string {
		return fmt.Sprintf(`<a class="next" href="/?p=%d">Next</a>`, p+1)
	})
	resp := crawlSite(t, start, 3)
	if got := strings.Join(firstTitles(resp), ","); got != "page-1-job-1,page-2-job-1,page-3-job-1" {
		t.Fatalf("pages visited: %s (stopped: %s)", got, resp.StoppedReason)
	}
	if resp.AdvancedBy != nil {
		t.Errorf("plain clicks are not reported: %v", resp.AdvancedBy)
	}
}

func TestTheControlsLinkIsFollowedWhenItsScriptClickIsCancelled(t *testing.T) {
	start := pagedSite(t, true, func(p int) string {
		return fmt.Sprintf(`<a class="next" href="/?p=%d" onclick="return false">Next</a>`, p+1)
	})
	resp := crawlSite(t, start, 3)
	if got := strings.Join(firstTitles(resp), ","); got != "page-1-job-1,page-2-job-1,page-3-job-1" {
		t.Fatalf("pages visited: %s (stopped: %s)", got, resp.StoppedReason)
	}
	if strings.Join(resp.AdvancedBy, ",") != "href,href" {
		t.Errorf("advancedBy = %v, want the link followed twice", resp.AdvancedBy)
	}
}

// A single-page app that updates the URL a little late must not be advanced
// twice by the fallback: pages arrive in order, none skipped.
func TestASlowPageIsNotAdvancedTwice(t *testing.T) {
	start := pagedSite(t, false, func(p int) string {
		return fmt.Sprintf(`<a class="next" href="/?p=%d" onclick="var h=this.href;setTimeout(function(){location.href=h},2200);return false">Next</a>`, p+1)
	})
	resp := crawlSite(t, start, 3)
	if got := strings.Join(firstTitles(resp), ","); got != "page-1-job-1,page-2-job-1,page-3-job-1" {
		t.Fatalf("pages visited: %s (stopped: %s) — a page was skipped or repeated", got, resp.StoppedReason)
	}
}

func TestWhenNothingMovesThePageTheCrawlStopsAsBefore(t *testing.T) {
	// A link back to the same page: every attempt leaves the URL as it was.
	start := pagedSite(t, true, func(p int) string { return `<a class="next" href="/?p=1">Next</a>` })
	resp := crawlSite(t, start, 3)
	if resp.StoppedReason != "url_unchanged" || resp.PagesVisited != 1 {
		t.Fatalf("stopped %q after %d pages, want url_unchanged after 1", resp.StoppedReason, resp.PagesVisited)
	}
	if resp.AdvancedBy != nil {
		t.Errorf("advancedBy = %v", resp.AdvancedBy)
	}
}

func TestALinkToAPrivateAddressIsNeverFollowed(t *testing.T) {
	start := pagedSite(t, true, func(p int) string {
		return `<a class="next" href="http://10.0.0.5/admin" onclick="return false">Next</a>`
	})
	resp := crawlSite(t, start, 3)
	if resp.StoppedReason != "url_unchanged" || resp.PagesVisited != 1 {
		t.Fatalf("a link to an internal address must be refused: stopped %q after %d pages", resp.StoppedReason, resp.PagesVisited)
	}
}

func TestAMissingControlStillStopsWithNoNextLink(t *testing.T) {
	start := pagedSite(t, false, func(p int) string { return `` })
	resp := crawlSite(t, start, 3)
	if resp.StoppedReason != "no_next_link" || resp.PagesVisited != 1 {
		t.Fatalf("stopped %q after %d pages", resp.StoppedReason, resp.PagesVisited)
	}
}

func TestTheAdvanceScriptsQuoteTheSelector(t *testing.T) {
	sel := `a[aria-label="Nächste"]`
	for name, js := range map[string]string{"click": jsClickScript(sel), "href": controlHrefScript(sel)} {
		if !strings.Contains(js, `"a[aria-label=\"Nächste\"]"`) {
			t.Errorf("%s script must embed the selector as a JSON string: %s", name, js)
		}
	}
}

func TestAnyFallback(t *testing.T) {
	for in, want := range map[string]bool{"": false, "click,click": false, "click,js_click": true, "href": true} {
		var methods []string
		if in != "" {
			methods = strings.Split(in, ",")
		}
		if anyFallback(methods) != want {
			t.Errorf("anyFallback(%q) = %v, want %v", in, !want, want)
		}
	}
}
