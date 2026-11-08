package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"browser-tool-backend/shared"
)

// fakeLoginWindows stands in for auth's registry of login windows.
type fakeLoginWindows struct {
	err      error
	acquires atomic.Int32
	lost     []string
	// ai: the windows a person opened for an AI conversation, by id (plan 183); asked: the ids AcquireTab was asked for.
	ai    map[string]shared.AIWindowInfo
	mu    sync.Mutex
	asked []string
}

func (f *fakeLoginWindows) AIWindow(id string) shared.AIWindowInfo { return f.ai[id] }

func (f *fakeLoginWindows) AcquireTab(ctx context.Context, id string) (context.Context, func(), error) {
	f.mu.Lock()
	f.asked = append(f.asked, id)
	f.mu.Unlock()
	f.acquires.Add(1)
	if f.err != nil {
		return nil, nil, f.err
	}
	return ctx, func() {}, nil
}
func (f *fakeLoginWindows) LoginLost(id string) { f.lost = append(f.lost, id) }

func useLoginWindows(t *testing.T, access shared.LoginWindowAccess) {
	t.Helper()
	// PaginatedCrawlHandler reads the stored settings before it looks at the flag.
	root := t.TempDir()
	t.Setenv("TOOL_DB_DIR", filepath.Join(root, "db"))
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := shared.InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { shared.DB.Close() })
	old := shared.LoginWindows
	shared.LoginWindows = access
	t.Cleanup(func() { shared.LoginWindows = old })
}

type loginWindowCase struct {
	name    string
	handler http.HandlerFunc
	body    func(extra string) string
}

func loginWindowCases() []loginWindowCase {
	return []loginWindowCase{
		{"crawl", CrawlHandler, func(extra string) string {
			return `{"url":"https://example.com/jobs","useLoginWindow":true` + extra + `}`
		}},
		{"crawl-paginated", PaginatedCrawlHandler, func(extra string) string {
			return `{"url":"https://example.com/jobs","useLoginWindow":true,"fields":[{"label":"t","selector":"h2"}],"requestedMaxPages":1,"effectiveMaxPages":1` + extra + `}`
		}},
		{"extract", ExtractHandler, func(extra string) string {
			return `{"useLoginWindow":true,"fields":[{"label":"t","selector":"h2"}]` + extra + `}`
		}},
	}
}

func post(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
	return rec
}

func TestAnyLoginWindowRequestWithoutARunIdIsRefusedBeforeAnythingElse(t *testing.T) {
	f := &fakeLoginWindows{}
	useLoginWindows(t, f)
	for _, c := range loginWindowCases() {
		rec := post(c.handler, c.body(""))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requestId is required with useLoginWindow") {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if f.acquires.Load() != 0 {
		t.Errorf("a refused request asked for a window")
	}
}

func TestTheLoginWindowHasNoOffsetWindowing(t *testing.T) {
	useLoginWindows(t, &fakeLoginWindows{})
	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","useLoginWindow":true,"requestId":"r","offset":0}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "offset is not supported") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestARunWithoutAConfirmedWindowIsAnsweredLoginRequiredAtOnceByEveryRoute(t *testing.T) {
	for _, reason := range []string{"no_window", "waiting", "ended", "login_lost"} {
		for _, c := range loginWindowCases() {
			f := &fakeLoginWindows{err: &shared.LoginRequiredError{Reason: reason}}
			useLoginWindows(t, f)
			id := "run-" + c.name + "-" + reason
			rec := post(c.handler, c.body(`,"requestId":"`+id+`"`))
			if rec.Code != http.StatusConflict {
				t.Errorf("%s/%s: status %d, want 409: %s", c.name, reason, rec.Code, rec.Body.String())
				continue
			}
			var got struct{ Code, Message, Reason string }
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Errorf("%s/%s: %v %s", c.name, reason, err, rec.Body.String())
				continue
			}
			if got.Code != "login_required" || got.Reason != reason || !strings.Contains(got.Message, "login") {
				t.Errorf("%s/%s: %+v", c.name, reason, got)
			}
			if entry, ok := getCrawlStatus(id); !ok || entry.Phase != phaseWaitingForLogin {
				t.Errorf("%s/%s: the live phase is %+v, want waiting_for_login", c.name, reason, entry)
			}
			if f.acquires.Load() != 1 {
				t.Errorf("%s/%s: asked for the window %d times", c.name, reason, f.acquires.Load())
			}
		}
	}
}

