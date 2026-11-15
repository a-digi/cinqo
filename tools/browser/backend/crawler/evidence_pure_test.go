package crawler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"

	"browser-tool-backend/shared"
)

// ---- the frame builder (no browser: a node tree is made by hand) ----

func el(name string, attrs []string, children ...*proto.DOMNode) *proto.DOMNode {
	return &proto.DOMNode{NodeName: name, Attributes: attrs, Children: children}
}
func txt(s string) *proto.DOMNode { return &proto.DOMNode{NodeName: "#text", NodeValue: s} }
func doc(u string, children ...*proto.DOMNode) *proto.DOMNode {
	return &proto.DOMNode{NodeName: "#document", DocumentURL: u, Children: children}
}
func iframeNode(frameID string, content *proto.DOMNode, attrs ...string) *proto.DOMNode {
	n := el("IFRAME", attrs)
	n.FrameID = proto.PageFrameID(frameID)
	n.ContentDocument = content
	return n
}
func htmlEl(frameID string, children ...*proto.DOMNode) *proto.DOMNode {
	n := el("HTML", nil, children...)
	n.FrameID = proto.PageFrameID(frameID)
	return n
}

func TestBuildFrameTreeReadsFramesParentsFirstWithAttributes(t *testing.T) {
	inner := doc("https://c.example/inner", htmlEl("F3", el("BODY", nil, txt("Inner deep"))))
	player := doc("https://b.example/player", htmlEl("F2", el("BODY", nil, txt("Player text"), iframeNode("F3", inner, "src", "/inner"))))
	top := doc("https://a.example/", htmlEl("F1", el("HEAD", nil, el("TITLE", nil, txt("Top page"))),
		el("BODY", nil, txt("Top text"), el("A", []string{"href", "/x"}, txt("x")),
			iframeNode("F2", player, "src", "https://b.example/player", "sandbox", "allow-scripts", "allow", "autoplay", "name", "pl"))))
	frames, trunc := buildFrameTree(top, 3, 10)
	if trunc || len(frames) != 3 {
		t.Fatalf("frames=%d truncated=%v", len(frames), trunc)
	}
	f0, f1, f2 := frames[0], frames[1], frames[2]
	if f0.FrameID != "F1" || f0.ParentID != "" || f0.Depth != 0 || f0.URL != "https://a.example/" || f0.Origin != "https://a.example" || f0.Title != "Top page" {
		t.Errorf("top: %+v", f0)
	}
	if f1.FrameID != "F2" || f1.ParentID != "F1" || f1.Depth != 1 || f1.Sandbox != "allow-scripts" || f1.Allow != "autoplay" || f1.Name != "pl" || f1.Origin != "https://b.example" {
		t.Errorf("player: %+v", f1)
	}
	if f2.FrameID != "F3" || f2.ParentID != "F2" || f2.Depth != 2 || f2.Text != "Inner deep" {
		t.Errorf("inner: %+v", f2)
	}
	if strings.Contains(f0.Text, "Player text") || strings.Contains(f1.Text, "Inner deep") {
		t.Error("a frame's text must not include its child frames' text")
	}
	if len(f0.Links) != 2 || f0.Links[0] != "https://a.example/x" || f0.Links[1] != "https://b.example/player" {
		t.Errorf("links of the top frame (the anchor and the iframe address): %v", f0.Links)
	}
}

func TestBuildFrameTreeCaps(t *testing.T) {
	mk := func(n int) *proto.DOMNode {
		var node *proto.DOMNode = doc("https://x.example/leaf", htmlEl("L", el("BODY", nil, txt("leaf"))))
		for i := n; i >= 1; i-- {
			node = doc("https://x.example/"+string(rune('a'+i)), htmlEl("F"+string(rune('a'+i)), el("BODY", nil, iframeNode("c"+string(rune('a'+i)), node, "src", "/n"))))
		}
		return node
	}
	frames, trunc := buildFrameTree(mk(6), 2, 30)
	if len(frames) != 3 || !trunc {
		t.Errorf("depth cap: %d frames, truncated=%v (want 3 frames: depth 0,1,2, and truncated)", len(frames), trunc)
	}
	frames, trunc = buildFrameTree(mk(6), 4, 2)
	if len(frames) != 2 || !trunc {
		t.Errorf("frame cap: %d frames, truncated=%v (want exactly 2, truncated)", len(frames), trunc)
	}
	frames, trunc = buildFrameTree(mk(1), 4, 30)
	if len(frames) != 2 || trunc {
		t.Errorf("no cap hit: %d frames, truncated=%v", len(frames), trunc)
	}
	if f, tr := buildFrameTree(nil, 3, 10); f != nil || tr {
		t.Error("nil root: no frames")
	}
}

