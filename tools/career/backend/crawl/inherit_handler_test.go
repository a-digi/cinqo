package crawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/portal"
)

type inheritResp struct {
	Inherited bool `json:"inherited"`
	Listing   bool `json:"listing"`
	JobDetail bool `json:"jobDetail"`
	From      *struct {
		LinkID string `json:"linkId"`
		Title  string `json:"title"`
	} `json:"from"`
	Tried   []CandidateOutcome `json:"tried"`
	Portals []struct {
		ID    string `json:"id"`
		Links []struct {
			ID                         string `json:"id"`
			CrawlInstructions          string `json:"crawlInstructions"`
			JobDetailCrawlInstructions string `json:"jobDetailCrawlInstructions"`
		} `json:"links"`
	} `json:"portals"`
}

func postInherit(ctx context.Context, body string, withCookie bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/portal-links/inherit-instructions", strings.NewReader(body)).WithContext(ctx)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	}
	rec := httptest.NewRecorder()
	InheritInstructionsHandler(rec, req)
	return rec
}

func decodeInherit(t *testing.T, rec *httptest.ResponseRecorder) inheritResp {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var r inheritResp
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func stored(t *testing.T, id string) (listing, detail string) {
	t.Helper()
	l, err := portal.GetPortalLinkCrawlInstructions(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	if err != nil {
		t.Fatal(err)
	}
	return l, d
}

func TestInheritHandler_SavesBothAndPublishesTheEvent(t *testing.T) {
	setupTrialDB(t)
	t.Setenv("TOOL_SERVICE_TOKEN", "svc")
	_, newLink, sib := trialPortal(t, [2]string{trialListingA, trialDetail})
	long := strings.Repeat("d", 150)
	fb := startBrowser(t, func(c browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, listPage(5))
			return
		}
		writeJSON(w, detailPage(long))
	})

	rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true)
	got := decodeInherit(t, rec)
	if !got.Inherited || !got.Listing || !got.JobDetail || got.From == nil || got.From.LinkID != sib[0] || got.From.Title != "Sibling A" {
		t.Fatalf("response = %+v", got)
	}
	if len(got.Tried) != 1 || got.Tried[0].Listing != ListingOK || got.Tried[0].JobDetail != portal.DetailOK {
		t.Fatalf("tried = %+v", got.Tried)
	}
	if l, d := stored(t, newLink); l != trialListingA || d != trialDetail {
		t.Fatalf("stored %q / %q", l, d)
	}
	// The refreshed list carries the new instructions.
	var seen bool
	for _, p := range got.Portals {
		for _, l := range p.Links {
			if l.ID == newLink && l.CrawlInstructions == trialListingA && l.JobDetailCrawlInstructions == trialDetail {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("the portals in the response do not show the saved instructions")
	}
	if ev := fb.published(); len(ev) != 1 || ev[0] != "career.portal_link.listing_instructions_ready:"+newLink {
		t.Fatalf("events = %v", ev)
	}
	// The sibling is untouched.
	if l, _ := stored(t, sib[0]); l != trialListingA {
		t.Fatal("the sibling's instructions changed")
	}
}

func TestInheritHandler_ListingOnlyWhenDetailsDoNotWork(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, trialDetail})
	startBrowser(t, func(c browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if c.Container == ".job" {
			writeJSON(w, listPage(5))
			return
		}
		writeJSON(w, detailPage("short"))
	})
	got := decodeInherit(t, postInherit(context.Background(), `{"id":"`+newLink+`"}`, true))
	if !got.Inherited || !got.Listing || got.JobDetail {
		t.Fatalf("response = %+v", got)
	}
	if l, d := stored(t, newLink); l != trialListingA || d != "" {
		t.Fatalf("stored %q / %q", l, d)
	}
	if got.Tried[0].JobDetail != portal.DetailFailed {
		t.Fatalf("tried = %+v", got.Tried)
	}
}

func TestInheritHandler_NothingWorksSavesNothing(t *testing.T) {
	setupTrialDB(t)
	t.Setenv("TOOL_SERVICE_TOKEN", "svc")
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, trialDetail})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"pages": []any{map[string]any{"url": "l"}}})
	})
	got := decodeInherit(t, postInherit(context.Background(), `{"id":"`+newLink+`"}`, true))
	if got.Inherited || got.Listing || got.JobDetail || got.From != nil {
		t.Fatalf("response = %+v", got)
	}
	if len(got.Tried) != 1 || got.Tried[0].Listing != ListingFailed || got.Tried[0].Reason != "no items found" {
		t.Fatalf("tried = %+v", got.Tried)
	}
	if l, d := stored(t, newLink); l != "" || d != "" {
		t.Fatalf("stored %q / %q", l, d)
	}
	if len(fb.published()) != 0 {
		t.Fatalf("no event may be published: %v", fb.published())
	}
}

