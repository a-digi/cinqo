package shared

// tabs.go holds the headless browser PROCESS (as opposed to the primary
// tab, session.go's Ctx) and the pool of worker tabs opened in it.
//
// Why tabs, not more browser processes: every tab of one Chrome process
// shares its cookie jar, so a cf_clearance cookie earned in ANY tab —
// a worker that waited out a challenge, or one imported from the headed
// browser (ImportCookiesToHeadless) — immediately applies to every
// other tab, the primary one included. Tabs are also cheap, and with
// site-per-process a hung renderer in one tab doesn't freeze the others.
//
// Before this, one tab guarded by one mutex (Mu) served every caller,
// so a single slow page — a Cloudflare challenge waiting to clear, a
// paced multi-page crawl — made every other crawl, extract and login
// wait behind it.

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	stealth "github.com/go-rod/stealth"
)

// browserStateMu guards browserCtx/browserCancels. Writers also hold Mu
// (every launch/teardown path already does); readers — worker-tab
// acquisition and cookie import — take only browserStateMu, so they
// never wait behind a long primary-tab operation just to find the
// browser.
var (
	browserStateMu sync.Mutex
	browserCtx     context.Context
	browserCancels []context.CancelFunc
)

func setBrowser(root context.Context, cancels []context.CancelFunc) {
	browserStateMu.Lock()
	defer browserStateMu.Unlock()
	browserCtx, browserCancels = root, cancels
}

func currentBrowser() (context.Context, []context.CancelFunc) {
	browserStateMu.Lock()
	defer browserStateMu.Unlock()
	return browserCtx, browserCancels
}

func clearBrowser() {
	browserStateMu.Lock()
	cancels := browserCancels
	browserCtx, browserCancels = nil, nil
	browserStateMu.Unlock()
	cancelAll(cancels)
}

// browserProbeTimeout bounds probeBrowser — a healthy browser process
// answers Browser.getVersion near-instantly.
const browserProbeTimeout = 3 * time.Second

// probeBrowser checks that the browser PROCESS behind root still
// answers, independent of any one tab's renderer.
func probeBrowser(root context.Context) error {
	if root.Err() != nil {
		return root.Err()
	}
	ctx, cancel := context.WithTimeout(root, browserProbeTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, _, err := browser.GetVersion().Do(ctx)
		return err
	}))
}

// newTab opens a tab in the browser behind parent and prepares it the
// way every page-loading tab must be: the stealth script and native-
// dialog auto-dismissal are PER TARGET in CDP, so each new tab needs its
// own (the process-wide --user-agent flag needs nothing per tab).
// Cancelling the returned func closes just this tab (chromedp closes
// any non-first target on cancel), never the browser. opts are passed to
// chromedp.NewContext (e.g. WithTargetID for a pre-created target).
func newTab(parent context.Context, label string, opts ...chromedp.ContextOption) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, fmt.Errorf("%s: browser is not running", label)
	}
	ctx, cancel := chromedp.NewContext(parent, opts...)
	installDialogAutoDismiss(ctx, label)
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// stealth.JS enthält das vollständige, aus puppeteer-extra extrahierte Skript.
			// Es injiziert über 17 komplexe Patches (WebGL, Plugins, Navigator, Codecs, etc.) via CDP.
			_, err := page.AddScriptToEvaluateOnNewDocument(stealth.JS).Do(ctx)
			return err
		}),
		chromedp.Navigate("about:blank"),
	); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

// defaultMaxWorkerTabs — BROWSER_TOOL_MAX_TABS overrides it. Each tab
// is a renderer process (~100–300 MB), so this is a memory bound as
// much as a concurrency one.
const defaultMaxWorkerTabs = 4

var (
	workerSlotsOnce sync.Once
	workerSlots     chan struct{}
)

