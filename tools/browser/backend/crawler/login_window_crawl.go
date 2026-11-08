package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// login_window_crawl.go (plan 182, step 2) crawls and extracts INSIDE a run's login window: the throw-away visible Chrome the person
// logged in to and confirmed (auth/login_window.go). A request opts in with "useLoginWindow": true plus its requestId (the run's id, the
// same one /crawl-status and /crawl-cancel already use). The work then runs in that window's tab, never on the headless session, and
// never falls back to it: a run without a CONFIRMED window is answered `login_required` at once (HTTP 409), and so is a page that shows
// its login form again in the middle of the crawl (the window goes back to "waiting" for the person to log in again). Nothing here reads
// or keeps a cookie or a password. Requests without the flag are untouched. See plan/ai/tools/browser/step-182-crawl-with-login.md.

const (
	// codeLoginRequired is the answer when the run has no confirmed login window, or the site asked for the login again.
	codeLoginRequired = "login_required"
	// codeBotCheckInWindow: the page in the login window is a bot check; the person has to finish it in the window.
	codeBotCheckInWindow = "bot_check_in_login_window"
	// phaseWaitingForLogin is the live phase (/crawl-status) of a run that needs the person to log in and confirm.
	phaseWaitingForLogin crawlPhase = "waiting_for_login"
	// loginWindowCrawlTimeout bounds one single-page navigation and read in the window.
	loginWindowCrawlTimeout = 2 * time.Minute
)

// loginRequiredMessage is what a caller (and the person behind it) is told.
const loginRequiredMessage = "A login is needed: log in in the login window, press \"I am logged in\", then crawl again"

// loginRequiredAdvice is what a caller that is an AI is told to do (the text reaches the model through shared.CallSibling): it cannot log in
// and cannot open or confirm the window, so it must not retry in a loop but hand over to the person.
const loginRequiredAdvice = "Do not call a browser tool for this site again until the person has logged in: tell them the login window needs them (log in there, then press \"I am logged in\") and wait for their answer"

func newLoginRequiredError(reason string) *crawlError {
	return &crawlError{Code: codeLoginRequired, Message: loginRequiredMessage, Reason: reason, Advice: loginRequiredAdvice}
}

// isLoginRequired reports whether err is the login-required answer.
func isLoginRequired(err error) bool {
	var ce *crawlError
	return errors.As(err, &ce) && ce.Code == codeLoginRequired
}

// writeLoginWindowFailure answers a failed login-window call: login_required is a 409 with the error's JSON (a caller branches on the
// code and asks the person), everything else is the same 502 the other crawl routes use.
func writeLoginWindowFailure(w http.ResponseWriter, requestID, what string, err error) {
	if isLoginRequired(err) {
		setCrawlPhase(requestID, phaseWaitingForLogin, "waiting for the login to be confirmed")
		writeCrawlErrorJSON(w, http.StatusConflict, err)
		return
	}
	setCrawlPhase(requestID, phaseFailed, err.Error())
	var ce *crawlError
	if errors.As(err, &ce) {
		writeCrawlErrorJSON(w, http.StatusBadGateway, ce)
		return
	}
	http.Error(w, fmt.Sprintf("%s failed: %v", what, err), http.StatusBadGateway)
}

func writeCrawlErrorJSON(w http.ResponseWriter, status int, err error) {
	var ce *crawlError
	if !errors.As(err, &ce) {
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ce)
}

// acquireLoginWindowTab returns the run's confirmed window tab, one operation at a time, or a login_required error.
func acquireLoginWindowTab(reqCtx context.Context, requestID string) (context.Context, func(), error) {
	if requestID == "" || shared.LoginWindows == nil {
		return nil, nil, newLoginRequiredError("no_window")
	}
	tab, release, err := shared.LoginWindows.AcquireTab(reqCtx, requestID)
	if err == nil {
		return tab, release, nil
	}
	var lr *shared.LoginRequiredError
	if errors.As(err, &lr) {
		return nil, nil, newLoginRequiredError(lr.Reason)
	}
	if reqCtx.Err() != nil {
		return nil, nil, newCrawlCancelledError()
	}
	return nil, nil, err
}

// checkLoginWindowPage looks at the page in the window: a visible login form means the login is gone (the window goes back to waiting and
// the call answers login_required); a bot-check page is reported as such. It changes nothing on the page.
func checkLoginWindowPage(ctx context.Context, requestID string) error {
	page, err := shared.InspectPage(ctx)
	if err != nil {
		return err
	}
	if page.PasswordField {
		if shared.LoginWindows != nil {
			shared.LoginWindows.LoginLost(requestID)
		}
		return newLoginRequiredError("login_lost")
	}
	if page.Challenge {
		return &crawlError{Code: codeBotCheckInWindow, Message: "The page in the login window is a bot check: finish it in the window, then crawl again"}
	}
	return nil
}

// openPageInLoginWindow navigates the window's tab to rawURL and lets the page settle.
func openPageInLoginWindow(ctx context.Context, requestID, rawURL string) error {
	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL+" in the login window")
	return chromedp.Run(ctx, chromedp.Navigate(rawURL), chromedp.Sleep(SettleDelay))
}

