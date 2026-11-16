package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFindClickablesHandlerRefusesBadRequestsBeforeAnyBrowser(t *testing.T) {
	withTestHosts(t)
	for name, body := range map[string]string{
		"no url": `{}`, "blank url": `{"url":"  "}`, "unknown field": `{"url":"https://a.example/","click":true}`, "not json": `nope`, "negative wait": `{"url":"https://a.example/","waitMs":-1}`,
		"loopback": `{"url":"http://127.0.0.1/"}`, "private": `{"url":"http://10.0.0.1/"}`, "metadata": `{"url":"http://169.254.169.254/latest"}`, "file": `{"url":"file:///etc/passwd"}`,
	} {
		if rec := postJSON(FindClickablesHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	FindClickablesHandler(rec, httptest.NewRequest(http.MethodGet, "/find-clickables", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestTheItemsAreNumberedCutShortAndCapped(t *testing.T) {
	long := strings.Repeat("a", 90)
	raw := clickablesRaw{
		Frames:   []Clickable{{Selector: "iframe#p", Host: "b.example", Text: long}, {Selector: ""}, {Selector: strings.Repeat("a", 201)}, {Selector: "a\nb"}},
		Controls: []Clickable{{Selector: "button.s1", Text: "  Server   1 \n", Label: long, Href: strings.Repeat("h", 100)}, {Selector: "button.s2", Kind: "frame"}},
	}
	items, trunc := arrangeClickables(raw)
	if trunc || len(items) != 3 {
		t.Fatalf("%v %+v", trunc, items)
	}
	if items[0].ID != "f1" || items[0].Kind != "frame" || items[1].ID != "c1" || items[1].Kind != "control" || items[2].ID != "c2" || items[2].Kind != "control" {
		t.Errorf("frames are f, controls are c, the kind is set here: %+v", items)
	}
	if items[1].Text != "Server 1" || len([]rune(items[1].Label)) != clickableTextMax || !strings.HasSuffix(items[1].Label, "…") || len([]rune(items[1].Href)) != 80 || len([]rune(items[0].Text)) != clickableTextMax {
		t.Errorf("words are one line and short: %+v", items[1])
	}
	if items, _ := arrangeClickables(clickablesRaw{Controls: []Clickable{{Selector: strings.Repeat("a", clickableSelectorMax)}, {Selector: strings.Repeat("a", clickableSelectorMax+1)}}}); len(items) != 1 {
		t.Errorf("a selector of exactly %d characters is kept, one longer is dropped: %d", clickableSelectorMax, len(items))
	}
	var many clickablesRaw
	for i := 0; i < 15; i++ {
		many.Frames = append(many.Frames, Clickable{Selector: fmt.Sprintf("iframe.f%d", i)})
	}
	for i := 0; i < 60; i++ {
		many.Controls = append(many.Controls, Clickable{Selector: fmt.Sprintf("button.c%d", i)})
	}
	items, trunc = arrangeClickables(many)
	frames, controls := 0, 0
	for _, it := range items {
		if it.Kind == "frame" {
			frames++
		} else {
			controls++
		}
	}
	if !trunc || len(items) != maxClickables || frames != maxClickableFrames || controls != maxClickables-maxClickableFrames {
		t.Errorf("at most %d, %d of them frames: %d items, %d frames, %d controls, truncated %v", maxClickables, maxClickableFrames, len(items), frames, controls, trunc)
	}
	exact := clickablesRaw{}
	for i := 0; i < maxClickables; i++ {
		exact.Controls = append(exact.Controls, Clickable{Selector: fmt.Sprintf("button.c%d", i)})
	}
	if items, trunc := arrangeClickables(exact); trunc || len(items) != maxClickables {
		t.Errorf("exactly the cap is not truncated: %d %v", len(items), trunc)
	}
	if items, trunc := arrangeClickables(clickablesRaw{}); trunc || items == nil || len(items) != 0 {
		t.Errorf("an empty page is an empty list, not null: %v %v", items, trunc)
	}
}

func TestCutTextIsOneShortLine(t *testing.T) {
	if cutText("  a \n b\t c ", 10) != "a b c" || cutText("abcdef", 6) != "abcdef" || cutText("abcdefg", 6) != "abcde…" || cutText("ééééééé", 4) != "ééé…" {
		t.Error("cutText")
	}
}

// clickMock is a page that counts every press: nothing may ever press it.
func clickMock(t *testing.T) (port string, presses *int32) {
	t.Helper()
	var n int32
	var srv *httptest.Server
	mux := http.NewServeMux()
	p := func() string { return strings.Split(srv.Listener.Addr().String(), ":")[1] }
	mux.HandleFunc("/pressed", func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&n, 1) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		many := ""
		if r.URL.Query().Get("many") != "" {
			for i := 0; i < 50; i++ {
				many += fmt.Sprintf(`<button class="srv srv-%d">Mirror %d</button>`, i, i)
			}
		}
		fmt.Fprintf(w, `<html><head><title>Watch</title></head><body onclick="fetch('/pressed')">
			<a href="/about">About us</a><a href="/contact">Contact</a>
			<nav><a href="#" id="menu">Menu</a></nav>
			<button id="s2" class="server tab">Server 2</button>
			<div class="play-cover" style="width:200px;height:80px;background:#ccc;cursor:pointer">Press to play</div>
			<a href="https://ads.example.net/x" class="play-now">Play now</a>
			<a href="#" role="tab" class="tab-x">Quality HD</a>
			<button style="display:none" class="play-hidden">Play hidden</button>
			<button style="visibility:hidden" class="play-invisible">Play invisible</button>
			<span class="text-only">Server tells a story but is no control</span>
			<div id="wrap" style="width:2000px;height:2000px" onclick="1">player wrapper <button class="inner play-inner">Play inner</button></div>
			<iframe id="pf" src="http://b.localhost:%s/frame" style="width:400px;height:200px;border:0"></iframe>
			<iframe id="tiny" src="http://b.localhost:%s/frame" style="width:1px;height:1px;border:0"></iframe>
			<iframe class="ad" src="/frame" style="width:300px;height:100px"></iframe>
			<div id="many">%s</div></body></html>`, p(), p(), many)
	})
	mux.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `<html><body>frame</body></html>`) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return p(), &n
}

func TestFindClickablesListsTheControlsAndFramesOfARealPageAndPressesNothing(t *testing.T) {
	requireRodChrome(t)
	port, presses := clickMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := findClickables(ctx, findClickablesRequest{URL: "http://a.localhost:" + port + "/", WaitMs: 500})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(presses) != 0 {
		t.Fatalf("the page was pressed %d times: find-clickables is read-only", *presses)
	}
	if res.Truncated || len(res.Items) != 7 {
		t.Fatalf("a small page: everything is listed (%d, %v)", len(res.Items), res.Truncated)
	}
	byText := map[string]Clickable{}
	var frames []Clickable
	for _, it := range res.Items {
		if it.Kind == "frame" {
			frames = append(frames, it)
		} else if _, ok := byText[it.Text]; !ok {
			byText[it.Text] = it
		}
	}
	if len(frames) != 2 || frames[0].ID != "f1" || frames[1].ID != "f2" {
		t.Fatalf("the player frame and the other big frame are listed, the 1 pixel one is not: %+v", frames)
	}
	pf := frames[0]
	if pf.Selector != "#pf" || pf.Host != "b.localhost" || pf.Width != 400 || pf.Height != 200 || !strings.Contains(pf.Why[0], "another site") {
		t.Errorf("%+v", pf)
	}
	for _, want := range []string{"Server 2", "Press to play", "Quality HD", "Play inner"} {
		if _, ok := byText[want]; !ok {
			t.Errorf("%q should be a candidate: %v", want, byText)
		}
	}
	for _, not := range []string{"About us", "Contact", "Menu", "Play hidden", "Play invisible", "Server tells a story but is no control"} {
		if _, ok := byText[not]; ok {
			t.Errorf("%q is not a candidate", not)
		}
	}
	if s2 := byText["Server 2"]; s2.Selector != "#s2" || s2.Tag != "button" || s2.Width == 0 || len(s2.Why) == 0 {
		t.Errorf("%+v", s2)
	}
	ad := byText["Play now"]
	if ad.Selector == "" || !ad.Leaves || !strings.HasPrefix(ad.Href, "ads.example.net") {
		t.Errorf("a link to another site says it leaves the page: %+v", ad)
	}
	if wrap := byText["player wrapper Play inner"]; wrap.Selector != "" {
		t.Errorf("a whole-page wrapper is not a button: %+v", wrap)
	}
	// every selector matches exactly one element of the page: that is what a recipe step needs
	ep, err := openEvidencePage(ctx, "http://a.localhost:"+port+"/", "")
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()
	_ = sleepCtx(ctx, 500*time.Millisecond)
	for _, it := range res.Items {
		raw, err := rodEvalExpression(ep.P, fmt.Sprintf(`document.querySelectorAll(%s).length`, jsString(it.Selector)))
		var n int
		if err != nil || json.Unmarshal(raw, &n) != nil || n != 1 {
			t.Errorf("%s %q matches %s elements (%v)", it.ID, it.Selector, raw, err)
		}
		if len(it.Selector) > clickableSelectorMax {
			t.Errorf("%s: selector too long", it.ID)
		}
	}
	if atomic.LoadInt32(presses) != 0 {
		t.Error("still nothing pressed")
	}
	// the controls come best first: a button of a server beats a plain pointer-cursor block
	order := map[string]int{}
	for i, it := range res.Items {
		order[it.Text] = i
	}
	if order["Server 2"] > order["Press to play"] {
		t.Errorf("order: %v", order)
	}
}

func TestAPageWithMoreThanFortyCandidatesIsCappedAndSaysSo(t *testing.T) {
	requireRodChrome(t)
	port, presses := clickMock(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := findClickables(ctx, findClickablesRequest{URL: "http://a.localhost:" + port + "/?many=1", WaitMs: 500})
	if err != nil {
		t.Fatal(err)
	}
	frames := 0
	for _, it := range res.Items {
		if it.Kind == "frame" {
			frames++
		}
	}
	if !res.Truncated || len(res.Items) != maxClickables || frames != 2 || atomic.LoadInt32(presses) != 0 {
		t.Errorf("%d items, %d frames, truncated %v, presses %d", len(res.Items), frames, res.Truncated, *presses)
	}
	best := map[string]bool{}
	for _, it := range res.Items {
		best[it.Text] = true
	}
	if !best["Server 2"] || !best["Quality HD"] {
		t.Errorf("the best candidates survive the cap, the plain mirrors fill the rest: %v", best)
	}
}

func TestAPageWithNothingToPressGivesAnEmptyListNotAnError(t *testing.T) {
	requireRodChrome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><p>Just text.</p><a href="/x">Read more</a></body></html>`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	port := strings.Split(srv.Listener.Addr().String(), ":")[1]
	res, err := findClickables(ctx, findClickablesRequest{URL: "http://a.localhost:" + port + "/", WaitMs: 200})
	if err != nil || res.Truncated || res.Items == nil || len(res.Items) != 0 || !strings.HasPrefix(res.FinalURL, "http://a.localhost:") {
		t.Errorf("%v %+v", err, res)
	}
}

func TestTheLayersAreNumberedCutShortFilteredAndCapped(t *testing.T) {
	long := strings.Repeat("a", 90)
	got := arrangeLayers([]Layer{
		{Kind: "close", Selector: "#x", Text: "  ×  ", Label: long},
		{Kind: "nonsense", Selector: "#y"},
		{Kind: "cookie", Selector: ""},
		{Kind: "age", Selector: strings.Repeat("a", clickableSelectorMax+1)},
		{Kind: "skip", Selector: "a\nb"},
		{Kind: "age", Selector: strings.Repeat("a", clickableSelectorMax)},
		{Kind: "cover", Selector: "div.cover", CoversPlayer: true},
	})
	if len(got) != 3 || got[0].ID != "l1" || got[1].ID != "l2" || got[2].ID != "l3" || got[2].Kind != "cover" || !got[2].CoversPlayer {
		t.Fatalf("%+v", got)
	}
	if got[0].Text != "×" || len([]rune(got[0].Label)) != clickableTextMax || !strings.HasSuffix(got[0].Label, "…") {
		t.Errorf("words are one short line: %+v", got[0])
	}
	var many []Layer
	for i := 0; i < maxLayers+4; i++ {
		many = append(many, Layer{Kind: "close", Selector: fmt.Sprintf("#c%d", i)})
	}
	if got := arrangeLayers(many); len(got) != maxLayers || got[maxLayers-1].ID != fmt.Sprintf("l%d", maxLayers) {
		t.Errorf("at most %d layers: %d", maxLayers, len(got))
	}
	if got := arrangeLayers(nil); got == nil || len(got) != 0 {
		t.Errorf("no layers is an empty list, not null: %#v", got)
	}
}

// layerMock serves pages with things in the way; /count/ records any press, so the test can show the read pressed nothing.
func layerMock(t *testing.T) (string, *int32) {
	t.Helper()
	var n int32
	mux := http.NewServeMux()
	mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&n, 1) })
	count := `onclick="fetch('/count')"`
	mux.HandleFunc("/sponsor", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body style="margin:0"><button id="server2" class="server" %[1]s>Server 2</button>
<iframe id="pf" src="about:blank" width="400" height="220" style="display:block;border:0"></iframe>
<div id="sponsor" style="position:fixed;left:0;top:0;width:100%%;height:100%%;background:#000;color:#fff;z-index:99">Our sponsor <button id="shut" %[1]s style="width:40px;height:30px">×</button></div></body></html>`, count)
	})
	mux.HandleFunc("/coveronly", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body style="margin:0"><iframe id="pf" src="about:blank" width="400" height="220" style="display:block;border:0"></iframe>
<div id="veil" %[1]s style="position:fixed;left:0;top:0;width:100%%;height:100%%;z-index:99;background:#222"></div></body></html>`, count)
	})
	mux.HandleFunc("/gates", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><iframe id="pf" src="about:blank" width="400" height="220"></iframe>
<div id="cookie-banner"><button id="acc" %[1]s>Accept all</button></div>
<div id="plain"><button id="acc2" %[1]s>Accept all</button></div>
<button id="gate" %[1]s>I am over 18</button>
<span id="skipbar" role="button" %[1]s>Skip ad</span>
<a id="adx" href="http://ads.example.net/x" aria-label="Close" style="display:inline-block;width:30px;height:30px">x</a>
<button id="hidden" style="display:none" %[1]s>Accept all</button>
<p>Close to nothing</p></body></html>`, count)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.Split(srv.Listener.Addr().String(), ":")[1], &n
}

