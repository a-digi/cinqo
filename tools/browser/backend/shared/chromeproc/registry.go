// Package chromeproc keeps the list of every Chrome process the Browser tool starts (plan 224). Each launch registers its process id, the
// operating system's own start time of that process (so a process id reused by another program is never mistaken for ours), what it is
// for and the profile folder it uses; each stop releases it. The list is also written to a ledger file that survives the tool, so a
// Chrome left behind by a crash can be found and stopped at the next start.
//
// This package never stops a process on its own in step 1: it records and reports. It is the lowest package of the tool (it imports
// nothing of it), so the crawler, the shared sessions and the login windows can all use it.
package chromeproc

import (
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Kind says what a Chrome is for.
type Kind string

const (
	KindRod            Kind = "rod"             // the shared headless Chrome of the rod engine
	KindKatana         Kind = "katana"          // the shared headless Chrome of the Katana engine
	KindSharedHeadless Kind = "shared-headless" // the shared headless chromedp session
	KindHeaded         Kind = "headed"          // the headed chromedp fallback session
	KindWindow         Kind = "window"          // the visible Chrome of the Katana window (challenges, logins)
	KindLoginWindow    Kind = "login-window"    // the throw-away login window
)

// Launch is what a launcher tells the registry about a Chrome it just started.
type Launch struct {
	Kind       Kind
	PID        int
	ProfileDir string // "" when not known
	Binary     string // "" when not known
}

// Entry is one registered Chrome.
type Entry struct {
	PID          int       `json:"pid"`
	Kind         Kind      `json:"kind"`
	StartedAt    time.Time `json:"startedAt"`
	ProcessStart string    `json:"processStart"` // the operating system's start time of the process; "" when it could not be read
	ProfileDir   string    `json:"profileDir"`
	Binary       string    `json:"binary"`
	LastUsedAt   time.Time `json:"lastUsedAt"`
}

// Proc reads facts about a process from the operating system. An interface so the tests can use a fake.
type Proc interface {
	// Alive says whether a process with this id exists.
	Alive(pid int) bool
	// StartTime is the operating system's start time of the process, as one comparable string; "" when it cannot be read.
	StartTime(pid int) string
	// CommandLine is the process's whole command line as one line of text; "" when it cannot be read. It is only ever searched, never run or logged.
	CommandLine(pid int) string
	// Parents maps every process id to the id of its parent (one read of the whole process table); nil when it cannot be read.
	Parents() map[int]int
	// List reads the whole process table once: every process with its parent and its command line; nil when it cannot be read.
	List() []ProcInfo
	// Terminate asks a process to end (SIGTERM); Kill ends it at once (SIGKILL). Both report whether the signal was sent.
	Terminate(pid int) bool
	Kill(pid int) bool
}

// ProcInfo is one row of the process table.
type ProcInfo struct {
	PID     int
	PPID    int
	Stat    string // the process state letters ("S", "Ss+", "Z" for a defunct process)
	Command string // searched only, never run or logged
}

// Registry is the list of the Chromes now running that the tool started.
type Registry struct {
	mu         sync.Mutex
	entries    map[int]*Entry
	ledgerPath string
	now        func() time.Time
	proc       Proc

	wdMu     sync.Mutex
	wd       watchdogCounters
	busy     map[Kind]func() bool
	stoppers map[Kind]func()
}

// New makes a registry. ledgerPath "" keeps it in memory only.
func New(ledgerPath string, proc Proc, now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{entries: map[int]*Entry{}, ledgerPath: ledgerPath, now: now, proc: proc}
}

// Register records a Chrome that was just started. A process id of 0 or less (the launcher could not say) is ignored: nothing can be
// tracked without it. Registering the same id again replaces the entry (the id was reused after the earlier Chrome ended).
func (r *Registry) Register(l Launch) {
	if l.PID <= 0 {
		return
	}
	now := r.now()
	e := &Entry{PID: l.PID, Kind: l.Kind, StartedAt: now, ProfileDir: l.ProfileDir, Binary: l.Binary, LastUsedAt: now}
	if r.proc != nil {
		e.ProcessStart = r.proc.StartTime(l.PID)
	}
	r.mu.Lock()
	r.entries[l.PID] = e
	r.mu.Unlock()
	r.saveLedger()
}

// Release forgets a Chrome that was stopped. Releasing an id that is not registered does nothing.
func (r *Registry) Release(pid int) {
	r.mu.Lock()
	_, ok := r.entries[pid]
	delete(r.entries, pid)
	r.mu.Unlock()
	if ok {
		r.saveLedger()
	}
}

// Touch records that the Chrome with this id just did work.
func (r *Registry) Touch(pid int) {
	r.mu.Lock()
	if e, ok := r.entries[pid]; ok {
		e.LastUsedAt = r.now()
	}
	r.mu.Unlock()
}

// TouchKind records that every registered Chrome of this kind just did work (for the callers that know the engine, not the process).
func (r *Registry) TouchKind(k Kind) {
	r.mu.Lock()
	now := r.now()
	for _, e := range r.entries {
		if e.Kind == k {
			e.LastUsedAt = now
		}
	}
	r.mu.Unlock()
}

// Entries is a copy of the registered Chromes, oldest first.
func (r *Registry) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].PID < out[j].PID
	})
	return out
}

