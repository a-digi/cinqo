// rod_engine.go (step 49) — an OPT-IN, pure-Go alternative to
// playwright_engine.go for the primary single-page fetch: go-rod/rod
// drives Chrome over CDP directly, with no Node driver. This file holds
// the browser LIFECYCLE only (explicit Chrome path, launch flags,
// user-agent derivation, liveness, shutdown); fetchWithRod itself and the
// engine switch come in later steps. playwright_engine.go is not touched
// by any of it. See plan/ai/tools/browser/step-49-rod-fetch-engine.md.
//
// One shared headless Chrome per process, started lazily — mirrors
// shared.EnsureSharedSession and ensurePlaywrightEngine. A page per call
// (not a shared tab) is what the later fetch step will use, so, like the
// Playwright engine, this needs no tab-pool machinery of its own.
package crawler

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// errRodNoChrome: no Chrome binary could be found. A plain error — never
// a *crawlError — so a caller treats it as "this engine is unavailable"
// and falls back to the chromedp path, exactly as for a Playwright start
// failure.
var errRodNoChrome = errors.New("rod engine: no Chrome/Chromium binary found (set BROWSER_TOOL_CHROME_PATH)")

// rodLivenessTimeout bounds the cheap CDP round trip that proves the
// shared browser is still alive before a caller is handed it.
const rodLivenessTimeout = 5 * time.Second

// rodLookPath is launcher.LookPath, a var so a test can simulate a
// machine with no browser installed.
var rodLookPath = launcher.LookPath

var (
	rodMu       sync.Mutex
	rodBrowser  *rod.Browser
	rodLauncher *launcher.Launcher

	// rodUserAgent caches the UA the browser is launched with: its OWN
	// native UA with only "HeadlessChrome/" rewritten to "Chrome/". Empty
	// until the first launch derives it. rodUAFlag records whether that
	// differs from the native one, i.e. whether launches must pass it as
	// --user-agent. A flag, deliberately NOT a CDP override: a CDP
	// override wipes Sec-CH-UA/navigator.userAgentData unless the full
	// metadata is supplied, and never reaches a cross-site iframe (where
	// Turnstile runs) — the same measured reasoning as
	// shared.headlessUserAgent. Guarded by rodMu.
	rodUserAgent string
	rodUAFlag    bool
)

// rodChromePath resolves the Chrome binary: BROWSER_TOOL_CHROME_PATH if
// set (and it must exist), else rod's own search of the usual install
// locations. Never empty on success — an empty path would make rod's
// launcher DOWNLOAD a Chromium of its own, which this tool must never do.
func rodChromePath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("BROWSER_TOOL_CHROME_PATH")); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("rod engine: BROWSER_TOOL_CHROME_PATH %q is not usable: %w", p, err)
		}
		return p, nil
	}
	if p, ok := rodLookPath(); ok && p != "" {
		return p, nil
	}
	return "", errRodNoChrome
}

// deriveRodUserAgent is deriveHeadlessUserAgent (shared/session.go): the
// native UA with only the "HeadlessChrome/" token rewritten.
func deriveRodUserAgent(nativeUA string) string {
	return strings.Replace(nativeUA, "HeadlessChrome/", "Chrome/", 1)
}

// rodMajorVersion extracts the Chrome major version from a product
// string ("Chrome/153.0.8010.53" or "HeadlessChrome/153...") or a full UA;
// "" when there is no Chrome token.
func rodMajorVersion(s string) string {
	i := strings.Index(s, "Chrome/")
	if i < 0 {
		return ""
	}
	v := s[i+len("Chrome/"):]
	if j := strings.IndexByte(v, '.'); j >= 0 {
		v = v[:j]
	}
	return v
}

// launchRodBrowser starts a fresh headless Chrome and connects rod to it.
// The flags mirror shared.allocatorOptions (the chromedp headless
// session) so every engine presents the same browser: new headless,
// AutomationControlled blink feature off, enable-automation off (rod's
// launcher turns it on by default), infobars and notifications off, an
// explicit language, and — when userAgent is set — the UA as a flag.
// The CDP port is rod's default (a random one chosen by Chrome, bound to
// localhost). The Chrome binary is always given explicitly, so rod never
// downloads one.
func launchRodBrowser(chromePath, userAgent string) (*rod.Browser, *launcher.Launcher, error) {
	l := launcher.New().
		Bin(chromePath).
		Set(flags.Headless, "new").
		Set("disable-blink-features", "AutomationControlled").
		Delete("enable-automation").
		Set("disable-infobars").
		Set("disable-notifications").
		Set("lang", "en-US,en;q=0.9").
		// --lang alone does not change the Accept-Language header on every
		// OS (on macOS the system locale wins, so a German machine sent
		// "de-DE,de;q=0.9" while stealth.JS reports navigator.languages as
		// en-US — a header/JS mismatch). --accept-lang sets the header
		// itself, so the two agree everywhere.
		Set("accept-lang", "en-US,en")
	if userAgent != "" {
		l.Set("user-agent", userAgent)
	}

	wsURL, err := l.Launch()
	if err != nil {
		l.Kill()
		l.Cleanup()
		return nil, nil, fmt.Errorf("rod engine: failed to launch Chrome: %w", err)
	}
	// NoDefaultDevice is essential: rod otherwise emulates a built-in
	// "laptop" device on EVERY new page, which is a CDP user-agent
	// override (a fixed, stale "Chrome/87 ... Mac OS X 11_0_0" UA). That is
	// exactly the override this tool avoids (see rodUserAgent): it
	// disagrees with Sec-CH-UA and navigator.userAgentData, and never
	// reaches a cross-site iframe. Measured in
	// TestRodBrowserPresentsAHumanFingerprint.
	b := rod.New().ControlURL(wsURL).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		l.Kill()
		l.Cleanup()
		return nil, nil, fmt.Errorf("rod engine: failed to connect to Chrome: %w", err)
	}
	return b, l, nil
}

