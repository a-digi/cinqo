package chromeproc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var quick = WatchdogConfig{Interval: 20 * time.Millisecond, UnusedAfter: time.Minute, KillAfter: 100 * time.Millisecond}

// A registry with a clock the test moves, a Katana Chrome (pid 100, our folder, parent is this tool) and its owner's hooks.
type wdWorld struct {
	r       *Registry
	fp      *fakeProc
	now     time.Time
	root    string
	busy    atomic.Bool
	stopped atomic.Int32
	// what the owner's stopper does to the process: it ends it unless stubborn
	stubborn bool
}

func newWDWorld(t *testing.T) *wdWorld {
	t.Helper()
	w := &wdWorld{root: withRoots(t), now: time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)}
	w.fp = newFake()
	w.fp.alive = map[int]bool{100: true, 5000: true}
	w.fp.parents = map[int]int{100: os.Getpid(), 5000: 1}
	w.fp.starts[100] = "Mon Nov 9 14:00:00 2026"
	w.fp.cmds[100] = "/Chrome --headless --user-data-dir=" + w.root + "/katana-aaaa0001"
	w.r = New("", w.fp, func() time.Time { return w.now })
	w.r.Register(Launch{Kind: KindKatana, PID: 100, ProfileDir: w.root + "/katana-aaaa0001"})
	os.MkdirAll(w.root+"/katana-aaaa0001", 0o700)
	w.r.SetBusyCheck(KindKatana, func() bool { return w.busy.Load() })
	w.r.SetStopper(KindKatana, func() {
		w.stopped.Add(1)
		if !w.stubborn {
			w.fp.mu.Lock()
			w.fp.alive[100] = false
			w.fp.mu.Unlock()
			w.r.Release(100)
		}
	})
	return w
}

func TestAnUnusedChromeIsStoppedByItsOwnerFirst(t *testing.T) {
	w := newWDWorld(t)
	w.now = w.now.Add(61 * time.Second)
	res := w.r.WatchdogOnce(quick)
	if len(res.KilledUnused) != 1 || res.KilledUnused[0] != 100 || w.stopped.Load() != 1 {
		t.Fatalf("%+v stopped=%d", res, w.stopped.Load())
	}
	if len(w.fp.killed) != 0 || len(w.fp.terminated) != 0 {
		t.Errorf("the owner's polite stop was enough, nothing may be signalled: %v %v", w.fp.terminated, w.fp.killed)
	}
	if len(w.r.Entries()) != 0 {
		t.Error("released")
	}
}

func TestAnUnusedChromeThatIgnoresItsOwnerIsStoppedByTheTreeStopper(t *testing.T) {
	w := newWDWorld(t)
	w.stubborn = true
	w.now = w.now.Add(61 * time.Second)
	start := time.Now()
	res := w.r.WatchdogOnce(quick)
	if len(res.KilledUnused) != 1 || w.fp.alive[100] {
		t.Fatalf("%+v alive=%v", res, w.fp.alive[100])
	}
	if time.Since(start) < quick.KillAfter {
		t.Error("the owner must be given KillAfter before the tree stopper")
	}
	if len(w.r.Entries()) != 0 {
		t.Error("released by StopTree")
	}
}

func TestAChromeThatCannotBeStoppedAtAllIsReported(t *testing.T) {
	w := newWDWorld(t)
	w.stubborn = true
	w.fp.ignoreTerm[100], w.fp.surviveKill[100] = true, true
	w.now = w.now.Add(61 * time.Second)
	res := w.r.WatchdogOnce(quick)
	if len(res.Failed) != 1 || len(res.KilledUnused) != 0 {
		t.Errorf("%+v", res)
	}
}

