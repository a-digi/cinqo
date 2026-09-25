package shared

// headed.go holds the headed ("normal", visible) Chrome browser used as
// the Cloudflare fallback — one long-lived process, persistent profile
// (normalSessionProfileDir), several tabs.
//
// It replaces NormalSessionMu + one fresh headed process per fallback
// attempt. That mutex was held for the fallback's ENTIRE duration — up
// to 31 minutes while waiting for a person to solve a challenge — so
// every other request that hit a challenge, or targeted a domain already
// known to be behind Cloudflare, queued behind it. Now each attempt
// gets its own tab (at most BROWSER_TOOL_MAX_HEADED_TABS at once); only
// the process itself is shared, which the persistent profile requires
// anyway (two Chrome processes can't share one user-data-dir).
//
// "Close after it is finished" is kept: once the last tab is released,
// the browser closes after headedIdleClose — long enough that
// back-to-back fallbacks reuse one window instead of flashing a new one
// each time.

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// defaultMaxHeadedTabs — BROWSER_TOOL_MAX_HEADED_TABS overrides it.
// Kept small: every headed tab may be waiting for a person.
const defaultMaxHeadedTabs = 2

// headedIdleClose — how long the headed browser stays open with no tab
// in use before it's closed.
const headedIdleClose = 60 * time.Second

var (
	headedMu      sync.Mutex
	headedRoot    context.Context
	headedCancels []context.CancelFunc
	headedRefs    int
	headedIdle    *time.Timer

	headedSlotsOnce sync.Once
	headedSlots     chan struct{}
)

func headedSlotPool() chan struct{} {
	headedSlotsOnce.Do(func() {
		headedSlots = make(chan struct{}, envInt("BROWSER_TOOL_MAX_HEADED_TABS", defaultMaxHeadedTabs))
	})
	return headedSlots
}

// AcquireHeadedTab opens a tab in the headed browser (launching the
// browser first if it isn't running, or has died — e.g. a person closed
// its window) and returns the tab's context and an idempotent release
// func that closes it. Blocks — cancellably, via ctx — while
// BROWSER_TOOL_MAX_HEADED_TABS headed tabs are already open. Like
// AcquireWorkerTab's release, this one doubles as the tab's own hang
// recovery.
func AcquireHeadedTab(ctx context.Context) (context.Context, func(), error) {
	slots := headedSlotPool()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	freeSlot := func() { <-slots }

	headedMu.Lock()
	if headedIdle != nil {
		headedIdle.Stop()
		headedIdle = nil
	}
	if headedRoot == nil || probeBrowser(headedRoot) != nil {
		stopHeadedLocked()
		if err := launchHeadedLocked(); err != nil {
			headedMu.Unlock()
			freeSlot()
			return nil, nil, fmt.Errorf("normal-session fallback: failed to start: %w", err)
		}
	}
	headedRefs++
	root := headedRoot
	headedMu.Unlock()

	tabCtx, cancel, err := newTab(root, "headed fallback tab")
	if err != nil {
		releaseHeadedRef()
		freeSlot()
		return nil, nil, fmt.Errorf("normal-session fallback: failed to open a tab: %w", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			releaseHeadedRef()
			freeSlot()
		})
	}
	return tabCtx, release, nil
}

// releaseHeadedRef drops one reference and, at zero, schedules the
// browser to close after headedIdleClose. closeIfIdle re-checks the
// count under the lock, so an AcquireHeadedTab that raced in first
// always wins.
func releaseHeadedRef() {
	headedMu.Lock()
	defer headedMu.Unlock()
	headedRefs--
	if headedRefs > 0 {
		return
	}
	headedRefs = 0
	headedIdle = time.AfterFunc(headedIdleClose, func() {
		headedMu.Lock()
		defer headedMu.Unlock()
		if headedRefs == 0 {
			stopHeadedLocked()
		}
	})
}

// launchHeadedLocked starts the headed Chrome process with the
// persistent profile. Assumes headedMu is held and no headed browser of
// ours is running (removeStaleChromeSingletonLock's own precondition).
// The root tab stays blank; every real page gets its own tab via newTab
// — which is also where the stealth script is installed (a real,
// reproduced bug once: the headed session went out with no stealth
// patches at all, and Cloudflare's Turnstile never rendered a solvable
// checkbox in it).
func launchHeadedLocked() error {
	profileDir, err := normalSessionProfileDir()
	if err != nil {
		return err
	}
	removeStaleChromeSingletonLock(profileDir)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), normalAllocatorOptions(profileDir)...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	cancels := []context.CancelFunc{ctxCancel, allocCancel}
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		cancelAll(cancels)
		return err
	}
	headedRoot, headedCancels = ctx, cancels
	return nil
}

// stopHeadedLocked closes the headed browser (every headed tab with
// it). Assumes headedMu is held.
func stopHeadedLocked() {
	if headedRoot == nil {
		return
	}
	if headedRefs > 0 {
		log.Printf("headed fallback browser: closing with %d tab(s) still in use", headedRefs)
	}
	cancelAll(headedCancels)
	headedRoot, headedCancels = nil, nil
}

// StopHeadedBrowser closes the headed browser if it's open — process
// shutdown only.
func StopHeadedBrowser() {
	headedMu.Lock()
	defer headedMu.Unlock()
	if headedIdle != nil {
		headedIdle.Stop()
		headedIdle = nil
	}
	stopHeadedLocked()
}
