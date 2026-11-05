package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// login_window.go (plan 182, step 1) is the human-only way to log in to a website: the tool opens a throw-away visible window at the
// site's own login page (shared/loginwindow.go), the PERSON logs in there, presses "I am logged in", and the tool checks that the page no
// longer shows a login form before it says the window is ready. Nothing about the login is stored: no password, cookie or session record,
// no table, no file, no log line. What exists while a run lives is a small in-memory record (which window belongs to which run and what
// state it is in), like the crawl phase (crawler/crawl_status.go); the window and its temporary folder are deleted when the run ends.
//
// Deliberately never an MCP tool (no MCP tool can name a login window): the AI cannot start, confirm or end a login;
// only a signed-in human's own request can, through the ordinary tool proxy. Reached under scope tool:browser:login.
// See plan/ai/tools/browser/step-182-crawl-with-login.md.

// The states of a run's login window. A window that ended is no longer in the live set; its end is remembered for a short while so a
// caller can tell "ended" from "never opened".
const (
	loginWindowWaiting   = "waiting"
	loginWindowConfirmed = "confirmed"
	loginWindowEnded     = "ended"
)

// The reasons a window ended.
const (
	endedCancelled   = "cancelled"
	endedTimedOut    = "timed_out"
	endedClosed      = "window_closed"
	endedToolStopped = "tool_stopped"
)

// loginWindowMaxWait is how long the tool waits for the person to log in and confirm. loginWindowMaxLife is the ceiling for a window
// that was confirmed (a run that is never ended by its caller still cannot keep a window open for ever). endedKeep is how long the end
// of a run is remembered. Vars so a test can shrink them.
var (
	loginWindowMaxWait = 30 * time.Minute
	loginWindowMaxLife = 3 * time.Hour
	endedKeep          = 10 * time.Minute
	inspectTimeout     = 30 * time.Second
)

// pageInspection is what the confirmation looks at on the open page (shared with the crawler: shared/loginwindow_access.go).
type pageInspection = shared.PageInspection

// loginWindowHandle is one open window as the manager sees it. The real one wraps shared.LoginWindow; tests use a fake.
type loginWindowHandle interface {
	// Inspect loads checkURL in the window first when it is not empty, then reads the page.
	Inspect(ctx context.Context, checkURL string) (pageInspection, error)
	// TabContext is the window's tab, for the crawler to work in once the login is confirmed.
	TabContext() context.Context
	// Done is closed when the window has ended on its own (the person closed it).
	Done() <-chan struct{}
	// Close ends the window and deletes its temporary profile folder; safe to call more than once.
	Close()
}

// openLoginWindow opens a window at startURL. A var so tests can replace the real Chrome.
var openLoginWindow = func(startURL string) (loginWindowHandle, error) {
	w, err := shared.OpenLoginWindow(startURL)
	if err != nil {
		return nil, err
	}
	return &chromeWindow{w: w}, nil
}

type chromeWindow struct{ w *shared.LoginWindow }

func (c *chromeWindow) Done() <-chan struct{}       { return c.w.Done() }
func (c *chromeWindow) TabContext() context.Context { return c.w.Ctx }
func (c *chromeWindow) Close()                      { c.w.Close() }

func (c *chromeWindow) Inspect(reqCtx context.Context, checkURL string) (pageInspection, error) {
	ctx, cancel := context.WithTimeout(c.w.Ctx, inspectTimeout)
	defer cancel()
	// The caller giving up (the person's request was cancelled) stops the look at the page too.
	defer context.AfterFunc(reqCtx, cancel)()
	if checkURL != "" {
		if err := chromedp.Run(ctx, chromedp.Navigate(checkURL)); err != nil {
			return pageInspection{}, err
		}
	}
	return shared.InspectPage(ctx)
}

// run is one run's login window.
type run struct {
	requestID string
	domain    string
	state     string
	handle    loginWindowHandle
	timer     *time.Timer
	// busy is the window's lock: one operation (a crawl, an extraction) works in the tab at a time.
	busy chan struct{}
}

type endedRun struct {
	reason string
	at     time.Time
}

