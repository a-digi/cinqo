package chromeproc

import (
	"log"
	"strings"
	"sync"
	"time"
)

// The watchdog (plan 224, step 4) does not trust the timers of the code that started a Chrome. Every pass it looks again, on its own:
//  1. a registered headless Chrome that is not in use and has not been used for UnusedAfter is stopped, first through its owner's stopper (so
//     the owner's state stays right) and, if it is still alive KillAfter later, by the tree stopper;
//  2. a registered Chrome whose process is gone is dropped (its owner is told through the same stopper, so the next use starts a fresh one);
//  3. an unregistered Chrome of this tool whose parent is gone is an orphan and is stopped;
//  4. a defunct process (state Z) is counted and, when registered, dropped: only its parent can reap it, so it is reported and never "killed".

// WatchdogConfig sets the timings.
type WatchdogConfig struct {
	Interval    time.Duration // between two passes (30 s)
	UnusedAfter time.Duration // how long a Chrome may sit unused before it is stopped (60 s)
	KillAfter   time.Duration // how long after the owner's stop a Chrome may still run before the tree stopper is used (5 s)
}

// DefaultWatchdog is the plan's timing (Q4).
var DefaultWatchdog = WatchdogConfig{Interval: 30 * time.Second, UnusedAfter: 60 * time.Second, KillAfter: 5 * time.Second}

type watchdogCounters struct {
	lastRun      time.Time
	runs         int
	killedUnused int
	killedOrphan int
	deadDropped  int
	defunct      int
}

// WatchdogStatus is the watchdog's part of the status route.
type WatchdogStatus struct {
	LastRunAt    string `json:"lastRunAt"` // "" before the first pass
	Runs         int    `json:"runs"`
	KilledUnused int    `json:"killedUnused"`
	KilledOrphan int    `json:"killedOrphan"`
	DeadDropped  int    `json:"deadDropped"`
	Defunct      int    `json:"defunct"`
}

func (r *Registry) watchdogStatus() WatchdogStatus {
	r.wdMu.Lock()
	defer r.wdMu.Unlock()
	w := WatchdogStatus{Runs: r.wd.runs, KilledUnused: r.wd.killedUnused, KilledOrphan: r.wd.killedOrphan, DeadDropped: r.wd.deadDropped, Defunct: r.wd.defunct}
	if !r.wd.lastRun.IsZero() {
		w.LastRunAt = r.wd.lastRun.UTC().Format(time.RFC3339)
	}
	return w
}

// SetBusyCheck tells the watchdog how to ask an owner whether a Chrome of this kind is in use right now. Without one the watchdog never
// stops an unused-looking Chrome of that kind (it cannot know it is not in use).
func (r *Registry) SetBusyCheck(k Kind, f func() bool) {
	r.wdMu.Lock()
	defer r.wdMu.Unlock()
	if r.busy == nil {
		r.busy = map[Kind]func() bool{}
	}
	r.busy[k] = f
}

// SetStopper tells the watchdog how to ask an owner to stop its Chrome of this kind cleanly (the owner's own idempotent stop function).
func (r *Registry) SetStopper(k Kind, f func()) {
	r.wdMu.Lock()
	defer r.wdMu.Unlock()
	if r.stoppers == nil {
		r.stoppers = map[Kind]func(){}
	}
	r.stoppers[k] = f
}

func (r *Registry) busyOf(k Kind) (check func() bool, ok bool) {
	r.wdMu.Lock()
	defer r.wdMu.Unlock()
	check, ok = r.busy[k]
	return
}

func (r *Registry) stopperOf(k Kind) func() {
	r.wdMu.Lock()
	defer r.wdMu.Unlock()
	return r.stoppers[k]
}

// watched says whether the unused rule applies to a kind: the headless Chromes of the engines and sessions. The visible window and the login
// window belong to a person and follow their own, longer rules; they are only dropped when dead and stopped when orphaned.
func watched(k Kind) bool {
	switch k {
	case KindRod, KindKatana, KindSharedHeadless, KindHeaded:
		return true
	}
	return false
}

// WatchdogResult is what one pass did.
type WatchdogResult struct {
	KilledUnused []int
	KilledOrphan []int
	DeadDropped  []int
	Defunct      []int
	Failed       []int
}

