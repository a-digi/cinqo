package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestParseHLSMediaPlaylist(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-KEY:METHOD=AES-128,URI=\"keys/k.bin\",IV=0x1\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.5,\nseg0.ts\n#EXTINF:5.0,title\n/abs/seg1.ts\n#EXTINF:3,\njavascript:evil()\n#EXTINF:2,\nhttps://other.example/seg2.ts?x=1\n#EXT-X-ENDLIST\n"
	pl := parseHLS(text, mustURL(t, "https://cdn.example/path/media.m3u8"))
	if pl.Master || !pl.Ended || !pl.Encrypted {
		t.Fatalf("%+v", pl)
	}
	if pl.KeyURI != "https://cdn.example/path/keys/k.bin" || pl.MapURI != "https://cdn.example/path/init.mp4" {
		t.Errorf("key/map: %q %q", pl.KeyURI, pl.MapURI)
	}
	want := []hlsSegment{{"https://cdn.example/path/seg0.ts", 4.5}, {"https://cdn.example/abs/seg1.ts", 5}, {"https://other.example/seg2.ts?x=1", 2}}
	if len(pl.Segments) != len(want) {
		t.Fatalf("segments %v (a javascript: line is dropped and does not shift the next duration)", pl.Segments)
	}
	for i := range want {
		if pl.Segments[i] != want[i] {
			t.Errorf("segment %d = %+v, want %+v", i, pl.Segments[i], want[i])
		}
	}
}

func TestParseHLSMasterPlaylistAndBestVariant(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360\nlow/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2400000,CODECS=\"avc1.4d401f,mp4a.40.2\"\nhigh/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=1200000\nmid/index.m3u8\n"
	pl := parseHLS(text, mustURL(t, "https://cdn.example/m.m3u8"))
	if !pl.Master || len(pl.Variants) != 3 || len(pl.Segments) != 0 {
		t.Fatalf("%+v", pl)
	}
	best, ok := bestVariant(pl.Variants)
	if !ok || best.URL != "https://cdn.example/high/index.m3u8" || best.Bandwidth != 2400000 {
		t.Errorf("best: %+v", best)
	}
	if _, ok := bestVariant(nil); ok {
		t.Error("no variants, no best")
	}
	if b, _ := bestVariant([]hlsVariant{{"a", 5}, {"b", 5}}); b.URL != "a" {
		t.Error("the first of equals")
	}
	if pl := parseHLS("#EXTM3U\n#EXT-X-KEY:METHOD=NONE\nseg.ts\n", mustURL(t, "https://x/a.m3u8")); pl.Encrypted || len(pl.Segments) != 1 {
		t.Errorf("METHOD=NONE is not encryption: %+v", pl)
	}
}

func TestSegmentExtAndMediaTypeRefused(t *testing.T) {
	for in, want := range map[string]string{"https://x/a/seg.ts?x=1": ".ts", "https://x/seg.M4S": ".m4s", "https://x/seg": ".bin", "https://x/seg.toolongext": ".bin", "https://x/seg.a$b": ".bin"} {
		if got := segmentExt(in); got != want {
			t.Errorf("segmentExt(%q) = %q, want %q", in, got, want)
		}
	}
	for _, ct := range []string{"text/html", "text/plain; charset=utf-8", "application/json", "image/png", "application/javascript", "font/woff"} {
		if !mediaTypeRefused(ct) {
			t.Errorf("%q can never be media", ct)
		}
	}
	for _, ct := range []string{"video/mp4", "audio/mpeg", "application/octet-stream", "video/mp2t", "application/vnd.apple.mpegurl", ""} {
		if mediaTypeRefused(ct) {
			t.Errorf("%q must be allowed", ct)
		}
	}
}

// ---- the downloads, against local servers (the test hosts list lets the loopback address through) ----

func mediaServer(t *testing.T, mux map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	m := http.NewServeMux()
	for p, h := range mux {
		m.HandleFunc(p, h)
	}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv
}

func bytesHandler(ct string, data []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(data)
	}
}

func textHandler(ct, body string) http.HandlerFunc { return bytesHandler(ct, []byte(body)) }

