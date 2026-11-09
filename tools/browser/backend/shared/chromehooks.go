package shared

import (
	"time"

	"browser-tool-backend/shared/chromeproc"
)

// StopHeadlessSession stops only the shared headless browser (StopSharedSession also stops the headed one).
func StopHeadlessSession() {
	Mu.Lock()
	stopHeadlessLocked()
	Mu.Unlock()
}

// RegisterChromeHooks tells the Chrome watchdog (plan 224, step 4) how to ask this package whether its Chromes are in use and how to stop
// them cleanly. Call it once at start-up.
func RegisterChromeHooks() {
	chromeproc.SetBusyCheck(chromeproc.KindSharedHeadless, func() bool {
		if len(workerSlotPool()) > 0 { // a worker tab is open
			return true
		}
		if !Mu.TryLock() { // the primary tab is in use
			return true
		}
		Mu.Unlock()
		// used a moment ago counts as in use too (the same rule as the idle reaper)
		return time.Since(time.Unix(0, lastSessionActivity.Load())) < SessionIdleStop
	})
	chromeproc.SetStopper(chromeproc.KindSharedHeadless, StopHeadlessSession)

	chromeproc.SetBusyCheck(chromeproc.KindHeaded, func() bool {
		headedMu.Lock()
		defer headedMu.Unlock()
		return headedRefs > 0
	})
	chromeproc.SetStopper(chromeproc.KindHeaded, StopHeadedBrowser)
}