func TestInheritHandler_NoCandidatesGivesAnEmptyTriedList(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t)
	startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })
	rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// "tried" is an empty list, not null or missing (a frontend must never meet an absent field).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["tried"]) != "[]" || string(raw["inherited"]) != "false" {
		t.Fatalf("tried=%s inherited=%s", raw["tried"], raw["inherited"])
	}
	if _, ok := raw["from"]; ok {
		t.Fatal("from must be absent when nothing was inherited")
	}
}

func TestInheritHandler_Guards(t *testing.T) {
	setupTrialDB(t)
	_, newLink, sib := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) { writeJSON(w, listPage(5)) })

	cases := []struct {
		name   string
		method string
		body   string
		cookie bool
		want   int
	}{
		{"GET is not allowed", http.MethodGet, "", true, http.StatusMethodNotAllowed},
		{"missing id", http.MethodPost, `{}`, true, http.StatusBadRequest},
		{"not json", http.MethodPost, `nope`, true, http.StatusBadRequest},
		{"no session cookie", http.MethodPost, `{"id":"` + newLink + `"}`, false, http.StatusUnauthorized},
		{"unknown link", http.MethodPost, `{"id":"nope"}`, true, http.StatusBadRequest},
		{"the link already has instructions", http.MethodPost, `{"id":"` + sib[0] + `"}`, true, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "/portal-links/inherit-instructions", strings.NewReader(c.body))
			if c.cookie {
				req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
			}
			rec := httptest.NewRecorder()
			InheritInstructionsHandler(rec, req)
			if rec.Code != c.want {
				t.Fatalf("%d %s, want %d", rec.Code, rec.Body.String(), c.want)
			}
		})
	}
	if len(fb.seen()) != 0 {
		t.Fatalf("a refused request reached the browser: %+v", fb.seen())
	}
	if l, _ := stored(t, sib[0]); l != trialListingA {
		t.Fatal("a refused request changed the sibling")
	}
}

func TestInheritHandler_SecondRequestForTheSameLinkIsRefusedWhileOneRuns(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	inBrowser := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	startBrowser(t, func(_ browserCall, n int, w http.ResponseWriter, r *http.Request) {
		if n == 1 {
			close(inBrowser)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		writeJSON(w, listPage(5))
	})
	// Registered after the server's own cleanup, so it runs first: a failing test must
	// never leave the held request waiting, or the server could not shut down.
	t.Cleanup(releaseNow)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- postInherit(context.Background(), `{"id":"`+newLink+`"}`, true) }()
	select {
	case <-inBrowser:
	case <-time.After(5 * time.Second):
		t.Fatal("the first request never reached the browser")
	}
	if rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true); rec.Code != http.StatusConflict {
		t.Fatalf("second request: %d %s", rec.Code, rec.Body.String())
	}
	releaseNow()
	first := <-done
	if !decodeInherit(t, first).Inherited {
		t.Fatal("the first request must still succeed")
	}
	// Once it is over, the link is free again (here it now has instructions, so a new request is a 400, not a 409).
	if rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("after: %d %s", rec.Code, rec.Body.String())
	}
}

func TestInheritHandler_TimeoutIs504AndSavesNothing(t *testing.T) {
	setupTrialDB(t)
	old := inheritTrialTimeout
	inheritTrialTimeout = 50 * time.Millisecond
	t.Cleanup(func() { inheritTrialTimeout = old })
	t.Setenv("TOOL_SERVICE_TOKEN", "svc")
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, _ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l, d := stored(t, newLink); l != "" || d != "" || len(fb.published()) != 0 {
		t.Fatalf("a timed-out trial left %q / %q / %v", l, d, fb.published())
	}
	// The link is free again after a timeout.
	if !startInherit(newLink) {
		t.Fatal("the in-flight mark was not released")
	}
	endInherit(newLink)
}

// If the link gets instructions of its own while the trial runs, nothing is overwritten.
func TestInheritHandler_InstructionsAddedMeanwhileAreKept(t *testing.T) {
	setupTrialDB(t)
	t.Setenv("TOOL_SERVICE_TOKEN", "svc")
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	fb := startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, _ *http.Request) {
		if err := portal.UpdatePortalLinkCrawlInstructions(newLink, trialListingB); err != nil {
			t.Error(err)
		}
		writeJSON(w, listPage(5))
	})
	rec := postInherit(context.Background(), `{"id":"`+newLink+`"}`, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l, _ := stored(t, newLink); l != trialListingB {
		t.Fatalf("the user's own instructions were overwritten: %q", l)
	}
	if len(fb.published()) != 0 {
		t.Fatalf("no event may be published: %v", fb.published())
	}
}

func TestInheritHandler_CallerGoneSavesNothing(t *testing.T) {
	setupTrialDB(t)
	_, newLink, _ := trialPortal(t, [2]string{trialListingA, ""})
	ctx, cancel := context.WithCancel(context.Background())
	startBrowser(t, func(_ browserCall, _ int, w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	postInherit(ctx, `{"id":"`+newLink+`"}`, true)
	if l, d := stored(t, newLink); l != "" || d != "" {
		t.Fatalf("stored %q / %q", l, d)
	}
}
