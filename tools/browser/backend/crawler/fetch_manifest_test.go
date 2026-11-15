package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"browser-tool-backend/shared"
)

func TestKeptHeadersKeepOnlyTheAllowedOnesAndNeverACookie(t *testing.T) {
	h := http.Header{}
	h.Add("Content-Type", "application/vnd.apple.mpegurl")
	h.Add("Set-Cookie", "session=secret")
	h.Add("Authorization", "Bearer x")
	h.Add("Server", "nginx")
	h.Add("Server", "second")
	h.Add("X-Custom", "nope")
	h.Add("Location", strings.Repeat("a", 500))
	got := keptHeaders(h)
	if got["content-type"] != "application/vnd.apple.mpegurl" || got["server"] != "nginx" || len(got["location"]) != 300 {
		t.Errorf("%v", got)
	}
	for _, bad := range []string{"set-cookie", "authorization", "x-custom"} {
		if _, ok := got[bad]; ok {
			t.Errorf("%s must not be kept", bad)
		}
	}
	if len(keptHeaders(http.Header{})) != 0 {
		t.Error("none")
	}
}

func TestManifestKindRecognisesAPlaylistAndADashManifestOnly(t *testing.T) {
	for in, want := range map[string]string{
		"#EXTM3U\n#EXT-X-VERSION:3": "hls", "  \n#EXTM3U": "hls", "\xef\xbb\xbf#EXTM3U": "hls", `<?xml version="1.0"?><MPD xmlns="x">`: "dash",
		"<html><body>hello</body></html>": "", "": "", "#EXTINF:5\nseg.ts": "", "{\"a\":1}": "",
	} {
		if got := manifestKind(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

func TestFetchManifestStoresTheTextAndTheHeadersWithTheirHashes(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/live/master.m3u8": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Set-Cookie", "tracker=1")
			w.Header().Set("Server", "mock")
			if r.Header.Get("Referer") != "https://pirate.example.com/watch" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4,\nseg0.ts\n"))
		},
		"/stream.mpd": textHandler("application/dash+xml", `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011"></MPD>`),
		"/page":       textHandler("text/html", "<html>not a manifest</html>"),
		"/redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/live/master.m3u8", http.StatusFound)
		},
	})
	res, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/live/master.m3u8", Referer: "https://pirate.example.com/watch"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "hls" || res.Status != 200 || res.CaptureID == "" || len(res.Artifacts) != 2 || res.Artifacts[0].Name != "manifest.m3u8" || res.Artifacts[0].Kind != "manifest" ||
		res.Artifacts[1].Name != "headers.json" || res.Artifacts[1].Kind != "headers" || res.Artifacts[0].SHA256 == "" || res.CapturedAt == "" {
		t.Fatalf("%+v", res)
	}
	if got := readCaptureFile(t, MediaResult{CaptureID: res.CaptureID}, "manifest.m3u8"); !strings.HasPrefix(got, "#EXTM3U") {
		t.Errorf("the stored text: %q", got)
	}
	headers := readCaptureFile(t, MediaResult{CaptureID: res.CaptureID}, "headers.json")
	var parsed struct {
		Status  int
		Headers []map[string]string
	}
	if err := json.Unmarshal([]byte(headers), &parsed); err != nil || parsed.Status != 200 {
		t.Fatalf("%v %s", err, headers)
	}
	if strings.Contains(headers, "tracker") || strings.Contains(strings.ToLower(headers), "set-cookie") || !strings.Contains(headers, "mpegurl") {
		t.Errorf("a cookie is never kept, the content type is: %s", headers)
	}
	if r2, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/stream.mpd"}); err != nil || r2.Kind != "dash" || r2.Artifacts[0].Name != "manifest.mpd" {
		t.Errorf("%+v %v", r2, err)
	}
	if r3, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/redirect", Referer: "https://pirate.example.com/watch"}); err != nil || r3.FinalURL != srv.URL+"/live/master.m3u8" {
		t.Errorf("a redirect is followed and the final address is kept: %+v %v", r3, err)
	}
	before, _ := os.ReadDir(shared.CapturesDir)
	var nm *notManifestError
	if _, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/page"}); err == nil || !asNotManifest(err, &nm) {
		t.Errorf("a page is not a manifest: %v", err)
	}
	if _, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/live/master.m3u8"}); err == nil {
		t.Error("the server refused (403): that is an error")
	}
	after, _ := os.ReadDir(shared.CapturesDir)
	if len(after) != len(before) {
		t.Errorf("a failed fetch leaves no capture folder behind: %d → %d", len(before), len(after))
	}
}

func asNotManifest(err error, target **notManifestError) bool {
	nm, ok := err.(*notManifestError)
	if ok {
		*target = nm
	}
	return ok
}

func TestAManifestOverTheSizeCapIsRefused(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/big.m3u8": textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n"+strings.Repeat("x", maxManifestBytes)),
		"/ok.m3u8":  textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n"+strings.Repeat("x", maxManifestBytes-len("#EXTM3U\n"))),
	})
	if _, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/big.m3u8"}); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("%v", err)
	}
	if _, err := fetchManifest(t.Context(), fetchManifestRequest{URL: srv.URL + "/ok.m3u8"}); err != nil {
		t.Errorf("exactly the cap is fine: %v", err)
	}
}

func TestFetchManifestHandlerValidatesAndMapsErrors(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{"/m.m3u8": textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n"), "/page": textHandler("text/html", "<html>")})
	if rec := postJSON(FetchManifestHandler, `{"url":"`+srv.URL+`/m.m3u8"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"hls"`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec := postJSON(FetchManifestHandler, `{"url":"`+srv.URL+`/page"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("not a manifest: %d", rec.Code)
	}
	if rec := postJSON(FetchManifestHandler, `{"url":"`+srv.URL+`/nothing"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("a server error: %d", rec.Code)
	}
	for name, b := range map[string]string{"unknown field, valid address": `{"url":"` + srv.URL + `/m.m3u8","x":1}`, "bad referer, valid address": `{"url":"` + srv.URL + `/m.m3u8","referer":"javascript:x"}`} {
		if rec := postJSON(FetchManifestHandler, b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	withTestHosts(t)
	for name, b := range map[string]string{"no url": `{}`, "unknown field": `{"url":"https://a.example/x","x":1}`, "bad referer": `{"url":"https://a.example/x","referer":"javascript:x"}`, "loopback": `{"url":"http://127.0.0.1:1/x"}`, "private": `{"url":"http://10.0.0.1/x"}`, "not json": `nope`} {
		if rec := postJSON(FetchManifestHandler, b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	FetchManifestHandler(rec, httptest.NewRequest(http.MethodGet, "/fetch-manifest", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}
