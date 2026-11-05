package shared

import (
	"context"

	"github.com/chromedp/chromedp"
)

// loginwindow_access.go (plan 182, step 2; plan 183, step 1: AIWindow) is how the crawler reaches a run's login window without importing the package that owns it
// (auth imports crawler, so the crawler cannot import auth): auth registers the real access here at start-up, the crawler only asks.

// LoginRequiredError says a run has no usable login window: the person has not logged in and confirmed yet, or the window ended, or the
// site showed its login page again in the middle of a crawl. Reason is one of "no_window", "waiting", "ended" or "login_lost".
type LoginRequiredError struct{ Reason string }

func (e *LoginRequiredError) Error() string { return "login required: " + e.Reason }

// LoginWindowAccess is what the crawler may do with a run's login window.
type LoginWindowAccess interface {
	// AcquireTab waits (until ctx ends) for the run's window to be free and returns its tab. One operation at a time works in a window.
	// *LoginRequiredError when the run has no CONFIRMED window. release must be called when the operation is done.
	AcquireTab(ctx context.Context, requestID string) (tab context.Context, release func(), err error)
	// LoginLost puts the run's confirmed window back to "waiting": the site showed its login page again, so the person has to log in
	// again in the same window and confirm again.
	LoginLost(requestID string)
	// AIWindow says whether a window exists under id that a person opened for the AI to use (aiMayUse), and for which site. Used only to decide
	// where an AI tool call runs; whether the window is confirmed is decided by AcquireTab as for any run.
	AIWindow(id string) AIWindowInfo
}

// AIWindowInfo describes a window an AI conversation may work in (plan 183). Domain is the lower-case host the window was opened for.
type AIWindowInfo struct {
	Exists   bool
	AIMayUse bool
	Domain   string
}

// LoginWindows is set by package auth when it is loaded. Nil in a process that never loads it (the --mcp adapter): then no run has a window.
var LoginWindows LoginWindowAccess

// PageInspection is what a look at the open page tells: a visible password field (the person is at a login form), a bot-check page,
// and how much visible text there is.
type PageInspection struct {
	PasswordField bool `json:"passwordField"`
	Challenge     bool `json:"challenge"`
	TextLength    int  `json:"textLength"`
}

const inspectPageJS = `(function () {
  var visible = function (el) {
    var r = el.getBoundingClientRect();
    var s = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none';
  };
  var pw = Array.prototype.some.call(document.querySelectorAll('input[type="password"]'), visible);
  var title = (document.title || '').toLowerCase();
  var challenge = /just a moment|nur einen moment|un instant|attention required|checking your browser/.test(title);
  var text = document.body ? (document.body.innerText || '').trim().length : 0;
  return { passwordField: pw, challenge: challenge, textLength: text };
})()`

// InspectPage reads the page currently loaded in ctx's tab; it never navigates and changes nothing.
func InspectPage(ctx context.Context) (PageInspection, error) {
	var out PageInspection
	if err := chromedp.Run(ctx, chromedp.Evaluate(inspectPageJS, &out)); err != nil {
		return PageInspection{}, err
	}
	return out, nil
}