var windows = struct {
	mu     sync.Mutex
	active map[string]*run
	ended  map[string]endedRun
}{active: map[string]*run{}, ended: map[string]endedRun{}}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

const maxLoginURLLength = 2000

// parseHTTPURL accepts only an http or https address with a host and no embedded credentials, and returns it with its lower-case host.
func parseHTTPURL(raw string) (*url.URL, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLoginURLLength {
		return nil, "", fmt.Errorf("the address must be 1 to %d characters", maxLoginURLLength)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, "", fmt.Errorf("the address must be an http or https address with a host")
	}
	if u.User != nil {
		return nil, "", fmt.Errorf("the address must not contain a user name or password")
	}
	return u, strings.ToLower(u.Hostname()), nil
}

// forgetOldEnds drops the remembered ends that are older than endedKeep. The caller holds windows.mu.
func forgetOldEnds(now time.Time) {
	for id, e := range windows.ended {
		if now.Sub(e.at) > endedKeep {
			delete(windows.ended, id)
		}
	}
}

// endLoginWindow ends a run's window for a reason: removes it from the live set, stops its timer, remembers the end, and closes the
// window (outside the lock; closing waits for Chrome and the folder). Safe to call twice: the second call does nothing.
func endLoginWindow(requestID, reason string) bool {
	windows.mu.Lock()
	r, ok := windows.active[requestID]
	if ok {
		delete(windows.active, requestID)
		if r.timer != nil {
			r.timer.Stop()
		}
		windows.ended[requestID] = endedRun{reason: reason, at: time.Now()}
	}
	windows.mu.Unlock()
	// A run that is still opening has no handle yet; its opener closes the window when it sees the reservation is gone.
	if ok && r.handle != nil {
		r.handle.Close()
	}
	return ok
}

// StopLoginWindows closes every open login window; called when the tool shuts down.
func StopLoginWindows() {
	windows.mu.Lock()
	ids := make([]string, 0, len(windows.active))
	for id := range windows.active {
		ids = append(ids, id)
	}
	windows.mu.Unlock()
	for _, id := range ids {
		endLoginWindow(id, endedToolStopped)
	}
}

type loginWindowOpenRequest struct {
	RequestID string `json:"requestId"`
	URL       string `json:"url"`
	LoginURL  string `json:"loginUrl"`
}

type loginWindowStateResponse struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Domain string `json:"domain,omitempty"`
}

func writeStateJSON(w http.ResponseWriter, status int, v loginWindowStateResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeLoginWindowBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(into); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// LoginWindowOpenHandler handles POST /login-window/open: opens the throw-away window for a run at the login page (loginUrl, or url when
// there is none) and answers {"state":"waiting"}. One window per run, and at most one per site at a time (409 otherwise); 400 for a bad
// request id or address; 502 when Chrome cannot be started.
func LoginWindowOpenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body loginWindowOpenRequest
	if !decodeLoginWindowBody(w, r, &body) {
		return
	}
	if !requestIDPattern.MatchString(body.RequestID) {
		http.Error(w, "requestId is required: 1 to 100 letters, digits, _ or -", http.StatusBadRequest)
		return
	}
	_, domain, err := parseHTTPURL(body.URL)
	if err != nil {
		http.Error(w, "url: "+err.Error(), http.StatusBadRequest)
		return
	}
	start := body.URL
	if strings.TrimSpace(body.LoginURL) != "" {
		if _, _, err := parseHTTPURL(body.LoginURL); err != nil {
			http.Error(w, "loginUrl: "+err.Error(), http.StatusBadRequest)
			return
		}
		start = body.LoginURL
	}

	// Reserve the run before the slow part (starting Chrome), so two requests for the same run or site cannot both pass.
	windows.mu.Lock()
	forgetOldEnds(time.Now())
	if _, taken := windows.active[body.RequestID]; taken {
		windows.mu.Unlock()
		http.Error(w, "this run already has a login window", http.StatusConflict)
		return
	}
	for _, other := range windows.active {
		if other.domain == domain {
			windows.mu.Unlock()
			http.Error(w, "a login window for this site is already open", http.StatusConflict)
			return
		}
	}
	reserved := &run{requestID: body.RequestID, domain: domain, state: loginWindowWaiting, busy: make(chan struct{}, 1)}
	windows.active[body.RequestID] = reserved
	delete(windows.ended, body.RequestID)
	windows.mu.Unlock()

	handle, err := openLoginWindow(strings.TrimSpace(start))
	if err != nil {
		windows.mu.Lock()
		if windows.active[body.RequestID] == reserved {
			delete(windows.active, body.RequestID)
		}
		windows.mu.Unlock()
		http.Error(w, "could not open the login window: "+err.Error(), http.StatusBadGateway)
		return
	}

	windows.mu.Lock()
	// Cancelled or stopped while Chrome was starting: do not leave a window nobody owns.
	if windows.active[body.RequestID] != reserved {
		windows.mu.Unlock()
		handle.Close()
		http.Error(w, "the login window was cancelled while it was opening", http.StatusConflict)
		return
	}
	reserved.handle = handle
	reserved.timer = time.AfterFunc(loginWindowMaxWait, func() { endLoginWindow(body.RequestID, endedTimedOut) })
	windows.mu.Unlock()
	go func() {
		<-handle.Done()
		endLoginWindow(body.RequestID, endedClosed)
	}()
	writeStateJSON(w, http.StatusOK, loginWindowStateResponse{State: loginWindowWaiting, Domain: domain})
}