func runMedia(t *testing.T, req fetchMediaRequest) (MediaResult, error) {
	t.Helper()
	return downloadMedia(context.Background(), req)
}

func readCaptureFile(t *testing.T, res MediaResult, name string) string {
	t.Helper()
	p, err := captureFile(res.CaptureID, name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	b, _ := os.ReadFile(p)
	return string(b)
}

func TestProgressiveDownloadStoresTheFileHashedAndRefusesPages(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/film.mp4": bytesHandler("video/mp4", []byte("0123456789")), "/page": textHandler("text/html", "<html>"), "/missing": func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"/redirect": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/film.mp4", http.StatusFound) },
	})
	res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/film.mp4", Kind: "progressive"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated || res.TotalBytes != 10 || len(res.Files) != 1 || res.Files[0].Name != "media/media.mp4" || res.Files[0].Kind != "stream_media" ||
		res.Files[0].SHA256 != "84d89877f0d4041efb6bf91a16f0248f2fd573e6af05c19f96bedb9f882f7882" {
		t.Errorf("%+v", res)
	}
	if readCaptureFile(t, res, "media/media.mp4") != "0123456789" {
		t.Error("the file content")
	}
	if res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/redirect", Kind: "progressive"}); err != nil || res.TotalBytes != 10 {
		t.Errorf("a redirect is followed: %+v %v", res, err)
	}
	for name, u := range map[string]string{"an html page": "/page", "a 404": "/missing"} {
		if _, err := runMedia(t, fetchMediaRequest{URL: srv.URL + u, Kind: "progressive"}); err == nil {
			t.Errorf("%s must fail", name)
		}
	}
	entries, _ := os.ReadDir(shared.CapturesDir)
	for _, e := range entries {
		if files, _ := os.ReadDir(filepath.Join(shared.CapturesDir, e.Name())); len(files) == 0 {
			t.Errorf("a failed download must not leave an empty capture folder behind: %s", e.Name())
		}
	}
}

func TestProgressiveDownloadStopsAtTheByteCapAndSaysSo(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{"/big.mp4": bytesHandler("video/mp4", []byte(strings.Repeat("x", 5000)))})
	res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/big.mp4", Kind: "progressive", MaxBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || res.TotalBytes != 1000 || res.Files[0].Size != 1000 {
		t.Errorf("%+v", res)
	}
	p, _ := captureFile(res.CaptureID, "media/media.mp4")
	if info, _ := os.Stat(p); info.Size() != 1000 {
		t.Errorf("the file on disk is %d bytes, want 1000", info.Size())
	}
	size, sum, _ := hashFile(p)
	if size != 1000 || sum != res.Files[0].SHA256 {
		t.Error("the reported hash must be the hash of the truncated file")
	}
}

func TestHLSDownloadFollowsMasterToBestVariantAndStoresEverythingAsServed(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	var gotReferer, gotOrigin atomic.Value
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/m.m3u8": textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100\nlow.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=900\nhigh/index.m3u8\n"),
		"/high/index.m3u8": func(w http.ResponseWriter, r *http.Request) {
			gotReferer.Store(r.Header.Get("Referer"))
			gotOrigin.Store(r.Header.Get("Origin"))
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"k.bin\"\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4,\ns0.ts\n#EXTINF:6,\ns1.ts\n#EXT-X-ENDLIST\n"))
		},
		"/high/k.bin":    bytesHandler("application/octet-stream", []byte("KEYKEYKEYKEYKEYK")),
		"/high/init.mp4": bytesHandler("video/mp4", []byte("INIT")),
		"/high/s0.ts":    bytesHandler("video/mp2t", []byte("SEG0")),
		"/high/s1.ts":    bytesHandler("video/mp2t", []byte("SEGMENT-ONE")),
		"/low.m3u8":      func(w http.ResponseWriter, r *http.Request) { t.Error("the low variant must not be fetched") },
	})
	res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/m.m3u8", Kind: "hls", Referer: "https://pirate.example/watch?id=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated || res.SegmentsTotal != 2 || res.SegmentsDownloaded != 2 || res.DurationHint != 10 || !res.Encrypted || res.Note == "" {
		t.Errorf("%+v", res)
	}
	names := map[string]string{}
	for _, f := range res.Files {
		names[f.Name] = f.Kind
	}
	for _, want := range []string{"playlist.m3u8", "media-playlist.m3u8", "key.bin", "segments/init.mp4", "segments/00000.ts", "segments/00001.ts"} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing %s in %v", want, names)
		}
	}
	if readCaptureFile(t, res, "segments/00001.ts") != "SEGMENT-ONE" || readCaptureFile(t, res, "key.bin") != "KEYKEYKEYKEYKEYK" || !strings.Contains(readCaptureFile(t, res, "playlist.m3u8"), "STREAM-INF") {
		t.Error("contents are stored exactly as served")
	}
	if gotReferer.Load() != "https://pirate.example/watch?id=1" || gotOrigin.Load() != "https://pirate.example" {
		t.Errorf("referer/origin sent: %v %v", gotReferer.Load(), gotOrigin.Load())
	}
	var sum int64
	for _, f := range res.Files {
		sum += f.Size
	}
	if sum != res.TotalBytes {
		t.Errorf("total %d != sum of files %d", res.TotalBytes, sum)
	}
}