// ChromeStatus is one Chrome as the status route shows it: no folder, no command line, no address.
type ChromeStatus struct {
	PID         int    `json:"pid"`
	Kind        string `json:"kind"`
	AgeSeconds  int    `json:"ageSeconds"`
	IdleSeconds int    `json:"idleSeconds"`
	State       string `json:"state"` // "running", or "gone" when the process has ended without being released
}

// Status is what the status route answers.
type Status struct {
	Limit    int            `json:"limit"`   // crawl sessions that may run at once
	Running  int            `json:"running"` // crawl sessions running now
	Waiting  int            `json:"waiting"` // crawl requests waiting for a free slot
	Chromes  []ChromeStatus `json:"chromes"`
	Watchdog WatchdogStatus `json:"watchdog"`
}

// Snapshot reads the registry and asks the operating system whether each process still exists.
func (r *Registry) Snapshot() Status {
	now := r.now()
	entries := r.Entries()
	ls := sessions.Status()
	out := Status{Limit: ls.Limit, Running: ls.Running, Waiting: ls.Waiting, Chromes: make([]ChromeStatus, 0, len(entries)), Watchdog: r.watchdogStatus()}
	for _, e := range entries {
		state := "running"
		if r.proc != nil && !r.proc.Alive(e.PID) {
			state = "gone"
		}
		out.Chromes = append(out.Chromes, ChromeStatus{
			PID: e.PID, Kind: string(e.Kind), State: state,
			AgeSeconds: seconds(now.Sub(e.StartedAt)), IdleSeconds: seconds(now.Sub(e.LastUsedAt)),
		})
	}
	return out
}

func seconds(d time.Duration) int {
	if d < 0 {
		return 0
	}
	return int(d / time.Second)
}

// The tool's one registry. Without Configure it keeps the list in memory only.
var std = New("", realProc{}, nil)

// Configure points the tool's registry at its ledger folder: the ledger file lives there. A ledger left by an earlier run is kept aside as
// chrome-pids.previous.json (so the start-up sweep can find what that run left behind) and a fresh ledger starts empty.
func Configure(dir string) error {
	path, err := prepareLedger(dir)
	if err != nil {
		return err
	}
	setLedgerDir(dir)
	setProfileRoot(filepath.Join(dir, "profiles"))
	std.mu.Lock()
	std.ledgerPath = path
	std.mu.Unlock()
	std.saveLedger()
	return nil
}

func Register(l Launch)     { std.Register(l) }
func Release(pid int)       { std.Release(pid) }
func Touch(pid int)         { std.Touch(pid) }
func TouchKind(k Kind)      { std.TouchKind(k) }
func Entries() []Entry      { return std.Entries() }
func CurrentStatus() Status { return std.Snapshot() }
