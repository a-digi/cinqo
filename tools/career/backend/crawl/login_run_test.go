package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/portal"
)

// scriptedBrowser stands in for core's proxy to the Browser tool, login-window routes included. Every request is recorded in order.
type scriptedBrowser struct {
	mu      sync.Mutex
	events  []string // "open", "status", "cancel", "crawl"
	opens   []map[string]string
	crawls  []map[string]any
	auths   []string
	cancels int

	// status answers the nth status poll (1-based): state and reason ("" reason = none); httpStatus 0 = 200.
	status func(n int) (httpStatus int, state, reason string)
	// crawl answers the nth crawl-paginated call (1-based); it writes the whole response.
	crawl func(n int, w http.ResponseWriter)
	// open answers the open call; nil = 200 waiting.
	open func(w http.ResponseWriter)
	// onStatus runs on each status poll (before it answers), with its number.
	onStatus func(n int)
}

func (s *scriptedBrowser) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}

func (s *scriptedBrowser) count(ev string) int {
	n := 0
	for _, e := range s.seen() {
		if e == ev {
			n++
		}
	}
	return n
}

func startScripted(t *testing.T, s *scriptedBrowser) {
	t.Helper()
	statusN, crawlN := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const base = "/api/v1/tools/browser/proxy/"
		if r.URL.Path == "/api/v1/events/publish" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, base)
		s.mu.Lock()
		s.auths = append(s.auths, r.Header.Get("Authorization"))
		s.mu.Unlock()
		switch path {
		case "login-window/open":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			s.mu.Lock()
			s.events = append(s.events, "open")
			s.opens = append(s.opens, b)
			s.mu.Unlock()
			if s.open != nil {
				s.open(w)
				return
			}
			writeJSON(w, map[string]string{"state": "waiting"})
		case "login-window/status":
			s.mu.Lock()
			statusN++
			n := statusN
			s.events = append(s.events, "status")
			s.mu.Unlock()
			if s.onStatus != nil {
				s.onStatus(n)
			}
			code, state, reason := s.status(n)
			if code != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_ = json.NewEncoder(w).Encode(map[string]string{"state": "confirmed"}) // a wrong answer must not count, whatever it says
				return
			}
			writeJSON(w, map[string]string{"state": state, "reason": reason})
		case "login-window/cancel":
			s.mu.Lock()
			s.events = append(s.events, "cancel")
			s.cancels++
			s.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "crawl-paginated":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			s.mu.Lock()
			crawlN++
			n := crawlN
			s.events = append(s.events, "crawl")
			s.crawls = append(s.crawls, b)
			s.mu.Unlock()
			s.crawl(n, w)
		default:
			http.NotFound(w, r) // crawl-status etc.: nothing to report
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
}

func loginRequired409(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "login_required", "message": "A login is needed", "reason": reason})
}

func fastLogin(t *testing.T) {
	t.Helper()
	oldPoll, oldLimit, oldRe := loginStatusPollInterval, loginWaitLimit, maxLoginRewaits
	loginStatusPollInterval = 3 * time.Millisecond
	loginWaitLimit = 5 * time.Second
	maxLoginRewaits = 3
	t.Cleanup(func() { loginStatusPollInterval, loginWaitLimit, maxLoginRewaits = oldPoll, oldLimit, oldRe })
}

const loginToken = "tok-login-1"