func workerSlotPool() chan struct{} {
	workerSlotsOnce.Do(func() {
		workerSlots = make(chan struct{}, envInt("BROWSER_TOOL_MAX_TABS", defaultMaxWorkerTabs))
	})
	return workerSlots
}

// envInt reads a positive integer from the environment, falling back to
// def when unset or invalid.
func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

// AcquireWorkerTab opens a fresh tab in the shared headless browser for
// ONE self-contained operation (navigate + read/extract, start to
// finish) and returns its context and a release func that closes it.
// Never touches Mu or the primary tab, so a worker can wait out a
// Cloudflare challenge or a paced multi-page crawl without holding up
// anybody else. Blocks — cancellably, via ctx — while
// BROWSER_TOOL_MAX_TABS workers are already open.
//
// release is idempotent, so callers can both defer it and hand it to
// navigateWithHangGuard (crawl.go) as that tab's own hang recovery:
// closing a hung tab is a browser-level command that works even when
// the tab's renderer doesn't answer, and it frees the slot at once.
//
// The returned context is done once the tab is closed — including when
// the whole browser is relaunched underneath it (RecreateSharedSessionLocked's
// fallback), which callers classify as browser_session_interrupted.
func AcquireWorkerTab(ctx context.Context) (context.Context, func(), error) {
	slots := workerSlotPool()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	freeSlot := func() { <-slots }

	root, _ := currentBrowser()
	if root == nil || root.Err() != nil {
		// Cold start (or a dead browser): the only case a worker takes
		// Mu, via EnsureSharedSession's own lazy launch.
		if err := EnsureSharedSession(); err != nil {
			freeSlot()
			return nil, nil, err
		}
		root, _ = currentBrowser()
	}

	tabCtx, cancel, err := newTab(root, "worker tab")
	if err != nil {
		freeSlot()
		return nil, nil, fmt.Errorf("failed to open a worker tab: %w", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			freeSlot()
		})
	}
	return tabCtx, release, nil
}

// HeadlessHasCookie reports whether the shared headless browser would
// send a cookie called name to rawURL — used to decide whether a
// domain already known to be behind Cloudflare is worth trying headless
// first (a cf_clearance earned earlier, or imported from the headed
// browser, usually still works). false when the browser isn't running.
func HeadlessHasCookie(rawURL, name string) bool {
	root, _ := currentBrowser()
	if root == nil || root.Err() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(root, browserProbeTimeout)
	defer cancel()
	found := false
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().WithURLs([]string{rawURL}).Do(ctx)
		for _, c := range cookies {
			if c.Name == name && c.Value != "" {
				found = true
			}
		}
		return err
	}))
	return found
}

// ImportCookiesToHeadless copies cookies (read from the headed browser
// after a challenge was cleared there) into the shared headless
// browser's cookie jar, so the primary tab and later worker tabs can
// reach the same site without the headed browser. Partitioned cookies
// are skipped (their partition key can't be carried over faithfully);
// session cookies stay session cookies. Best-effort by design: a
// cf_clearance cookie is bound to the browser's UA and IP as well, which
// match here (same machine, identical UA since the fingerprint fix), but
// Cloudflare may still decline it — callers must never depend on this
// succeeding.
func ImportCookiesToHeadless(cookies []*network.Cookie) error {
	root, _ := currentBrowser()
	if root == nil || root.Err() != nil {
		return fmt.Errorf("headless browser is not running")
	}
	params := make([]*network.CookieParam, 0, len(cookies))
	for _, c := range cookies {
		if c.PartitionKey != nil {
			continue
		}
		p := &network.CookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
		}
		if !c.Session && c.Expires > 0 {
			sec := int64(c.Expires)
			exp := cdp.TimeSinceEpoch(time.Unix(sec, int64((c.Expires-float64(sec))*1e9)))
			p.Expires = &exp
		}
		params = append(params, p)
	}
	if len(params) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(root, browserProbeTimeout)
	defer cancel()
	return chromedp.Run(ctx, network.SetCookies(params))
}
