package shared

import (
	"context"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared/chromeproc"
)

// chromedpPID is the process id of the Chrome a chromedp context drives; 0 when it cannot be told (the browser has not started yet).
func chromedpPID(ctx context.Context) int {
	c := chromedp.FromContext(ctx)
	if c == nil || c.Browser == nil {
		return 0
	}
	p := c.Browser.Process()
	if p == nil {
		return 0
	}
	return p.Pid
}

// registerChromedpChrome records the Chrome of a chromedp context in the registry of Chrome processes (plan 224) and returns the cancel
// functions with one more at the end that releases it, so every existing stop path (cancelAll) also releases it, with no new call sites.
func registerChromedpChrome(kind chromeproc.Kind, ctx context.Context, profileDir string, cancels []context.CancelFunc) []context.CancelFunc {
	pid := chromedpPID(ctx)
	if pid <= 0 {
		return cancels
	}
	chromeproc.Register(chromeproc.Launch{Kind: kind, PID: pid, ProfileDir: profileDir})
	return append(cancels, func() { chromeproc.Release(pid) })
}