// loginLinkSetup makes a link with crawl instructions, optionally needing a login, and starts a run row for it.
func loginLinkSetup(t *testing.T, needsLogin bool, loginURL string) (linkID, runID string) {
	t.Helper()
	setupTrialDB(t)
	_, linkID, _ = trialPortal(t)
	if err := portal.UpdatePortalLinkCrawlInstructions(linkID, trialListingA); err != nil {
		t.Fatal(err)
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(linkID, trialDetail); err != nil {
		t.Fatal(err)
	}
	if needsLogin {
		yes := true
		if err := portal.UpdatePortalLinkLogin(linkID, &yes, &loginURL); err != nil {
			t.Fatal(err)
		}
	}
	return linkID, ""
}

func startRun(t *testing.T, linkID, kind string) string {
	t.Helper()
	run, err := startCrawlRun(linkID, kind, "manual")
	if err != nil {
		t.Fatal(err)
	}
	return run.ID
}

type runRow struct{ status, phase, errMsg, log string }

func readRun(t *testing.T, id string) runRow {
	t.Helper()
	var r runRow
	var phase, errMsg *string
	if err := db.JobsDB.QueryRow(`SELECT status, phase, error_message, log FROM crawl_runs WHERE id = ?`, id).Scan(&r.status, &phase, &errMsg, &r.log); err != nil {
		t.Fatal(err)
	}
	if phase != nil {
		r.phase = *phase
	}
	if errMsg != nil {
		r.errMsg = *errMsg
	}
	return r
}

func okListing(w http.ResponseWriter) { writeJSON(w, listPage(2)) }

func runListing(t *testing.T, ctx context.Context, linkID, runID string) {
	t.Helper()
	runCrawlNow(ctx, runID, linkID, loginToken)
}

func indexOf(events []string, ev string, from int) int {
	for i := from; i < len(events); i++ {
		if events[i] == ev {
			return i
		}
	}
	return -1
}

func TestLoginRun_LinkWithoutLoginNeverTouchesTheLoginWindow(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, false, "")
	sb := &scriptedBrowser{crawl: func(_ int, w http.ResponseWriter) { okListing(w) }}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	if got := sb.seen(); len(got) != 1 || got[0] != "crawl" {
		t.Fatalf("events = %v, want only one crawl", got)
	}
	if _, has := sb.crawls[0]["useLoginWindow"]; has {
		t.Errorf("a link without a login must not send useLoginWindow: %v", sb.crawls[0])
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("status = %s (%s)", r.status, r.errMsg)
	}
}

func TestLoginRun_ConfirmedAtOnceCrawlsInTheWindowAndClosesIt(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "https://shop.example/login")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)

	if len(sb.opens) != 1 {
		t.Fatalf("opens = %v", sb.opens)
	}
	o := sb.opens[0]
	if o["requestId"] != runID || o["url"] != "https://shop.example/new-search" || o["loginUrl"] != "https://shop.example/login" {
		t.Errorf("open body = %v", o)
	}
	if sb.crawls[0]["useLoginWindow"] != true || sb.crawls[0]["requestId"] != runID {
		t.Errorf("crawl body = %v", sb.crawls[0])
	}
	ev := sb.seen()
	if ev[0] != "open" || ev[len(ev)-1] != "cancel" || sb.count("cancel") != 1 {
		t.Errorf("events = %v, want open first and exactly one cancel last", ev)
	}
	for _, a := range sb.auths {
		if a != "Bearer "+loginToken {
			t.Errorf("a call carried %q", a)
		}
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("status = %s (%s)", r.status, r.errMsg)
	}
}

func TestLoginRun_ConfirmedLaterWaitsInThePhaseAndCrawlsOnlyAfterwards(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	var runID string
	var phaseWhileWaiting string
	sb := &scriptedBrowser{
		status: func(n int) (int, string, string) {
			if n < 4 {
				return 0, "waiting", ""
			}
			return 0, "confirmed", ""
		},
		crawl: func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	sb.onStatus = func(n int) {
		if n == 3 {
			phaseWhileWaiting = readRun(t, runID).phase
		}
	}
	startScripted(t, sb)
	runID = startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)

	if phaseWhileWaiting != "waiting_for_login" {
		t.Errorf("phase while waiting = %q", phaseWhileWaiting)
	}
	ev := sb.seen()
	if c, lastStatus := indexOf(ev, "crawl", 0), strings.LastIndex(strings.Join(ev, ","), "status"); c < 0 || sb.count("status") != 4 || lastStatus < 0 {
		t.Fatalf("events = %v", ev)
	} else if indexOf(ev, "crawl", 0) < 4 { // open + 4 status polls come first
		t.Errorf("crawl started before the confirmation: %v", ev)
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("status = %s (%s)", r.status, r.errMsg)
	}
}

