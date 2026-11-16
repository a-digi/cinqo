package crawler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"browser-tool-backend/shared"
)

func switchMediaOn(t *testing.T) {
	t.Helper()
	if err := shared.SaveSecuritySettings(shared.SecuritySettings{AllowMediaDownloads: true}); err != nil {
		t.Fatal(err)
	}
}

func TestRelayAnswersWithTheBytesAndOnlyTheListedFacts(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	switchMediaOn(t)
	var sawCookie, sawAuth, sawReferer, sawRange, sawOrigin string
	srv := mediaServer(t, map[string]http.HandlerFunc{"/s.ts": func(w http.ResponseWriter, r *http.Request) {
		sawCookie, sawAuth, sawReferer, sawRange, sawOrigin = r.Header.Get("Cookie"), r.Header.Get("Authorization"), r.Header.Get("Referer"), r.Header.Get("Range"), r.Header.Get("Origin")
		w.Header().Set("Set-Cookie", "a=b")
		w.Header().Set("Server", "pirate/1")
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Range", "bytes 2-5/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("2345"))
	}})
	req := httptest.NewRequest(http.MethodPost, "/relay", strings.NewReader(`{"url":"`+srv.URL+`/s.ts","kind":"part","referer":"https://page.example/watch/1","range":"bytes=2-5"}`))
	req.Header.Set("Cookie", "person=secret")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	RelayHandler(rec, req)
	if rec.Code != 200 || rec.Body.String() != "2345" {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	if h.Get("X-Relay-Status") != "206" || h.Get("Content-Range") != "bytes 2-5/10" || h.Get("Content-Length") != "4" {
		t.Errorf("the facts: %v", h)
	}
	if h.Get("Content-Type") != "application/octet-stream" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-store" {
		t.Errorf("the type is forced, never the host's: %v", h)
	}
	if h.Get("Set-Cookie") != "" || h.Get("Server") != "" {
		t.Errorf("no other upstream header passes: %v", h)
	}
	if sawCookie != "" || sawAuth != "" {
		t.Errorf("no cookie and no authorisation reach the host: %q %q", sawCookie, sawAuth)
	}
	if sawReferer != "https://page.example/watch/1" || sawOrigin != "https://page.example" || sawRange != "bytes=2-5" {
		t.Errorf("referer %q origin %q range %q", sawReferer, sawOrigin, sawRange)
	}
}

func TestRelayRefusesBeforeFetchingAnything(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	var hits int
	srv := mediaServer(t, map[string]http.HandlerFunc{"/x": func(w http.ResponseWriter, r *http.Request) { hits++; _, _ = w.Write([]byte("x")) }})
	body := `{"url":"` + srv.URL + `/x","kind":"part"}`
	if rec := postJSON(RelayHandler, body); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "media_downloads_disabled") {
		t.Fatalf("switch off: %d %s", rec.Code, rec.Body)
	}
	switchMediaOn(t)
	for name, b := range map[string]string{
		"no url": `{"kind":"part"}`, "no kind": `{"url":"https://a.example/x"}`, "bad kind": `{"url":"https://a.example/x","kind":"page"}`, "unknown field": `{"url":"https://a.example/x","kind":"part","cookie":"a=b"}`,
		"file": `{"url":"file:///etc/passwd","kind":"part"}`, "javascript": `{"url":"javascript:1","kind":"part"}`, "data": `{"url":"data:text/plain,hi","kind":"part"}`, "no host": `{"url":"http:///x","kind":"part"}`,
		"bad referer": `{"url":"https://a.example/x","kind":"part","referer":"javascript:x"}`,
		"ftp referer": `{"url":"https://a.example/x","kind":"part","referer":"ftp://a.example/x"}`, "hostless referer": `{"url":"https://a.example/x","kind":"part","referer":"https:///x"}`,
		"relative referer": `{"url":"https://a.example/x","kind":"part","referer":"watch/1"}`, "long referer": `{"url":"https://a.example/x","kind":"part","referer":"https://a.example/` + strings.Repeat("a", 2000) + `"}`, "bad range": `{"url":"https://a.example/x","kind":"part","range":"bytes=0-1,5-9"}`,
		"range text": `{"url":"https://a.example/x","kind":"part","range":"items=0-1"}`, "not json": `nope`,
		"private": `{"url":"http://10.0.0.1/x","kind":"part"}`, "metadata": `{"url":"http://169.254.169.254/latest","kind":"key"}`,
	} {
		withTestHosts(t)
		if rec := postJSON(RelayHandler, b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	withTestHosts(t, "127.0.0.1")
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/x","kind":"part"}`); rec.Code != 200 || hits != 1 {
		t.Errorf("a good request still works: %d hits %d", rec.Code, hits)
	}
	rec := httptest.NewRecorder()
	RelayHandler(rec, httptest.NewRequest(http.MethodGet, "/relay", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestRelayNeverReachesTheInsideNotEvenByRedirect(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	switchMediaOn(t)
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/to-private": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://10.255.255.1/s.ts", http.StatusFound)
		},
		"/to-metadata": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/x", http.StatusFound)
		},
		"/to-file": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		},
		"/loop": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) },
		"/ok":   func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("fine")) },
		"/hop":  func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusFound) },
	})
	for _, p := range []string{"/to-private", "/to-metadata", "/to-file", "/loop"} {
		if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+p+`","kind":"part"}`); rec.Code != http.StatusBadGateway {
			t.Errorf("%s: %d, want 502", p, rec.Code)
		}
	}
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/hop","kind":"part"}`); rec.Code != 200 || rec.Body.String() != "fine" {
		t.Errorf("a redirect to a safe address is followed: %d %q", rec.Code, rec.Body)
	}
	withTestHosts(t)
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/ok","kind":"part"}`); rec.Code == 200 {
		t.Error("a loopback host that is not an allowed test host is refused")
	}
}