// LoginWindowStatusHandler handles GET /login-window/status?requestId=: waiting, confirmed, or ended (with the reason); 404 for a run
// that never had a window (or whose end is no longer remembered).
func LoginWindowStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("requestId")
	if !requestIDPattern.MatchString(id) {
		http.Error(w, "requestId is required", http.StatusBadRequest)
		return
	}
	windows.mu.Lock()
	forgetOldEnds(time.Now())
	if run, ok := windows.active[id]; ok {
		resp := loginWindowStateResponse{State: run.state, Domain: run.domain}
		windows.mu.Unlock()
		writeStateJSON(w, http.StatusOK, resp)
		return
	}
	if e, ok := windows.ended[id]; ok {
		windows.mu.Unlock()
		writeStateJSON(w, http.StatusOK, loginWindowStateResponse{State: loginWindowEnded, Reason: e.reason})
		return
	}
	windows.mu.Unlock()
	http.Error(w, "no login window for this run", http.StatusNotFound)
}

type loginWindowConfirmRequest struct {
	RequestID string `json:"requestId"`
	CheckURL  string `json:"checkUrl"`
}

// LoginWindowConfirmHandler handles POST /login-window/confirm: the person says they are logged in; the tool checks. It loads checkUrl
// in the window when given (it must be on the same site the run was opened for), then refuses with 409 and a reason when the page still
// shows a login form, looks like a bot check or is empty. On success the state becomes confirmed and the window's lifetime limit is the
// long ceiling, not the waiting limit. Confirming twice is fine (200). 404 without an open window for the run.
func LoginWindowConfirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body loginWindowConfirmRequest
	if !decodeLoginWindowBody(w, r, &body) {
		return
	}
	if !requestIDPattern.MatchString(body.RequestID) {
		http.Error(w, "requestId is required", http.StatusBadRequest)
		return
	}
	windows.mu.Lock()
	run, ok := windows.active[body.RequestID]
	var handle loginWindowHandle
	var domain, state string
	if ok {
		handle, domain, state = run.handle, run.domain, run.state
	}
	windows.mu.Unlock()
	if !ok || handle == nil {
		http.Error(w, "no open login window for this run", http.StatusNotFound)
		return
	}
	if state == loginWindowConfirmed {
		writeStateJSON(w, http.StatusOK, loginWindowStateResponse{State: loginWindowConfirmed, Domain: domain})
		return
	}
	check := strings.TrimSpace(body.CheckURL)
	if check != "" {
		_, host, err := parseHTTPURL(check)
		if err != nil {
			http.Error(w, "checkUrl: "+err.Error(), http.StatusBadRequest)
			return
		}
		if host != domain {
			http.Error(w, "checkUrl must be on the site the login window was opened for", http.StatusBadRequest)
			return
		}
	}
	page, err := handle.Inspect(r.Context(), check)
	if err != nil {
		http.Error(w, "could not look at the page in the login window: "+err.Error(), http.StatusBadGateway)
		return
	}
	switch {
	case page.PasswordField:
		http.Error(w, "the page still shows a login form: log in in the window first", http.StatusConflict)
		return
	case page.Challenge:
		http.Error(w, "the page is a bot check, not the site's own page: finish the check in the window first", http.StatusConflict)
		return
	case page.TextLength == 0:
		http.Error(w, "the page in the window is empty: wait until it has loaded", http.StatusConflict)
		return
	}
	windows.mu.Lock()
	run, ok = windows.active[body.RequestID]
	if !ok {
		windows.mu.Unlock()
		http.Error(w, "the login window ended while it was being checked", http.StatusConflict)
		return
	}
	run.state = loginWindowConfirmed
	if run.timer != nil {
		run.timer.Stop()
	}
	id := body.RequestID
	run.timer = time.AfterFunc(loginWindowMaxLife, func() { endLoginWindow(id, endedTimedOut) })
	windows.mu.Unlock()
	writeStateJSON(w, http.StatusOK, loginWindowStateResponse{State: loginWindowConfirmed, Domain: domain})
}

