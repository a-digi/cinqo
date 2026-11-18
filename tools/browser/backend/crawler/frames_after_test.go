package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOnlyWebFramesAreKeptOnceAndTheNewOnesComeFirst(t *testing.T) {
	baseline := []FrameAfter{{URL: "https://a.example/old", Depth: 1}, {URL: "about:blank", Depth: 1}}
	now := []FrameAfter{
		{URL: "about:blank", Depth: 1},
		{URL: "https://a.example/old", ParentURL: "https://a.example/", Depth: 1},
		{URL: "https://b.example/player", ParentURL: "https://a.example/", Depth: 1},
		{URL: "https://b.example/player", Depth: 1},
		{URL: "data:text/html,<p>x", Depth: 1},
		{URL: "javascript:void(0)", Depth: 1},
		{URL: "blob:https://a.example/1", Depth: 2},
		{URL: "ftp://a.example/x", Depth: 1},
		{URL: "https:///nohost", Depth: 1},
		{URL: "https://c.example/deeper", ParentURL: "https://b.example/player", Depth: 2},
	}
	got := arrangeFrames(now, baseline)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	if got[0].URL != "https://b.example/player" || !got[0].New || got[0].ParentURL != "https://a.example/" || got[1].URL != "https://c.example/deeper" || !got[1].New || got[2].URL != "https://a.example/old" || got[2].New {
		t.Errorf("the new ones first, in page order; the old one is not new: %+v", got)
	}
}

func TestFramesAreCutShortAndCappedAndNoneIsAnEmptyList(t *testing.T) {
	long := "https://a.example/" + strings.Repeat("p", 400)
	got := arrangeFrames([]FrameAfter{{URL: long, ParentURL: long, Depth: 1}}, nil)
	if len(got) != 1 || len([]rune(got[0].URL)) != frameAfterURLMax || !strings.HasSuffix(got[0].URL, "…") || len([]rune(got[0].ParentURL)) != frameAfterURLMax || !got[0].New {
		t.Errorf("%+v", got)
	}
	// a long address that was in the baseline is still not new
	if got := arrangeFrames([]FrameAfter{{URL: long}}, []FrameAfter{{URL: long}}); got[0].New {
		t.Error("an address seen at load is not new, however long")
	}
	var many []FrameAfter
	for i := 0; i < maxFramesAfter+5; i++ {
		many = append(many, FrameAfter{URL: fmt.Sprintf("https://a.example/%d", i), Depth: 1})
	}
	if got := arrangeFrames(many, nil); len(got) != maxFramesAfter {
		t.Errorf("at most %d: %d", maxFramesAfter, len(got))
	}
	if got := arrangeFrames(nil, nil); got == nil || len(got) != 0 {
		t.Errorf("no frames is an empty list, not null: %#v", got)
	}
	if got := arrangeFrames([]FrameAfter{{URL: "about:blank"}}, nil); got == nil || len(got) != 0 {
		t.Errorf("only blank frames: %#v", got)
	}
}

func TestAFrameAddressNeedsAWebSchemeAndAHost(t *testing.T) {
	for raw, want := range map[string]bool{"https://a.example/x": true, "http://a.example": true, " https://a.example/x ": true, "about:blank": false, "": false, "data:text/html,x": false, "javascript:1": false, "blob:https://a.example/1": false, "ftp://a.example/": false, "https:///x": false, "://": false} {
		if frameAddressOK(raw) != want {
			t.Errorf("%q: want %v", raw, !want)
		}
	}
}

