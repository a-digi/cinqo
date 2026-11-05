package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// A real Chrome test of step 2 (plan 182): a throw-away login window logs in to a local site (the program types for the person), then
// /crawl, /crawl-paginated and /extract run in that window, and a login that ends in the middle of the listing stops the crawl with
// login_required. It opens a real visible window for about ten seconds, so it only runs when asked:
//
//	BROWSER_TOOL_LIVE_CHROME_TEST=1 go test ./crawler -run TestLoginWindowCrawlLive

type liveSite struct {
	srv       *httptest.Server
	sessions  atomic.Int32 // 1 while a session exists
	armed     atomic.Bool  // page 2 shows the login form (the session "ended" in the middle of the listing)
	requested atomic.Int32
}

func newLiveSite() *liveSite {
	s := &liveSite{}
	mux := http.NewServeMux()
	loginPage := func(w http.ResponseWriter) {
		fmt.Fprint(w, `<html><title>Sign in</title><body><h1>Sign in</h1><form method="post" action="/do-login"><input name="user"><input name="pass" type="password"><button id="go">Log in</button></form></body></html>`)
	}
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) { loginPage(w) })
	mux.HandleFunc("/do-login", func(w http.ResponseWriter, r *http.Request) {
		s.sessions.Store(1)
		http.Redirect(w, r, "/jobs?page=1", http.StatusFound)
	})
	mux.HandleFunc("/arm", func(w http.ResponseWriter, r *http.Request) { s.armed.Store(true) })
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		s.requested.Add(1)
		page := int(r.URL.Query().Get("page")[0] - '0')
		if s.sessions.Load() == 0 || (page == 2 && s.armed.Load()) {
			loginPage(w)
			return
		}
		next := ""
		if page < 3 {
			next = fmt.Sprintf(`<a id="next" href="/jobs?page=%d">next</a>`, page+1)
		}
		fmt.Fprintf(w, `<html><title>Jobs</title><body><ul><li class="job"><span class="title">Job %d-1</span></li><li class="job"><span class="title">Job %d-2</span></li></ul>%s</body></html>`, page, page, next)
	})
	s.srv = httptest.NewServer(mux)
	return s
}

func TestLoginWindowCrawlLive(t *testing.T) {
	if os.Getenv("BROWSER_TOOL_LIVE_CHROME_TEST") == "" {
		t.Skip("opens a real visible Chrome window; set BROWSER_TOOL_LIVE_CHROME_TEST=1 to run it")
	}
	if _, err := shared.ChromePath(); err != nil {
		t.Skipf("no Chrome on this machine: %v", err)
	}
	site := newLiveSite()
	defer site.srv.Close()

	w, err := shared.OpenLoginWindow(site.srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	f := &fakeLoginWindowsTab{tab: w.Ctx}
	useLoginWindows(t, f)

	// not logged in yet: the page in the window is the login form, so a crawl says login_required (and the window would go back to waiting)
	_, err = crawlInLoginWindow(context.Background(), crawlRequest{URL: site.srv.URL + "/jobs?page=1", RequestID: "live"})
	if !isLoginRequired(err) {
		t.Fatalf("before the login: %v", err)
	}
	if len(f.lost) != 1 {
		t.Errorf("the lost login was not reported (%v)", f.lost)
	}

	// the person logs in (typed by the test)
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	must(chromedp.Run(w.Ctx, chromedp.Navigate(site.srv.URL+"/login"), chromedp.SendKeys(`input[name=user]`, "ada"), chromedp.SendKeys(`input[name=pass]`, "secret"), chromedp.Click(`#go`), chromedp.WaitVisible(`ul`)), "log in")

	// /crawl in the window
	page, err := crawlInLoginWindow(context.Background(), crawlRequest{URL: site.srv.URL + "/jobs?page=1", RequestID: "live"})
	if err != nil || !strings.Contains(page.HTML, "Job 1-1") {
		t.Fatalf("crawl in the window: %v %q", err, page.HTML)
	}

	// /crawl-paginated in the window: with a url, all 3 pages
	args := paginatedArgs{container: "li.job", fields: []extractField{{Label: "title", Selector: ".title"}}, nextSelector: "a#next", requestedMaxPages: 3, effectiveMaxPages: 3, requestID: "live"}
	resp, _, err := performPaginatedCrawlInLoginWindow(context.Background(), site.srv.URL+"/jobs?page=1", args)
	if err != nil {
		t.Fatalf("paginated crawl in the window: %v", err)
	}
	items := 0
	for _, p := range resp.Pages {
		items += len(p.Items)
	}
	if resp.PagesVisited != 3 || items != 6 {
		t.Errorf("visited %d pages with %d items, want 3 and 6: %+v", resp.PagesVisited, items, resp)
	}

	// /extract in the window reads the page it shows (the last page)
	ex, err := extractInLoginWindow(context.Background(), "live", "li.job", []extractField{{Label: "title", Selector: ".title"}}, nil)
	if err != nil || len(ex.Items) != 2 {
		t.Errorf("extract in the window: %v %+v", err, ex)
	}

	// the login ends in the middle of the listing: page 2 shows the login form, so the crawl stops with login_required, not with the
	// login page extracted as data
	site.armed.Store(true)
	f.lost = nil
	_, _, err = performPaginatedCrawlInLoginWindow(context.Background(), site.srv.URL+"/jobs?page=1", args)
	if !isLoginRequired(err) {
		t.Fatalf("a login that ended mid-listing: %v", err)
	}
	if len(f.lost) != 1 {
		t.Errorf("the lost login was not reported (%v)", f.lost)
	}

	// the window now shows the login page, so an extraction there says the same
	if _, err := extractInLoginWindow(context.Background(), "live", "li.job", []extractField{{Label: "title", Selector: ".title"}}, nil); !isLoginRequired(err) {
		t.Errorf("extract while the window shows the login form: %v", err)
	}
}

// fakeLoginWindowsTab hands out one real tab.
type fakeLoginWindowsTab struct {
	tab  context.Context
	lost []string
}

func (f *fakeLoginWindowsTab) AcquireTab(ctx context.Context, id string) (context.Context, func(), error) {
	return f.tab, func() {}, nil
}
func (f *fakeLoginWindowsTab) LoginLost(id string) { f.lost = append(f.lost, id) }
