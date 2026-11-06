// katana_engine.go (plan 184) — the Chrome behind the Katana fetch engine.
//
// Katana (projectdiscovery/katana, hybrid mode) is a crawler that drives a
// headless Chrome over CDP. This tool uses it as ONE more fetch engine for
// the primary single-page fetch (fetch_page_html), next to rod.
// Katana is never allowed to start a Chrome of its own: its own
// launcher ignores TLS errors, may download a Chromium and keeps a temp
// profile. Instead THIS file starts our own Chrome — with the very flags
// rod's engine uses (newRodLauncher), the same Chrome resolver
// (shared.ChromePath) and the same user-agent derivation — and Katana
// attaches to it over its websocket address (Options.ChromeWSUrl). Katana
// then makes an incognito browser context inside it and, when it is closed,
// disposes only that context and its socket; it never touches our process.
//
// The browser is shared per process and started lazily, exactly like the rod
// engine's; StopKatanaEngine ends it (shutdown, and when the engine is
// switched away). See katana_fetch.go for the fetch itself.
package crawler

import (
	"fmt"
	"log"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

var (
	katanaMu       sync.Mutex
	katanaBrowser  *rod.Browser // our connection to the Chrome: version and liveness only
	katanaLauncher *launcher.Launcher
	katanaWSURL    string // the address Katana attaches to
)

// launchKatanaChrome starts one headless Chrome with the shared flags and
// connects to it. userAgent "" means Chrome's own.
func launchKatanaChrome(chromePath, userAgent string) (*rod.Browser, *launcher.Launcher, string, error) {
	l := newRodLauncher(chromePath, userAgent)
	ws, err := l.Launch()
	if err != nil {
		l.Kill()
		l.Cleanup()
		return nil, nil, "", fmt.Errorf("katana engine: failed to launch Chrome: %w", err)
	}
	// NoDefaultDevice for the same reason as launchRodBrowser: no CDP
	// user-agent override on pages created through this connection.
	b := rod.New().ControlURL(ws).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		l.Kill()
		l.Cleanup()
		return nil, nil, "", fmt.Errorf("katana engine: failed to connect to Chrome: %w", err)
	}
	return b, l, ws, nil
}

// startKatanaChromeLocked launches Chrome the way startRodBrowserLocked
// does: Chrome's native user agent is read, and when it says
// "HeadlessChrome/" the browser is relaunched once with only that token
// rewritten (a flag, never a CDP override). Assumes katanaMu is held.
func startKatanaChromeLocked() error {
	path, err := rodChromePath() // the ONE resolver; never a download
	if err != nil {
		return err
	}
	b, l, ws, err := launchKatanaChrome(path, "")
	if err != nil {
		return err
	}
	v, err := rodBrowserVersion(b)
	if err != nil {
		closeRodBrowser(b, l)
		return fmt.Errorf("katana engine: Chrome did not answer: %w", err)
	}
	if derived := deriveRodUserAgent(v.UserAgent); derived != v.UserAgent {
		closeRodBrowser(b, l)
		b, l, ws, err = launchKatanaChrome(path, derived)
		if err != nil {
			return err
		}
	}
	katanaBrowser, katanaLauncher, katanaWSURL = b, l, ws
	return nil
}

func stopKatanaChromeLocked() {
	closeRodBrowser(katanaBrowser, katanaLauncher)
	katanaBrowser, katanaLauncher, katanaWSURL = nil, nil, ""
}

// ensureKatanaEngine returns the websocket address of the shared Chrome,
// starting it on first use and restarting it when it has died (proven by a
// cheap CDP round trip, so a caller is never handed a dead address).
func ensureKatanaEngine() (string, error) {
	katanaMu.Lock()
	defer katanaMu.Unlock()
	if katanaBrowser != nil {
		if _, err := rodBrowserVersion(katanaBrowser); err == nil {
			return katanaWSURL, nil
		}
		log.Printf("katana engine: its Chrome is unresponsive, restarting")
		stopKatanaChromeLocked()
	}
	if err := startKatanaChromeLocked(); err != nil {
		return "", err
	}
	return katanaWSURL, nil
}

// StopKatanaEngine ends the shared Chrome. A no-op when it was never
// started; safe to call repeatedly. A later fetch starts a fresh one.
func StopKatanaEngine() {
	katanaMu.Lock()
	defer katanaMu.Unlock()
	stopKatanaChromeLocked()
}

// katanaRunning reports whether the engine's Chrome is up (tests).
func katanaRunning() bool {
	katanaMu.Lock()
	defer katanaMu.Unlock()
	return katanaBrowser != nil
}