// WatchdogOnce makes one pass and returns what it did.
func (r *Registry) WatchdogOnce(cfg WatchdogConfig) WatchdogResult {
	res := WatchdogResult{}
	if r.proc == nil {
		return res
	}
	now := r.now()
	table := r.proc.List() // nil when it cannot be read: the orphan and defunct checks are then skipped, the others still run
	state := map[int]string{}
	for _, p := range table {
		state[p.PID] = p.Stat
	}

	for _, e := range r.Entries() {
		if !r.registered(e.PID) { // an earlier step of this pass (its owner's stop) already released it
			continue
		}
		defunct := strings.HasPrefix(state[e.PID], "Z")
		if !r.proc.Alive(e.PID) || defunct {
			// 2 and 4: gone, or defunct (nothing is left to stop)
			if stop := r.stopperOf(e.Kind); stop != nil {
				stop() // the owner forgets its handle; its own close also releases the entry
			}
			r.Release(e.PID)
			if e.ProfileDir != "" {
				_ = RemoveProfileDir(e.ProfileDir) // refuses anything outside the profile root, so a persistent profile is never deleted
			}
			res.DeadDropped = append(res.DeadDropped, e.PID)
			if defunct {
				res.Defunct = append(res.Defunct, e.PID)
			}
			continue
		}
		if !watched(e.Kind) {
			continue
		}
		busy, known := r.busyOf(e.Kind)
		stop := r.stopperOf(e.Kind)
		if !known || stop == nil || busy() || now.Sub(e.LastUsedAt) < cfg.UnusedAfter {
			continue
		}
		// 1: unused and running. Ask the owner first, then give it KillAfter, then use the tree stopper.
		stop()
		deadline := time.Now().Add(cfg.KillAfter)
		for r.proc.Alive(e.PID) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if r.proc.Alive(e.PID) {
			if err := r.StopTree(e.PID, 3*time.Second); err != nil {
				res.Failed = append(res.Failed, e.PID)
				continue
			}
		}
		res.KilledUnused = append(res.KilledUnused, e.PID)
	}

	// 3: orphans, and 4: defunct processes that are not registered but are children of this tool
	if table != nil {
		stopped, failed := r.huntOrphans(table, 3*time.Second)
		res.KilledOrphan = append(res.KilledOrphan, stopped...)
		res.Failed = append(res.Failed, failed...)
		for _, p := range table {
			if strings.HasPrefix(p.Stat, "Z") && p.PPID == ownPID() && !contains(res.Defunct, p.PID) && looksLikeChrome(p.Command) {
				res.Defunct = append(res.Defunct, p.PID)
			}
		}
	}

	r.wdMu.Lock()
	r.wd.lastRun = now
	r.wd.runs++
	r.wd.killedUnused += len(res.KilledUnused)
	r.wd.killedOrphan += len(res.KilledOrphan)
	r.wd.deadDropped += len(res.DeadDropped)
	r.wd.defunct += len(res.Defunct)
	r.wdMu.Unlock()
	return res
}

func contains(l []int, v int) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// looksLikeChrome is for a defunct child, whose command line is gone ("(Google Chrome)", "<defunct>"): the name is all there is.
func looksLikeChrome(cmd string) bool {
	c := strings.ToLower(cmd)
	return strings.Contains(c, "chrome") || strings.Contains(c, "chromium")
}

func (r WatchdogResult) acted() bool {
	return len(r.KilledUnused)+len(r.KilledOrphan)+len(r.DeadDropped)+len(r.Defunct)+len(r.Failed) > 0
}

// StartWatchdog runs a pass every cfg.Interval, and at once after Kick, until the returned stop function is called. A pass that did something
// is logged in one line.
func (r *Registry) StartWatchdog(cfg WatchdogConfig) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(cfg.Interval)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
			case <-kick:
			}
			if res := r.WatchdogOnce(cfg); res.acted() {
				log.Printf("chrome watchdog: unused stopped %v, orphans stopped %v, dead dropped %v, defunct %v, could not stop %v",
					res.KilledUnused, res.KilledOrphan, res.DeadDropped, res.Defunct, res.Failed)
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
		wg.Wait()
	}
}

var kick = make(chan struct{}, 1)

// Kick asks the running watchdog for a pass now (a crawl just ended); it never blocks and many kicks count as one.
func Kick() {
	select {
	case kick <- struct{}{}:
	default:
	}
}

// The tool's one watchdog.
func SetBusyCheck(k Kind, f func() bool)      { std.SetBusyCheck(k, f) }
func SetStopper(k Kind, f func())             { std.SetStopper(k, f) }
func StartWatchdog(cfg WatchdogConfig) func() { return std.StartWatchdog(cfg) }

// WatchdogOnce makes one pass of the tool's watchdog now.
func WatchdogOnce(cfg WatchdogConfig) WatchdogResult { return std.WatchdogOnce(cfg) }
