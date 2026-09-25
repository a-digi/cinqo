// Package shared holds the Browser tool's own infrastructure that both
// the login/extract-adjacent main package and the crawler package
// depend on — the one shared headless browser session, the tool's own
// SQLite handle, and the sibling-HTTP-call helper the --mcp adapter
// uses. Carved out of main.go/login_credentials.go specifically so the
// crawler package (crawl.go, paginate.go, extract.go, and friends) can
// be its own package without importing main (which Go forbids, since
// main already imports crawler for HTTP/MCP route registration).
//
// This file holds the shared headless session itself — originally
// main.go's own architecture (step 2,
// plan/ai/tools/browser/step-02-shared-browser-session.md). Since the
// browser-pool step, "the shared session" is ONE headless browser
// process (browserCtx, tabs.go) holding a primary tab (Ctx, below) for
// every caller that depends on "whatever page is currently loaded",
// plus short-lived worker tabs (AcquireWorkerTab, tabs.go) for
// self-contained URL operations; the headed fallback browser lives in
// headed.go.
package shared

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Mu guards every access to Ctx — the PRIMARY tab: every HTTP handler
// that touches the shared page must hold this for the duration of its
// own chromedp actions, so two requests never race on the same tab.
// Worker tabs (tabs.go) are never guarded by Mu — each belongs to
// exactly one request. Ctx itself is
// nil until EnsureSharedSession's first successful call (lazy start,
// step-43-lazy-shared-session.md) — every handler that needs it calls
// EnsureSharedSession first, never assumes it's already set.
var (
	Mu  sync.Mutex
	Ctx context.Context
	// primaryCancel closes the primary tab only (never the browser) —
	// see RecreateSharedSessionLocked. Guarded by Mu.
	primaryCancel context.CancelFunc
	// LastHeadlessFetchURL is the exact URL string fetch_page_html last
	// successfully navigated the shared headless session to —
	// crawlPage's own fetch-cache check (fetch_cache.go, step 45) uses
	// it to tell "the shared session is already showing this URL"
	// (safe to serve a cached result with zero browser work) apart
	// from "it's showing something else" (must navigate for real
	// regardless of cache freshness). Correct because crawlPage is the
	// *only* thing that ever navigates this shared session anywhere —
	// verified directly, not assumed (grep confirms exactly one
	// caller of crawlPage, and every other MCP tool in this process
	// only ever reads the page the session is already on). Guarded by
	// Mu, same as Ctx itself. Never reset on a normal navigation, but
	// explicitly cleared to "" whenever Ctx itself is replaced
	// (RecreateSharedSessionLocked, step 47.3/63.3 — a wedged-tab
	// recovery is the one in-process session-relaunch path that
	// exists; see that function's own doc comment) — a fresh chromedp
	// context can never already be showing whatever URL this field
	// last held. See
	// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
	LastHeadlessFetchURL string
)

// EnsureSharedSession lazily starts the shared headless session on
// first actual need — replacing this tool's previous behavior of
// launching it unconditionally at process boot (before this process
// ever reported itself healthy), regardless of whether any browser
// feature was ever going to be used. A no-op once already started, so
// every call site below can call this unconditionally on every
// request at negligible cost (a single mutex lock/unlock) once warm.
// Must be called BEFORE the caller's own Mu.Lock() for its actual
// crawl/login/extract operation — this function fully acquires and
// releases Mu itself, so calling it while already holding Mu would
// deadlock (sync.Mutex isn't reentrant). See
// plan/ai/tools/browser/step-43-lazy-shared-session.md.
//
// A primary tab whose context is already done (the browser process died
// or was killed underneath this process) counts as "not started": it is
// torn down and relaunched here instead of being handed out dead.
func EnsureSharedSession() error {
	Mu.Lock()
	defer Mu.Unlock()
	if Ctx != nil && Ctx.Err() == nil {
		return nil
	}
	stopHeadlessLocked()
	return startSharedSessionLocked()
}