func TestBuildFrameTreeTextLinksMediaAndUnreadable(t *testing.T) {
	long := strings.Repeat("word ", 2000)
	top := doc("https://a.example/dir/page", htmlEl("F1", el("BODY", nil,
		el("SCRIPT", nil, txt("var secret=1")), el("STYLE", nil, txt(".x{}")), el("NOSCRIPT", nil, txt("no script")), el("TEMPLATE", nil, txt("tpl")),
		txt("  Visible   text \n here  "), txt(long),
		el("A", []string{"href", "rel/path"}), el("A", []string{"href", "javascript:alert(1)"}), el("A", []string{"href", "data:text/html,x"}), el("A", []string{"href", "mailto:a@b.c"}),
		el("A", []string{"href", "//cdn.example/p"}), el("A", []string{"href", "rel/path"}),
		el("VIDEO", []string{"src", "/v.mp4"}, el("SOURCE", []string{"src", "/v.webm"})), el("AUDIO", []string{"src", "blob:x"}),
		iframeNode("U1", nil, "src", "https://blocked.example/frame"))))
	frames, _ := buildFrameTree(top, 3, 10)
	f := frames[0]
	if strings.Contains(f.Text, "secret") || strings.Contains(f.Text, "no script") || strings.Contains(f.Text, "tpl") || strings.Contains(f.Text, ".x{}") {
		t.Errorf("script/style/noscript/template text must never be returned: %q", f.Text[:60])
	}
	if !strings.HasPrefix(f.Text, "Visible text here word") {
		t.Errorf("whitespace is collapsed: %q", f.Text[:40])
	}
	if len(f.Text) > frameTextCap || !f.TextTruncated {
		t.Errorf("text cap: len=%d truncated=%v", len(f.Text), f.TextTruncated)
	}
	want := []string{"https://a.example/dir/rel/path", "https://cdn.example/p", "https://blocked.example/frame"}
	if len(f.Links) != len(want) {
		t.Fatalf("links %v, want %v (relative resolved, duplicates dropped, javascript:/data:/mailto: dropped)", f.Links, want)
	}
	for i := range want {
		if f.Links[i] != want[i] {
			t.Errorf("link %d = %q, want %q", i, f.Links[i], want[i])
		}
	}
	if len(f.Media) != 2 || f.Media[0] != "https://a.example/v.mp4" || f.Media[1] != "https://a.example/v.webm" {
		t.Errorf("media sources (blob: dropped): %v", f.Media)
	}
	u := frames[1]
	if !u.Unreadable || u.URL != "https://blocked.example/frame" || u.FrameID != "U1" || u.Origin != "https://blocked.example" {
		t.Errorf("an unreadable frame still reports its address: %+v", u)
	}
}

func TestBuildFrameTreeCapsLinkCount(t *testing.T) {
	var as []*proto.DOMNode
	for i := 0; i < frameLinksCap+50; i++ {
		as = append(as, el("A", []string{"href", "/l" + strings.Repeat("x", i)}))
	}
	frames, _ := buildFrameTree(doc("https://a.example/", htmlEl("F", el("BODY", nil, as...))), 3, 10)
	if len(frames[0].Links) != frameLinksCap {
		t.Errorf("links = %d, want the cap %d", len(frames[0].Links), frameLinksCap)
	}
}

func TestClampIntAndOriginAndAttrs(t *testing.T) {
	if clampInt(0, 3, 4) != 3 || clampInt(2, 3, 4) != 2 || clampInt(9, 3, 4) != 4 {
		t.Error("clampInt")
	}
	if originOf("https://a.example:8443/x?y") != "https://a.example:8443" || originOf("about:blank") != "" || originOf("nope") != "" {
		t.Error("originOf")
	}
	a := attrs(&proto.DOMNode{Attributes: []string{"SRC", "x", "odd"}})
	if a["src"] != "x" || len(a) != 1 {
		t.Errorf("attrs: %v", a)
	}
}

// ---- media classification and the network log ----