func TestLoginRun_NeverConfirmedFailsClosesAndNeverCrawls(t *testing.T) {
	fastLogin(t)
	loginWaitLimit = 80 * time.Millisecond
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "waiting", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "not confirmed") {
		t.Errorf("run = %+v", r)
	}
	if sb.count("crawl") != 0 || sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
}

func TestLoginRun_WindowEndedBeforeConfirmationFails(t *testing.T) {
	for reason, want := range map[string]string{
		"window_closed": "closed before the login was confirmed",
		"timed_out":     "not confirmed in time",
		"cancelled":     "cancelled",
		"tool_stopped":  "tool stopped",
		"":              "ended before",
	} {
		fastLogin(t)
		linkID, _ := loginLinkSetup(t, true, "")
		sb := &scriptedBrowser{
			status: func(n int) (int, string, string) {
				if n == 1 {
					return 0, "waiting", ""
				}
				return 0, "ended", reason
			},
			crawl: func(_ int, w http.ResponseWriter) { okListing(w) },
		}
		startScripted(t, sb)
		runID := startRun(t, linkID, "listing")
		runListing(t, context.Background(), linkID, runID)
		r := readRun(t, runID)
		if r.status != "failed" || !strings.Contains(r.errMsg, want) {
			t.Errorf("reason %q: run = %+v, want message containing %q", reason, r, want)
		}
		if sb.count("crawl") != 0 || sb.count("cancel") != 1 {
			t.Errorf("reason %q: events = %v", reason, sb.seen())
		}
	}
}

func TestLoginRun_UnknownWindowOnStatusCountsAsEnded(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return http.StatusNotFound, "", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "no login window") || sb.count("crawl") != 0 {
		t.Errorf("run = %+v events = %v", r, sb.seen())
	}
}

func TestLoginRun_BrowserGoneWhileWaitingFailsAfterSomeTries(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return http.StatusInternalServerError, "", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "lost contact") {
		t.Errorf("run = %+v", r)
	}
	if sb.count("status") != maxLoginStatusFailures {
		t.Errorf("polls = %d, want %d", sb.count("status"), maxLoginStatusFailures)
	}
}

func TestLoginRun_AStatusBlipDoesNotEndTheWait(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(n int) (int, string, string) {
			switch n {
			case 1, 2, 3, 4, 6, 7, 8, 9: // more failures in all than the limit, but never that many in a row
				return http.StatusInternalServerError, "", ""
			case 5:
				return 0, "waiting", ""
			}
			return 0, "confirmed", ""
		},
		crawl: func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_FailedOpenFailsTheRunWithoutCrawling(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		open: func(w http.ResponseWriter) {
			http.Error(w, "a login window for this site is already open", http.StatusConflict)
		},
		crawl: func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "could not open the login window") || !strings.Contains(r.errMsg, "already open") {
		t.Errorf("run = %+v", r)
	}
	if sb.count("crawl") != 0 || sb.count("status") != 0 {
		t.Errorf("events = %v", sb.seen())
	}
}

