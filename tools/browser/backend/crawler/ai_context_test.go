package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"browser-tool-backend/shared"
)

// Plan 183, step 1: where an AI tool call runs. A request carries "aiContextId" only because shared.CallSibling put the conversation id there;
// when a person opened a login window for that conversation (aiMayUse) the call runs in it, on the same site only.

const aiID = "conv-1234"

func aiWindows(domain string, mayUse bool) *fakeLoginWindows {
	return &fakeLoginWindows{
		err: &shared.LoginRequiredError{Reason: "waiting"}, // the window is not confirmed: the answer shows the call WAS routed to it
		ai:  map[string]shared.AIWindowInfo{aiID: {Exists: true, AIMayUse: mayUse, Domain: domain}},
	}
}

func aiCases() []loginWindowCase {
	return []loginWindowCase{
		{"crawl", CrawlHandler, func(extra string) string {
			return `{"url":"https://example.com/jobs"` + extra + `}`
		}},
		{"crawl-paginated", PaginatedCrawlHandler, func(extra string) string {
			return `{"url":"https://example.com/jobs","fields":[{"label":"t","selector":"h2"}],"requestedMaxPages":1,"effectiveMaxPages":1` + extra + `}`
		}},
		{"extract", ExtractHandler, func(extra string) string {
			return `{"fields":[{"label":"t","selector":"h2"}]` + extra + `}`
		}},
		{"find-repeating-items", FindRepeatingItemsHandler, func(extra string) string {
			return `{"url":"https://example.com/jobs"` + extra + `}`
		}},
	}
}