func TestTheCaptureHandlerChecksTheRefererBeforeAnyBrowser(t *testing.T) {
	withTestHosts(t)
	for name, body := range map[string]string{
		"loopback referer": `{"url":"https://a.example/","referer":"http://127.0.0.1/"}`, "private referer": `{"url":"https://a.example/","referer":"http://10.0.0.1/x"}`,
		"file referer": `{"url":"https://a.example/","referer":"file:///etc/passwd"}`, "long referer": `{"url":"https://a.example/","referer":"https://a.example/` + strings.Repeat("a", 2000) + `"}`,
		"unknown field": `{"url":"https://a.example/","cookie":"x"}`,
	} {
		if rec := postJSON(CaptureNetworkHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
}

// lateMock serves a page whose frame is blank until a button is pressed (then it gets the address of a page on another host), and records the referrer a page was
// opened with.
func lateMock(t *testing.T) (string, *refererLog) {
	t.Helper()
	rl := &refererLog{}
	mux := http.NewServeMux()
	mux.HandleFunc("/late", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><button id="go" class="server">Server 2</button><iframe id="pf" src="about:blank" width="400" height="220"></iframe>
<script>document.getElementById("go").addEventListener("click", function(){ document.getElementById("pf").src = "http://b.localhost:" + location.port + "/inner"; });</script></body></html>`)
	})
	mux.HandleFunc("/early", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><iframe id="pf" src="http://b.localhost:`+strings.Split(r.Host, ":")[1]+`/inner" width="400" height="220"></iframe></body></html>`)
	})
	mux.HandleFunc("/inner", func(w http.ResponseWriter, r *http.Request) {
		rl.add(r.Header.Get("Referer"))
		fmt.Fprint(w, `<html><body>inner</body></html>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.Split(srv.Listener.Addr().String(), ":")[1], rl
}

type refererLog struct {
	mu   sync.Mutex
	seen []string
}

func (r *refererLog) add(s string) {
	r.mu.Lock()
	r.seen = append(r.seen, s)
	r.mu.Unlock()
}

func (r *refererLog) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func TestAFrameThatGetsItsAddressAfterAPressIsListedAsNew(t *testing.T) {
	requireRodChrome(t)
	port, _ := lateMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	base := "http://a.localhost:" + port
	res, err := captureNetwork(ctx, captureNetworkRequest{URL: base + "/late", WaitMs: 1500, Frames: true, Steps: []RecipeStep{{Do: "click", Selector: "#go"}, {Do: "wait", Ms: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 1 || res.Frames[0].URL != "http://b.localhost:"+port+"/inner" || !res.Frames[0].New || res.Frames[0].Depth != 1 || res.Frames[0].ParentURL != base+"/late" {
		t.Errorf("the frame the press made appear: %+v", res.Frames)
	}
	// without the flag the answer carries no frames (callers that never asked are unchanged)
	res, err = captureNetwork(ctx, captureNetworkRequest{URL: base + "/late", WaitMs: 300})
	if err != nil || res.Frames != nil {
		t.Errorf("%v %+v", err, res.Frames)
	}
}

func TestAFrameThatWasThereAtLoadIsNotNew(t *testing.T) {
	requireRodChrome(t)
	port, _ := lateMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := captureNetwork(ctx, captureNetworkRequest{URL: "http://a.localhost:" + port + "/early", WaitMs: 500, Frames: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 1 || res.Frames[0].New || !strings.HasSuffix(res.Frames[0].URL, "/inner") {
		t.Errorf("%+v", res.Frames)
	}
}

func TestAnAddressOpenedWithARefererSendsItAndOneWithoutDoesNot(t *testing.T) {
	requireRodChrome(t)
	port, rl := lateMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	parent := "http://a.localhost:" + port + "/late"
	if _, err := captureNetwork(ctx, captureNetworkRequest{URL: "http://b.localhost:" + port + "/inner", WaitMs: 300, Referer: parent}); err != nil {
		t.Fatal(err)
	}
	if _, err := captureNetwork(ctx, captureNetworkRequest{URL: "http://b.localhost:" + port + "/inner", WaitMs: 300}); err != nil {
		t.Fatal(err)
	}
	got := rl.all()
	if len(got) != 2 || !strings.HasPrefix(got[0], "http://a.localhost:"+port) || got[1] != "" {
		t.Errorf("the referer of the first open is the parent page, the second has none: %q", got)
	}
}

func TestFindClickablesChecksTheRefererToo(t *testing.T) {
	withTestHosts(t)
	for name, body := range map[string]string{"private": `{"url":"https://a.example/","referer":"http://10.0.0.1/x"}`, "file": `{"url":"https://a.example/","referer":"file:///x"}`} {
		if rec := postJSON(FindClickablesHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if err := validateReferer(""); err != nil {
		t.Errorf("no referer is fine: %v", err)
	}
	if err := validateReferer(strings.Repeat("a", 2001)); err == nil {
		t.Error("too long")
	}
}

func TestFindClickablesOpensAnAddressWithTheReferer(t *testing.T) {
	requireRodChrome(t)
	port, rl := lateMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	parent := "http://a.localhost:" + port + "/late"
	if _, err := findClickables(ctx, findClickablesRequest{URL: "http://b.localhost:" + port + "/inner", WaitMs: 200, Referer: parent}); err != nil {
		t.Fatal(err)
	}
	if got := rl.all(); len(got) != 1 || !strings.HasPrefix(got[0], "http://a.localhost:"+port) {
		t.Errorf("%q", got)
	}
}