func TestWithoutAnyLoginWindowRegistryEveryRunNeedsALogin(t *testing.T) {
	useLoginWindows(t, nil)
	for _, c := range loginWindowCases() {
		rec := post(c.handler, c.body(`,"requestId":"nil-`+c.name+`"`))
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"login_required"`) || !strings.Contains(rec.Body.String(), "no_window") {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
}

func TestAnotherFailureOfTheWindowIsABadGatewayAndTheRunFails(t *testing.T) {
	useLoginWindows(t, &fakeLoginWindows{err: context.DeadlineExceeded})
	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","useLoginWindow":true,"requestId":"boom"}`)
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "login_required") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if entry, _ := getCrawlStatus("boom"); entry.Phase != phaseFailed {
		t.Errorf("phase %+v", entry)
	}
}

func TestACallerThatWentAwayWhileWaitingForTheWindowIsReportedAsCancelled(t *testing.T) {
	useLoginWindows(t, &fakeLoginWindows{err: context.Canceled})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"url":"https://example.com/jobs","useLoginWindow":true,"requestId":"gone"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	CrawlHandler(rec, req)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "crawl_cancelled_by_user") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// Without the flag nothing changes: the headless path is used and the login windows are never asked.
func TestWithoutTheFlagTheLoginWindowsAreNeverTouched(t *testing.T) {
	f := &fakeLoginWindows{}
	useLoginWindows(t, f)
	old := crawlPageFn
	called := 0
	crawlPageFn = func(ctx context.Context, rawURL, requestID string, expected, remove, removeAttr []string, maxLen int, ignore []string) (crawlResponse, error) {
		called++
		return crawlResponse{HTML: "<p>ok</p>", Title: "t", FinalURL: rawURL}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	rec := post(CrawlHandler, `{"url":"https://example.com/jobs","requestId":"plain"}`)
	if rec.Code != 200 || called != 1 {
		t.Errorf("%d called=%d %s", rec.Code, called, rec.Body.String())
	}
	if f.acquires.Load() != 0 {
		t.Error("a request without useLoginWindow asked for a login window")
	}
}

func TestIsLoginRequiredOnlyRecognisesTheLoginRequiredError(t *testing.T) {
	if !isLoginRequired(newLoginRequiredError("waiting")) {
		t.Error("the login required error is not recognised")
	}
	for _, err := range []error{nil, context.Canceled, newCrawlCancelledError(), &crawlError{Code: codeBotCheckInWindow}} {
		if isLoginRequired(err) {
			t.Errorf("%v was taken for login required", err)
		}
	}
}

// The AI cannot reach a login window: none of the MCP tools' arguments carries the flag, the run id or anything that names a window, so
// whatever a model sends is never read as one (the routes are human-only, like the credential store).
func TestNoMCPToolCanAskForTheLoginWindow(t *testing.T) {
	for name, v := range map[string]any{
		"fetch_page_html":   fetchPageHTMLArgs{},
		"extract_page_data": extractPageDataArgs{},
		"extract_from_url":  extractFromURLArgs{},
		"find_repeating":    findRepeatingItemsArgs{},
		"crawl_paginated":   crawlPaginatedArgs{},
	} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := strings.ToLower(f.Tag.Get("json"))
			lower := strings.ToLower(f.Name)
			if strings.Contains(lower, "loginwindow") || strings.Contains(tag, "loginwindow") || strings.Contains(tag, "requestid") ||
				strings.Contains(lower, "aicontext") || strings.Contains(tag, "aicontext") || strings.Contains(tag, "conversation") {
				t.Errorf("%s has an argument (%s) that could name a login window", name, f.Name)
			}
		}
	}
}