func TestAChromeInUseOrRecentlyUsedOrUnknownToItsOwnerOrNotWatchedIsLeftAlone(t *testing.T) {
	// in use for as long as it likes
	w := newWDWorld(t)
	w.busy.Store(true)
	w.now = w.now.Add(10 * time.Hour)
	if res := w.r.WatchdogOnce(quick); res.acted() || w.stopped.Load() != 0 {
		t.Errorf("busy: %+v", res)
	}
	// used a moment ago
	w = newWDWorld(t)
	w.now = w.now.Add(59 * time.Second)
	if res := w.r.WatchdogOnce(quick); res.acted() {
		t.Errorf("recent: %+v", res)
	}
	// exactly at the limit counts as unused
	w.now = w.now.Add(time.Second)
	if res := w.r.WatchdogOnce(quick); len(res.KilledUnused) != 1 {
		t.Errorf("at the limit: %+v", res)
	}
	// no way to ask the owner: never stopped
	w = newWDWorld(t)
	w.r.mu.Lock()
	w.r.entries[100].Kind = KindRod // no busy check and no stopper for rod
	w.r.mu.Unlock()
	w.now = w.now.Add(time.Hour)
	if res := w.r.WatchdogOnce(quick); res.acted() {
		t.Errorf("unknown owner: %+v", res)
	}
	// the visible window follows its own rule
	w = newWDWorld(t)
	w.r.mu.Lock()
	w.r.entries[100].Kind = KindWindow
	w.r.mu.Unlock()
	w.r.SetBusyCheck(KindWindow, func() bool { return false })
	w.r.SetStopper(KindWindow, func() { w.stopped.Add(1) })
	w.now = w.now.Add(time.Hour)
	if res := w.r.WatchdogOnce(quick); res.acted() || w.stopped.Load() != 0 {
		t.Errorf("window: %+v", res)
	}
}

func TestADeadChromeIsDroppedItsOwnerToldAndItsFolderRemovedButNeverAPersistentProfile(t *testing.T) {
	w := newWDWorld(t)
	w.stubborn = true // the stopper only counts: the process is already gone
	w.fp.alive[100] = false
	persistent := filepath.Join(t.TempDir(), "window-profile")
	os.MkdirAll(persistent, 0o700)
	w.r.Register(Launch{Kind: KindWindow, PID: 101, ProfileDir: persistent})
	w.fp.alive[101] = false
	res := w.r.WatchdogOnce(quick)
	if len(res.DeadDropped) != 2 || len(w.r.Entries()) != 0 {
		t.Fatalf("%+v %+v", res, w.r.Entries())
	}
	if w.stopped.Load() != 1 {
		t.Errorf("the owner of the dead Katana Chrome must be told: %d", w.stopped.Load())
	}
	if _, err := os.Stat(w.root + "/katana-aaaa0001"); !os.IsNotExist(err) {
		t.Error("the dead Chrome's profile folder must be removed")
	}
	if _, err := os.Stat(persistent); err != nil {
		t.Error("a persistent profile must never be deleted")
	}
}

func TestADefunctChromeIsReportedAndDropped(t *testing.T) {
	w := newWDWorld(t)
	w.stubborn = true
	w.fp.stats[100] = "Z" // alive for kill(0), but defunct
	res := w.r.WatchdogOnce(quick)
	if len(res.Defunct) != 1 || len(res.DeadDropped) != 1 || len(w.fp.terminated)+len(w.fp.killed) != 0 {
		t.Errorf("%+v %v %v", res, w.fp.terminated, w.fp.killed)
	}
}

func TestAnUnregisteredDefunctChildOfThisToolThatLooksLikeChromeIsCounted(t *testing.T) {
	w := newWDWorld(t)
	w.fp.alive[600], w.fp.alive[601], w.fp.alive[602] = true, true, true
	w.fp.parents[600], w.fp.parents[601], w.fp.parents[602] = os.Getpid(), os.Getpid(), 5000
	w.fp.stats[600], w.fp.stats[601], w.fp.stats[602] = "Z", "Z", "Z"
	w.fp.cmds[600], w.fp.cmds[601], w.fp.cmds[602] = "(Google Chrome)", "(vim)", "(Google Chrome)"
	w.busy.Store(true)
	res := w.r.WatchdogOnce(quick)
	if len(res.Defunct) != 1 || res.Defunct[0] != 600 {
		t.Errorf("only a Chrome-looking defunct child of this tool counts: %+v", res)
	}
	if len(w.fp.terminated)+len(w.fp.killed) != 0 {
		t.Error("a defunct process is never signalled")
	}
}

func TestAnOrphanIsStoppedAndALiveParentsChromeIsNot(t *testing.T) {
	w := newWDWorld(t)
	w.busy.Store(true)
	for _, p := range []int{700, 701, 702} {
		w.fp.alive[p] = true
		w.fp.cmds[p] = "/Chrome --headless --user-data-dir=" + w.root + "/rod-x" + string(rune('a'+p-700))
	}
	w.fp.parents[700], w.fp.parents[701], w.fp.parents[702] = 1, 5000, 9999 // init, a live parent, a parent that is gone
	res := w.r.WatchdogOnce(quick)
	if len(res.KilledOrphan) != 2 || w.fp.alive[700] || w.fp.alive[702] || !w.fp.alive[701] || !w.fp.alive[100] {
		t.Errorf("%+v alive: 700=%v 701=%v 702=%v 100=%v", res, w.fp.alive[700], w.fp.alive[701], w.fp.alive[702], w.fp.alive[100])
	}
}