// installDialogAutoDismiss (step 47.1) auto-accepts any native
// alert()/confirm()/beforeunload dialog that appears on ctx's own
// page, for the lifetime of ctx. A native dialog freezes its target's
// JS engine and CDP responsiveness while it's open — and since every
// caller of the shared headless session (Ctx) or the headed fallback
// session inherits whatever state the previous caller left the tab in
// (Mu and the tab pools only serialize access, they never reset a tab
// — see
// plan/ai/tools/browser/step-47-shared-tab-wedge-and-stuck-crawl-fix.md's
// own root-cause writeup), an unhandled dialog left open by one caller
// would otherwise wedge every subsequent caller indefinitely, with no
// way to recover short of restarting this whole process. Registered
// once, at session creation — not per request — so it stays armed for
// that session's entire lifetime. The actual dismissal runs in its own
// goroutine because chromedp.ListenTarget's callback runs synchronously
// on the CDP event-read loop; blocking it on another chromedp.Run call
// (page.HandleJavaScriptDialog needs the browser to respond) would
// deadlock that loop. label distinguishes the shared headless session
// from the headed fallback in logs.
func installDialogAutoDismiss(ctx context.Context, label string) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			go func() {
				if err := chromedp.Run(ctx, page.HandleJavaScriptDialog(true)); err != nil {
					log.Printf("%s: failed to auto-dismiss JS dialog: %v", label, err)
				}
			}()
		}
	})
}

// sessionLivenessProbeTimeout (step 47.2) bounds how long
// ProbeSessionLiveness is allowed to take before concluding the shared
// tab is wedged. Short and fixed since a healthy tab evaluates a
// trivial expression near-instantly — anything past a few seconds
// already means something is wrong, not just "a bit slow."
const sessionLivenessProbeTimeout = 5 * time.Second

// ProbeSessionLiveness (step 47.2) runs a trivial, non-navigating,
// non-mutating JS evaluation against parentCtx (the shared session's
// own Ctx) to confirm the tab is actually still responsive BEFORE a
// caller starts its own real work — every caller of the shared session
// inherits whatever state the previous caller left the tab in (Mu only
// serializes access, it never resets the tab — see
// installDialogAutoDismiss's own doc comment and
// plan/ai/tools/browser/step-47-shared-tab-wedge-and-stuck-crawl-fix.md),
// so a wedge left behind by one caller (a still-in-flight navigation,
// an unusually slow script, or anything else that leaves the renderer
// unresponsive) would otherwise only be discovered by the *next*
// unrelated caller after it burns its own full — often much longer —
// timeout on real work that was never going to complete either way.
//
// Deliberately does NOT navigate: findLoginElements, performExtraction,
// and performLogin all operate on whatever page the shared session
// already has loaded and must never have that page changed out from
// under them, so a Navigate-based reset (which would otherwise double
// as a liveness check) is not an option for a probe shared across
// every Mu-guarded caller. A trivial chromedp.Evaluate leaves the page
// completely untouched while still exercising the exact same CDP
// round-trip (Runtime.evaluate) that would hang if the tab itself were
// wedged.
func ProbeSessionLiveness(parentCtx context.Context) error {
	ctx, cancel := context.WithTimeout(parentCtx, sessionLivenessProbeTimeout)
	defer cancel()
	var discard int
	if err := chromedp.Run(ctx, chromedp.Evaluate("1", &discard)); err != nil {
		return fmt.Errorf("shared browser session appears unresponsive: %w", err)
	}
	return nil
}

