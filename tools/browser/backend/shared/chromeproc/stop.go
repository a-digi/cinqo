package chromeproc

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrNotOurs is returned when a process cannot be proven to be a Chrome of this tool; it is then never signalled.
var ErrNotOurs = errors.New("chromeproc: the process is not provably one of this tool's Chromes")

// ErrProtected is returned for a process id that must never be signalled (this tool, its parent, init, or a non-process id).
var ErrProtected = errors.New("chromeproc: the process id is protected")

// descendants lists every process below pid in the process table (children first, deepest first), never including pid itself.
func descendants(parents map[int]int, pid int) []int {
	kids := map[int][]int{}
	for c, p := range parents {
		kids[p] = append(kids[p], c)
	}
	var out []int
	var walk func(int, int)
	walk = func(p, depth int) {
		if depth > 32 { // a loop in the table, or something absurd: stop going down
			return
		}
		for _, c := range kids[p] {
			walk(c, depth+1)
			out = append(out, c)
		}
	}
	walk(pid, 0)
	return out
}

// provablyOurs is the rule that decides whether a process may be signalled at all. It is ours when its command line starts a Chrome with a
// profile in one of this tool's folders; or when it is registered here, and the operating system's start time of the process is still the
// one recorded at the launch (a process id reused by another program has another start time).
func (r *Registry) provablyOurs(pid int) bool {
	if r.proc == nil {
		return false
	}
	r.mu.Lock()
	e, registered := r.entries[pid]
	var recorded string
	if registered {
		recorded = e.ProcessStart
	}
	r.mu.Unlock()
	if registered {
		if recorded == "" {
			return false // no start time was recorded, so a reused id could not be told apart: never act on it
		}
		return r.proc.StartTime(pid) == recorded
	}
	return IsOurs(r.proc.CommandLine(pid))
}

func protected(pid int) bool {
	return pid <= 1 || pid == os.Getpid() || pid == os.Getppid()
}

// StopTree stops a Chrome of this tool and everything below it: children first, each asked to end (SIGTERM), then, after grace, whatever is
// still alive is killed (SIGKILL). It first proves the process is ours (provablyOurs) and refuses otherwise; it never signals this tool, its
// parent or init. The entry is released when the process is gone. It returns nil when the process was already gone.
func (r *Registry) StopTree(pid int, grace time.Duration) error {
	if protected(pid) {
		return ErrProtected
	}
	if r.proc == nil || !r.proc.Alive(pid) {
		r.Release(pid)
		return nil
	}
	if !r.provablyOurs(pid) {
		return fmt.Errorf("%w (pid %d)", ErrNotOurs, pid)
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

// terminateAll asks every process of the tree to end (children first), waits up to grace, kills what is left, waits until all are gone and
// releases the root's entry. It is the one place that signals.
func (r *Registry) terminateAll(tree []int, root int, grace time.Duration) error {
	for _, p := range tree {
		r.proc.Terminate(p)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if r.allGone(tree) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, p := range tree {
		if r.proc.Alive(p) {
			r.proc.Kill(p)
		}
	}
	// a killed process is gone as soon as the system has taken it down: wait briefly for that
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end) && !r.allGone(tree); {
		time.Sleep(10 * time.Millisecond)
	}
	if !r.allGone(tree) {
		return fmt.Errorf("chromeproc: pid %d or one of its children is still running after SIGKILL", root)
	}
	r.Release(root)
	return nil
}

func (r *Registry) allGone(pids []int) bool {
	for _, p := range pids {
		if r.proc.Alive(p) {
			return false
		}
	}
	return true
}

// StopTree on the tool's registry.
func StopTree(pid int, grace time.Duration) error { return std.StopTree(pid, grace) }

// IsOursPID says whether the running process with this id has a profile folder of this tool in its command line (read from the system now).
func IsOursPID(pid int) bool { return std.proc != nil && IsOurs(std.proc.CommandLine(pid)) }
