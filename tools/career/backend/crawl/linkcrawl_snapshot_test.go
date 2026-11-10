package crawl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const snapID = "0123456789abcdef0123456789abcdef"

const snapshotListingJSON = `{"finalUrl":"https://duapune.com/search","title":"Listing","truncated":false,"droppedPrivate":0,
 "snapshot":{"id":"` + snapID + `","bytes":1234,"createdAt":"2026-11-10T19:00:00Z","truncated":true},
 "links":[{"url":"https://duapune.com/jobs/1-a","texts":["A"],"anchors":[3,7]},{"url":"https://duapune.com/jobs/2-b","texts":["B"],"anchors":[9]}]}`

func TestCandidatesAsksForAPageCopyOnlyWhenToldTo(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = snapshotListingJSON
	code, with := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","snapshot":true}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, with)
	}
	code, without := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, without)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.fetches) != 2 || f.fetches[0]["snapshot"] != "true" {
		t.Fatalf("the Browser must be asked for a copy: %v", f.fetches)
	}
	if _, has := f.fetches[1]["snapshot"]; has {
		t.Errorf("no copy may be asked for unless the screen asks: %v", f.fetches[1])
	}
	var got struct {
		Snapshot *PageSnapshot `json:"snapshot"`
		Links    []struct {
			URL     string `json:"url"`
			Anchors []int  `json:"anchors"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(with), &got); err != nil {
		t.Fatal(err)
	}
	if got.Snapshot == nil || got.Snapshot.ID != snapID || got.Snapshot.Bytes != 1234 || !got.Snapshot.Truncated || len(got.Links) != 2 ||
		len(got.Links[0].Anchors) != 2 || got.Links[0].Anchors[1] != 7 || got.Links[1].Anchors[0] != 9 {
		t.Errorf("the copy and the anchor numbers must reach the screen: %s", with)
	}
}

func TestCandidatesCarriesWhyThereIsNoCopy(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = `{"finalUrl":"https://duapune.com/search","title":"L","links":[{"url":"https://duapune.com/jobs/1-a"}],"snapshotError":"the page is too large to show (30 MB)"}`
	code, body := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","snapshot":true}`, true)
	if code != 200 || !strings.Contains(body, `"snapshotError":"the page is too large to show (30 MB)"`) || strings.Contains(body, `"snapshot":{`) {
		t.Errorf("%d %s", code, body)
	}
}

func relay(h http.HandlerFunc, method, target string, cookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if cookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-5"})
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestSnapshotRoutesRelayToTheBrowserWithTheSession(t *testing.T) {
	f := newFakeCore(t, "katana")
	f.snapshots = func(method, pq string) (int, string, string) {
		switch {
		case method == "GET" && strings.HasPrefix(pq, "/api/v1/tools/browser/proxy/page-snapshot?"):
			return 200, "text/html; charset=utf-8", "<html>copy</html>"
		case method == "DELETE" && strings.HasPrefix(pq, "/api/v1/tools/browser/proxy/page-snapshot?"):
			return 204, "", ""
		case method == "GET":
			return 200, "application/json", `{"count":2,"bytes":2048}`
		default:
			return 200, "application/json", `{"deleted":2}`
		}
	}
	rec := relay(LinkCrawlSnapshotHandler, "GET", "/s?id="+snapID, true)
	if rec.Code != 200 || rec.Body.String() != "<html>copy</html>" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "sandbox;") {
		t.Errorf("GET copy: %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	if rec := relay(LinkCrawlSnapshotHandler, "DELETE", "/s?id="+snapID, true); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("DELETE copy: %d %q", rec.Code, rec.Body.String())
	}
	if rec := relay(LinkCrawlSnapshotsHandler, "GET", "/s", true); rec.Code != 200 || rec.Body.String() != `{"count":2,"bytes":2048}` || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("GET stats: %d %q", rec.Code, rec.Body.String())
	}
	if rec := relay(LinkCrawlSnapshotsHandler, "DELETE", "/s", true); rec.Code != 200 || rec.Body.String() != `{"deleted":2}` {
		t.Errorf("DELETE all: %d %q", rec.Code, rec.Body.String())
	}
	want := []string{
		"GET /api/v1/tools/browser/proxy/page-snapshot?id=" + snapID + " Bearer tok-5",
		"DELETE /api/v1/tools/browser/proxy/page-snapshot?id=" + snapID + " Bearer tok-5",
		"GET /api/v1/tools/browser/proxy/page-snapshots Bearer tok-5",
		"DELETE /api/v1/tools/browser/proxy/page-snapshots Bearer tok-5",
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.snapshotCalls, "\n") != strings.Join(want, "\n") {
		t.Errorf("the Browser was asked:\n%s\nwant:\n%s", strings.Join(f.snapshotCalls, "\n"), strings.Join(want, "\n"))
	}
}