func TestHLSDownloadStopsAtSegmentAndByteCaps(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	var seg strings.Builder
	seg.WriteString("#EXTM3U\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&seg, "#EXTINF:2,\ns%d.ts\n", i)
	}
	mux := map[string]http.HandlerFunc{"/p.m3u8": textHandler("application/vnd.apple.mpegurl", seg.String())}
	for i := 0; i < 6; i++ {
		mux[fmt.Sprintf("/s%d.ts", i)] = bytesHandler("video/mp2t", []byte(strings.Repeat("s", 100)))
	}
	srv := mediaServer(t, mux)
	res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/p.m3u8", Kind: "hls", MaxSegments: 3})
	if err != nil || !res.Truncated || res.SegmentsDownloaded != 3 || res.SegmentsTotal != 6 {
		t.Errorf("segment cap: %+v %v", res, err)
	}
	res, err = runMedia(t, fetchMediaRequest{URL: srv.URL + "/p.m3u8", Kind: "hls", MaxBytes: int64(len(seg.String())) + 250})
	if err != nil || !res.Truncated || res.TotalBytes > int64(len(seg.String()))+250 {
		t.Errorf("byte cap must never be exceeded: %+v %v", res, err)
	}
	if res.SegmentsDownloaded >= 6 {
		t.Errorf("not all segments fit: %+v", res)
	}
}

func TestHLSDownloadStopsAtTheTimeLimit(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	mux := map[string]http.HandlerFunc{"/p.m3u8": textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n#EXTINF:1,\na.ts\n#EXTINF:1,\nb.ts\n#EXTINF:1,\nc.ts\n")}
	for _, n := range []string{"a", "b", "c"} {
		mux["/"+n+".ts"] = func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(700 * time.Millisecond)
			_, _ = w.Write([]byte("seg"))
		}
	}
	srv := mediaServer(t, mux)
	start := time.Now()
	res, err := downloadMediaWithin(context.Background(), fetchMediaRequest{URL: srv.URL + "/p.m3u8", Kind: "hls"}, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || res.SegmentsDownloaded >= 3 || time.Since(start) > 5*time.Second {
		t.Errorf("the time limit must stop the download and say so: %+v after %v", res, time.Since(start))
	}
}

func TestDashDownloadStoresOnlyTheManifestAndSaysSo(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{"/a.mpd": textHandler("application/dash+xml", `<?xml version="1.0"?><MPD type="static"></MPD>`), "/notdash": textHandler("text/plain", "hello"), "/notm3u": textHandler("text/plain", "hello")})
	res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/a.mpd", Kind: "dash"})
	if err != nil || len(res.Files) != 1 || res.Files[0].Name != "manifest.mpd" || !strings.Contains(res.Note, "not downloaded") {
		t.Errorf("%+v %v", res, err)
	}
	if _, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/notdash", Kind: "dash"}); err == nil {
		t.Error("not a DASH manifest")
	}
	if _, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/notm3u", Kind: "hls"}); err == nil {
		t.Error("not an HLS playlist")
	}
}

