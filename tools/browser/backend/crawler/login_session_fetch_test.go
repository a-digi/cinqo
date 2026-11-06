package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

const rotatedMarker = "-rotated"

// loginSite is a site that needs a login: /private answers only for a known session cookie and rotates it (a new value on every visit; the
// old one stays good), anything else is redirected to a page with a password field. hits counts every request, to prove a refused fetch
// never reaches the site.
type loginSite struct {
	srv  *httptest.Server
	hits atomic.Int64
	mu   sync.Mutex
	good map[string]string // sid cookie value -> user
}

func newLoginSite(t *testing.T, users ...string) *loginSite {
	t.Helper()
	s := &loginSite{good: map[string]string{}}
	for _, u := range users {
		s.good["session-of-"+u] = u
	}
	user := func(r *http.Request) string {
		c, err := r.Cookie("sid")
		if err != nil {
			return ""
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.good[c.Value]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		fmt.Fprint(w, `<html><head><title>Log in</title></head><body><form><input name="u"><input type="password" name="p"></form>please log in</body></html>`)
	})
	mux.HandleFunc("/private", func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		u := user(r)
		if u == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		rotated := "session-of-" + u + rotatedMarker
		s.mu.Lock()
		s.good[rotated] = u
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: rotated, Path: "/", HttpOnly: true, Expires: time.Now().Add(24 * time.Hour)})
		fmt.Fprintf(w, `<html><head><title>Jobs of %s</title></head><body><h1>Job list of %s</h1><a href="/job/1">Senior Engineer</a><div id="late">loading</div>
<script>fetch('/private/data').then(r=>r.text()).then(t=>{document.getElementById('late').textContent=t})</script></body></html>`, u, u)
	})
	mux.HandleFunc("/private/data", func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if u := user(r); u != "" {
			fmt.Fprintf(w, "subresource-ok-for-%s", u)
			return
		}
		http.Error(w, "no session", http.StatusUnauthorized)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *loginSite) saveSession(t *testing.T, user string) shared.LoginSessionInfo {
	t.Helper()
	info, err := shared.SaveLoginSession("127.0.0.1", []shared.SessionCookie{
		{Name: "sid", Value: "session-of-" + user, Domain: "127.0.0.1", Path: "/", HTTPOnly: true, Expires: float64(time.Now().Add(24 * time.Hour).Unix())},
	})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func errCode(err error) string {
	var ce *crawlError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestKatanaFetchesALoggedInPageWithAKeptLoginAndNeverMixesSessions(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	site := newLoginSite(t, "alice", "bob")
	alice, bob := site.saveSession(t, "alice"), site.saveSession(t, "bob")

	// Without a session the page is the login form.
	resp, err := fetchKatana(context.Background(), site.srv.URL+"/private")
	if err != nil || strings.Contains(resp.HTML, "Job list of") || !strings.Contains(resp.HTML, `type="password"`) {
		t.Fatalf("without a session: %v %q", err, resp.Title)
	}

	// With it, the private page, and the page's own request carries the cookie as well.
	resp, err = fetchKatana(withLoginSession(context.Background(), alice.ID), site.srv.URL+"/private")
	if err != nil || !strings.Contains(resp.HTML, "Job list of alice") || !strings.Contains(resp.HTML, "subresource-ok-for-alice") {
		t.Fatalf("with a session: %v %q", err, resp.Title)
	}
	if info, _ := shared.GetLoginSession(alice.ID); info.LastOKAt == "" || info.State != shared.LoginSessionValid {
		t.Errorf("a good page must record last_ok_at: %+v", info)
	}

	// Ten fetches at once with two sessions: each gets its own, none is mixed.
	var wg sync.WaitGroup
	var bad atomic.Int64
	for i := 0; i < 10; i++ {
		who, id, other := "alice", alice.ID, "bob"
		if i%2 == 1 {
			who, id, other = "bob", bob.ID, "alice"
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := fetchKatana(withLoginSession(context.Background(), id), site.srv.URL+"/private")
			if err != nil || !strings.Contains(r.HTML, "Job list of "+who) || strings.Contains(r.HTML, "Job list of "+other) {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Errorf("%d of 10 parallel fetches got the wrong session or failed", bad.Load())
	}
}

func TestAKeptLoginsRotatedCookieIsWrittenBack(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	site := newLoginSite(t, "alice")
	info := site.saveSession(t, "alice")

	if _, err := fetchKatana(withLoginSession(context.Background(), info.ID), site.srv.URL+"/private"); err != nil {
		t.Fatal(err)
	}
	got, err := shared.OpenLoginSession(info.ID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value != "session-of-alice"+rotatedMarker {
		t.Errorf("the rotated session cookie must be stored (name only shown): %d cookies, rotated=%v", len(got), len(got) == 1 && got[0].Value == "session-of-alice"+rotatedMarker)
	}
}

func TestASiteThatNoLongerAcceptsTheLoginMarksItExpiredAndIsNotAskedAgain(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	site := newLoginSite(t, "alice")
	info := site.saveSession(t, "nobody") // a session the site does not know

	_, err := fetchKatana(withLoginSession(context.Background(), info.ID), site.srv.URL+"/private")
	if errCode(err) != codeLoginExpired {
		t.Fatalf("a login form after the fetch must answer login_expired, got %v", err)
	}
	if got, _ := shared.GetLoginSession(info.ID); got.State != shared.LoginSessionExpired {
		t.Errorf("state must be expired: %+v", got)
	}
	hits := site.hits.Load()
	_, err = fetchKatana(withLoginSession(context.Background(), info.ID), site.srv.URL+"/private")
	if errCode(err) != codeLoginExpired {
		t.Errorf("the next fetch must be refused: %v", err)
	}
	if site.hits.Load() != hits {
		t.Error("an expired session must not reach the site again")
	}
	if err != nil && strings.Contains(err.Error(), "session-of") {
		t.Errorf("an error text carries a cookie value: %v", err)
	}
}

func TestAKeptLoginIsNeverSentToAnotherSiteOrWhenGone(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana) // no Chrome needed: refused before any browser work
	site := newLoginSite(t, "alice")
	other, err := shared.SaveLoginSession("other.example", []shared.SessionCookie{{Name: "sid", Value: "session-of-alice", Domain: "other.example", Path: "/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetchKatana(withLoginSession(context.Background(), other.ID), site.srv.URL+"/private"); errCode(err) != codeLoginSessionOtherSite {
		t.Errorf("another site's session: %v", err)
	}
	if _, err := fetchKatana(withLoginSession(context.Background(), strings.Repeat("a", 32)), site.srv.URL+"/private"); errCode(err) != codeLoginExpired {
		t.Errorf("a session that does not exist: %v", err)
	}
	if site.hits.Load() != 0 {
		t.Errorf("a refused fetch reached the site %d times", site.hits.Load())
	}
}

// --- the routes (stubbed fetches, real settings database) ---

func postCrawl(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	CrawlHandler(rec, httptest.NewRequest(http.MethodPost, "/crawl", strings.NewReader(body)))
	return rec
}

func TestCrawlWithAKeptLoginTakesTheKatanaOnlyPathWithoutCache(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	var seen []string
	origKatana, origPage := katanaFetch, crawlPageFn
	katanaFetch = func(ctx context.Context, rawURL, requestID string, _, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		seen = append(seen, loginSessionFrom(ctx))
		return crawlResponse{HTML: "<p>private</p>", Title: "t", FinalURL: rawURL}, nil
	}
	crawlPageFn = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		t.Error("the normal crawl (cache, headed fallback) must not run for a kept login")
		return crawlResponse{}, nil
	}
	t.Cleanup(func() { katanaFetch, crawlPageFn = origKatana, origPage })

	id := strings.Repeat("ab", 16)
	body := `{"url":"` + publicTestURL + `","loginSessionId":"` + id + `","requestId":"r1"}`
	for i := 0; i < 2; i++ { // twice: nothing is cached or remembered
		if rec := postCrawl(body); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "private") {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	if len(seen) != 2 || seen[0] != id || seen[1] != id {
		t.Errorf("the engine must be reached twice with the session: %v", seen)
	}

	for _, bad := range []string{
		`{"url":"` + publicTestURL + `","loginSessionId":"short"}`,
		`{"url":"` + publicTestURL + `","loginSessionId":"` + strings.ToUpper(id) + `"}`,
		`{"url":"` + publicTestURL + `","loginSessionId":"` + id + `","useLoginWindow":true,"requestId":"r"}`,
		`{"url":"` + publicTestURL + `","loginSessionId":"` + id + `","aiContextId":"c"}`,
		`{"url":"` + publicTestURL + `","loginSessionId":"` + id + `","offset":0}`,
	} {
		if rec := postCrawl(bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	setEngine(t, shared.FetchEngineRod)
	rec := postCrawl(body)
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusConflict || out["code"] != codeEngineNotKatana {
		t.Errorf("with Rod selected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestACrawlWithoutAKeptLoginIsUnchanged(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	called := false
	origPage := crawlPageFn
	crawlPageFn = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		called = true
		return crawlResponse{HTML: "<p>x</p>"}, nil
	}
	t.Cleanup(func() { crawlPageFn = origPage })
	if rec := postCrawl(`{"url":"` + publicTestURL + `"}`); rec.Code != http.StatusOK || !called {
		t.Errorf("%d called=%v", rec.Code, called)
	}
}

func TestFetchLinksAndDetailCandidatesPassTheSessionAndRefuseABadId(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	var linksSeen, candSeen string
	origL, origC := fetchLinksFn, fetchDetailCandidatesFn
	fetchLinksFn = func(ctx context.Context, rawURL, requestID string) (fetchLinksResponse, error) {
		linksSeen = loginSessionFrom(ctx)
		return fetchLinksResponse{}, nil
	}
	fetchDetailCandidatesFn = func(ctx context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error) {
		candSeen = loginSessionFrom(ctx)
		return fetchDetailCandidatesResponse{}, nil
	}
	t.Cleanup(func() { fetchLinksFn, fetchDetailCandidatesFn = origL, origC })

	id := strings.Repeat("0f", 16)
	if rec := postFetchLinks(`{"url":"` + publicTestURL + `","loginSessionId":"` + id + `"}`); rec.Code != 200 || linksSeen != id {
		t.Errorf("fetch-links: %d seen=%q", rec.Code, linksSeen)
	}
	rec := httptest.NewRecorder()
	FetchDetailCandidatesHandler(rec, httptest.NewRequest(http.MethodPost, "/fetch-detail-candidates", strings.NewReader(`{"url":"`+publicTestURL+`","loginSessionId":"`+id+`"}`)))
	if rec.Code != 200 || candSeen != id {
		t.Errorf("fetch-detail-candidates: %d seen=%q", rec.Code, candSeen)
	}
	for _, h := range []func(http.ResponseWriter, *http.Request){FetchLinksHandler, FetchDetailCandidatesHandler} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"url":"`+publicTestURL+`","loginSessionId":"nope"}`)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("a bad session id must be 400, got %d", rec.Code)
		}
	}
}

// No AI tool can name or learn a session: the field exists only on the HTTP bodies, never on a line that also describes an MCP tool argument.
func TestNoAIToolArgumentCarriesALoginSession(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "loginSessionId") && strings.Contains(line, "jsonschema") {
				t.Errorf("%s:%d offers a login session to an AI tool: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// --- the paginated crawl with a kept login (plan 195, step 4) ---

func TestPaginatedCrawlWithAKeptLoginExtractsALoggedInPageInAnIsolatedTab(t *testing.T) {
	requireRodChrome(t)
	t.Cleanup(func() { shared.StopSharedSession() })
	useBrowserSettings(t, shared.FetchEngineKatana)
	site := newLoginSite(t, "alice", "bob")
	alice, bob := site.saveSession(t, "alice"), site.saveSession(t, "bob")
	fields := []extractField{{Label: "title", Selector: "h1"}}

	run := func(id string) (paginatedCrawlResponse, error) {
		args := paginatedArgs{fields: fields, requestedMaxPages: 1, effectiveMaxPages: 1, requestID: "", deadline: time.Time{}}
		resp, _, err := performPaginatedCrawlWithKeptLogin(withLoginSession(context.Background(), id), site.srv.URL+"/private", args)
		return resp, err
	}
	resp, err := run(alice.ID)
	if err != nil || len(resp.Pages) != 1 || !strings.Contains(fmt.Sprint(resp.Pages[0].Results["title"]), "Job list of alice") {
		t.Fatalf("alice: %v %+v", err, resp.Pages)
	}
	// Two sessions at once: no mixing (each tab has its own browser context), and the shared session holds no cookie of the site afterwards.
	var wg sync.WaitGroup
	var bad atomic.Int64
	for i := 0; i < 6; i++ {
		who, id := "alice", alice.ID
		if i%2 == 1 {
			who, id = "bob", bob.ID
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := run(id)
			if err != nil || len(r.Pages) != 1 || !strings.Contains(fmt.Sprint(r.Pages[0].Results["title"]), "Job list of "+who) {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Errorf("%d of 6 parallel reads got the wrong session or failed", bad.Load())
	}
	if shared.HeadlessHasCookie(site.srv.URL+"/private", "sid") {
		t.Error("the kept login's cookie reached the shared headless session")
	}
	if got, _ := shared.OpenLoginSession(alice.ID, "127.0.0.1"); len(got) != 1 || got[0].Value != "session-of-alice"+rotatedMarker {
		t.Error("the rotated cookie must be written back after a good read")
	}
}

func TestPaginatedCrawlWithAKeptLoginSaysLoginExpiredWhenTheSiteRefusesIt(t *testing.T) {
	requireRodChrome(t)
	t.Cleanup(func() { shared.StopSharedSession() })
	useBrowserSettings(t, shared.FetchEngineKatana)
	site := newLoginSite(t, "alice")
	bad := site.saveSession(t, "nobody")
	args := paginatedArgs{fields: []extractField{{Label: "title", Selector: "h1"}}, requestedMaxPages: 1, effectiveMaxPages: 1}
	_, _, err := performPaginatedCrawlWithKeptLogin(withLoginSession(context.Background(), bad.ID), site.srv.URL+"/private", args)
	if errCode(err) != codeLoginExpired {
		t.Fatalf("got %v", err)
	}
	if got, _ := shared.GetLoginSession(bad.ID); got.State != shared.LoginSessionExpired {
		t.Errorf("state: %+v", got)
	}
	hits := site.hits.Load()
	if _, _, err := performPaginatedCrawlWithKeptLogin(withLoginSession(context.Background(), bad.ID), site.srv.URL+"/private", args); errCode(err) != codeLoginExpired || site.hits.Load() != hits {
		t.Errorf("an expired session must be refused without touching the site: %v", err)
	}
}

func TestPaginatedRouteWithAKeptLoginIsGuarded(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	var seen string
	orig := performPaginatedCrawlWithKeptLoginFn
	performPaginatedCrawlWithKeptLoginFn = func(ctx context.Context, rawURL string, args paginatedArgs) (paginatedCrawlResponse, []string, error) {
		seen = loginSessionFrom(ctx)
		return paginatedCrawlResponse{PagesVisited: 1}, nil, nil
	}
	t.Cleanup(func() { performPaginatedCrawlWithKeptLoginFn = orig })
	post := func(body string) int {
		rec := httptest.NewRecorder()
		PaginatedCrawlHandler(rec, httptest.NewRequest(http.MethodPost, "/crawl-paginated", strings.NewReader(body)))
		return rec.Code
	}
	id := strings.Repeat("1e", 16)
	base := `"url":"` + publicTestURL + `","fields":[{"label":"title","selector":"h1"}],"requestedMaxPages":1,"effectiveMaxPages":1`
	if code := post(`{` + base + `,"loginSessionId":"` + id + `"}`); code != 200 || seen != id {
		t.Errorf("good request: %d seen=%q", code, seen)
	}
	for _, bad := range []string{
		`{` + base + `,"loginSessionId":"nope"}`,
		`{` + base + `,"loginSessionId":"` + id + `","useLoginWindow":true,"requestId":"r"}`,
		`{` + base + `,"loginSessionId":"` + id + `","aiContextId":"c"}`,
		`{` + base + `,"loginSessionId":"` + id + `","container":".x"}`,
		`{"fields":[{"label":"title","selector":"h1"}],"requestedMaxPages":1,"effectiveMaxPages":1,"loginSessionId":"` + id + `"}`,
	} {
		if code := post(bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
}
