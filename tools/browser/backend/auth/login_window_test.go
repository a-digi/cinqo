package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

// fakeWindow stands in for a real Chrome window.
type fakeWindow struct {
	startURL string
	mu       sync.Mutex
	checked  []string
	page     pageInspection
	err      error
	gate     chan struct{} // when set, Inspect waits for it
	closes   atomic.Int32
	done     chan struct{}
	doneOnce sync.Once
	// holdDone: Close does not signal Done (a window whose teardown is slow), so the person-closes signal can be sent later by the test.
	holdDone atomic.Bool
	// cookies the window "has" (plan 195) and the addresses they were asked for.
	cookies    []shared.SessionCookie
	cookiesErr error
	cookieURLs []string
}

func (f *fakeWindow) Inspect(ctx context.Context, checkURL string) (pageInspection, error) {
	f.mu.Lock()
	f.checked = append(f.checked, checkURL)
	page, err, gate := f.page, f.err, f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return pageInspection{}, ctx.Err()
		}
	}
	return page, err
}
func (f *fakeWindow) Done() <-chan struct{}       { return f.done }
func (f *fakeWindow) TabContext() context.Context { return context.Background() }
func (f *fakeWindow) Close() {
	f.closes.Add(1)
	if !f.holdDone.Load() {
		f.doneOnce.Do(func() { close(f.done) })
	}
}
func (f *fakeWindow) Cookies(ctx context.Context, pageURL string) ([]shared.SessionCookie, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cookieURLs = append(f.cookieURLs, pageURL)
	return f.cookies, f.cookiesErr
}
func (f *fakeWindow) personCloses() { f.doneOnce.Do(func() { close(f.done) }) }
func (f *fakeWindow) setPage(p pageInspection) {
	f.mu.Lock()
	f.page = p
	f.mu.Unlock()
}
func (f *fakeWindow) checkedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.checked...)
}

// useFakes replaces Chrome, resets the manager and returns the windows opened so far.
type fakes struct {
	mu      sync.Mutex
	opened  []*fakeWindow
	openErr error
	block   chan struct{} // when set, opening waits for it
	page    pageInspection
}

func (f *fakes) all() []*fakeWindow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeWindow(nil), f.opened...)
}

func useFakes(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{page: pageInspection{TextLength: 500}}
	oldOpen, oldWait, oldLife, oldKeep := openLoginWindow, loginWindowMaxWait, loginWindowMaxLife, endedKeep
	openLoginWindow = func(start string) (loginWindowHandle, error) {
		f.mu.Lock()
		block, err, page := f.block, f.openErr, f.page
		f.mu.Unlock()
		if block != nil {
			<-block
		}
		if err != nil {
			return nil, err
		}
		w := &fakeWindow{startURL: start, page: page, done: make(chan struct{})}
		f.mu.Lock()
		f.opened = append(f.opened, w)
		f.mu.Unlock()
		return w, nil
	}
	resetWindows()
	t.Cleanup(func() {
		StopLoginWindows()
		resetWindows()
		openLoginWindow, loginWindowMaxWait, loginWindowMaxLife, endedKeep = oldOpen, oldWait, oldLife, oldKeep
	})
	return f
}

func resetWindows() {
	windows.mu.Lock()
	for _, r := range windows.active {
		if r.timer != nil {
			r.timer.Stop()
		}
	}
	windows.active = map[string]*run{}
	windows.ended = map[string]endedRun{}
	windows.mu.Unlock()
}

func call(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func openRun(id, rawURL, loginURL string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"requestId": id, "url": rawURL, "loginUrl": loginURL})
	return call(LoginWindowOpenHandler, http.MethodPost, "/login-window/open", string(b))
}

func confirmRun(id, checkURL string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"requestId": id, "checkUrl": checkURL})
	return call(LoginWindowConfirmHandler, http.MethodPost, "/login-window/confirm", string(b))
}

func cancelRun(id string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"requestId": id})
	return call(LoginWindowCancelHandler, http.MethodPost, "/login-window/cancel", string(b))
}

