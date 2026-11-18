package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateRecipe(t *testing.T) {
	click := func(sel string) RecipeStep { return RecipeStep{Do: "click", Selector: sel} }
	wait := func(ms int) RecipeStep { return RecipeStep{Do: "wait", Ms: ms} }
	good := [][]RecipeStep{nil, {}, {click("button.server-2")}, {click("a"), {Do: "clickFrame", Selector: "iframe#p"}, wait(5000), wait(5000)},
		{click("a"), click("a"), click("a"), click("a"), click("a"), click("a")}, {wait(1)}, {click(strings.Repeat("a", 200))}}
	for i, s := range good {
		if err := validateRecipe(s); err != nil {
			t.Errorf("good %d: %v", i, err)
		}
	}
	seven := []RecipeStep{click("a"), click("a"), click("a"), click("a"), click("a"), click("a"), click("a")}
	bad := map[string][]RecipeStep{
		"seven steps": seven, "no action": {{Selector: "a"}}, "type": {{Do: "type", Selector: "input"}}, "navigate": {{Do: "navigate", Selector: "https://x.example/"}}, "script": {{Do: "eval", Selector: "alert(1)"}},
		"click without selector": {{Do: "click"}}, "blank selector": {click("   ")}, "frame without selector": {{Do: "clickFrame"}}, "long selector": {click(strings.Repeat("a", 201))},
		"newline in selector": {click("a\nb")}, "nul in selector": {click("a\x00b")}, "click with ms": {{Do: "click", Selector: "a", Ms: 5}}, "wait zero": {wait(0)}, "wait negative": {wait(-1)},
		"wait too long": {wait(5001)}, "waits add up": {wait(5000), wait(5000), wait(1)}, "wait with selector": {{Do: "wait", Ms: 5, Selector: "a"}},
	}
	for name, s := range bad {
		if err := validateRecipe(s); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestCaptureNetworkHandlerRefusesABadRecipeBeforeAnyBrowser(t *testing.T) {
	withTestHosts(t)
	for name, body := range map[string]string{
		"unknown action": `{"url":"https://a.example/","steps":[{"do":"type","selector":"input"}]}`, "unknown step field": `{"url":"https://a.example/","steps":[{"do":"click","selector":"a","script":"x"}]}`,
		"too many":         `{"url":"https://a.example/","steps":[{"do":"wait","ms":1},{"do":"wait","ms":1},{"do":"wait","ms":1},{"do":"wait","ms":1},{"do":"wait","ms":1},{"do":"wait","ms":1},{"do":"wait","ms":1}]}`,
		"steps not a list": `{"url":"https://a.example/","steps":"click"}`, "unknown field": `{"url":"https://a.example/","recipe":[]}`,
	} {
		if rec := postJSON(CaptureNetworkHandler, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
}

// recipeMock is a stand-in for pages whose player starts only after something is pressed.
func recipeMock(t *testing.T) (port string) {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	p := func() string { return strings.Split(srv.Listener.Addr().String(), ":")[1] }
	hls := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		fmt.Fprint(w, "#EXTM3U\n")
	}
	for _, path := range []string{"/cross/live.m3u8", "/cover/live.m3u8", "/popup/live.m3u8", "/play/live.m3u8", "/late/live.m3u8"} {
		mux.HandleFunc(path, hls)
	}
	// a server tab that adds the player frame (another site); the stream is requested only by a press INSIDE that frame
	mux.HandleFunc("/press", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><button id="reveal" onclick="var f=document.createElement('iframe');f.id='pf';f.style.cssText='width:400px;height:200px;border:0';f.src='http://b.localhost:%s/startframe';document.body.appendChild(f)">Server 2</button>
			<div style="display:none"><button id="hid">hidden</button></div><p id="text">not a frame</p></body></html>`, p())
	})
	mux.HandleFunc("/startframe", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body style="margin:0"><div style="width:400px;height:200px;background:#ddd" onclick="fetch('/cross/live.m3u8').catch(function(){})">press me</div></body></html>`)
	})
	mux.HandleFunc("/cover", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body style="margin:0"><button id="under" style="position:absolute;left:20px;top:20px;width:100px;height:40px">Play</button>
			<div id="ad" style="position:absolute;left:0;top:0;width:300px;height:200px;background:rgba(0,0,0,.1)" onclick="fetch('/cover/live.m3u8').catch(function(){})"></div></body></html>`)
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><button id="go" onclick="location.href='/elsewhere'">Play</button><button id="later" onclick="fetch('/play/live.m3u8')">Later</button></body></html>`)
	})
	mux.HandleFunc("/elsewhere", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>an ad landing page</body></html>`)
	})
	mux.HandleFunc("/popup", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><button id="p" onclick="fetch('/popup/live.m3u8').catch(function(){}); window.open('/ad','_blank')">Play</button></body></html>`)
	})
	mux.HandleFunc("/ad", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `<html><body>ad</body></html>`) })
	// a button that is not there yet when the page has loaded (a script adds it after 0.8 s)
	mux.HandleFunc("/late", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><script>setTimeout(function(){var b=document.createElement('button');b.id='late';b.textContent='Play';b.onclick=function(){fetch('/late/live.m3u8').catch(function(){})};document.body.appendChild(b)},800)</script></body></html>`)
	})
	// the fixed play script finds this button; a recipe can ask for it NOT to run
	mux.HandleFunc("/vjs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><div class="vjs-big-play-button" style="width:80px;height:40px" onclick="fetch('/play/live.m3u8').catch(function(){})">Play</div></body></html>`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return p()
}