func TestRelayCapsAndStatuses(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	switchMediaOn(t)
	big := func(n int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("a", n))) }
	}
	chunked := func(n int) http.HandlerFunc { // no Content-Length: the cap is met while reading
		return func(w http.ResponseWriter, r *http.Request) {
			f := w.(http.Flusher)
			for sent := 0; sent < n; sent += 1024 {
				_, _ = w.Write([]byte(strings.Repeat("a", 1024)))
				f.Flush()
			}
		}
	}
	srv := mediaServer(t, map[string]http.HandlerFunc{
		"/pl-ok": big(maxPlaylistBytes), "/pl-big": big(maxPlaylistBytes + 1), "/key-ok": big(maxKeyBytes), "/key-big": big(maxKeyBytes + 1),
		"/part-ok": big(maxRelayPartBytes), "/part-big": big(maxRelayPartBytes + 1), "/chunk-big": chunked(maxKeyBytes + 4096),
		"/404": http.NotFound, "/500": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) },
	})
	for _, c := range []struct {
		path, kind string
		want       int
	}{{"/pl-ok", "playlist", 200}, {"/pl-big", "playlist", 502}, {"/key-ok", "key", 200}, {"/key-big", "key", 502}, {"/part-ok", "part", 200}, {"/part-big", "part", 502}, {"/chunk-big", "key", 502}, {"/404", "part", 502}, {"/500", "part", 502}} {
		rec := postJSON(RelayHandler, `{"url":"`+srv.URL+c.path+`","kind":"`+c.kind+`"}`)
		if rec.Code != c.want {
			t.Errorf("%s as %s: %d, want %d", c.path, c.kind, rec.Code, c.want)
		}
		if c.want == 200 && rec.Header().Get("Content-Length") != strconv.Itoa(rec.Body.Len()) {
			t.Errorf("%s: length %s vs %d", c.path, rec.Header().Get("Content-Length"), rec.Body.Len())
		}
	}
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/404","kind":"part"}`); strings.Contains(rec.Body.String(), "<") {
		t.Error("the host's answer is never echoed")
	}
}

func TestRelayCapIsExactWhenTheHostSaysTheLength(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	switchMediaOn(t)
	sized := func(n int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(n))
			_, _ = w.Write([]byte(strings.Repeat("a", n)))
		}
	}
	srv := mediaServer(t, map[string]http.HandlerFunc{"/at": sized(maxKeyBytes), "/over": sized(maxKeyBytes + 1)})
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/at","kind":"key"}`); rec.Code != 200 || rec.Body.Len() != maxKeyBytes {
		t.Errorf("exactly at the cap is fine: %d", rec.Code)
	}
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/over","kind":"key"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("one byte over, said up front: %d", rec.Code)
	}
}

func TestRelayHoldsAtMostEightAtOnce(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	withTestHosts(t, "127.0.0.1")
	switchMediaOn(t)
	release := make(chan struct{})
	entered := make(chan struct{}, maxRelaysInFlight)
	srv := mediaServer(t, map[string]http.HandlerFunc{"/slow": func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte("x"))
	}})
	var wg sync.WaitGroup
	codes := make(chan int, maxRelaysInFlight)
	for i := 0; i < maxRelaysInFlight; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- postJSON(RelayHandler, `{"url":"`+srv.URL+`/slow","kind":"part"}`).Code
		}()
	}
	for i := 0; i < maxRelaysInFlight; i++ {
		<-entered
	}
	rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/slow","kind":"part"}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("the ninth: %d", rec.Code)
	}
	close(release)
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 200 {
			t.Errorf("a relay in flight: %d", c)
		}
	}
	if rec := postJSON(RelayHandler, `{"url":"`+srv.URL+`/slow","kind":"part"}`); rec.Code != 200 {
		t.Errorf("a slot is free again: %d", rec.Code)
	}
}