func statusOf(t *testing.T, id string) (int, loginWindowStateResponse) {
	t.Helper()
	rec := call(LoginWindowStatusHandler, http.MethodGet, "/login-window/status?requestId="+id, "")
	var out loginWindowStateResponse
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%v: %s", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func wantState(t *testing.T, id, state, reason string) {
	t.Helper()
	code, got := statusOf(t, id)
	if code != 200 || got.State != state || got.Reason != reason {
		t.Errorf("status of %s: %d %+v, want %s/%s", id, code, got, state, reason)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestOpeningAWindowOpensItAtTheLoginPageAndWaits(t *testing.T) {
	f := useFakes(t)
	rec := openRun("run-1", "https://Jobs.Example.com/search?q=go", "https://accounts.example.com/login")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"waiting"`) || !strings.Contains(rec.Body.String(), `"domain":"jobs.example.com"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	w := f.all()
	if len(w) != 1 || w[0].startURL != "https://accounts.example.com/login" {
		t.Fatalf("the window must open at the login page: %+v", w)
	}
	wantState(t, "run-1", "waiting", "")

	// without a separate login page it opens at the link's own address
	if rec := openRun("run-2", "https://other.example.org/jobs", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if w := f.all(); len(w) != 2 || w[1].startURL != "https://other.example.org/jobs" {
		t.Fatalf("%+v", w)
	}
}

func TestEveryRefusedOpenOpensNothing(t *testing.T) {
	f := useFakes(t)
	long := "https://example.com/" + strings.Repeat("a", maxLoginURLLength)
	cases := []struct{ name, body string }{
		{"not json", `{`},
		{"no request id", `{"url":"https://example.com"}`},
		{"request id with a slash", `{"requestId":"a/b","url":"https://example.com"}`},
		{"request id too long", `{"requestId":"` + strings.Repeat("a", 101) + `","url":"https://example.com"}`},
		{"no url", `{"requestId":"r"}`},
		{"ftp", `{"requestId":"r","url":"ftp://example.com/x"}`},
		{"javascript", `{"requestId":"r","url":"javascript:alert(1)"}`},
		{"file", `{"requestId":"r","url":"file:///etc/passwd"}`},
		{"no host", `{"requestId":"r","url":"https:///path"}`},
		{"credentials in the address", `{"requestId":"r","url":"https://user:pw@example.com/"}`},
		{"too long", `{"requestId":"r","url":"` + long + `"}`},
		{"bad login url", `{"requestId":"r","url":"https://example.com","loginUrl":"ftp://example.com/login"}`},
		{"login url with credentials", `{"requestId":"r","url":"https://example.com","loginUrl":"https://u:p@example.com/login"}`},
	}
	for _, c := range cases {
		rec := call(LoginWindowOpenHandler, http.MethodPost, "/login-window/open", c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
	if n := len(f.all()); n != 0 {
		t.Errorf("a refused request opened %d window(s)", n)
	}
	if code, _ := statusOf(t, "r"); code != http.StatusNotFound {
		t.Errorf("a refused request left a run behind: %d", code)
	}
}

func TestOnlyOneWindowPerRunAndPerSiteAtATime(t *testing.T) {
	f := useFakes(t)
	if rec := openRun("a", "https://jobs.example.com/x", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if rec := openRun("a", "https://elsewhere.example.net/", ""); rec.Code != http.StatusConflict {
		t.Errorf("the same run twice: %d", rec.Code)
	}
	if rec := openRun("b", "https://JOBS.example.com/other", ""); rec.Code != http.StatusConflict {
		t.Errorf("the same site for another run: %d", rec.Code)
	}
	if rec := openRun("c", "https://elsewhere.example.net/", ""); rec.Code != 200 {
		t.Errorf("another site is fine: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(f.all()); n != 2 {
		t.Errorf("%d windows opened, want 2", n)
	}
	// once the first ended, its site is free again
	cancelRun("a")
	if rec := openRun("d", "https://jobs.example.com/x", ""); rec.Code != 200 {
		t.Errorf("after the end: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAWindowThatCannotOpenLeavesNothingAndCanBeTriedAgain(t *testing.T) {
	f := useFakes(t)
	f.openErr = errors.New("no chrome")
	rec := openRun("r", "https://example.com/", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "no chrome") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if code, _ := statusOf(t, "r"); code != http.StatusNotFound {
		t.Errorf("a failed open left a run: %d", code)
	}
	f.openErr = nil
	if rec := openRun("r", "https://example.com/", ""); rec.Code != 200 {
		t.Errorf("trying again: %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfirmingRefusesAPageThatStillLooksLoggedOutAndAcceptsOneThatDoesNot(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/jobs", "")
	w := f.all()[0]

	for _, c := range []struct {
		name string
		page pageInspection
		want string
	}{
		{"login form", pageInspection{PasswordField: true, TextLength: 900}, "login form"},
		{"bot check", pageInspection{Challenge: true, TextLength: 900}, "bot check"},
		{"empty page", pageInspection{}, "empty"},
	} {
		w.setPage(c.page)
		rec := confirmRun("r", "")
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
		wantState(t, "r", "waiting", "")
	}

	w.setPage(pageInspection{TextLength: 900})
	rec := confirmRun("r", "https://example.com/jobs")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"confirmed"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	wantState(t, "r", "confirmed", "")
	if got := w.checkedURLs(); got[len(got)-1] != "https://example.com/jobs" {
		t.Errorf("the page to check was not loaded: %v", got)
	}
	// confirming again is fine and does not look at the page again
	before := len(w.checkedURLs())
	if rec := confirmRun("r", ""); rec.Code != 200 {
		t.Errorf("second confirm: %d", rec.Code)
	}
	if len(w.checkedURLs()) != before {
		t.Error("a second confirmation looked at the page again")
	}
}

func TestConfirmingWithoutAWindowOrWithABadCheckAddressIsRefused(t *testing.T) {
	f := useFakes(t)
	if rec := confirmRun("nobody", ""); rec.Code != http.StatusNotFound {
		t.Errorf("no window: %d", rec.Code)
	}
	if rec := call(LoginWindowConfirmHandler, http.MethodPost, "/", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", rec.Code)
	}
	if rec := confirmRun("bad id!", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", rec.Code)
	}
	openRun("r", "https://jobs.example.com/", "https://accounts.example.com/login")
	w := f.all()[0]
	for _, bad := range []string{"ftp://jobs.example.com/x", "https://elsewhere.example.net/x", "https://accounts.example.com/after", "https://u:p@jobs.example.com/"} {
		if rec := confirmRun("r", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("checkUrl %q: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	if n := len(w.checkedURLs()); n != 0 {
		t.Errorf("a refused check address still looked at a page (%d)", n)
	}
	wantState(t, "r", "waiting", "")
}

func TestAPageThatCannotBeReadIsABadGatewayNotAConfirmation(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/", "")
	w := f.all()[0]
	w.mu.Lock()
	w.err = errors.New("tab crashed")
	w.mu.Unlock()
	rec := confirmRun("r", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "tab crashed") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	wantState(t, "r", "waiting", "")
}

func TestCancellingClosesTheWindowOnceAndIsIdempotent(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/", "")
	w := f.all()[0]
	if rec := cancelRun("r"); rec.Code != http.StatusNoContent {
		t.Fatalf("%d", rec.Code)
	}
	if w.closes.Load() != 1 {
		t.Errorf("closed %d times, want 1", w.closes.Load())
	}
	wantState(t, "r", "ended", "cancelled")
	if rec := cancelRun("r"); rec.Code != http.StatusNoContent {
		t.Errorf("second cancel: %d", rec.Code)
	}
	if rec := cancelRun("never-existed"); rec.Code != http.StatusNoContent {
		t.Errorf("cancel of nothing: %d", rec.Code)
	}
	if w.closes.Load() != 1 {
		t.Errorf("a second cancel closed the window again (%d)", w.closes.Load())
	}
	if rec := confirmRun("r", ""); rec.Code != http.StatusNotFound {
		t.Errorf("confirm after the end: %d", rec.Code)
	}
}

func TestTheWaitingLimitEndsTheWindowButAConfirmedOneGetsTheLongCeiling(t *testing.T) {
	f := useFakes(t)
	loginWindowMaxWait = 80 * time.Millisecond
	loginWindowMaxLife = 400 * time.Millisecond
	openRun("slow", "https://slow.example.com/", "")
	openRun("quick", "https://quick.example.com/", "")
	if rec := confirmRun("quick", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	waitFor(t, "the waiting window to time out", func() bool { code, s := statusOf(t, "slow"); return code == 200 && s.State == "ended" })
	wantState(t, "slow", "ended", "timed_out")
	if f.all()[0].closes.Load() != 1 {
		t.Errorf("the timed-out window was closed %d times", f.all()[0].closes.Load())
	}
	// past the waiting limit, the confirmed one is still there ...
	time.Sleep(60 * time.Millisecond)
	wantState(t, "quick", "confirmed", "")
	// ... and ends at the ceiling
	waitFor(t, "the confirmed window to reach its ceiling", func() bool { _, s := statusOf(t, "quick"); return s.State == "ended" })
	wantState(t, "quick", "ended", "timed_out")
}

func TestAPersonClosingTheWindowEndsTheRun(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/", "")
	w := f.all()[0]
	w.personCloses()
	waitFor(t, "the run to notice", func() bool { _, s := statusOf(t, "r"); return s.State == "ended" })
	wantState(t, "r", "ended", "window_closed")
	if rec := openRun("r", "https://example.com/", ""); rec.Code != 200 {
		t.Errorf("opening again after the person closed it: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStoppingTheToolClosesEveryWindow(t *testing.T) {
	f := useFakes(t)
	openRun("a", "https://a.example.com/", "")
	openRun("b", "https://b.example.com/", "")
	StopLoginWindows()
	for _, w := range f.all() {
		if w.closes.Load() != 1 {
			t.Errorf("a window was closed %d times", w.closes.Load())
		}
	}
	wantState(t, "a", "ended", "tool_stopped")
	wantState(t, "b", "ended", "tool_stopped")
	StopLoginWindows() // nothing left: must not panic or close again
	for _, w := range f.all() {
		if w.closes.Load() != 1 {
			t.Errorf("a second stop closed a window again (%d)", w.closes.Load())
		}
	}
}

func TestAnEndIsRememberedForAWhileAndThenForgotten(t *testing.T) {
	useFakes(t)
	endedKeep = 40 * time.Millisecond
	openRun("r", "https://example.com/", "")
	cancelRun("r")
	wantState(t, "r", "ended", "cancelled")
	time.Sleep(80 * time.Millisecond)
	if code, _ := statusOf(t, "r"); code != http.StatusNotFound {
		t.Errorf("an old end is still remembered: %d", code)
	}
}

func TestCancellingWhileTheWindowIsStillOpeningClosesTheLateWindow(t *testing.T) {
	f := useFakes(t)
	f.block = make(chan struct{})
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- openRun("r", "https://example.com/", "") }()
	waitFor(t, "the run to be reserved", func() bool { code, s := statusOf(t, "r"); return code == 200 && s.State == "waiting" })
	if rec := cancelRun("r"); rec.Code != http.StatusNoContent {
		t.Fatal(rec.Code)
	}
	close(f.block)
	rec := <-result
	if rec.Code != http.StatusConflict {
		t.Errorf("the open that lost the race: %d %s", rec.Code, rec.Body.String())
	}
	w := f.all()
	if len(w) != 1 || w[0].closes.Load() != 1 {
		t.Fatalf("the late window must be closed exactly once: %+v", w)
	}
	wantState(t, "r", "ended", "cancelled")
}

func TestAWindowThatEndsWhileItsPageIsBeingCheckedIsNotConfirmed(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/", "")
	w := f.all()[0]
	w.mu.Lock()
	w.gate = make(chan struct{})
	w.mu.Unlock()
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- confirmRun("r", "") }()
	waitFor(t, "the check to start", func() bool { return len(w.checkedURLs()) == 1 })
	cancelRun("r")
	close(w.gate)
	rec := <-result
	if rec.Code != http.StatusConflict {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	wantState(t, "r", "ended", "cancelled")
}

func TestStatusRefusesABadRunIdAndAnUnknownRunIsNotFound(t *testing.T) {
	useFakes(t)
	if code, _ := statusOf(t, "a%2Fb"); code != http.StatusBadRequest {
		t.Errorf("bad id: %d", code)
	}
	if code, _ := statusOf(t, "unknown"); code != http.StatusNotFound {
		t.Errorf("unknown: %d", code)
	}
	if rec := call(LoginWindowStatusHandler, http.MethodGet, "/login-window/status", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no id: %d", rec.Code)
	}
}

func TestEveryLoginWindowRouteRefusesTheWrongMethod(t *testing.T) {
	useFakes(t)
	for name, c := range map[string]struct {
		h      http.HandlerFunc
		method string
	}{
		"open":    {LoginWindowOpenHandler, http.MethodGet},
		"confirm": {LoginWindowConfirmHandler, http.MethodGet},
		"cancel":  {LoginWindowCancelHandler, http.MethodDelete},
		"status":  {LoginWindowStatusHandler, http.MethodPost},
	} {
		if rec := call(c.h, c.method, "/x?requestId=r", `{"requestId":"r"}`); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
}

// What an open window's state reveals: the state, the site's host, an end reason. Never an address with a query, a page or a cookie.
func TestTheAnswersCarryNoPageContentOrAddress(t *testing.T) {
	f := useFakes(t)
	rec := openRun("r", "https://example.com/private/path?token=SECRET", "")
	if strings.Contains(rec.Body.String(), "SECRET") || strings.Contains(rec.Body.String(), "private") {
		t.Errorf("the open answer leaks the address: %s", rec.Body.String())
	}
	f.all()[0].setPage(pageInspection{TextLength: 100})
	rec = confirmRun("r", "https://example.com/private/path?token=SECRET")
	if strings.Contains(rec.Body.String(), "SECRET") {
		t.Errorf("the confirm answer leaks the address: %s", rec.Body.String())
	}
	rec = call(LoginWindowStatusHandler, http.MethodGet, "/login-window/status?requestId=r", "")
	if strings.Contains(rec.Body.String(), "SECRET") {
		t.Errorf("the status leaks the address: %s", rec.Body.String())
	}
}

func acquire(ctx context.Context, id string) (context.Context, func(), error) {
	return loginWindowAccess{}.AcquireTab(ctx, id)
}

func wantLoginRequired(t *testing.T, err error, reason string) {
	t.Helper()
	var lr *shared.LoginRequiredError
	if !errors.As(err, &lr) || lr.Reason != reason {
		t.Errorf("error %v, want a login required error with reason %q", err, reason)
	}
}

func TestOnlyAConfirmedWindowGivesItsTabAndEveryOtherRunIsToldWhy(t *testing.T) {
	f := useFakes(t)
	if shared.LoginWindows == nil {
		t.Fatal("the crawler's access to the login windows is not registered")
	}
	_, _, err := acquire(context.Background(), "nobody")
	wantLoginRequired(t, err, "no_window")

	openRun("r", "https://example.com/", "")
	_, _, err = acquire(context.Background(), "r")
	wantLoginRequired(t, err, "waiting")

	if rec := confirmRun("r", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	tab, release, err := acquire(context.Background(), "r")
	if err != nil || tab == nil || release == nil {
		t.Fatalf("a confirmed window must give its tab and a release: tab=%v err=%v", tab, err)
	}
	release()
	release() // twice is fine

	cancelRun("r")
	_, _, err = acquire(context.Background(), "r")
	wantLoginRequired(t, err, "ended")
	_ = f
}

func TestOneOperationAtATimeWorksInAWindowAndAWaitingOneCanGiveUp(t *testing.T) {
	useFakes(t)
	openRun("r", "https://example.com/", "")
	confirmRun("r", "")
	_, release, err := acquire(context.Background(), "r")
	if err != nil {
		t.Fatal(err)
	}

	// a second operation waits for the first ...
	got := make(chan error, 1)
	go func() {
		_, rel, err := acquire(context.Background(), "r")
		if err == nil {
			rel()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("the second operation did not wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case err := <-got:
		if err != nil {
			t.Errorf("after the release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second operation never got its turn")
	}

	// ... and a waiting operation whose caller goes away stops waiting
	_, release, _ = acquire(context.Background(), "r")
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := acquire(ctx, "r"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a waiting caller that gave up: %v", err)
	}
}

func TestAWindowThatEndsOrLosesItsLoginWhileAnOperationWaitsIsNotHandedOver(t *testing.T) {
	useFakes(t)
	openRun("r", "https://example.com/", "")
	confirmRun("r", "")
	_, release, _ := acquire(context.Background(), "r")

	got := make(chan error, 1)
	go func() {
		_, _, err := acquire(context.Background(), "r")
		got <- err
	}()
	time.Sleep(50 * time.Millisecond)
	loginWindowAccess{}.LoginLost("r") // the first operation found the login form again
	release()
	wantLoginRequired(t, <-got, "waiting")
	wantState(t, "r", "waiting", "")

	// an operation waiting when the window ends is told it ended
	confirmRun("r", "")
	_, release, _ = acquire(context.Background(), "r")
	go func() {
		_, _, err := acquire(context.Background(), "r")
		got <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancelRun("r")
	release()
	wantLoginRequired(t, <-got, "ended")
}

func TestLosingTheLoginPutsTheWindowBackToWaitingWithTheFullWaitingTime(t *testing.T) {
	useFakes(t)
	loginWindowMaxWait = 150 * time.Millisecond
	loginWindowMaxLife = 10 * time.Second
	openRun("r", "https://example.com/", "")
	confirmRun("r", "")
	time.Sleep(100 * time.Millisecond) // most of a waiting time passes while the crawl works
	loginWindowAccess{}.LoginLost("r")
	wantState(t, "r", "waiting", "")
	time.Sleep(100 * time.Millisecond) // the original waiting time would be over by now
	wantState(t, "r", "waiting", "")
	waitFor(t, "the new waiting time to run out", func() bool { _, s := statusOf(t, "r"); return s.State == "ended" })
	wantState(t, "r", "ended", "timed_out")

	// losing a login that is not confirmed, or of a run that does not exist, changes nothing
	loginWindowAccess{}.LoginLost("nobody")
	openRun("w", "https://waiting.example.com/", "")
	loginWindowAccess{}.LoginLost("w")
	wantState(t, "w", "waiting", "")
}

func TestAConfirmedWindowAnswersConfirmedAgainAfterConfirmingAgainFollowingALostLogin(t *testing.T) {
	f := useFakes(t)
	openRun("r", "https://example.com/", "")
	confirmRun("r", "")
	loginWindowAccess{}.LoginLost("r")
	w := f.all()[0]
	w.setPage(pageInspection{PasswordField: true, TextLength: 100})
	if rec := confirmRun("r", ""); rec.Code != http.StatusConflict {
		t.Errorf("confirming while the login form shows: %d", rec.Code)
	}
	w.setPage(pageInspection{TextLength: 100})
	if rec := confirmRun("r", ""); rec.Code != 200 {
		t.Errorf("confirming again: %d %s", rec.Code, rec.Body.String())
	}
	wantState(t, "r", "confirmed", "")
	if _, release, err := acquire(context.Background(), "r"); err != nil {
		t.Errorf("after the second confirmation: %v", err)
	} else {
		release()
	}
}

func TestAWaitingWindowAnswersAtOnceEvenWhileAnotherOperationHoldsTheTab(t *testing.T) {
	useFakes(t)
	openRun("r", "https://example.com/", "")
	confirmRun("r", "")
	_, release, err := acquire(context.Background(), "r")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	loginWindowAccess{}.LoginLost("r") // the operation that holds the tab found the login form again

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, _, err = acquire(ctx, "r")
	wantLoginRequired(t, err, "waiting")
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("the answer for a waiting window took %v: it must not wait for the tab", time.Since(start))
	}
}

func TestLosingTheLoginOfAWindowThatIsAlreadyWaitingGivesItNoExtraTime(t *testing.T) {
	useFakes(t)
	loginWindowMaxWait = 200 * time.Millisecond
	openRun("w", "https://waiting.example.com/", "")
	time.Sleep(120 * time.Millisecond)
	loginWindowAccess{}.LoginLost("w") // not confirmed: nothing to lose, no new waiting time
	time.Sleep(150 * time.Millisecond) // past the original limit
	wantState(t, "w", "ended", "timed_out")
}

// ---- plan 183, step 1: a window a person opened for the AI conversation whose id is the run's id ----

func openRunForAI(id, rawURL string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]any{"requestId": id, "url": rawURL, "aiMayUse": true})
	return call(LoginWindowOpenHandler, http.MethodPost, "/login-window/open", string(b))
}

func TestAWindowOpenedForTheAIIsReportedAsSuchWithItsSite(t *testing.T) {
	useFakes(t)
	if rec := openRunForAI("conv-1", "https://WWW.Example.com/jobs"); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := loginWindowAccess{}.AIWindow("conv-1")
	if !got.Exists || !got.AIMayUse || got.Domain != "www.example.com" {
		t.Errorf("%+v", got)
	}
}

func TestAWindowOpenedWithoutAIMayUseIsNotAnAIWindow(t *testing.T) {
	useFakes(t)
	if rec := openRun("run-1", "https://a.example/jobs", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	got := loginWindowAccess{}.AIWindow("run-1")
	if !got.Exists || got.AIMayUse {
		t.Errorf("%+v", got)
	}
}

func TestAnUnknownOrEndedWindowIsNoAIWindow(t *testing.T) {
	useFakes(t)
	if got := (loginWindowAccess{}).AIWindow("never-opened"); got != (shared.AIWindowInfo{}) {
		t.Errorf("unknown: %+v", got)
	}
	openRunForAI("conv-2", "https://b.example/")
	cancelRun("conv-2")
	if got := (loginWindowAccess{}).AIWindow("conv-2"); got != (shared.AIWindowInfo{}) {
		t.Errorf("ended: %+v", got)
	}
}

// aiMayUse changes nothing else about the window: it still waits for the person, the confirmation is the person's, and only a confirmed window
// hands out its tab.
func TestAnAIWindowStillNeedsThePersonsConfirmationBeforeAnyToolGetsItsTab(t *testing.T) {
	useFakes(t)
	openRunForAI("conv-3", "https://c.example/jobs")
	if _, _, err := (loginWindowAccess{}).AcquireTab(context.Background(), "conv-3"); err == nil {
		t.Fatal("an unconfirmed AI window handed out its tab")
	} else {
		wantLoginRequired(t, err, "waiting")
	}
	if rec := confirmRun("conv-3", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	_, release, err := (loginWindowAccess{}).AcquireTab(context.Background(), "conv-3")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestTheStatusAnswerNeverShowsWhetherAWindowIsForTheAI(t *testing.T) {
	useFakes(t)
	openRunForAI("conv-4", "https://d.example/")
	rec := call(LoginWindowStatusHandler, http.MethodGet, "/login-window/status?requestId=conv-4", "")
	if strings.Contains(strings.ToLower(rec.Body.String()), "ai") && strings.Contains(strings.ToLower(rec.Body.String()), "mayuse") {
		t.Errorf("%s", rec.Body.String())
	}
}

// The watcher of a window ends only ITS window: a late "the window closed" of an earlier window must not end a newer one opened under the same id.
func TestAnEarlierWindowsLateWatcherDoesNotEndANewerWindowWithTheSameId(t *testing.T) {
	useFakes(t)
	if rec := openRun("same", "https://a.example/", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	windows.mu.Lock()
	newer := windows.active["same"].handle
	windows.mu.Unlock()
	earlier := &fakeWindow{done: make(chan struct{})} // a window of an earlier run under this id, long gone
	if endLoginWindowOf("same", earlier, endedClosed) {
		t.Fatal("the earlier window's watcher ended the newer window")
	}
	wantState(t, "same", "waiting", "")
	windows.mu.Lock()
	still := windows.active["same"] != nil && windows.active["same"].handle == newer
	windows.mu.Unlock()
	if !still || newer.(*fakeWindow).closes.Load() != 0 {
		t.Error("the newer window was touched")
	}
	// its own watcher does end it, with the reason it gives
	if !endLoginWindowOf("same", newer, endedClosed) {
		t.Fatal("the window's own watcher could not end it")
	}
	wantState(t, "same", "ended", "window_closed")
	// and a call for an id that is not open does nothing
	if endLoginWindowOf("never", &fakeWindow{done: make(chan struct{})}, endedClosed) {
		t.Error("ended a window that was not open")
	}
}

// Through the REAL watcher: window A of a run ends but is slow to report that it closed; a new window B is opened under the same id; A's late
// "closed" signal then arrives. B must live on.
func TestTheLateClosedSignalOfAnEarlierWindowLeavesANewerWindowOfTheSameIdAlone(t *testing.T) {
	f := useFakes(t)
	if rec := openRun("same", "https://a.example/", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	a := f.all()[0]
	a.holdDone.Store(true)
	cancelRun("same")
	if rec := openRun("same", "https://a.example/", ""); rec.Code != 200 {
		t.Fatalf("reopen: %d %s", rec.Code, rec.Body.String())
	}
	b := f.all()[1]
	a.personCloses() // A's watcher fires now, late
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if code, got := statusOf(t, "same"); code != 200 || got.State != "waiting" {
			t.Fatalf("the newer window was ended by the earlier one's signal: %d %+v", code, got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if b.closes.Load() != 0 {
		t.Error("the newer window was closed")
	}
	// the newer window's own closing is still noticed
	b.personCloses()
	waitFor(t, "the newer window's own close to be noticed", func() bool { _, s := statusOf(t, "same"); return s.State == "ended" })
	wantState(t, "same", "ended", "window_closed")
}