type loginWindowCancelRequest struct {
	RequestID string `json:"requestId"`
}

// LoginWindowCancelHandler handles POST /login-window/cancel: closes the run's window and discards everything in it. Idempotent: 204
// whether or not there was a window.
func LoginWindowCancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body loginWindowCancelRequest
	if !decodeLoginWindowBody(w, r, &body) {
		return
	}
	if !requestIDPattern.MatchString(body.RequestID) {
		http.Error(w, "requestId is required", http.StatusBadRequest)
		return
	}
	endLoginWindow(body.RequestID, endedCancelled)
	w.WriteHeader(http.StatusNoContent)
}

// loginWindowAccess is what the crawler gets (shared.LoginWindows): the confirmed window's tab, one operation at a time.
type loginWindowAccess struct{}

func init() { shared.LoginWindows = loginWindowAccess{} }

// AcquireTab returns the tab of the run's CONFIRMED window once no other operation is using it. A run whose window is missing, still
// waiting for the login, or ended gets a *shared.LoginRequiredError (the crawler answers login_required at once, without waiting).
func (loginWindowAccess) AcquireTab(ctx context.Context, requestID string) (context.Context, func(), error) {
	windows.mu.Lock()
	r, ok := windows.active[requestID]
	var busy chan struct{}
	var handle loginWindowHandle
	reason := "no_window"
	if ok {
		busy, handle = r.busy, r.handle
		switch {
		case handle == nil || r.state == loginWindowWaiting:
			reason = "waiting"
			ok = false
		}
	} else if _, ended := windows.ended[requestID]; ended {
		reason = "ended"
	}
	windows.mu.Unlock()
	if !ok {
		return nil, nil, &shared.LoginRequiredError{Reason: reason}
	}
	select {
	case busy <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	// The window may have ended or lost its login while this operation waited for its turn.
	windows.mu.Lock()
	r, ok = windows.active[requestID]
	still := ok && r.state == loginWindowConfirmed && r.handle == handle
	windows.mu.Unlock()
	if !still {
		<-busy
		reason = "ended"
		windows.mu.Lock()
		if r, ok := windows.active[requestID]; ok && r.state == loginWindowWaiting {
			reason = "waiting"
		}
		windows.mu.Unlock()
		return nil, nil, &shared.LoginRequiredError{Reason: reason}
	}
	var once sync.Once
	return handle.TabContext(), func() { once.Do(func() { <-busy }) }, nil
}

// LoginLost sets a confirmed window back to waiting (and gives the person the waiting time again to log in once more).
func (loginWindowAccess) LoginLost(requestID string) {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	r, ok := windows.active[requestID]
	if !ok || r.state != loginWindowConfirmed {
		return
	}
	r.state = loginWindowWaiting
	if r.timer != nil {
		r.timer.Stop()
	}
	id := requestID
	r.timer = time.AfterFunc(loginWindowMaxWait, func() { endLoginWindow(id, endedTimedOut) })
}