func TestWithAnUnreadableTableTheOrphanChecksAreSkippedButTheOthersRun(t *testing.T) {
	w := newWDWorld(t)
	w.fp.alive[700] = true
	w.fp.cmds[700] = "/Chrome --user-data-dir=" + w.root + "/rod-z"
	w.fp.parents[700] = 1
	w.fp.noList = true
	w.now = w.now.Add(2 * time.Minute)
	res := w.r.WatchdogOnce(quick)
	if len(res.KilledUnused) != 1 || len(res.KilledOrphan) != 0 || !w.fp.alive[700] {
		t.Errorf("%+v", res)
	}
}

func TestTwoEntriesOfOneKindAreNotCountedTwiceWhenTheOwnersStopEndsBoth(t *testing.T) {
	w := newWDWorld(t)
	w.fp.alive[101] = true
	w.fp.parents[101] = os.Getpid()
	w.fp.starts[101] = "x"
	w.fp.cmds[101] = "/Chrome --user-data-dir=" + w.root + "/katana-second"
	w.r.Register(Launch{Kind: KindKatana, PID: 101})
	w.r.SetStopper(KindKatana, func() {
		w.stopped.Add(1)
		w.fp.mu.Lock()
		w.fp.alive[100], w.fp.alive[101] = false, false
		w.fp.mu.Unlock()
		w.r.Release(100)
		w.r.Release(101)
	})
	w.now = w.now.Add(2 * time.Minute)
	res := w.r.WatchdogOnce(quick)
	if len(res.KilledUnused) != 1 || len(res.DeadDropped) != 0 || w.stopped.Load() != 1 {
		t.Errorf("%+v stops=%d", res, w.stopped.Load())
	}
}

func TestTheCountersAndTheStatusFollowThePasses(t *testing.T) {
	w := newWDWorld(t)
	if st := w.r.Snapshot().Watchdog; st.Runs != 0 || st.LastRunAt != "" {
		t.Errorf("before any pass: %+v", st)
	}
	w.now = w.now.Add(2 * time.Minute)
	w.r.WatchdogOnce(quick) // stops 100
	w.fp.alive[700] = true
	w.fp.cmds[700] = "/Chrome --user-data-dir=" + w.root + "/rod-q"
	w.fp.parents[700] = 1
	w.r.WatchdogOnce(quick) // stops the orphan
	st := w.r.Snapshot().Watchdog
	if st.Runs != 2 || st.KilledUnused != 1 || st.KilledOrphan != 1 || st.LastRunAt != "2026-11-09T14:02:00Z" {
		t.Errorf("%+v", st)
	}
	raw, _ := json.Marshal(w.r.Snapshot())
	for _, want := range []string{`"killedUnused":1`, `"killedOrphan":1`, `"runs":2`, `"lastRunAt":"2026-11-09T14:02:00Z"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s missing in %s", want, raw)
		}
	}
}

func TestAWatchdogWithoutAProcessReaderDoesNothing(t *testing.T) {
	if res := New("", nil, nil).WatchdogOnce(quick); res.acted() {
		t.Errorf("%+v", res)
	}
}

func TestTheLoopRunsEveryIntervalAfterAKickAndStopsWhenAsked(t *testing.T) {
	w := newWDWorld(t)
	stop := w.r.StartWatchdog(quick)
	deadline := time.Now().Add(2 * time.Second)
	for w.r.Snapshot().Watchdog.Runs < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if w.r.Snapshot().Watchdog.Runs < 3 {
		t.Fatalf("the loop must run again and again: %+v", w.r.Snapshot().Watchdog)
	}
	stop()
	stop() // stopping twice is fine
	after := w.r.Snapshot().Watchdog.Runs
	time.Sleep(100 * time.Millisecond)
	if w.r.Snapshot().Watchdog.Runs != after {
		t.Error("no pass may run after stop returned")
	}

	// a kick makes an immediate pass even with a long interval
	slow := WatchdogConfig{Interval: time.Hour, UnusedAfter: time.Minute, KillAfter: 50 * time.Millisecond}
	w2 := newWDWorld(t)
	stop2 := w2.r.StartWatchdog(slow)
	defer stop2()
	Kick()
	Kick()
	Kick() // many kicks never block
	deadline = time.Now().Add(2 * time.Second)
	for w2.r.Snapshot().Watchdog.Runs < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if w2.r.Snapshot().Watchdog.Runs < 1 {
		t.Error("a kick must make a pass at once")
	}
}