func TestClassifyMedia(t *testing.T) {
	cases := []struct{ url, mime, rt, want string }{
		{"https://x/a/master.m3u8?token=1", "", "Fetch", mediaHLS},
		{"https://x/a", "application/vnd.apple.mpegurl", "XHR", mediaHLS},
		{"https://x/a", "application/x-mpegURL; charset=utf-8", "XHR", mediaHLS},
		{"https://x/a.mpd", "", "XHR", mediaDASH},
		{"https://x/a", "application/dash+xml", "XHR", mediaDASH},
		{"https://x/seg1.ts", "", "Media", mediaSegment},
		{"https://x/seg1", "video/mp2t", "Media", mediaSegment},
		{"https://x/init.m4s", "", "Fetch", mediaSegment},
		{"https://x/film.mp4", "video/mp4", "Media", mediaProgressive},
		{"https://x/film.mp4", "", "Media", mediaProgressive},
		{"https://x/stream", "video/webm", "Media", mediaProgressive},
		{"https://x/blob", "application/octet-stream", "Fetch", ""},
		{"https://x/film.mkv", "application/octet-stream", "Fetch", mediaProgressive},
		{"https://x/thing", "", "Media", mediaProgressive},
		{"https://x/page.html", "text/html", "Document", ""},
		{"https://x/api.json", "application/json", "Fetch", ""},
		{"https://x/app.js", "text/javascript", "Script", ""},
		{"https://x/logo.png", "image/png", "Image", ""},
		{"https://x/index.m3u8.html", "text/html", "Document", ""},
		{"https://x/video.mp4", "text/html", "Document", ""},
		{"https://x/font.woff2", "font/woff2", "Font", ""},
		{"https://x/data", "", "XHR", ""},
	}
	for _, c := range cases {
		if got := classifyMedia(c.url, c.mime, c.rt); got != c.want {
			t.Errorf("classifyMedia(%q,%q,%q) = %q, want %q", c.url, c.mime, c.rt, got, c.want)
		}
	}
}

func TestNetworkLogCollectsMediaRequestsWithRedirectsAndHeaders(t *testing.T) {
	l := newNetworkLog()
	send := func(id, u, typ, frame string, redirectFrom *proto.NetworkResponse) {
		l.requestSent(&proto.NetworkRequestWillBeSent{RequestID: proto.NetworkRequestID(id), Type: proto.NetworkResourceType(typ), FrameID: proto.PageFrameID(frame),
			Request: &proto.NetworkRequest{URL: u, Method: "GET"}, Initiator: &proto.NetworkInitiator{Type: proto.NetworkInitiatorTypeScript, URL: "https://a.example/player.js"}, RedirectResponse: redirectFrom})
	}
	send("1", "https://a.example/page", "Document", "F1", nil)
	send("2", "https://cdn.example/start.m3u8", "Fetch", "F2", nil)
	send("2", "https://cdn2.example/real.m3u8", "Fetch", "F2", &proto.NetworkResponse{Status: 302})
	l.responseReceived(&proto.NetworkResponseReceived{RequestID: "2", Response: &proto.NetworkResponse{Status: 200, MIMEType: "application/vnd.apple.mpegurl",
		Headers: proto.NetworkHeaders{"Content-Type": gsonStr("application/vnd.apple.mpegurl"), "Set-Cookie": gsonStr("session=secret"), "Server": gsonStr("nginx"), "Authorization": gsonStr("x")}}})
	l.finished(&proto.NetworkLoadingFinished{RequestID: "2", EncodedDataLength: 321})
	send("3", "https://cdn2.example/seg.ts", "Media", "F2", nil)
	l.webSocket(&proto.NetworkWebSocketCreated{RequestID: "9", URL: "wss://live.example/feed"})
	entries, trunc, seen := l.entries()
	if trunc || seen != 4 || len(entries) != 3 {
		t.Fatalf("entries=%d seen=%d truncated=%v", len(entries), seen, trunc)
	}
	e := entries[0]
	if e.URL != "https://cdn2.example/real.m3u8" || e.Kind != mediaHLS || e.Status != 200 || e.Size != 321 || e.FrameID != "F2" || e.Initiator != "https://a.example/player.js" {
		t.Errorf("hls entry: %+v", e)
	}
	if len(e.Redirects) != 1 || e.Redirects[0] != "https://cdn.example/start.m3u8" {
		t.Errorf("redirect chain: %v", e.Redirects)
	}
	if _, bad := e.Headers["set-cookie"]; bad || e.Headers["server"] != "nginx" || e.Headers["authorization"] != "" {
		t.Errorf("only safe headers are kept, never cookies or credentials: %v", e.Headers)
	}
	if entries[1].Kind != mediaSegment || entries[2].Kind != mediaWebSocket || entries[2].URL != "wss://live.example/feed" {
		t.Errorf("segment and websocket: %+v %+v", entries[1], entries[2])
	}
}