func TestLoginRun_LoginLostMidCrawlGoesBackToWaitingThenContinues(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(n int) (int, string, string) {
			if n == 2 { // right after the 409 the window is waiting again
				return 0, "waiting", ""
			}
			return 0, "confirmed", ""
		},
		crawl: func(n int, w http.ResponseWriter) {
			if n == 1 {
				loginRequired409(w, "login_lost")
				return
			}
			okListing(w)
		},
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	ev := sb.seen()
	// open, status(confirmed), crawl(409), status(waiting), status(confirmed), crawl(ok), cancel
	want := []string{"open", "status", "crawl", "status", "status", "crawl", "cancel"}
	if strings.Join(ev, ",") != strings.Join(want, ",") {
		t.Errorf("events = %v, want %v", ev, want)
	}
	r := readRun(t, runID)
	if r.status != "completed" || !strings.Contains(r.log, "asks for the login again (1/3)") {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_LoginLostTooOftenFails(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { loginRequired409(w, "login_lost") },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "too often") {
		t.Errorf("run = %+v", r)
	}
	if sb.count("crawl") != maxLoginRewaits+1 || sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
}

func TestLoginRun_LoginRewaitDoesNotUseTheTransientRetryBudget(t *testing.T) {
	fastLogin(t)
	oldInt, oldMax := crawlNowSessionRetryInterval, maxCrawlNowSessionRetries
	crawlNowSessionRetryInterval, maxCrawlNowSessionRetries = time.Millisecond, 2
	t.Cleanup(func() { crawlNowSessionRetryInterval, maxCrawlNowSessionRetries = oldInt, oldMax })
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl: func(n int, w http.ResponseWriter) {
			if n <= 3 { // three logins lost in a row, then a transient error twice, then success
				loginRequired409(w, "login_lost")
				return
			}
			if n <= 5 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "browser_session_interrupted", "message": "restarted"})
				return
			}
			okListing(w)
		},
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_StoppedWhileWaitingClosesTheWindowAndNeverCrawls(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	ctx, cancel := context.WithCancel(context.Background())
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "waiting", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	sb.onStatus = func(n int) {
		if n == 3 {
			cancel()
		}
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	done := make(chan struct{})
	go func() { runListing(t, ctx, linkID, runID); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not stop")
	}
	if sb.count("crawl") != 0 || sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
	r := readRun(t, runID)
	if r.status == "failed" || !strings.Contains(r.log, "cancelled while waiting for the login") {
		t.Errorf("a stop is not a failure: %+v", r)
	}
}

func TestLoginRun_AFailingCrawlStillClosesTheWindow(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { http.Error(w, "boom", http.StatusInternalServerError) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	if r := readRun(t, runID); r.status != "failed" {
		t.Errorf("run = %+v", r)
	}
	if sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
}

func TestLoginRun_AConflictThatIsNotLoginRequiredIsAPlainFailure(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { http.Error(w, "something else", http.StatusConflict) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, context.Background(), linkID, runID)
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "409") || sb.count("crawl") != 1 {
		t.Errorf("run = %+v events = %v", r, sb.seen())
	}
}

func TestLoginRun_UnattendedCrawlOfALoginLinkIsRefusedBeforeAnyRun(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	_, err := RunListingCrawlNow(linkID, loginToken, "auto_discovery")
	if !errors.Is(err, ErrLoginNeeded) {
		t.Fatalf("err = %v, want ErrLoginNeeded", err)
	}
	if !strings.Contains(err.Error(), "login needed") {
		t.Errorf("message = %q", err)
	}
	if n := rowCount(t, "crawl_runs"); n != 0 {
		t.Errorf("crawl_runs rows = %d", n)
	}
	if len(sb.seen()) != 0 {
		t.Errorf("the browser was called: %v", sb.seen())
	}
}

func TestLoginRun_UnattendedCrawlOfALinkWithoutLoginStillRuns(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, false, "")
	sb := &scriptedBrowser{crawl: func(_ int, w http.ResponseWriter) { okListing(w) }}
	startScripted(t, sb)
	runID, err := RunListingCrawlNow(linkID, loginToken, "auto_discovery")
	if err != nil {
		t.Fatal(err)
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_ManualCrawlOfALoginLinkThroughRunListingOpensTheWindow(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID, err := RunListingCrawlNow(linkID, loginToken, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if sb.count("open") != 1 || sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_SingleJobRecrawlOfALoginLinkIsRefused(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{crawl: func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage("x")) }}
	startScripted(t, sb)
	_, err := RecrawlOneJob(context.Background(), jobs.RecrawlTarget{ID: "j", SourceURL: "https://shop.example/job/1", PortalLinkID: linkID}, loginToken)
	var np *NotPossibleError
	if !errors.As(err, &np) || !strings.Contains(np.Reason, "needs a login") {
		t.Fatalf("err = %v", err)
	}
	if len(sb.seen()) != 0 {
		t.Errorf("the browser was called: %v", sb.seen())
	}
}

// ---- the job detail run ----

func seedJobs(t *testing.T, linkID string, n int) []jobs.JobDetailCrawlTarget {
	t.Helper()
	if _, err := portal.IngestCrawlResults(linkID, uniqueListing(n)); err != nil {
		t.Fatal(err)
	}
	targets, err := jobs.ListJobsForDetailCrawl(linkID)
	if err != nil || len(targets) != n {
		t.Fatalf("targets = %v, %v", targets, err)
	}
	return targets
}

func TestLoginRun_JobDetailRunOpensItsOwnWindowAndCrawlsEveryJobInIt(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "https://shop.example/login")
	targets := seedJobs(t, linkID, 2)
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage(strings.Repeat("d", 150))) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)

	if sb.count("open") != 1 || sb.count("crawl") != 2 || sb.count("cancel") != 1 {
		t.Fatalf("events = %v", sb.seen())
	}
	if sb.opens[0]["requestId"] != runID || sb.opens[0]["url"] != "https://shop.example/new-search" || sb.opens[0]["loginUrl"] != "https://shop.example/login" {
		t.Errorf("open = %v", sb.opens[0])
	}
	for i, c := range sb.crawls {
		if c["useLoginWindow"] != true || c["requestId"] != runID {
			t.Errorf("crawl %d body = %v", i, c)
		}
	}
	if ev := sb.seen(); indexOf(ev, "crawl", 0) < indexOf(ev, "status", 0) {
		t.Errorf("a job was crawled before the confirmation: %v", ev)
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestLoginRun_JobDetailRunWithoutLoginIsUntouched(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, false, "")
	targets := seedJobs(t, linkID, 1)
	sb := &scriptedBrowser{crawl: func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage(strings.Repeat("d", 150))) }}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)
	if got := sb.seen(); len(got) != 1 || got[0] != "crawl" {
		t.Fatalf("events = %v", got)
	}
	if _, has := sb.crawls[0]["useLoginWindow"]; has {
		t.Errorf("body = %v", sb.crawls[0])
	}
}