func TestAnAICallWithAWindowOpenedForItsConversationRunsInThatWindow(t *testing.T) {
	for _, c := range aiCases() {
		f := aiWindows("example.com", true)
		useLoginWindows(t, f)
		rec := post(c.handler, c.body(`,"aiContextId":"`+aiID+`"`))
		// The window is not confirmed: login_required (409) proves the call took the window path and asked for THIS conversation's window.
		var got struct{ Code, Reason, Advice string }
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusConflict || got.Code != "login_required" || got.Reason != "waiting" {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
		// The AI is told to hand over to the person, not to retry in a loop (the text reaches the model through CallSibling).
		if !strings.Contains(got.Advice, "tell them") || !strings.Contains(got.Advice, "I am logged in") || !strings.Contains(got.Advice, "Do not call") {
			t.Errorf("%s: the advice for the AI is %q", c.name, got.Advice)
		}
		if len(f.asked) != 1 || f.asked[0] != aiID {
			t.Errorf("%s: the window asked for was %v, want [%s]", c.name, f.asked, aiID)
		}
	}
}

func TestAnAICallWithNoWindowForItsConversationStaysOnTheHeadlessTab(t *testing.T) {
	f := &fakeLoginWindows{err: &shared.LoginRequiredError{Reason: "waiting"}}
	useLoginWindows(t, f)
	called := false
	old := crawlPageFn
	crawlPageFn = func(ctx context.Context, url, requestID string, _ []string, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		called = true
		return crawlResponse{HTML: "<p>public</p>", Title: "t", FinalURL: url}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("%d %s (headless called: %v)", rec.Code, rec.Body.String(), called)
	}
	if f.acquires.Load() != 0 {
		t.Error("a window was asked for although none was opened for this conversation")
	}
}

func TestAWindowNotOpenedForTheAIIsNeverUsedByAnAICall(t *testing.T) {
	// On the decision itself: nothing is changed (a run's window under the same id is not an AI window).
	useLoginWindows(t, aiWindows("example.com", false))
	requestID, flag := "", false
	if viaAI, refusal := applyAIContext(aiID, "https://example.com/jobs", &requestID, &flag); viaAI || refusal != nil || requestID != "" || flag {
		t.Errorf("viaAI=%v refusal=%v requestID=%q flag=%v", viaAI, refusal, requestID, flag)
	}
	// And through the one route that can be run without a browser: the headless fetch is what answers, no window is asked for.
	f := aiWindows("example.com", false)
	useLoginWindows(t, f)
	old := crawlPageFn
	crawlPageFn = func(ctx context.Context, url, requestID string, _ []string, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		return crawlResponse{HTML: "<p>public</p>", FinalURL: url}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	if rec := post(CrawlHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`); rec.Code != http.StatusOK {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if f.acquires.Load() != 0 {
		t.Error("an AI call used a window that was not opened for it")
	}
}

func TestAnAICallForAnotherSiteThanTheWindowIsRefusedNotSentToTheHeadlessTab(t *testing.T) {
	for _, c := range aiCases() {
		if c.name == "extract" {
			continue // it works on the page already in the window and takes no url
		}
		f := aiWindows("www.linkedin.com", true)
		useLoginWindows(t, f)
		rec := post(c.handler, c.body(`,"aiContextId":"`+aiID+`"`)) // the url is example.com
		var got struct{ Code, Message, Reason string }
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusConflict || got.Code != "login_window_other_site" || got.Reason != "www.linkedin.com" || !strings.Contains(got.Message, "another site") {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
		if f.acquires.Load() != 0 {
			t.Errorf("%s: a refused call asked for the window", c.name)
		}
	}
}

func TestTheSiteCheckIgnoresCaseAndNeedsTheSameHost(t *testing.T) {
	for url, ok := range map[string]bool{
		"https://WWW.LinkedIn.com/jobs/": true, "https://www.linkedin.com/jobs/search/?q=go": true, "http://www.linkedin.com:8080/x": true,
		"https://linkedin.com/jobs/": false, "https://evil.example/www.linkedin.com": false, "https://www.linkedin.com.evil.example/": false, "not a url ://": false,
		"https://evilwww.linkedin.com/": false, "https://x.www.linkedin.com/": false,
	} {
		f := aiWindows("www.linkedin.com", true)
		useLoginWindows(t, f)
		rec := post(CrawlHandler, `{"url":`+jsonString(url)+`,"aiContextId":"`+aiID+`"}`)
		routed := f.acquires.Load() == 1
		if rec.Code == http.StatusBadRequest {
			continue // an address the SSRF guard or the parser refuses earlier: nothing reached a window either
		}
		if routed != ok {
			t.Errorf("%s: routed to the window = %v, want %v (%d %s)", url, routed, ok, rec.Code, rec.Body.String())
		}
	}
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestAnExplicitWindowOrRunWinsOverTheAIContext(t *testing.T) {
	// A crawl run's own call (useLoginWindow + its requestId) keeps its window even when an aiContextId is present.
	f := aiWindows("example.com", true)
	useLoginWindows(t, f)
	post(CrawlHandler, `{"url":"https://example.com/jobs","useLoginWindow":true,"requestId":"run-7","aiContextId":"`+aiID+`"}`)
	if len(f.asked) != 1 || f.asked[0] != "run-7" {
		t.Errorf("asked %v, want [run-7]", f.asked)
	}
	// A call that only has a requestId (Career's own phase tracking) is not an AI call.
	f2 := aiWindows("example.com", true)
	useLoginWindows(t, f2)
	old := crawlPageFn
	crawlPageFn = func(ctx context.Context, url, requestID string, _ []string, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		return crawlResponse{HTML: "x", FinalURL: url}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	if rec := post(CrawlHandler, `{"url":"https://example.com/jobs","requestId":"plain","aiContextId":"`+aiID+`"}`); rec.Code != http.StatusOK {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if f2.acquires.Load() != 0 {
		t.Error("a request with its own requestId was taken for an AI call")
	}
}

func TestWithoutAnyWindowRegistryAnAICallIsUnchanged(t *testing.T) {
	useLoginWindows(t, nil)
	old := crawlPageFn
	crawlPageFn = func(ctx context.Context, url, requestID string, _ []string, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		return crawlResponse{HTML: "x", FinalURL: url}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	if rec := post(CrawlHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`); rec.Code != http.StatusOK {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// The AI reads a long page in windows (offset). In its login window the page is fetched and cut the same way; an explicit run call still has no offset.
func TestAnAICallInTheWindowCanReadALongPageWithAnOffsetButARunCannot(t *testing.T) {
	f := &fakeLoginWindows{ai: map[string]shared.AIWindowInfo{aiID: {Exists: true, AIMayUse: true, Domain: "example.com"}}}
	useLoginWindows(t, f)
	html := strings.Repeat("a", 50)
	old := crawlInLoginWindowFn
	var seen crawlRequest
	crawlInLoginWindowFn = func(ctx context.Context, body crawlRequest) (crawlResponse, error) {
		seen = body
		return crawlResponse{HTML: html, Title: "t", FinalURL: body.URL}, nil
	}
	t.Cleanup(func() { crawlInLoginWindowFn = old })

	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","offset":10,"aiContextId":"`+aiID+`"}`)
	var got struct {
		HTML   string
		Window *struct{ Start, End, Total int }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got.Window == nil || got.Window.Start != 10 || got.Window.Total != 50 || len(got.HTML) != got.Window.End-10 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if seen.RequestID != aiID || !seen.UseLoginWindow {
		t.Errorf("the window call got %+v", seen)
	}
	// the explicit run call with an offset is still refused
	if rec := post(CrawlHandler, `{"url":"https://example.com/jobs","useLoginWindow":true,"requestId":"run-1","offset":0}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a run call with an offset: %d %s", rec.Code, rec.Body.String())
	}
}

// A caller that names a window but gives no id made a mistake: it is told so, never taken for an AI call because an aiContextId is present.
func TestAnExplicitWindowFlagWithoutAnIdIsAMistakeEvenWithAnAIContext(t *testing.T) {
	f := aiWindows("example.com", true)
	useLoginWindows(t, f)
	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","useLoginWindow":true,"aiContextId":"`+aiID+`"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requestId is required with useLoginWindow") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if f.acquires.Load() != 0 {
		t.Error("a window was asked for")
	}
}

// The host rule itself, on the pure function (no address guard or DNS in the way): the window's host and no other.
func TestApplyAIContextAcceptsTheWindowsHostAndNoOtherHost(t *testing.T) {
	for rawURL, wantRouted := range map[string]bool{
		"https://www.linkedin.com/jobs/":            true,
		"https://WWW.LINKEDIN.COM/jobs/search/?q=g": true,
		"http://www.linkedin.com:8080/x":            true,
		"":                                          true, // a call that works on the page already in the window
		"https://linkedin.com/jobs/":                false,
		"https://evilwww.linkedin.com/":             false,
		"https://x.www.linkedin.com/":               false,
		"https://www.linkedin.com.evil.example/":    false,
		"https://evil.example/www.linkedin.com":     false,
		"https://example.com/":                      false,
		"://bad":                                    false,
	} {
		useLoginWindows(t, aiWindows("www.linkedin.com", true))
		requestID, flag := "", false
		viaAI, refusal := applyAIContext(aiID, rawURL, &requestID, &flag)
		if wantRouted {
			if !viaAI || refusal != nil || requestID != aiID || !flag {
				t.Errorf("%q: viaAI=%v refusal=%v requestID=%q flag=%v", rawURL, viaAI, refusal, requestID, flag)
			}
		} else if viaAI || refusal == nil || refusal.Code != "login_window_other_site" || requestID != "" || flag {
			t.Errorf("%q: viaAI=%v refusal=%v requestID=%q flag=%v", rawURL, viaAI, refusal, requestID, flag)
		}
	}
}

// ---- plan 183, step 2: find_repeating_items in the window ----

// stubWindowSteps scripts the three browser steps of the find-repeating window path and records their order.
func stubWindowSteps(t *testing.T, open, check func() error, analyze func() (findRepeatingResponse, error)) *[]string {
	t.Helper()
	var order []string
	oo, oc, oa := openPageInLoginWindowFn, checkLoginWindowPageFn, analyzeCurrentPageFn
	openPageInLoginWindowFn = func(ctx context.Context, requestID, rawURL string) error {
		order = append(order, "open:"+requestID+":"+rawURL)
		return open()
	}
	checkLoginWindowPageFn = func(ctx context.Context, requestID string) error {
		order = append(order, "check:"+requestID)
		return check()
	}
	analyzeCurrentPageFn = func(ctx context.Context, minItems, maxCandidates int) (findRepeatingResponse, error) {
		order = append(order, "analyze")
		return analyze()
	}
	t.Cleanup(func() { openPageInLoginWindowFn, checkLoginWindowPageFn, analyzeCurrentPageFn = oo, oc, oa })
	return &order
}

func confirmedAIWindow() *fakeLoginWindows {
	return &fakeLoginWindows{ai: map[string]shared.AIWindowInfo{aiID: {Exists: true, AIMayUse: true, Domain: "example.com"}}}
}

func TestFindRepeatingItemsInTheWindowOpensChecksThenAnalysesAndAnswers(t *testing.T) {
	f := confirmedAIWindow()
	useLoginWindows(t, f)
	order := stubWindowSteps(t,
		func() error { return nil }, func() error { return nil },
		func() (findRepeatingResponse, error) {
			return findRepeatingResponse{PageTitle: "Jobs", FinalURL: "https://example.com/jobs", Candidates: []RepeatingCandidate{{Selector: ".job", Count: 7}}}, nil
		})
	rec := post(FindRepeatingItemsHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`)
	var got struct {
		PageTitle  string
		Candidates []struct {
			Selector string
			Count    int
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got.PageTitle != "Jobs" || len(got.Candidates) != 1 || got.Candidates[0].Selector != ".job" || got.Candidates[0].Count != 7 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	want := []string{"open:" + aiID + ":https://example.com/jobs", "check:" + aiID, "analyze"}
	if strings.Join(*order, ",") != strings.Join(want, ",") {
		t.Errorf("steps %v, want %v", *order, want)
	}
	if len(f.asked) != 1 || f.asked[0] != aiID {
		t.Errorf("the window asked for was %v", f.asked)
	}
	if entry, ok := getCrawlStatus(aiID); !ok || entry.Phase != phaseCompleted {
		t.Errorf("the phase is %+v", entry)
	}
}

func TestAFindRepeatingCallThatLandsOnTheLoginFormAnswersLoginRequiredAndAnalysesNothing(t *testing.T) {
	f := confirmedAIWindow()
	useLoginWindows(t, f)
	order := stubWindowSteps(t,
		func() error { return nil },
		func() error { return newLoginRequiredError("login_lost") },
		func() (findRepeatingResponse, error) {
			t.Error("the login form was analysed")
			return findRepeatingResponse{}, nil
		})
	rec := post(FindRepeatingItemsHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`)
	var got struct{ Code, Reason, Advice string }
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusConflict || got.Code != "login_required" || got.Reason != "login_lost" || !strings.Contains(got.Advice, "tell them") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(*order) != 2 {
		t.Errorf("steps %v", *order)
	}
	if entry, ok := getCrawlStatus(aiID); !ok || entry.Phase != phaseWaitingForLogin {
		t.Errorf("the phase is %+v, want waiting_for_login", entry)
	}
}

func TestAFindRepeatingCallInTheWindowThatFailsOtherwiseIsABadGateway(t *testing.T) {
	useLoginWindows(t, confirmedAIWindow())
	stubWindowSteps(t,
		func() error { return nil }, func() error { return nil },
		func() (findRepeatingResponse, error) { return findRepeatingResponse{}, errors.New("script failed") })
	rec := post(FindRepeatingItemsHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "login_required") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	useLoginWindows(t, confirmedAIWindow())
	stubWindowSteps(t,
		func() error { return errors.New("navigation failed") }, func() error { return nil },
		func() (findRepeatingResponse, error) {
			t.Error("analysed after a failed navigation")
			return findRepeatingResponse{}, nil
		})
	if rec := post(FindRepeatingItemsHandler, `{"url":"https://example.com/jobs","aiContextId":"`+aiID+`"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// Without a window of its own the call is never routed to one (the headless path is the real browser, so it is not run here: the decision is).
func TestFindRepeatingItemsWithoutAnAIWindowIsNotRouted(t *testing.T) {
	for _, f := range []*fakeLoginWindows{{}, aiWindows("example.com", false)} {
		useLoginWindows(t, f)
		requestID, flag := "", false
		if viaAI, refusal := applyAIContext(aiID, "https://example.com/jobs", &requestID, &flag); viaAI || refusal != nil || flag {
			t.Errorf("routed: viaAI=%v refusal=%v flag=%v", viaAI, refusal, flag)
		}
	}
}