func TestSnapshotRoutesRefuseBeforeTheBrowserIsAsked(t *testing.T) {
	f := newFakeCore(t, "katana")
	f.snapshots = func(string, string) (int, string, string) { return 200, "text/html", "x" }
	for _, c := range []struct {
		h              http.HandlerFunc
		method, target string
		cookie         bool
		want           int
	}{
		{LinkCrawlSnapshotHandler, "GET", "/s", true, 400},
		{LinkCrawlSnapshotHandler, "GET", "/s?id=../../etc/passwd", true, 400},
		{LinkCrawlSnapshotHandler, "DELETE", "/s?id=" + strings.ToUpper(snapID), true, 400},
		{LinkCrawlSnapshotHandler, "DELETE", "/s?id=" + snapID + "/x", true, 400},
		{LinkCrawlSnapshotHandler, "GET", "/s?id=" + snapID, false, 401},
		{LinkCrawlSnapshotsHandler, "GET", "/s", false, 401},
		{LinkCrawlSnapshotsHandler, "DELETE", "/s", false, 401},
		{LinkCrawlSnapshotHandler, "POST", "/s?id=" + snapID, true, 405},
		{LinkCrawlSnapshotsHandler, "PUT", "/s", true, 405},
	} {
		if got := relay(c.h, c.method, c.target, c.cookie).Code; got != c.want {
			t.Errorf("%s %s cookie=%v: %d, want %d", c.method, c.target, c.cookie, got, c.want)
		}
	}
	if len(f.snapshotCalls) != 0 {
		t.Errorf("the Browser must not be asked for a refused request: %v", f.snapshotCalls)
	}
}

func TestSnapshotRoutesSayWhatABrowserAnswerMeans(t *testing.T) {
	f := newFakeCore(t, "katana")
	status := 404
	f.snapshots = func(string, string) (int, string, string) { return status, "text/plain", "nope" }
	if rec := relay(LinkCrawlSnapshotHandler, "GET", "/s?id="+snapID, true); rec.Code != 404 || !strings.Contains(rec.Body.String(), "gone") {
		t.Errorf("a missing copy: %d %q", rec.Code, rec.Body.String())
	}
	if rec := relay(LinkCrawlSnapshotsHandler, "GET", "/s", true); rec.Code != 404 || !strings.Contains(rec.Body.String(), "latest version") {
		t.Errorf("an older Browser tool: %d %q", rec.Code, rec.Body.String())
	}
	status = 401
	if rec := relay(LinkCrawlSnapshotHandler, "GET", "/s?id="+snapID, true); rec.Code != 401 || !strings.Contains(rec.Body.String(), "sign in") {
		t.Errorf("an expired session: %d %q", rec.Code, rec.Body.String())
	}
	status = 500
	if rec := relay(LinkCrawlSnapshotsHandler, "DELETE", "/s", true); rec.Code != 502 {
		t.Errorf("a Browser failure is a bad gateway: %d", rec.Code)
	}
	t.Setenv("CORE_API_URL", "")
	if rec := relay(LinkCrawlSnapshotHandler, "GET", "/s?id="+snapID, true); rec.Code != 500 {
		t.Errorf("no core address: %d", rec.Code)
	}
}