func TestNetworkLogCapsEntries(t *testing.T) {
	l := newNetworkLog()
	for i := 0; i < maxNetworkEntries+10; i++ {
		id := proto.NetworkRequestID(strings.Repeat("r", 1) + string(rune('A'+i%26)) + url.QueryEscape(strings.Repeat("x", i)))
		l.requestSent(&proto.NetworkRequestWillBeSent{RequestID: id, Type: "Media", Request: &proto.NetworkRequest{URL: "https://x/" + string(id) + ".mp4", Method: "GET"}})
	}
	entries, trunc, _ := l.entries()
	if len(entries) != maxNetworkEntries || !trunc {
		t.Errorf("entries=%d truncated=%v", len(entries), trunc)
	}
}

// ---- the capture store ----

func TestCaptureIDsAndNamesAreValidatedAndConfined(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	id, dir, err := newCapture()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeArtifact(dir, "meta.json", "meta", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if _, err := writeArtifact(dir, "media/media.mp4", "stream_media", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := writeArtifact(dir, "../escape.txt", "x", []byte("x")); err == nil {
		t.Error("a name that leaves the capture folder must be refused")
	}
	outside := filepath.Join(filepath.Dir(dir), "secret.txt")
	_ = os.WriteFile(outside, []byte("top secret"), 0o600)
	_ = os.Symlink(outside, filepath.Join(dir, "link.txt"))
	for _, bad := range []string{"../secret.txt", "..", "a/../../secret.txt", "/etc/passwd", "media/../../secret.txt", "link.txt", "nothere.json", "", "a b", "media//x", "other/x.mp4", ".hidden"} {
		if _, err := captureFile(id, bad); err == nil {
			t.Errorf("name %q must be refused", bad)
		}
	}
	for _, badID := range []string{"", "../x", "ABCDEF0123456789ABCDEF0123456789", id[:31], id + "0", strings.Repeat("g", 32)} {
		if _, err := captureFile(badID, "meta.json"); err == nil {
			t.Errorf("id %q must be refused", badID)
		}
	}
	if p, err := captureFile(id, "meta.json"); err != nil || filepath.Base(p) != "meta.json" {
		t.Errorf("a good name resolves: %v %v", p, err)
	}
	if _, err := captureFile(id, "media/media.mp4"); err != nil {
		t.Errorf("a media name resolves: %v", err)
	}
}

func getCapture(h http.HandlerFunc, id, name string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/x?id="+id+"&name="+url.QueryEscape(name), nil))
	return rec
}

func TestCaptureFilesAreServedAsOpaqueAttachmentsAndMediaIsSeparate(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	id, dir, _ := newCapture()
	_, _ = writeArtifact(dir, "dom.html", "dom_archive", []byte("<script>alert(1)</script>"))
	_, _ = writeArtifact(dir, "media/media.mp4", "stream_media", []byte("0123456789"))
	rec := getCapture(CaptureFileHandler, id, "dom.html")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/octet-stream" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment;") || rec.Header().Get("Cache-Control") != "no-store" || rec.Body.String() != "<script>alert(1)</script>" {
		t.Errorf("evidence file: %d %v", rec.Code, rec.Header())
	}
	if rec := getCapture(CaptureFileHandler, id, "media/media.mp4"); rec.Code != http.StatusBadRequest {
		t.Errorf("the evidence route must refuse media names: %d", rec.Code)
	}
	if rec := getCapture(MediaFileHandler, id, "dom.html"); rec.Code != http.StatusBadRequest {
		t.Errorf("the media route must refuse evidence names: %d", rec.Code)
	}
	rec = getCapture(MediaFileHandler, id, "media/media.mp4")
	if rec.Code != 200 || rec.Body.String() != "0123456789" || rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("media file: %d %v", rec.Code, rec.Header())
	}
	req := httptest.NewRequest(http.MethodGet, "/x?id="+id+"&name=media%2Fmedia.mp4", nil)
	req.Header.Set("Range", "bytes=2-4")
	rr := httptest.NewRecorder()
	MediaFileHandler(rr, req)
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "234" {
		t.Errorf("a range request: %d %q", rr.Code, rr.Body.String())
	}
	if rec := getCapture(CaptureFileHandler, id, "nothere.json"); rec.Code != http.StatusNotFound {
		t.Errorf("missing: %d", rec.Code)
	}
	if rec := getCapture(CaptureFileHandler, "bad", "dom.html"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", rec.Code)
	}
	for _, h := range []http.HandlerFunc{CaptureFileHandler, MediaFileHandler} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST: %d", rec.Code)
		}
	}
}

