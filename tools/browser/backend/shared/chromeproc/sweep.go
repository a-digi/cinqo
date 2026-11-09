package chromeproc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ledgerDirMu sync.Mutex
	ledgerDir   string
)

func setLedgerDir(dir string) {
	ledgerDirMu.Lock()
	ledgerDir = dir
	ledgerDirMu.Unlock()
}

func currentLedgerDir() string {
	ledgerDirMu.Lock()
	defer ledgerDirMu.Unlock()
	return ledgerDir
}

// SweepReport is what the start-up sweep did, for the log.
type SweepReport struct {
	Stopped        []int  // Chrome processes of an earlier run (or orphans) that were stopped
	Skipped        []int  // processes in the earlier ledger that were NOT touched because they could not be proven to be the same Chrome
	Failed         []int  // processes that could not be stopped
	FoldersRemoved int    // profile folders deleted
	Note           string // why nothing could be done, when so
}

func (r SweepReport) String() string {
	return fmt.Sprintf("stopped %d, skipped %d, failed %d, %d profile folder(s) removed%s", len(r.Stopped), len(r.Skipped), len(r.Failed), r.FoldersRemoved, noteText(r.Note))
}

func noteText(n string) string {
	if n == "" {
		return ""
	}
	return " (" + n + ")"
}

const sweepGrace = 3 * time.Second

// Sweep is the start-up sweep (plan 224, step 3). Call it once, after Configure and before the first Chrome is started. It cleans up what an
// earlier run of the tool left behind (a crash, a kill):
//  1. every Chrome in the ledger of the earlier run that is still running and provably the same process (same start time, or our profile
//     folder in its command line) is stopped with its children; a process id that now belongs to another program is left alone;
//  2. every process whose command line shows a profile folder of this tool, whose parent is gone (init, or no such process) and that is not
//     registered here, is stopped: an orphan;
//  3. every profile folder in the profile root that no running process uses is deleted.
//
// Nothing is ever signalled that is not provably ours, and nothing is deleted outside the profile root. The earlier ledger is removed when
// done.
func Sweep() SweepReport { return std.Sweep() }

func (r *Registry) Sweep() SweepReport {
	rep := SweepReport{}
	dir := currentLedgerDir()
	if r.proc == nil {
		rep.Note = "no way to read processes on this system"
		return rep
	}
	table := r.proc.List()
	if table == nil {
		rep.Note = "the process table could not be read; nothing was touched"
		return rep
	}

	// 1. the earlier run's ledger
	var previous []Entry
	if dir != "" {
		prevPath := filepath.Join(dir, previousLedgerName)
		if entries, err := ReadLedger(prevPath); err == nil {
			previous = entries
			defer func() { _ = os.Remove(prevPath) }()
		}
	}
	for _, e := range previous {
		if !r.proc.Alive(e.PID) {
			continue
		}
		if protected(e.PID) || !r.sameProcess(e) {
			rep.Skipped = append(rep.Skipped, e.PID)
			continue
		}
		if err := r.stopTreeNow(e.PID, sweepGrace); err != nil {
			rep.Failed = append(rep.Failed, e.PID)
			continue
		}
		rep.Stopped = append(rep.Stopped, e.PID)
	}

	// 2. orphans: ours by their command line, nobody's child any more, not registered here
	stopped, failed := r.huntOrphans(r.proc.List(), sweepGrace)
	rep.Stopped = append(rep.Stopped, stopped...)
	rep.Failed = append(rep.Failed, failed...)

	// 3. profile folders that nothing uses (and the folders of the earlier ledger's entries, when nothing uses them)
	table = r.proc.List()
	if table == nil {
		return rep
	}
	rep.FoldersRemoved = sweepFolders(table)
	return rep
}

// huntOrphans stops every process in the table that is a Chrome of this tool (our profile folder in its command line), is not registered
// here, and has lost its parent (init, or a parent that no longer exists). A Chrome whose parent is alive belongs to a running tool (this
// one, still starting it, or another instance) and is never an orphan. It returns the ids it stopped and the ones it could not.
func (r *Registry) huntOrphans(table []ProcInfo, grace time.Duration) (stopped, failed []int) {
	alive := map[int]bool{}
	for _, p := range table {
		alive[p.PID] = true
	}
	var orphans []int
	for _, p := range table {
		if protected(p.PID) || !IsOurs(p.Command) || r.registered(p.PID) {
			continue
		}
		if strings.HasPrefix(p.Stat, "Z") { // defunct: nothing to stop, the watchdog counts it
			continue
		}
		if p.PPID == 1 || p.PPID == 0 || !alive[p.PPID] {
			orphans = append(orphans, p.PID)
		}
	}
	sort.Ints(orphans)
	for _, pid := range orphans {
		if !r.proc.Alive(pid) { // already taken down as the child of an earlier orphan
			continue
		}
		if err := r.stopTreeNow(pid, grace); err != nil {
			failed = append(failed, pid)
			continue
		}
		stopped = append(stopped, pid)
	}
	return stopped, failed
}

// sameProcess says whether the process now running under an earlier ledger entry's id is that same Chrome: its start time is the recorded
// one, or (without a recorded start time) its command line shows one of our profile folders.
func (r *Registry) sameProcess(e Entry) bool {
	if e.ProcessStart != "" {
		return r.proc.StartTime(e.PID) == e.ProcessStart
	}
	return IsOurs(r.proc.CommandLine(e.PID))
}

func (r *Registry) registered(pid int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[pid]
	return ok
}

// stopTreeNow stops a process and everything below it without asking provablyOurs again (the caller has proved it). The guards on protected
// ids stay.
func (r *Registry) stopTreeNow(pid int, grace time.Duration) error {
	if protected(pid) {
		return ErrProtected
	}
	tree := []int{}
	for _, d := range descendants(r.proc.Parents(), pid) {
		if !protected(d) {
			tree = append(tree, d)
		}
	}
	tree = append(tree, pid)
	return r.terminateAll(tree, pid, grace)
}

// sweepFolders deletes every folder directly inside the profile root that no running process names in its command line. It returns how many.
func sweepFolders(table []ProcInfo) int {
	root := ProfileRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	removed := 0
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		path := filepath.Join(root, de.Name())
		if folderInUse(table, path) {
			continue
		}
		if RemoveProfileDir(path) == nil {
			removed++
		}
	}
	return removed
}

func folderInUse(table []ProcInfo, folder string) bool {
	needle := "--user-data-dir=" + folder
	for _, p := range table {
		if commandUnderRoot(p.Command, folder) || strings.Contains(p.Command, needle+"/") {
			return true
		}
	}
	return false
}
