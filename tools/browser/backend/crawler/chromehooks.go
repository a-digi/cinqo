package crawler

import "browser-tool-backend/shared/chromeproc"

func engineBusy(engine string) bool {
	engineMu.Lock()
	defer engineMu.Unlock()
	return engineInFlight[engine] > 0
}

// RegisterChromeHooks tells the Chrome watchdog (plan 224, step 4) how to ask the engines whether their Chromes are in use and how to stop
// them cleanly. Call it once at start-up.
func RegisterChromeHooks() {
	chromeproc.SetBusyCheck(chromeproc.KindRod, func() bool { return engineBusy(fetchEngineRod) })
	chromeproc.SetStopper(chromeproc.KindRod, StopRodEngine)
	chromeproc.SetBusyCheck(chromeproc.KindKatana, func() bool { return engineBusy(fetchEngineKatana) })
	chromeproc.SetStopper(chromeproc.KindKatana, StopKatanaEngine)
}