func layersOf(t *testing.T, port, path string) ClickablesResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := findClickables(ctx, findClickablesRequest{URL: "http://a.localhost:" + port + path, WaitMs: 400})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestALayerOverThePlayerOffersItsCloseButtonAndNothingIsPressed(t *testing.T) {
	requireRodChrome(t)
	port, presses := layerMock(t)
	res := layersOf(t, port, "/sponsor")
	if len(res.Layers) == 0 {
		t.Fatalf("no layers: %+v", res)
	}
	first := res.Layers[0]
	if first.ID != "l1" || first.Kind != "close" || first.Selector != "#shut" || first.Text != "×" || !first.CoversPlayer || first.Leaves {
		t.Errorf("the close button inside the layer over the player comes first: %+v", first)
	}
	if atomic.LoadInt32(presses) != 0 {
		t.Errorf("the page was pressed %d times: layers are only listed", *presses)
	}
	for _, it := range res.Items {
		if it.Selector == "#shut" {
			t.Errorf("a layer is never part of the items: %+v", it)
		}
	}
}

func TestACoverWithNothingToCloseIsOfferedItself(t *testing.T) {
	requireRodChrome(t)
	port, presses := layerMock(t)
	res := layersOf(t, port, "/coveronly")
	if len(res.Layers) != 1 || res.Layers[0].Kind != "cover" || res.Layers[0].Selector != "#veil" || !res.Layers[0].CoversPlayer || atomic.LoadInt32(presses) != 0 {
		t.Errorf("%+v presses %d", res.Layers, *presses)
	}
}

