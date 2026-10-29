// fetch_engine.go (step 49, step 50) — chooses which engine serves the
// PRIMARY single-page fetch (crawl.go's call just before the chromedp
// primary tab). Two engines share one contract (a *crawlError Cloudflare
// verdict, or a plain error meaning "this engine failed, fall through to
// chromedp"): go-rod/rod (rod_fetch.go, pure Go over CDP — the default)
// and Playwright (playwright_engine.go, needs a Node-based driver, whose
// code is untouched).
//
// The choice is the stored Browser-tool setting (shared.BrowserSettings.
// FetchEngine, changed on the Engine page), read on every fetch like the
// Debug toggles, so a switch applies to the next page fetched with no
// restart. See plan/ai/tools/browser/step-49-rod-fetch-engine.md and
// step-50-fetch-engine-setting-page.md.
package crawler

import (
	"context"
	"log"
	"sync"

	"browser-tool-backend/shared"
)

const (
	fetchEngineRod        = shared.FetchEngineRod
	fetchEnginePlaywright = shared.FetchEnginePlaywright
	defaultFetchEngine    = shared.DefaultFetchEngine
)

// fetchFunc is the shared contract of both engines' fetch functions.
type fetchFunc func(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error)

// The engine implementations, as vars only so a test can substitute them.
var (
	playwrightFetch fetchFunc = fetchWithPlaywright
	rodFetch        fetchFunc = fetchWithRod
)

// engineStoppers shut an engine's browser down (both are idempotent). A
// var so a test can observe which engine gets stopped.
var engineStoppers = map[string]func(){
	fetchEngineRod:        StopRodEngine,
	fetchEnginePlaywright: StopPlaywrightEngine,
}

var warnSettingUnreadableOnce sync.Once

// selectedFetchEngine reports which engine the next fetch will use: the
// stored setting, read fresh. The setting is the single source of truth.
// When it cannot be read (no database yet, or a read error) the default,
// rod, is used — never Playwright — and the failure is logged once.
func selectedFetchEngine() string {
	if shared.DB == nil {
		return defaultFetchEngine
	}
	settings, err := shared.LoadBrowserSettings()
	if err != nil {
		warnSettingUnreadableOnce.Do(func() {
			log.Printf("fetch engine: browser settings unreadable (%v); using %q", err, defaultFetchEngine)
		})
		return defaultFetchEngine
	}
	return settings.FetchEngine
}

var (
	engineMu       sync.Mutex
	engineCurrent  string             // the engine the most recent fetch selected
	engineInFlight = map[string]int{} // fetches currently running per engine
)

// beginEngineFetch records that a fetch is starting on engine and returns
// the function to call when it ends. When the selection has CHANGED, the
// engine no longer selected is stopped so its idle browser stops holding
// memory — but only once nothing is still running on it: a fetch already
// in flight on the old engine is never interrupted, and the last such
// fetch stops it when it finishes. Done here, lazily, because the
// settings handler lives in package shared and cannot import this one.
// Stopping happens under engineMu so a quick switch back can't have its
// freshly selected engine stopped by a late cleanup.
func beginEngineFetch(engine string) (end func()) {
	engineMu.Lock()
	previous := engineCurrent
	engineCurrent = engine
	engineInFlight[engine]++
	if previous != "" && previous != engine && engineInFlight[previous] == 0 {
		stopEngineLocked(previous)
	}
	engineMu.Unlock()

	return func() {
		engineMu.Lock()
		defer engineMu.Unlock()
		engineInFlight[engine]--
		if engine != engineCurrent && engineInFlight[engine] == 0 {
			stopEngineLocked(engine)
		}
	}
}

func stopEngineLocked(engine string) {
	if stop := engineStoppers[engine]; stop != nil {
		stop()
	}
}

// fetchWithEngine is the single entry point crawl.go calls. It calls the
// selected engine's fetch with the arguments it was given and returns the
// results unchanged. With rod selected (the default) the Playwright
// engine is never started, so no Node driver is spawned.
func fetchWithEngine(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	engine := selectedFetchEngine()
	defer beginEngineFetch(engine)()
	if engine == fetchEnginePlaywright {
		return playwrightFetch(ctx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	return rodFetch(ctx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
}