// headlessUserAgent caches the User-Agent string startSharedSessionLocked
// launches the shared headless session with — the browser's OWN native
// UA with only the "HeadlessChrome/" token rewritten to "Chrome/"
// (deriveHeadlessUserAgent). Empty until the first launch derives it.
// Guarded by Mu, same as Ctx, since only startSharedSessionLocked (which
// already requires Mu) ever reads or writes it.
//
// Replaces a hard-coded "Windows NT 10.0 ... Chrome/120" UA flag — a
// real, measured fingerprint mismatch, not a hypothetical one (captured
// directly against a local test server, Chrome 153 on macOS): the
// --user-agent flag only changes the User-Agent header and
// navigator.userAgent, while Sec-CH-UA/Sec-CH-UA-Platform and
// navigator.userAgentData/navigator.platform kept reporting the REAL
// browser ("Google Chrome";v="153", "macOS", "MacIntel") — a Windows,
// 33-versions-old UA sitting next to macOS/153 client hints, exactly
// the kind of internal inconsistency Cloudflare's bot scoring flags,
// pushing otherwise-silent managed challenges into interactive ones.
//
// Deliberately still a --user-agent FLAG (not a CDP
// Emulation.setUserAgentOverride call per tab): also measured directly —
// a CDP override (a) wipes Sec-CH-UA and navigator.userAgentData.brands
// entirely unless full UserAgentMetadata is supplied too, and (b) never
// reaches a cross-site iframe (an out-of-process frame with its own
// target), which kept reporting "HeadlessChrome/153" — and a cross-site
// iframe is exactly where Cloudflare's Turnstile widget runs. The flag
// applies process-wide, every frame included, and leaves the browser's
// own (already-correct) client hints untouched.
var headlessUserAgent string

// startSharedSessionLocked creates the one chromedp browser context
// this whole process holds for its entire lifetime — assumes the
// caller (EnsureSharedSession, above) already holds Mu. A real, empty
// navigation (not just allocator/context creation) forces the browser
// to actually launch now rather than deferring even further, so a
// genuinely live browser (not just constructed Go-side handles) is
// confirmed before this returns successfully.
//
// The UA is derived from the browser itself (see headlessUserAgent), so
// the very first launch in this process has nothing to pass yet: it
// launches with Chrome's native UA, reads it, and — only if it actually
// contains the "HeadlessChrome/" token — relaunches once with the
// rewritten one. Every later launch (e.g. RecreateSharedSessionLocked)
// reuses the cached value directly, unless the browser's real major
// version no longer matches it (Chrome auto-updated underneath this
// long-running process), in which case it's re-derived the same way —
// a stale cached version would recreate the very UA/client-hint
// mismatch this exists to prevent.
func startSharedSessionLocked() error {
	if headlessUserAgent != "" {
		ctx, cancels, product, err := launchHeadlessBrowser(headlessUserAgent)
		if err != nil {
			return err
		}
		if majorVersion(product) == majorVersion(headlessUserAgent) {
			return adoptBrowserLocked(ctx, cancels)
		}
		log.Printf("shared headless session: browser is now %s, re-deriving user agent (cached: %q)", product, headlessUserAgent)
		cancelAll(cancels)
		headlessUserAgent = ""
	}

	ctx, cancels, _, err := launchHeadlessBrowser("")
	if err != nil {
		return err
	}
	var nativeUA string
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		_, _, _, nativeUA, _, err = browser.GetVersion().Do(ctx)
		return err
	})); err != nil {
		cancelAll(cancels)
		return err
	}

	headlessUserAgent = deriveHeadlessUserAgent(nativeUA)
	if headlessUserAgent == nativeUA {
		return adoptBrowserLocked(ctx, cancels)
	}

	cancelAll(cancels)
	ctx, cancels, _, err = launchHeadlessBrowser(headlessUserAgent)
	if err != nil {
		return err
	}
	return adoptBrowserLocked(ctx, cancels)
}

// adoptBrowserLocked publishes a freshly launched browser (root ctx +
// its cancel funcs) as the shared one and opens the primary tab in it.
// On a primary-tab failure the browser is torn down again, leaving
// everything nil for the next EnsureSharedSession to retry from scratch.
func adoptBrowserLocked(root context.Context, cancels []context.CancelFunc) error {
	setBrowser(root, cancels)
	return openPrimaryTabLocked()
}