func TestCookieAgeSkipAndCloseControlsAreListedWithTheirKind(t *testing.T) {
	requireRodChrome(t)
	port, presses := layerMock(t)
	res := layersOf(t, port, "/gates")
	byID := map[string]Layer{}
	for _, l := range res.Layers {
		byID[strings.TrimPrefix(l.Selector, "#")] = l
	}
	for sel, kind := range map[string]string{"acc": "cookie", "gate": "age", "skipbar": "skip", "adx": "close"} {
		if l, ok := byID[sel]; !ok || l.Kind != kind || l.CoversPlayer {
			t.Errorf("%s should be a %s layer: %+v", sel, kind, l)
		}
	}
	if _, ok := byID["acc2"]; ok {
		t.Error("an Accept button outside a cookie banner is not a layer")
	}
	if _, ok := byID["hidden"]; ok {
		t.Error("a hidden control is not a layer")
	}
	if l := byID["adx"]; !l.Leaves {
		t.Errorf("a close button that is an ad link says it leaves the page: %+v", l)
	}
	if atomic.LoadInt32(presses) != 0 {
		t.Errorf("pressed %d times", *presses)
	}
}

func TestAPageWithNothingInTheWayHasAnEmptyLayerList(t *testing.T) {
	requireRodChrome(t)
	port, _ := clickMock(t)
	res := layersOf(t, port, "/")
	if res.Layers == nil {
		t.Errorf("no layers must be an empty list: %+v", res)
	}
	for _, l := range res.Layers {
		if l.CoversPlayer {
			t.Errorf("nothing covers the player on this page: %+v", l)
		}
	}
}