func hasManifest(res NetworkResult, part string) bool {
	for _, e := range res.Entries {
		if e.Kind == mediaHLS && strings.Contains(e.URL, part) {
			return true
		}
	}
	return false
}

func capture(t *testing.T, url string, req captureNetworkRequest) NetworkResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req.URL = url
	if req.WaitMs == 0 {
		req.WaitMs = 1500
	}
	res, err := captureNetwork(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestARecipeStartsAPlayerInsideAnotherSitesFrame(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	url := "http://a.localhost:" + port + "/press"
	if res := capture(t, url, captureNetworkRequest{Interaction: "play"}); hasManifest(res, "/cross/live.m3u8") || len(res.Steps) != 0 {
		t.Fatalf("without a recipe nothing is pressed: %+v", res)
	}
	res := capture(t, url, captureNetworkRequest{Steps: []RecipeStep{{Do: "click", Selector: "#reveal"}, {Do: "wait", Ms: 500}, {Do: "clickFrame", Selector: "iframe#pf"}}})
	if !hasManifest(res, "b.localhost:"+port+"/cross/live.m3u8") {
		t.Fatalf("the press inside the cross-site frame must start the stream: %+v", res)
	}
	want := []StepOutcome{{Index: 1, Do: "click", Status: "done"}, {Index: 2, Do: "wait", Status: "done"}, {Index: 3, Do: "clickFrame", Status: "done"}}
	if len(res.Steps) != 3 {
		t.Fatalf("%+v", res.Steps)
	}
	for i, w := range want {
		if res.Steps[i] != w {
			t.Errorf("step %d: %+v, want %+v", i+1, res.Steps[i], w)
		}
	}
	if res.Played {
		t.Error("the play script only runs when the interaction is play")
	}
}

func TestAStepThatCannotBePressedIsSkippedAndTheRestStillRun(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	res := capture(t, "http://a.localhost:"+port+"/press", captureNetworkRequest{Steps: []RecipeStep{
		{Do: "click", Selector: "#nothing-like-this"}, {Do: "click", Selector: "#hid"}, {Do: "click", Selector: "[[["}, {Do: "clickFrame", Selector: "#text"},
		{Do: "click", Selector: "#reveal"}, {Do: "wait", Ms: 500}, {Do: "clickFrame", Selector: "iframe#pf"}}})
	notes := []string{"nothing on the page matches", "not visible", "not valid", "not a frame (it is a p)"}
	for i, n := range notes {
		if res.Steps[i].Status != "skipped" || !strings.Contains(res.Steps[i].Note, n) {
			t.Errorf("step %d: %+v, want skipped with %q", i+1, res.Steps[i], n)
		}
	}
	if res.Steps[4].Status != "done" || res.Steps[6].Status != "done" || !hasManifest(res, "/cross/live.m3u8") {
		t.Errorf("the steps after the skipped ones still run: %+v", res)
	}
}

func TestAPressOnACoveredElementPressesWhatIsOnTopAndSaysSo(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	res := capture(t, "http://a.localhost:"+port+"/cover", captureNetworkRequest{Steps: []RecipeStep{{Do: "click", Selector: "#under"}}})
	if !hasManifest(res, "/cover/live.m3u8") || res.Steps[0].Status != "done" || !strings.Contains(res.Steps[0].Note, "covered") {
		t.Errorf("%+v", res)
	}
}

func TestAClickThatTakesThePageAwayStopsTheRecipeAndGoesBack(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	url := "http://a.localhost:" + port + "/redir"
	res := capture(t, url, captureNetworkRequest{Steps: []RecipeStep{{Do: "click", Selector: "#go"}, {Do: "click", Selector: "#later"}, {Do: "wait", Ms: 100}}})
	if len(res.Steps) != 3 || res.Steps[0].Status != "stopped" || !strings.Contains(res.Steps[0].Note, "somewhere else") || res.Steps[1].Status != "not run" || res.Steps[2].Status != "not run" {
		t.Fatalf("%+v", res.Steps)
	}
	for i, w := range []string{"click", "click", "wait"} {
		if res.Steps[i].Index != i+1 || res.Steps[i].Do != w {
			t.Errorf("every step keeps its number and kind, also when it was not run: %+v", res.Steps)
		}
	}
	if res.FinalURL != url {
		t.Errorf("the run goes back to where it was: %s", res.FinalURL)
	}
	if hasManifest(res, "/play/live.m3u8") {
		t.Error("a step after the stop must not run")
	}
}

func TestPopupsOpenedByAPressAreClosedAndCounted(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	res := capture(t, "http://a.localhost:"+port+"/popup", captureNetworkRequest{Steps: []RecipeStep{{Do: "click", Selector: "#p"}}})
	if !hasManifest(res, "/popup/live.m3u8") || res.Popups < 1 {
		t.Errorf("%+v", res)
	}
	if res.Steps[0].Status != "done" {
		t.Errorf("%+v", res.Steps)
	}
	total := 0
	for _, st := range res.Steps {
		total += st.Popups
	}
	if res.Steps[0].Popups < 1 || total != res.Popups {
		t.Errorf("the press that opened the tab says so, and the steps add up to the total (%d): %+v", res.Popups, res.Steps)
	}
}

func TestAStepThatOpenedNoTabSaysNothingAboutPopups(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	res := capture(t, "http://a.localhost:"+port+"/vjs", captureNetworkRequest{Steps: []RecipeStep{{Do: "wait", Ms: 50}}})
	if res.Steps[0].Popups != 0 || res.Popups != 0 {
		t.Errorf("%+v", res)
	}
	raw, err := json.Marshal(res.Steps[0])
	if err != nil || strings.Contains(string(raw), "popups") {
		t.Errorf("zero is left out of the answer: %s %v", raw, err)
	}
}

func TestThePlayScriptStillRunsAfterTheStepsUnlessToldNotTo(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	url := "http://a.localhost:" + port + "/vjs"
	steps := []RecipeStep{{Do: "wait", Ms: 100}}
	res := capture(t, url, captureNetworkRequest{Interaction: "play", Steps: steps})
	if !hasManifest(res, "/play/live.m3u8") || !res.Played {
		t.Errorf("by default the play script follows the steps: %+v", res)
	}
	no := false
	res = capture(t, url, captureNetworkRequest{Interaction: "play", Steps: steps, PlayAfter: &no})
	if hasManifest(res, "/play/live.m3u8") || res.Played {
		t.Errorf("playAfter false: %+v", res)
	}
}

func TestAnElementThatAppearsAMomentLaterIsFoundByTheRetry(t *testing.T) {
	requireRodChrome(t)
	port := recipeMock(t)
	res := capture(t, "http://a.localhost:"+port+"/late", captureNetworkRequest{WaitMs: 800, Steps: []RecipeStep{{Do: "click", Selector: "#late"}}})
	if res.Steps[0].Status != "done" || !hasManifest(res, "/late/live.m3u8") {
		t.Errorf("a press may find its element after a short wait: %+v", res)
	}
}