// closeRodBrowser shuts a browser down and removes its temp profile.
// Best effort: a browser that already died must not make this fail.
func closeRodBrowser(b *rod.Browser, l *launcher.Launcher) {
	if b != nil {
		_ = b.Close()
	}
	if l != nil {
		l.Kill()
		l.Cleanup()
	}
}

func rodBrowserVersion(b *rod.Browser) (*proto.BrowserGetVersionResult, error) {
	return proto.BrowserGetVersion{}.Call(b.Timeout(rodLivenessTimeout))
}

// startRodBrowserLocked launches the shared browser, deriving the user
// agent the same way shared.startSharedSessionLocked does: use the cached
// one if the browser's major version still matches it; otherwise launch
// with Chrome's native UA, read it, and relaunch once with the rewritten
// one if it contained "HeadlessChrome/". Assumes rodMu is held.
func startRodBrowserLocked() error {
	path, err := rodChromePath()
	if err != nil {
		return err
	}

	adopt := func(b *rod.Browser, l *launcher.Launcher) {
		rodBrowser, rodLauncher = b, l
	}

	if rodUserAgent != "" {
		ua := ""
		if rodUAFlag {
			ua = rodUserAgent
		}
		b, l, err := launchRodBrowser(path, ua)
		if err != nil {
			return err
		}
		v, err := rodBrowserVersion(b)
		if err != nil {
			closeRodBrowser(b, l)
			return err
		}
		if rodMajorVersion(v.Product) == rodMajorVersion(rodUserAgent) {
			adopt(b, l)
			return nil
		}
		// Chrome auto-updated underneath this long-running process: a
		// stale cached UA would recreate the UA/client-hint mismatch it
		// exists to prevent, so re-derive.
		log.Printf("rod engine: browser is now %s, re-deriving user agent (cached: %q)", v.Product, rodUserAgent)
		closeRodBrowser(b, l)
		rodUserAgent, rodUAFlag = "", false
	}

	b, l, err := launchRodBrowser(path, "")
	if err != nil {
		return err
	}
	v, err := rodBrowserVersion(b)
	if err != nil {
		closeRodBrowser(b, l)
		return err
	}
	derived := deriveRodUserAgent(v.UserAgent)
	if derived == v.UserAgent {
		rodUserAgent, rodUAFlag = v.UserAgent, false
		adopt(b, l)
		return nil
	}

	closeRodBrowser(b, l)
	b, l, err = launchRodBrowser(path, derived)
	if err != nil {
		return err
	}
	rodUserAgent, rodUAFlag = derived, true
	adopt(b, l)
	return nil
}

// stopRodBrowserLocked tears the shared browser down. Assumes rodMu is held.
func stopRodBrowserLocked() {
	closeRodBrowser(rodBrowser, rodLauncher)
	rodBrowser, rodLauncher = nil, nil
}

// ensureRodEngine returns the shared browser, starting it on first use and
// restarting it if it has died (crashed, killed, or its CDP connection
// dropped) — proven by a cheap CDP round trip, so a caller is never handed
// a dead handle.
func ensureRodEngine() (*rod.Browser, error) {
	rodMu.Lock()
	defer rodMu.Unlock()

	if rodBrowser != nil {
		if _, err := rodBrowserVersion(rodBrowser); err == nil {
			return rodBrowser, nil
		}
		log.Printf("rod engine: shared browser is unresponsive, restarting")
		stopRodBrowserLocked()
	}
	if err := startRodBrowserLocked(); err != nil {
		return nil, err
	}
	return rodBrowser, nil
}

// StopRodEngine shuts the shared browser down — to be called once at
// process exit (wired in a later step), mirroring StopPlaywrightEngine. A
// no-op if the engine was never started; safe to call repeatedly. A later
// ensureRodEngine starts a fresh browser.
func StopRodEngine() {
	rodMu.Lock()
	defer rodMu.Unlock()
	stopRodBrowserLocked()
}

// rodCurrentUserAgent reports the cached derived UA ("" before the first
// launch). For tests and diagnostics.
func rodCurrentUserAgent() string {
	rodMu.Lock()
	defer rodMu.Unlock()
	return rodUserAgent
}