// openPrimaryTabLocked opens a fresh primary tab in the current browser.
func openPrimaryTabLocked() error {
	root, _ := currentBrowser()
	ctx, cancel, err := newTab(root, "shared headless session (primary tab)")
	if err != nil {
		stopHeadlessLocked()
		return err
	}
	Ctx, primaryCancel = ctx, cancel
	return nil
}

// launchHeadlessBrowser starts one headless Chrome process and returns
// its ROOT context (the process's own first tab, which is never used
// for pages — every real tab is opened from it via newTab, which also
// installs the stealth script and dialog auto-dismiss per tab), its
// cancel funcs, and the browser's
// real product string ("Chrome/153.0.8010.53") — the only reliable
// version source once userAgent is set, since Browser.getVersion's own
// userAgent field then just echoes the flag back. userAgent "" launches
// with Chrome's native UA.
func launchHeadlessBrowser(userAgent string) (context.Context, []context.CancelFunc, string, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions(userAgent)...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	cancels := []context.CancelFunc{ctxCancel, allocCancel}

	var product string
	actions := []chromedp.Action{
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			_, product, _, _, _, err = browser.GetVersion().Do(ctx)
			return err
		}),
	}

	if err := chromedp.Run(ctx, actions...); err != nil {
		cancelAll(cancels)
		return nil, nil, "", err
	}
	return ctx, cancels, product, nil
}

// deriveHeadlessUserAgent rewrites headless Chrome's native UA into the
// one the same browser reports when headed — verified directly (Chrome
// 153, macOS) to differ ONLY in the "HeadlessChrome/" product token;
// platform, version and client hints are already identical between the
// two modes. Returns nativeUA unchanged when the token isn't present.
func deriveHeadlessUserAgent(nativeUA string) string {
	return strings.Replace(nativeUA, "HeadlessChrome/", "Chrome/", 1)
}

