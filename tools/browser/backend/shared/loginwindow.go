package shared

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// loginwindow.go (plan 182, step 1) is the throw-away visible Chrome a person logs in to a website in, so a crawl can then run in that
// same window. NOTHING it holds is kept: the window is its own Chrome process with its own temporary profile folder, created when the
// window opens and deleted when the window closes (cancel, the end of the run, the time limit, the person closing it, the tool
// stopping). It never uses the persistent profile of the headed Cloudflare browser (headed.go, normalSessionProfileDir), which would
// leave the login's cookies on disk, and it is never a tab of that browser. The tool never sees what the person types: they type on the
// site's own page. See plan/ai/tools/browser/step-182-crawl-with-login.md, "Step 0 result".

// loginWindowRoot is the folder the throw-away profiles are made in. A var so a test can point it elsewhere. It is in the system's
// temporary area on purpose, never under TOOL_DB_DIR (the tool's durable storage).
var loginWindowRoot = func() string { return filepath.Join(os.TempDir(), "cinqo-browser-login-windows") }

// loginWindowDirPrefix starts the name of each run's profile folder.
const loginWindowDirPrefix = "run-"

// loginWindowNavigateTimeout bounds the first navigation to the login page.
const loginWindowNavigateTimeout = 60 * time.Second

// loginWindowFlags are the Chrome flags of the login window, by name. Pure, so a test can read exactly what is asked of Chrome.
//   - user-data-dir is the run's own temporary folder (never the persistent profile);
//   - headless is off: a person has to see and use the window;
//   - enable-automation is off and AutomationControlled is disabled: with chromedp's defaults the window says "controlled by automated
//     test software" and navigator.webdriver is true, which login pages that check for automation refuse or challenge (measured in the
//     step 0 spike). This is not a way around a block: the person logs in, and the site's own checks still apply.
func loginWindowFlags(dir string) map[string]any {
	return map[string]any{
		"user-data-dir":            dir,
		"headless":                 false,
		"enable-automation":        false,
		"disable-blink-features":   "AutomationControlled",
		"disable-infobars":         true,
		"disable-notifications":    true,
		"use-mock-keychain":        true,
		"no-first-run":             true,
		"no-default-browser-check": true,
	}
}

func loginWindowAllocatorOptions(dir string) []chromedp.ExecAllocatorOption {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	// The one resolver of where Chrome is (chrome.go).
	if p, ok := ChromeLaunchPath(); ok {
		opts = append(opts, chromedp.ExecPath(p))
	}
	flags := loginWindowFlags(dir)
	names := make([]string, 0, len(flags))
	for name := range flags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		opts = append(opts, chromedp.Flag(name, flags[name]))
	}
	return opts
}

// prepareLoginWindowDir makes a new, empty, private folder for one window's Chrome profile.
func prepareLoginWindowDir() (string, error) {
	root := loginWindowRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("login window: cannot make its folder: %w", err)
	}
	dir, err := os.MkdirTemp(root, loginWindowDirPrefix)
	if err != nil {
		return "", fmt.Errorf("login window: cannot make its profile folder: %w", err)
	}
	return dir, nil
}

// removeLoginWindowDir deletes a window's profile folder. Chrome may still be writing for a moment after it was told to stop, so it
// tries a few times; whatever is left is removed by the sweep at the next start (SweepLoginWindowFolders).
func removeLoginWindowDir(dir string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.RemoveAll(dir); err == nil {
			if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

// SweepLoginWindowFolders deletes every profile folder an earlier run of the tool left behind (a crash, a kill). Called once when the
// tool starts, when no login window can exist yet. It returns how many it removed.
func SweepLoginWindowFolders() int {
	root := loginWindowRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err == nil {
			removed++
		}
	}
	return removed
}

// LoginWindow is one open login window: its tab's context, its temporary profile folder and how to close it.
type LoginWindow struct {
	// Ctx is the window's tab; cancelling the window (Close) or the person closing it ends it.
	Ctx          context.Context
	dir          string
	cancelTab    context.CancelFunc
	cancelAlloc  context.CancelFunc
	done         chan struct{}
	closeOnce    sync.Once
	closeCleaned chan struct{}
}

// OpenLoginWindow starts a new Chrome with a new temporary profile, visible, and loads startURL in it. On any failure nothing is left
// behind (the folder is deleted).
func OpenLoginWindow(startURL string) (*LoginWindow, error) {
	dir, err := prepareLoginWindowDir()
	if err != nil {
		return nil, err
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), loginWindowAllocatorOptions(dir)...)
	tab, cancelTab := chromedp.NewContext(alloc)
	w := &LoginWindow{Ctx: tab, dir: dir, cancelTab: cancelTab, cancelAlloc: cancelAlloc, done: make(chan struct{}), closeCleaned: make(chan struct{})}

	// The FIRST chromedp.Run decides how long the browser lives: it must get the window's own context, never a context with a timeout
	// derived from it (cancelling that one would close the window again, as the live check of step 1 showed). The time limit is a timer
	// that ends the whole window instead, stopped as soon as the page has opened.
	limit := time.AfterFunc(loginWindowNavigateTimeout, cancelTab)
	err = chromedp.Run(tab, chromedp.Navigate(startURL))
	limit.Stop()
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("login window: could not open the login page: %w", err)
	}
	go func() {
		<-tab.Done()
		close(w.done)
	}()
	return w, nil
}

// Done is closed when the window has ended: closed by the person, or by Close.
func (w *LoginWindow) Done() <-chan struct{} { return w.done }

// Close ends the window: stops Chrome and deletes the profile folder. Safe to call more than once and from any goroutine; every call
// returns after the first has finished.
func (w *LoginWindow) Close() {
	w.closeOnce.Do(func() {
		w.cancelTab()
		w.cancelAlloc()
		if err := removeLoginWindowDir(w.dir); err != nil {
			log.Printf("login window: could not delete its temporary profile folder (the next start removes it): %v", err)
		}
		close(w.closeCleaned)
	})
	<-w.closeCleaned
}