func TestMediaDownloadNeverReachesPrivateAddressesNotEvenByRedirect(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1") // the server below is on loopback and listed; everything else private is not
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/to-private": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://10.255.255.1/film.mp4", http.StatusFound)
		},
		"/to-metadata": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
		},
		"/to-file": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		},
		"/loop":        func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) },
		"/seg-private": textHandler("application/vnd.apple.mpegurl", "#EXTM3U\n#EXTINF:1,\nhttp://10.255.255.1/s.ts\n"),
	})
	for _, p := range []string{"/to-private", "/to-metadata", "/to-file", "/loop"} {
		if _, err := runMedia(t, fetchMediaRequest{URL: srv.URL + p, Kind: "progressive"}); err == nil {
			t.Errorf("%s must be refused", p)
		}
	}
	if res, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/seg-private", Kind: "hls"}); err == nil && res.SegmentsDownloaded > 0 {
		t.Errorf("a segment on a private address must never be fetched: %+v", res)
	}
	// without the test host listed, even the loopback server itself is out of reach at dial time
	withTestHosts(t)
	if _, err := runMedia(t, fetchMediaRequest{URL: srv.URL + "/to-private", Kind: "progressive"}); err == nil {
		t.Error("an unlisted loopback host must be refused at dial time")
	}
}

func TestResolveForMediaChecksEveryAddress(t *testing.T) {
	withTestHosts(t)
	for _, h := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.9", "169.254.169.254", "::1", "0.0.0.0", "172.16.5.5"} {
		if _, err := resolveForMedia(context.Background(), h); err == nil {
			t.Errorf("%s must be refused", h)
		}
	}
	withTestHosts(t, "127.0.0.1")
	if _, err := resolveForMedia(context.Background(), "127.0.0.1"); err != nil {
		t.Errorf("a listed test host may be loopback: %v", err)
	}
	withTestHosts(t, "10.1.2.3")
	if _, err := resolveForMedia(context.Background(), "10.1.2.3"); err == nil {
		t.Error("a listed private address is still refused: only loopback may be let through")
	}
}

func TestFetchMediaHandlerGatesAndValidates(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	srv := mediaServer(t, map[string]http.HandlerFunc{"/film.mp4": bytesHandler("video/mp4", []byte("abc"))})
	body := `{"url":"` + srv.URL + `/film.mp4","kind":"progressive"}`
	rec := postJSON(FetchMediaHandler, body)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "media_downloads_disabled") {
		t.Fatalf("with the switch off: %d %s", rec.Code, rec.Body)
	}
	if entries, _ := os.ReadDir(shared.CapturesDir); len(entries) != 0 {
		t.Error("a refused download must create nothing")
	}
	if err := shared.SaveSecuritySettings(shared.SecuritySettings{AllowMediaDownloads: true}); err != nil {
		t.Fatal(err)
	}
	if rec := postJSON(FetchMediaHandler, body); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"totalBytes":3`) {
		t.Errorf("with the switch on: %d %s", rec.Code, rec.Body)
	}
	for name, b := range map[string]string{
		"no url": `{"kind":"hls"}`, "no kind": `{"url":"https://a.example/x"}`, "bad kind": `{"url":"https://a.example/x","kind":"ftp"}`, "unknown field": `{"url":"https://a.example/x","kind":"hls","x":1}`,
		"negative bytes": `{"url":"https://a.example/x","kind":"hls","maxBytes":-1}`, "negative seconds": `{"url":"https://a.example/x","kind":"hls","maxSeconds":-1}`, "bad referer": `{"url":"https://a.example/x","kind":"hls","referer":"javascript:x"}`,
		"loopback not listed": `{"url":"http://localhost:1/x","kind":"hls"}`, "private": `{"url":"http://10.0.0.1/x","kind":"hls"}`, "not json": `nope`,
	} {
		withTestHosts(t) // nothing listed for these
		if rec := postJSON(FetchMediaHandler, b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	FetchMediaHandler(rec, httptest.NewRequest(http.MethodGet, "/fetch-media", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}