// majorVersion extracts the Chrome major version from either a product
// string ("Chrome/153.0.8010.53") or a full UA ("... Chrome/153.0.0.0
// Safari/537.36") — "" when neither contains a "Chrome/" token.
func majorVersion(s string) string {
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

func cancelAll(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

// RecreateSharedSessionLocked (step 47.3) replaces a wedged primary
// tab while the caller still holds Mu, so no other request can observe
// or acquire the wedged tab in between. Called after
// ProbeSessionLiveness (step 47.2) found the primary tab unresponsive,
// and by navigateWithHangGuard (crawl.go) when a navigation outlives its
// own deadline. Assumes the caller already holds Mu.
//
// Replaces ONLY the primary tab when the browser process itself still
// answers (probeBrowser) — closing a tab is a browser-level CDP command,
// which works even when that tab's own renderer is hung, and it leaves
// every worker tab (tabs.go) running in the same process untouched.
// Only when the browser itself doesn't answer (or opening the new tab
// fails) does this fall back to a full relaunch, which also ends every
// in-flight worker tab (their requests fail as
// browser_session_interrupted and can be retried).
//
// If the fresh start itself fails, Ctx is left nil rather than
// pointing back at the torn-down session — matching
// EnsureSharedSession's own lazy-start convention, so the next caller
// attempts a completely fresh start instead of being handed a session
// that no longer exists.
func RecreateSharedSessionLocked() error {
	// step 63.3 — without this, fetch_cache.go's own "is the shared
	// session already showing this URL" check (crawlPage, crawl.go)
	// would still see whatever URL the OLD tab was last navigated to —
	// wrongly believing the brand new, blank tab is already showing it,
	// and serving a stale cached result instead of ever navigating the
	// new tab there at all.
	LastHeadlessFetchURL = ""

	if primaryCancel != nil {
		primaryCancel()
	}
	Ctx, primaryCancel = nil, nil

	if root, _ := currentBrowser(); root != nil && probeBrowser(root) == nil {
		err := openPrimaryTabLocked()
		if err == nil {
			return nil
		}
		log.Printf("shared headless session: replacing the primary tab failed (%v) — relaunching the browser", err)
	}
	stopHeadlessLocked()
	return startSharedSessionLocked()
}

// StopSharedSession shuts down both browsers — the headless one (its
// primary tab and every worker tab with it) and the headed fallback
// browser, if one is open.
func StopSharedSession() {
	Mu.Lock()
	stopHeadlessLocked()
	Mu.Unlock()
	StopHeadedBrowser()
}

// stopHeadlessLocked tears down the headless browser process (and with
// it the primary tab and every worker tab). Assumes Mu is held.
func stopHeadlessLocked() {
	if primaryCancel != nil {
		primaryCancel()
	}
	Ctx, primaryCancel = nil, nil
	LastHeadlessFetchURL = ""
	clearBrowser()
}

// normalSessionProfileDir resolves (and ensures exists) the persistent
// Chrome profile directory the headed fallback session reuses across
// every invocation — under TOOL_DB_DIR, this tool's own established
// convention for durable, tool-owned storage (see InitDB, db.go). A
// REAL, persistent profile — not the fresh temporary one chromedp
// creates by default when no UserDataDir is given — so cookies/local
// storage (in particular, whatever cf_clearance cookie a human
// manually earns by solving a challenge once) survive into the next
// fallback attempt against the same site, and the window looks and
// behaves like an ordinary standing Chrome profile a human recognizes,
// not a blank "private"-feeling one. Safe only because headed.go keeps
// at most one headed Chrome process running at a time (its tabs are
// what run concurrently) — two Chrome processes sharing one
// user-data-dir concurrently would conflict. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func normalSessionProfileDir() (string, error) {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return "", fmt.Errorf("TOOL_DB_DIR is not set")
	}
	dir := filepath.Join(dbDir, "normal-chrome-profile")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create normal-session Chrome profile directory: %w", err)
	}
	return dir, nil
}

// removeStaleChromeSingletonLock deletes Chrome's own SingletonLock/
// SingletonCookie/SingletonSocket files from profileDir before a new
// launch — a real, reproduced bug fix, not a hypothetical: Chrome's
// own singleton-instance protection is normally self-healing (it
// detects a dead owner via SingletonSocket and removes a stale lock
// itself), but that self-healing races against the previous headed
// Chrome process's own OS-level teardown — headed.go only guarantees
// the *Go-level* owner of the previous instance has let go of it (its
// own chromedp cancel() already invoked), not that the OS process it
// spawned has
// actually finished exiting and released its lock file by the time
// the very next call acquires the mutex and reaches here. Observed
// directly: "chrome failed to start ... Failed to create
// .../normal-chrome-profile/SingletonLock: File exists (17) ...
// Aborting now to avoid profile corruption" on a retry attempted only
// seconds after a previous headed session ended.
//
// Safe to remove unconditionally at this exact point, not just a
// best-effort guess: headed.go only calls this with headedMu held and
// no headed browser of its own running — so any lock files present here
// cannot belong to a session this process still considers live; they
// are, by construction, leftovers from an already-concluded (or
// externally terminated, e.g. a human closing the window directly)
// previous instance. Best-effort: a removal failure (e.g. the files
// genuinely don't exist) is not itself an error worth failing the
// whole launch over — chromedp's own subsequent Chrome launch will
// surface a real, actionable error if something else is wrong.
func removeStaleChromeSingletonLock(profileDir string) {
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(profileDir, name))
	}
}