func TestLoginRun_JobDetailLoginLostRetriesTheSameJobAndDoesNotMarkItFailed(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	targets := seedJobs(t, linkID, 2)
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl: func(n int, w http.ResponseWriter) {
			if n == 2 { // the second call (job 2's first attempt... or job 1's retry: see below) loses the login
				loginRequired409(w, "login_lost")
				return
			}
			writeJSON(w, detailPage(strings.Repeat("d", 150)))
		},
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)

	if sb.count("crawl") != 3 {
		t.Fatalf("crawl calls = %d, want 3 (job 1, job 2 refused, job 2 again): %v", sb.count("crawl"), sb.seen())
	}
	if sb.crawls[2]["useLoginWindow"] != true {
		t.Errorf("the retry of the refused job left the login window: %v", sb.crawls[2])
	}
	if sb.crawls[1]["url"] != sb.crawls[2]["url"] {
		t.Errorf("the refused job was not retried: %v then %v", sb.crawls[1]["url"], sb.crawls[2]["url"])
	}
	r := readRun(t, runID)
	if r.status != "completed" || !strings.Contains(r.log, "asks for the login again") || strings.Contains(r.log, "skipped \"") {
		t.Errorf("run = %+v", r)
	}
	var failed int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE detail_crawl_status = 'failed'`).Scan(&failed)
	if failed != 0 {
		t.Errorf("%d job(s) marked failed after a lost login", failed)
	}
}

func TestLoginRun_JobDetailStoppedWhileWaitingClosesTheWindow(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	targets := seedJobs(t, linkID, 1)
	ctx, cancel := context.WithCancel(context.Background())
	sb := &scriptedBrowser{
		status:   func(int) (int, string, string) { return 0, "waiting", "" },
		crawl:    func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage("x")) },
		onStatus: func(n int) {},
	}
	sb.onStatus = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(ctx, runID, linkID, targets, loginToken)
	if sb.count("crawl") != 0 || sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
	if r := readRun(t, runID); r.status == "failed" {
		t.Errorf("a stop is not a failure: %+v", r)
	}
}

func TestLoginRun_JobDetailNeverConfirmedFailsAndClosesTheWindow(t *testing.T) {
	fastLogin(t)
	loginWaitLimit = 60 * time.Millisecond
	linkID, _ := loginLinkSetup(t, true, "")
	targets := seedJobs(t, linkID, 1)
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "waiting", "" },
		crawl:  func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage("x")) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)
	r := readRun(t, runID)
	if r.status != "failed" || sb.count("crawl") != 0 || sb.count("cancel") != 1 {
		t.Errorf("run = %+v events = %v", r, sb.seen())
	}
}

// uniqueListing is a listing page whose n jobs differ in title and address (ingest merges equal ones).
func uniqueListing(n int) []portal.CrawlResultPage {
	items := []map[string]any{}
	for i := 1; i <= n; i++ {
		items = append(items, map[string]any{"title": "Job " + string(rune('A'+i)), "url": "https://shop.example/job/" + string(rune('0'+i))})
	}
	return []portal.CrawlResultPage{{URL: "list", Items: items}}
}

func TestLoginRun_StopWakesTheWaitAtOnceEvenWithASlowPollInterval(t *testing.T) {
	fastLogin(t)
	loginStatusPollInterval = time.Hour
	linkID, _ := loginLinkSetup(t, true, "")
	sb := &scriptedBrowser{
		status: func(int) (int, string, string) { return 0, "waiting", "" },
		crawl:  func(_ int, w http.ResponseWriter) { okListing(w) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runListing(t, ctx, linkID, runID); close(done) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a stop did not wake the wait")
	}
	if sb.count("cancel") != 1 {
		t.Errorf("events = %v", sb.seen())
	}
}

func TestLoginRun_StoppedWhileWaitingAgainIsNotAFailure(t *testing.T) {
	fastLogin(t)
	linkID, _ := loginLinkSetup(t, true, "")
	ctx, cancel := context.WithCancel(context.Background())
	sb := &scriptedBrowser{
		status: func(n int) (int, string, string) {
			if n == 1 {
				return 0, "confirmed", ""
			}
			return 0, "waiting", "" // after the 409 the person has not logged in again
		},
		crawl: func(_ int, w http.ResponseWriter) { loginRequired409(w, "login_lost") },
	}
	sb.onStatus = func(n int) {
		if n == 4 {
			cancel()
		}
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "listing")
	runListing(t, ctx, linkID, runID)
	r := readRun(t, runID)
	if r.status == "failed" || !strings.Contains(r.log, "cancelled while waiting for the login") || sb.count("crawl") != 1 || sb.count("cancel") != 1 {
		t.Errorf("run = %+v events = %v", r, sb.seen())
	}
}

func TestLoginRun_JobDetailLoginLostThenNeverConfirmedMarksNoJobFailed(t *testing.T) {
	fastLogin(t)
	loginWaitLimit = 80 * time.Millisecond
	linkID, _ := loginLinkSetup(t, true, "")
	targets := seedJobs(t, linkID, 1)
	sb := &scriptedBrowser{
		status: func(n int) (int, string, string) {
			if n == 1 {
				return 0, "confirmed", ""
			}
			return 0, "waiting", ""
		},
		crawl: func(_ int, w http.ResponseWriter) { loginRequired409(w, "login_lost") },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)
	if r := readRun(t, runID); r.status != "failed" {
		t.Errorf("run = %+v", r)
	}
	var failed int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE detail_crawl_status = 'failed'`).Scan(&failed)
	if failed != 0 {
		t.Errorf("%d job(s) marked failed by a lost login", failed)
	}
}
