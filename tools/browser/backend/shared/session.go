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
// plan/ai/tools/browser/step-02-shared-browser-session.md).
package shared

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	stealth "github.com/go-rod/stealth"
)

// Mu guards every access to Ctx — every HTTP handler that touches the
// shared page must hold this for the duration of its own chromedp
// actions, so two requests never race on the same tab. Ctx itself is
// nil until EnsureSharedSession's first successful call (lazy start,
// step-43-lazy-shared-session.md) — every handler that needs it calls
// EnsureSharedSession first, never assumes it's already set.
var (
	Mu     sync.Mutex
	Ctx    context.Context
	Cancel []context.CancelFunc
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
func EnsureSharedSession() error {
	Mu.Lock()
	defer Mu.Unlock()
	if Ctx != nil {
		return nil
	}
	return startSharedSessionLocked()
}

// installDialogAutoDismiss (step 47.1) auto-accepts any native
// alert()/confirm()/beforeunload dialog that appears on ctx's own
// page, for the lifetime of ctx. A native dialog freezes its target's
// JS engine and CDP responsiveness while it's open — and since every
// caller of the shared headless session (Ctx) or the headed fallback
// session inherits whatever state the previous caller left the tab in
// (Mu/NormalSessionMu only serialize access, they never reset the tab
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

// startSharedSessionLocked creates the one chromedp browser context
// this whole process holds for its entire lifetime — assumes the
// caller (EnsureSharedSession, above) already holds Mu. A real, empty
// navigation (not just allocator/context creation) forces the browser
// to actually launch now rather than deferring even further, so a
// genuinely live browser (not just constructed Go-side handles) is
// confirmed before this returns successfully.
func startSharedSessionLocked() error {
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions()...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	installDialogAutoDismiss(ctx, "shared headless session")

	// Build actions using the native cdproto/page Action wrapper
	actions := []chromedp.Action{
		chromedp.ActionFunc(func(ctx context.Context) error {
			// stealth.JS enthält das vollständige, aus puppeteer-extra extrahierte Skript.
			// Es injiziert über 17 komplexe Patches (WebGL, Plugins, Navigator, Codecs, etc.) via CDP.
			_, err := page.AddScriptToEvaluateOnNewDocument(stealth.JS).Do(ctx)
			return err
		}),
		chromedp.Navigate("about:blank"),
	}

	if err := chromedp.Run(ctx, actions...); err != nil {
		ctxCancel()
		allocCancel()
		return err
	}

	Ctx = ctx
	Cancel = []context.CancelFunc{ctxCancel, allocCancel}
	return nil
}

// RecreateSharedSessionLocked (step 47.3) tears down the current
// shared session — whatever state it's actually in — and immediately
// attempts to start a fresh one in its place, while the caller still
// holds Mu, so no other request can observe or acquire the wedged tab
// in between. Called only after ProbeSessionLiveness (step 47.2) has
// already confirmed the current tab is unresponsive. Assumes the
// caller already holds Mu, same precondition as startSharedSessionLocked
// itself.
//
// If the fresh start itself fails, Ctx/Cancel are left nil/empty
// rather than pointing back at the now-torn-down old session —
// matching EnsureSharedSession's own lazy-start convention (`if Ctx !=
// nil { return nil }`), so the very next caller's EnsureSharedSession
// call attempts a completely fresh start from scratch instead of being
// fooled by a stale non-nil Ctx into skipping straight to a session
// that no longer exists.
//
// Deliberately a full relaunch (a brand new chromedp allocator and
// browser process, via startSharedSessionLocked) rather than only
// opening a new tab on the existing browser process — simpler and
// reuses already-verified code (including installDialogAutoDismiss
// and the stealth script injection) instead of introducing a second,
// narrower "just replace the tab" path for what should be a rare
// recovery case.
func RecreateSharedSessionLocked() error {
	for _, cancel := range Cancel {
		cancel()
	}
	Ctx = nil
	Cancel = nil
	// step 63.3 — real bug fix, caught while reviewing this function's
	// own doc comment, not hypothetical: without this, fetch_cache.go's
	// own "is the shared session already showing this URL" check
	// (crawlPage, crawl.go) would still see whatever URL the OLD,
	// now-discarded tab was last navigated to — wrongly believing the
	// brand new, blank tab this function just created is already
	// showing it, and serving a stale cached result instead of ever
	// navigating the new tab there at all.
	LastHeadlessFetchURL = ""
	return startSharedSessionLocked()
}

func StopSharedSession() {
	Mu.Lock()
	defer Mu.Unlock()
	for _, cancel := range Cancel {
		cancel()
	}
}

// NormalSessionMu serializes headed-Chrome fallback attempts (crawl.go's
// crawlWithNormalSession) — at most one headed Chrome window is ever
// open at a time, regardless of how many concurrent crawl requests hit
// a Cloudflare block simultaneously. Separate from Mu (which guards the
// always-on shared headless session's own state) since this guards a
// completely different, ephemeral resource. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md.
var NormalSessionMu sync.Mutex

// StartSharedNormalSession launches a fresh, non-headless ("normal")
// Chrome instance — used only as a fallback when the lazily-started
// shared headless session (EnsureSharedSession/startSharedSessionLocked,
// above — never called by this function, never modified by this
// feature) fails to get past a Cloudflare challenge. Unlike that
// session, this does NOT store its context/cancel funcs into the
// package-level Ctx/Cancel — those remain exclusively the shared
// headless session's own state — and is never called at boot (nothing
// browser-related is started at boot anymore — see
// step-43-lazy-shared-session.md). The caller owns the returned cancel
// funcs and must call every one of them once done with this instance
// ("close after it is finished") — see crawl.go's crawlWithNormalSession,
// the one caller.
func StartSharedNormalSession() (context.Context, []context.CancelFunc, error) {
	profileDir, err := normalSessionProfileDir()
	if err != nil {
		return nil, nil, err
	}
	removeStaleChromeSingletonLock(profileDir)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), normalAllocatorOptions(profileDir)...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	installDialogAutoDismiss(ctx, "headed fallback session")

	actions := []chromedp.Action{
		chromedp.Navigate("about:blank"),
	}

	if err := chromedp.Run(ctx, actions...); err != nil {
		ctxCancel()
		allocCancel()
		return nil, nil, err
	}

	return ctx, []context.CancelFunc{ctxCancel, allocCancel}, nil
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
// not a blank "private"-feeling one. Safe to reuse across sequential
// invocations only because NormalSessionMu (crawl.go) already
// guarantees at most one headed instance is ever running at a time —
// two Chrome processes sharing one user-data-dir concurrently would
// conflict. See
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
// Chrome process's own OS-level teardown — NormalSessionMu (crawl.go)
// only guarantees the *Go-level* call that owned the previous instance
// has returned (its own chromedp cancel() already invoked, per this
// function's own caller), not that the OS process it spawned has
// actually finished exiting and released its lock file by the time
// the very next call acquires the mutex and reaches here. Observed
// directly: "chrome failed to start ... Failed to create
// .../normal-chrome-profile/SingletonLock: File exists (17) ...
// Aborting now to avoid profile corruption" on a retry attempted only
// seconds after a previous headed session ended.
//
// Safe to remove unconditionally at this exact point, not just a
// best-effort guess: NormalSessionMu (crawl.go) is already held by the
// caller before this function runs, and this same package never
// launches a second headed Chrome instance against this profile
// directory while the mutex is held — so any lock files present here
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
		chromedp.Flag("excludeSwitches", "enable-automation"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("headless", false), // the whole point of this fallback
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.Flag("lang", "en-US,en;q=0.9"),
	)
	return opts
}

func allocatorOptions() []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("BROWSER_TOOL_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	opts = append(opts,
		// 1. Strip the standard automation controls and markers
		chromedp.Flag("disable-blink-features", "AutomationControlled"),

		// KORREKTUR: In chromedp müssen Flags mit '=' oder als separates Argument übergeben werden.
		// 'excludeSwitches=enable-automation' stellt sicher, dass Chrome das Banner nicht rendert.
		chromedp.Flag("excludeSwitches", "enable-automation"),
		chromedp.Flag("use-mock-keychain", true),

		// 2. Erase core headless indicators and sandbox configurations
		chromedp.Flag("headless", "new"), // Modern headless engine is harder to spot than "old" headless
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-notifications", true),

		// 3. Set a standard, non-headless consumer User Agent matching current browser iterations
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),

		// 4. OPTIMIERUNG FÜR CLOUDFLARE: Sprache explizit mitsenden, da Headless Chrome hier oft 'null' liefert
		chromedp.Flag("lang", "en-US,en;q=0.9"),
	)
	return opts
}