// normalAllocatorOptions mirrors allocatorOptions' own stealth-oriented
// flags exactly, except headless is explicitly forced off and a real,
// persistent profileDir is used instead of chromedp's own default
// fresh-temp-dir-per-launch behavior. A deliberate, separate duplicate
// of allocatorOptions — not a shared helper with a headless bool
// parameter — specifically so allocatorOptions itself, and therefore
// startSharedSessionLocked's own behavior, is never touched by this
// feature.
func normalAllocatorOptions(profileDir string) []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("BROWSER_TOOL_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	opts = append(opts,
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		// "excludeSwitches" is a ChromeDriver/Selenium capability name,
		// not a real Chrome command-line flag — it's meaningless here
		// since chromedp launches Chrome directly (no chromedriver in
		// the picture), so it silently did nothing. The actual flag
		// that puts Chrome into automation-controlled mode (the "Chrome
		// is being controlled by automated test software" banner) is
		// enable-automation itself, already turned on by
		// chromedp.DefaultExecAllocatorOptions above — overriding it
		// here (Flag stores by map key, so this replaces that default)
		// is what actually suppresses it.
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("headless", false), // the whole point of this fallback
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-notifications", true),
		// Several tabs share this window (headed.go) — keep background
		// tabs running at full speed; see backgroundThrottlingFlags.
		backgroundThrottlingFlags[0], backgroundThrottlingFlags[1], backgroundThrottlingFlags[2],
		// No UA override at all: a headed browser's native UA already
		// matches its own client hints exactly (see headlessUserAgent's
		// own doc comment for the measured mismatch the former hard-coded
		// "Windows ... Chrome/120" override caused here).
		chromedp.Flag("lang", "en-US,en;q=0.9"),
	)
	return opts
}

// backgroundThrottlingFlags stop Chrome from throttling timers and
// deprioritizing renderers of tabs that aren't in the foreground — with
// several tabs open at once (worker tabs, several headed tabs sharing
// one window), all but one are "background", and a throttled Turnstile
// widget stalls instead of passing.
var backgroundThrottlingFlags = []chromedp.ExecAllocatorOption{
	chromedp.Flag("disable-background-timer-throttling", true),
	chromedp.Flag("disable-backgrounding-occluded-windows", true),
	chromedp.Flag("disable-renderer-backgrounding", true),
}

// allocatorOptions builds the shared headless session's launch flags.
// userAgent "" keeps Chrome's native UA — see startSharedSessionLocked
// for why the very first launch has to run that way once.
func allocatorOptions(userAgent string) []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("BROWSER_TOOL_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	opts = append(opts,
		// 1. Strip the standard automation controls and markers
		chromedp.Flag("disable-blink-features", "AutomationControlled"),

		// "excludeSwitches" is a ChromeDriver/Selenium capability name,
		// not a real Chrome command-line flag — it's meaningless here
		// since chromedp launches Chrome directly (no chromedriver in
		// the picture), so it silently did nothing. The actual flag
		// that puts Chrome into automation-controlled mode (the "Chrome
		// is being controlled by automated test software" banner) is
		// enable-automation itself, already turned on by
		// chromedp.DefaultExecAllocatorOptions above — overriding it
		// here (Flag stores by map key, so this replaces that default)
		// is what actually suppresses it.
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("use-mock-keychain", true),

		// 2. Erase core headless indicators and sandbox configurations
		chromedp.Flag("headless", "new"), // Modern headless engine is harder to spot than "old" headless
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-notifications", true),
		backgroundThrottlingFlags[0], backgroundThrottlingFlags[1], backgroundThrottlingFlags[2],

		// 4. OPTIMIERUNG FÜR CLOUDFLARE: Sprache explizit mitsenden, da Headless Chrome hier oft 'null' liefert
		chromedp.Flag("lang", "en-US,en;q=0.9"),
	)
	// 3. The browser's own UA with only "HeadlessChrome/" rewritten —
	// see headlessUserAgent's own doc comment.
	if userAgent != "" {
		opts = append(opts, chromedp.UserAgent(userAgent))
	}
	return opts
}