// loginWindowCall runs fn in the run's window tab: registers the run for /crawl-cancel, takes the window for the call, bounds it by
// timeout and ends it when the caller goes away. Cancellation and a window the person closed are classified like in the other tabs.
func loginWindowCall(parent context.Context, requestID string, timeout time.Duration, fn func(ctx context.Context) error) error {
	reqCtx, reqCancel := context.WithCancel(parent)
	registerCrawlCancel(requestID, reqCancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		reqCancel()
	}()
	tab, release, err := acquireLoginWindowTab(reqCtx, requestID)
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(tab, timeout)
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, tab)
	return classifyCancellation(fn(ctx), reqCtx, tab)
}

// crawlInLoginWindow is /crawl in the run's window: navigate, check the page is not the login form, read the HTML.
func crawlInLoginWindow(parent context.Context, body crawlRequest) (crawlResponse, error) {
	var result crawlResponse
	err := loginWindowCall(parent, body.RequestID, loginWindowCrawlTimeout, func(ctx context.Context) error {
		if err := openPageInLoginWindow(ctx, body.RequestID, body.URL); err != nil {
			return err
		}
		if err := checkLoginWindowPage(ctx, body.RequestID); err != nil {
			return err
		}
		setCrawlPhase(body.RequestID, phaseExtracting, "reading the page")
		var rerr error
		result, rerr = readCrawlResponse(ctx, body.RemoveSelectors, body.RemoveAttributes, body.MaxAttributeLength, body.IgnoreAttributesForMaxLength)
		return rerr
	})
	return result, err
}

// performPaginatedCrawlInLoginWindow is /crawl-paginated in the run's window. With a url it navigates there first (after the portal's
// pacing delay, like every other tab); without one it works on the page the window already shows (the run's earlier /crawl). Every page
// is checked for the login form before it is extracted, so a login that ends in the middle of the listing stops the crawl with
// login_required instead of extracting the login page.
func performPaginatedCrawlInLoginWindow(parent context.Context, url string, args paginatedArgs) (paginatedCrawlResponse, []string, error) {
	var (
		result   paginatedCrawlResponse
		pageHTML []string
	)
	timeout := paginatedCrawlTimeout
	if args.rateLimitKey != "" {
		timeout = paginatedCrawlTimeoutWithPacing
	}
	if url != "" {
		awaitPortalCrawlPacing(parent, args.rateLimitKey)
		if parent.Err() != nil {
			return result, nil, newCrawlCancelledError()
		}
	}
	err := loginWindowCall(parent, args.requestID, timeout, func(ctx context.Context) error {
		signals := watchChallengeSignals(ctx)
		if url != "" {
			if err := openPageInLoginWindow(ctx, args.requestID, url); err != nil {
				return err
			}
		} else if err := ensurePageLoaded(ctx); err != nil {
			return err
		}
		var lerr error
		result, pageHTML, lerr = runPaginatedCrawlLoop(ctx, loopTab{
			signals:   signals,
			autoWait:  cloudflareAutoWait,
			checkPage: func(c context.Context) error { return checkLoginWindowPage(c, args.requestID) },
		}, args)
		return lerr
	})
	return result, pageHTML, err
}

// performFindRepeatingInLoginWindow is /find-repeating-items in the window a person opened for an AI conversation (plan 183, step 2): navigate
// there, check the page is not the login form (a login seen again answers login_required and puts the window back to waiting), then analyse the
// page exactly as the headless path does.
func performFindRepeatingInLoginWindow(parent context.Context, requestID string, req findRepeatingRequest) (findRepeatingResponse, error) {
	var resp findRepeatingResponse
	err := loginWindowCall(parent, requestID, loginWindowCrawlTimeout, func(ctx context.Context) error {
		if err := openPageInLoginWindowFn(ctx, requestID, req.URL); err != nil {
			return err
		}
		if err := checkLoginWindowPageFn(ctx, requestID); err != nil {
			return err
		}
		setCrawlPhase(requestID, phaseExtracting, "looking for the job list")
		var aerr error
		resp, aerr = analyzeCurrentPageFn(ctx, req.MinItems, req.MaxCandidates)
		return aerr
	})
	return resp, err
}

// The three browser steps of the function above, behind variables so a test can script them without a browser.
var (
	analyzeCurrentPageFn    = analyzeCurrentPage
	openPageInLoginWindowFn = openPageInLoginWindow
	checkLoginWindowPageFn  = checkLoginWindowPage
)

// extractInLoginWindow is /extract in the run's window: the fields are read from the page the window already shows.
func extractInLoginWindow(parent context.Context, requestID, container string, fields []extractField, mapping map[string]string) (extractResponse, error) {
	var result extractResponse
	err := loginWindowCall(parent, requestID, extractTimeout, func(ctx context.Context) error {
		if err := ensurePageLoaded(ctx); err != nil {
			return err
		}
		if err := checkLoginWindowPage(ctx, requestID); err != nil {
			return err
		}
		var eerr error
		result, eerr = runExtractionOnCurrentPage(ctx, container, fields, mapping)
		return eerr
	})
	return result, err
}
