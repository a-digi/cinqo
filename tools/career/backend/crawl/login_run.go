package crawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// login_run.go (plan 182, step 4) lets a crawl run of a link that needs a login wait for the person. The run opens the Browser's login
// window (a throw-away window the person logs in in, on the site's own page), waits in the phase "waiting_for_login" until the person has
// confirmed (the confirmation is a person's click, answered by the Browser; this run only polls the window's state), crawls in that window
// and closes it however the run ends. Nothing about the login is read or kept here: no password, no cookie, no session, no table. The run
// status stays "running" while it waits, so the one-running-run-per-link guard and the terminal-write guard apply unchanged.

const (
	// phaseWaitingForLogin is the phase of a run that waits for the person (the Browser reports the same word for its own wait).
	phaseWaitingForLogin = "waiting_for_login"
	// codeLoginRequired is the Browser's answer (HTTP 409) when the run has no confirmed window or the site asked for the login again.
	codeLoginRequired = "login_required"
	browserProxyBase  = "/api/v1/tools/browser/proxy/"
)

var (
	// loginStatusPollInterval is how often the run asks the Browser whether the person has confirmed.
	loginStatusPollInterval = 2 * time.Second
	// loginWaitLimit is how long one wait for the person may last (the same 30 minutes as the Browser's own limit for a waiting window).
	loginWaitLimit = 30 * time.Minute
	// maxLoginRewaits is how many times a run goes back to waiting after the site asked for the login again.
	maxLoginRewaits = 3
	// maxLoginStatusFailures is how many failed status polls in a row end the wait (the Browser is gone).
	maxLoginStatusFailures = 5
)

// ErrLoginNeeded is returned when an unattended crawl (auto discovery) is asked for a link that needs a login: nobody is there to log in.
var ErrLoginNeeded = errors.New("login needed: this link needs a person to log in, so it is not crawled unattended")

// loginRequiredError is the Browser's login_required answer, kept apart from every other failure.
type loginRequiredError struct{ Message string }

func (e *loginRequiredError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "a login is needed"
}

func isLoginRequiredErr(err error) bool {
	var lr *loginRequiredError
	return errors.As(err, &lr)
}

// loginWindowEndedError: the window ended before the person confirmed (cancelled, closed by the person, time limit, tool stopped).
type loginWindowEndedError struct{ Reason string }

func (e *loginWindowEndedError) Error() string {
	switch e.Reason {
	case "window_closed":
		return "The login window was closed before the login was confirmed"
	case "timed_out":
		return "The login was not confirmed in time, the login window was closed"
	case "tool_stopped":
		return "The Browser tool stopped, the login window was closed"
	case "cancelled":
		return "The login was cancelled"
	case "no_window":
		return "There is no login window for this run any more"
	}
	return "The login window ended before the login was confirmed"
}

// loginRun is one run's use of the Browser's login window.
type loginRun struct {
	coreURL, runID, token string
	// siteURL is the link's own address, loginURL the page to log in at ("" = the link's own address).
	siteURL, loginURL string
	rewaits           int
}

func (l *loginRun) call(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.coreURL+browserProxyBase+path, rd)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	resp, err := crawlNowHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

// open asks the Browser for the login window of this run. The run's id is the window's id.
func (l *loginRun) open(ctx context.Context) error {
	b, status, err := l.call(ctx, http.MethodPost, "login-window/open", map[string]string{"requestId": l.runID, "url": l.siteURL, "loginUrl": l.loginURL})
	if err != nil {
		return fmt.Errorf("could not open the login window: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("could not open the login window (%d): %s", status, strings.TrimSpace(string(b)))
	}
	return nil
}

// state reads the window's state: "waiting", "confirmed" or "ended" (with the reason). A window the Browser does not know is "ended".
func (l *loginRun) state(ctx context.Context) (state, reason string, err error) {
	b, status, err := l.call(ctx, http.MethodGet, "login-window/status?requestId="+url.QueryEscape(l.runID), nil)
	if err != nil {
		return "", "", err
	}
	if status == http.StatusNotFound {
		return "ended", "no_window", nil
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("login window status (%d): %s", status, strings.TrimSpace(string(b)))
	}
	var body struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return "", "", err
	}
	return body.State, body.Reason, nil
}

// waitConfirmed puts the run in the phase waiting_for_login and polls until the person has confirmed. It returns nil when confirmed,
// ctx's error when the run is stopped, a loginWindowEndedError when the window ended, and a plain error when the limit passed.
func (l *loginRun) waitConfirmed(ctx context.Context) error {
	_ = setCrawlRunPhase(l.runID, phaseWaitingForLogin, "waiting for you to log in in the login window and confirm")
	deadline := time.NewTimer(loginWaitLimit)
	defer deadline.Stop()
	ticker := time.NewTicker(loginStatusPollInterval)
	defer ticker.Stop()
	failures := 0
	for {
		state, reason, err := l.state(ctx)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			failures++
			if failures >= maxLoginStatusFailures {
				return fmt.Errorf("lost contact with the login window: %w", err)
			}
		default:
			failures = 0
			switch state {
			case "confirmed":
				return nil
			case "ended":
				return &loginWindowEndedError{Reason: reason}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("the login was not confirmed within %s", loginWaitLimit)
		case <-ticker.C:
		}
	}
}

// begin opens the window and waits for the first confirmation.
func (l *loginRun) begin(ctx context.Context) error {
	if err := l.open(ctx); err != nil {
		return err
	}
	return l.waitConfirmed(ctx)
}

// waitAgain is for a login_required answer in the middle of the crawl: back to waiting, a bounded number of times per run.
func (l *loginRun) waitAgain(ctx context.Context, cause error) error {
	if l.rewaits >= maxLoginRewaits {
		return fmt.Errorf("the site asked for the login again too often: %w", cause)
	}
	l.rewaits++
	_ = appendCrawlRunLog(l.runID, fmt.Sprintf("the site asks for the login again (%d/%d): %v", l.rewaits, maxLoginRewaits, cause))
	return l.waitConfirmed(ctx)
}

// close ends the window. Best effort, on its own short context so a stopped run still closes it; an unknown window is not an error.
func (l *loginRun) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _, _ = l.call(ctx, http.MethodPost, "login-window/cancel", map[string]string{"requestId": l.runID})
}