func TestCaptureReleaseDeletesTheFolderAndIsIdempotent(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	id, dir, _ := newCapture()
	_, _ = writeArtifact(dir, "meta.json", "meta", []byte("{}"))
	del := func(id string) int {
		rec := httptest.NewRecorder()
		CaptureHandler(rec, httptest.NewRequest(http.MethodDelete, "/capture?id="+id, nil))
		return rec.Code
	}
	if del(id) != http.StatusNoContent {
		t.Error("release")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the folder must be gone")
	}
	if del(id) != http.StatusNoContent {
		t.Error("releasing again is fine")
	}
	for _, bad := range []string{"", "..", "../x", "ABC"} {
		if del(bad) != http.StatusBadRequest {
			t.Errorf("id %q must be refused", bad)
		}
	}
	rec := httptest.NewRecorder()
	CaptureHandler(rec, httptest.NewRequest(http.MethodGet, "/capture?id="+id, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Error("only DELETE")
	}
	// the root is never deletable through removeCaptureDir
	if err := removeCaptureDir(shared.CapturesDir); err == nil {
		t.Error("the captures root must not be deletable")
	}
	if err := removeCaptureDir(t.TempDir()); err == nil {
		t.Error("a folder outside the captures root must not be deletable")
	}
}

func TestACapturesFolderOverItsQuotaRefusesNewCaptures(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	old := capturesQuotaBytes
	capturesQuotaBytes = 100
	defer func() { capturesQuotaBytes = old }()
	_, dir, err := newCapture()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writeArtifact(dir, "media/media.bin", "stream_media", make([]byte, 200))
	if _, _, err := newCapture(); err == nil {
		t.Error("a full captures folder must refuse a new capture")
	}
}

func TestWriteArtifactHashesExactlyTheBytesWritten(t *testing.T) {
	dir := t.TempDir()
	a, err := writeArtifact(dir, "a.txt", "meta", []byte("abc"))
	if err != nil || a.Size != 3 || a.SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("%+v %v", a, err)
	}
	size, sum, err := hashFile(filepath.Join(dir, "a.txt"))
	if err != nil || size != 3 || sum != a.SHA256 {
		t.Errorf("hashFile: %d %s %v", size, sum, err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "a.txt")); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
}

// ---- the handlers that need no browser: their refusals ----

func postJSON(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
	return rec
}

func TestEvidenceHandlersRefuseBadRequestsBeforeAnyBrowser(t *testing.T) {
	withTestHosts(t)
	for name, h := range map[string]http.HandlerFunc{"fetch-frames": FetchFramesHandler, "capture-network": CaptureNetworkHandler, "page-archive": PageArchiveHandler} {
		for label, body := range map[string]string{
			"no url": `{}`, "blank url": `{"url":"  "}`, "unknown field": `{"url":"https://a.example/","x":1}`, "not json": `nope`,
			"negative wait": `{"url":"https://a.example/","waitMs":-1}`, "loopback": `{"url":"http://127.0.0.1/"}`, "private": `{"url":"http://10.0.0.1/"}`,
			"metadata": `{"url":"http://169.254.169.254/latest"}`, "file scheme": `{"url":"file:///etc/passwd"}`, "bad interaction": `{"url":"https://a.example/","interaction":"click-everything"}`,
		} {
			if name == "fetch-frames" && (label == "bad interaction") {
				continue // that route has no interaction field: the unknown field is the refusal
			}
			if rec := postJSON(h, body); rec.Code != http.StatusBadRequest {
				t.Errorf("%s / %s: status %d, want 400", name, label, rec.Code)
			}
		}
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s GET: %d", name, rec.Code)
		}
	}
	if rec := postJSON(FetchFramesHandler, `{"url":"https://a.example/","maxDepth":-1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("negative depth: %d", rec.Code)
	}
}

func gsonStr(s string) gson.JSON { return gson.New(s) }
