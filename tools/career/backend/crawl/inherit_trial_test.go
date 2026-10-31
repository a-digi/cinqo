package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const (
	trialListingA = `container: ".job"
fields:
  - label: title
    selector: h2
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.next
  maxPages: 5
`
	trialListingB = `container: ".result"
fields:
  - label: title
    selector: h3
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.more
  maxPages: 3
`
	trialDetail = `fields:
  - label: description
    selector: .body
`
)

// browserCall is what the fake browser saw in one request.
type browserCall struct {
	URL          string
	Container    string
	MaxPages     int
	RateLimitKey string
	Auth         string
}

// fakeBrowser stands in for core's proxy to the Browser tool. answer decides each reply;
// every request is recorded.
type fakeBrowser struct {
	mu     sync.Mutex
	calls  []browserCall
	events []string // "topic:portal_link_id" of every domain event published to the fake core
}

func (f *fakeBrowser) published() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fakeBrowser) seen() []browserCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]browserCall(nil), f.calls...)
}

func startBrowser(t *testing.T, answer func(c browserCall, n int, w http.ResponseWriter, r *http.Request)) *fakeBrowser {
	t.Helper()
	fb := &fakeBrowser{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events/publish" && r.Method == http.MethodPost {
			var evt struct {
				Topic   string            `json:"topic"`
				Payload map[string]string `json:"payload"`
			}
			_ = json.NewDecoder(r.Body).Decode(&evt)
			fb.mu.Lock()
			fb.events = append(fb.events, evt.Topic+":"+evt.Payload["portal_link_id"])
			fb.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Path != "/api/v1/tools/browser/proxy/crawl-paginated" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			URL               string `json:"url"`
			Container         string `json:"container"`
			EffectiveMaxPages int    `json:"effectiveMaxPages"`
			RateLimitKey      string `json:"rateLimitKey"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		c := browserCall{URL: body.URL, Container: body.Container, MaxPages: body.EffectiveMaxPages, RateLimitKey: body.RateLimitKey, Auth: r.Header.Get("Authorization")}
		fb.mu.Lock()
		fb.calls = append(fb.calls, c)
		n := len(fb.calls)
		fb.mu.Unlock()
		answer(c, n, w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return fb
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// listPage is a crawl-paginated answer with n listing items (urls https://shop.example/job/<i>).
func listPage(n int) map[string]any {
	items := []map[string]any{}
	for i := 1; i <= n; i++ {
		items = append(items, map[string]any{"title": "Job", "url": "https://shop.example/job/" + string(rune('0'+i))})
	}
	return map[string]any{"pages": []any{map[string]any{"url": "list", "items": items}}, "stoppedReason": "max_pages", "pagesVisited": 1}
}

func detailPage(description string) map[string]any {
	return map[string]any{"pages": []any{map[string]any{"url": "job", "results": map[string]any{"description": description}}}, "stoppedReason": "max_pages", "pagesVisited": 1}
}

func setupTrialDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

// trialPortal makes a portal with the new link (no instructions) and one sibling per
// entry of siblings (listing text, detail text), in that order.
func trialPortal(t *testing.T, siblings ...[2]string) (portalID, newLink string, siblingIDs []string) {
	t.Helper()
	portalID, err := portal.CreatePortal("Shop")
	if err != nil {
		t.Fatal(err)
	}
	newLink, err = portal.AddPortalLink(portalID, "https://shop.example/new-search", "New")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range siblings {
		id, err := portal.AddPortalLink(portalID, "https://shop.example/sibling-"+string(rune('a'+i)), "Sibling "+string(rune('A'+i)))
		if err != nil {
			t.Fatal(err)
		}
		if s[0] != "" {
			if err := portal.UpdatePortalLinkCrawlInstructions(id, s[0]); err != nil {
				t.Fatal(err)
			}
		}
		if s[1] != "" {
			if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, s[1]); err != nil {
				t.Fatal(err)
			}
		}
		siblingIDs = append(siblingIDs, id)
	}
	return portalID, newLink, siblingIDs
}

func rowCount(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func snapshot(t *testing.T) string {
	t.Helper()
	ps, err := portal.ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(ps)
	return string(raw) + "|jobs=" + string(rune('0'+rowCount(t, "jobs"))) + "|runs=" + string(rune('0'+rowCount(t, "crawl_runs"))) + "|attempts=" + string(rune('0'+rowCount(t, "instruction_attempts")))
}

func TestTrialInherit_ListAndDetailsWork(t *testing.T) {
	setupTrialDB(t)
	portalID, newLink, sib := trialPortal(t, [2]string{trialListingA, trialDetail})
	long := strings.Repeat("d", 150)
	fb := startBrowser(t, func(c browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, listPage(5))
			return
		}
		writeJSON(w, detailPage(long))
	})
	before := snapshot(t)

	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner == nil || res.Winner.LinkID != sib[0] || !res.CopyListing || !res.CopyJobDetail {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Tried) != 1 || res.Tried[0].Listing != ListingOK || res.Tried[0].JobDetail != portal.DetailOK || res.Tried[0].Reason != "" {
		t.Fatalf("tried = %+v", res.Tried)
	}
	if res.Winner.Listing != trialListingA || res.Winner.JobDetail != trialDetail {
		t.Fatal("the winner carries the documents to copy")
	}

	calls := fb.seen()
	if len(calls) != 3 {
		t.Fatalf("calls = %+v", calls)
	}
	// The list: the NEW link's url, the candidate's container, page one only, the caller's token, the portal's pacing key.
	if calls[0].URL != "https://shop.example/new-search" || calls[0].Container != ".job" || calls[0].MaxPages != 1 ||
		calls[0].Auth != "Bearer tok" || calls[0].RateLimitKey != portalID {
		t.Fatalf("list call = %+v", calls[0])
	}
	// The details: the first two jobs of that list, no container.
	if calls[1].URL != "https://shop.example/job/1" || calls[2].URL != "https://shop.example/job/2" || calls[1].Container != "" || calls[1].Auth != "Bearer tok" {
		t.Fatalf("detail calls = %+v %+v", calls[1], calls[2])
	}
	if after := snapshot(t); after != before {
		t.Fatalf("the trial wrote something:\nbefore %s\nafter  %s", before, after)
	}
}

func TestTrialInherit_ListWorksButDetailsFail(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, trialDetail})
	startBrowser(t, func(c browserCall, n int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, listPage(4))
			return
		}
		if n == 2 {
			writeJSON(w, detailPage(strings.Repeat("d", 150)))
			return
		}
		writeJSON(w, detailPage("too short")) // the second job fails
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner == nil || !res.CopyListing || res.CopyJobDetail {
		t.Fatalf("result = %+v", res)
	}
	if o := res.Tried[0]; o.Listing != ListingOK || o.JobDetail != portal.DetailFailed || !strings.Contains(o.Reason, "1 of 2") {
		t.Fatalf("outcome = %+v", o)
	}
}

func TestTrialInherit_CandidateWithoutDetailInstructions(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !res.CopyListing || res.CopyJobDetail || res.Tried[0].JobDetail != portal.DetailNoInstructions {
		t.Fatalf("result = %+v", res)
	}
	if len(fb.seen()) != 1 {
		t.Fatalf("no detail page may be opened: %+v", fb.seen())
	}
}

func TestTrialInherit_FirstCandidateFailsSecondPasses(t *testing.T) {
	setupTrialDB(t)
	_, newLink, sib := trialPortal(t, [2]string{trialListingA, ""}, [2]string{trialListingB, ""})
	fb := startBrowser(t, func(c browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, map[string]any{"pages": []any{map[string]any{"url": "list"}}, "stoppedReason": "max_pages"}) // nothing matched
			return
		}
		writeJSON(w, listPage(5))
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner == nil || res.Winner.LinkID != sib[1] || len(res.Tried) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if res.Tried[0].Listing != ListingFailed || res.Tried[0].Reason != "no items found" || res.Tried[1].Listing != ListingOK {
		t.Fatalf("tried = %+v", res.Tried)
	}
	if len(fb.seen()) != 2 {
		t.Fatalf("calls = %+v", fb.seen())
	}
}

func TestTrialInherit_LowCountAndMissingFieldsFail(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""}, [2]string{trialListingB, ""})
	startBrowser(t, func(c browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, listPage(2)) // two items: the low-count warning
			return
		}
		writeJSON(w, map[string]any{"pages": []any{map[string]any{"url": "l", "items": []any{
			map[string]any{"title": "A", "url": "https://x/a"}, map[string]any{"title": "B", "url": "https://x/b"},
			map[string]any{"title": "C", "url": "https://x/c"}, map[string]any{"title": "no url"},
		}}}})
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner != nil || res.CopyListing || len(res.Tried) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if res.Tried[0].Reason != "only 2 item(s) found" || res.Tried[1].Reason != "1 of 4 items had no title or url" {
		t.Fatalf("tried = %+v", res.Tried)
	}
}

func TestTrialInherit_NoCandidates(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{"", ""}) // a sibling without instructions
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil || res.Winner != nil || len(res.Tried) != 0 || res.CopyListing {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(fb.seen()) != 0 {
		t.Fatal("the browser must not be called without candidates")
	}
}

func TestTrialInherit_BrowserErrorStopsTheTrial(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""}, [2]string{trialListingB, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "browser exploded", http.StatusInternalServerError)
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner != nil || len(res.Tried) != 1 || res.Tried[0].Listing != ListingError || !strings.Contains(res.Tried[0].Reason, "browser exploded") {
		t.Fatalf("result = %+v", res)
	}
	if len(fb.seen()) != 1 {
		t.Fatalf("the second candidate must not be tried after a browser error: %+v", fb.seen())
	}
}

func TestTrialInherit_CloudflareBlockStopsTheTrial(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""}, [2]string{trialListingB, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"pages": []any{}, "stoppedReason": "cloudflare_blocked", "blockedReason": "challenge"})
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if res.Winner != nil || len(res.Tried) != 1 || res.Tried[0].Listing != ListingError || res.Tried[0].Reason != "blocked by Cloudflare" || len(fb.seen()) != 1 {
		t.Fatalf("result = %+v calls=%d", res, len(fb.seen()))
	}
}

func TestTrialInherit_TransientBrowserFailureIsRetried(t *testing.T) {
	setupTrialDB(t)
	old := crawlNowSessionRetryInterval
	crawlNowSessionRetryInterval = time.Millisecond
	t.Cleanup(func() { crawlNowSessionRetryInterval = old })
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, n int, w http.ResponseWriter, _ *http.Request) {
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, map[string]string{"code": "browser_session_interrupted", "message": "restarted"})
			return
		}
		writeJSON(w, listPage(5))
	})
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil || res.Winner == nil || len(fb.seen()) != 2 {
		t.Fatalf("result = %+v, %v, calls=%d", res, err, len(fb.seen()))
	}
}

func TestTrialInherit_TimeoutReturnsTheContextError(t *testing.T) {
	setupTrialDB(t)
	old := inheritTrialTimeout
	inheritTrialTimeout = 50 * time.Millisecond
	t.Cleanup(func() { inheritTrialTimeout = old })
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	before := snapshot(t)
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if !errors.Is(err, context.DeadlineExceeded) || res.Winner != nil {
		t.Fatalf("result = %+v, err = %v", res, err)
	}
	if snapshot(t) != before {
		t.Fatal("a timed-out trial must leave nothing behind")
	}
}

func TestTrialInherit_CallerCancelStops(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	ctx, cancel := context.WithCancel(context.Background())
	startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	if _, err := TrialInherit(ctx, newLink, "tok"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestTrialInherit_RefusesBadInput(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })

	if _, err := TrialInherit(context.Background(), newLink, ""); err == nil {
		t.Fatal("no token must be refused")
	}
	if _, err := TrialInherit(context.Background(), "nope", "tok"); !errors.Is(err, portal.ErrUnknownPortalLink) {
		t.Fatalf("unknown link: %v", err)
	}
	t.Setenv("CORE_API_URL", "")
	if _, err := TrialInherit(context.Background(), newLink, "tok"); err == nil {
		t.Fatal("no CORE_API_URL must be refused")
	}
	if len(fb.seen()) != 0 {
		t.Fatal("nothing may reach the browser")
	}
}

func TestTrialInherit_UsesOnlyTheOwnPortalsInstructions(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t) // the new link's portal has no instructed sibling
	other, _ := portal.CreatePortal("Other")
	stranger, _ := portal.AddPortalLink(other, "https://other.example/x", "Stranger")
	if err := portal.UpdatePortalLinkCrawlInstructions(stranger, trialListingA); err != nil {
		t.Fatal(err)
	}
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })
	res, err := TrialInherit(context.Background(), newLink, "tok")
	if err != nil || res.Winner != nil || len(res.Tried) != 0 || len(fb.seen()) != 0 {
		t.Fatalf("another portal's instructions must never be tried: %+v %v calls=%d", res, err, len(fb.seen()))
	}
}
