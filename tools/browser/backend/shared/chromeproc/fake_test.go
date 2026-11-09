package chromeproc

import "sync"

// fakeProc is a process table in memory: who is alive, their start times and command lines, their parents, and what a signal does.
type fakeProc struct {
	mu          sync.Mutex
	alive       map[int]bool
	starts      map[int]string
	cmds        map[int]string
	parents     map[int]int
	ignoreTerm  map[int]bool   // survives SIGTERM
	surviveKill map[int]bool   // survives SIGKILL (a stuck process)
	stats       map[int]string // process state letters; "Z" is defunct
	noList      bool           // the process table cannot be read
	terminated  []int
	killed      []int
}

func newFake() *fakeProc {
	return &fakeProc{
		alive:       map[int]bool{41: true, 42: true},
		starts:      map[int]string{41: "Mon Nov 9 14:00:00 2026", 42: "Mon Nov 9 14:00:05 2026"},
		cmds:        map[int]string{},
		parents:     map[int]int{},
		ignoreTerm:  map[int]bool{},
		surviveKill: map[int]bool{},
		stats:       map[int]string{},
	}
}

func (f *fakeProc) Alive(pid int) bool       { f.mu.Lock(); defer f.mu.Unlock(); return f.alive[pid] }
func (f *fakeProc) StartTime(pid int) string { f.mu.Lock(); defer f.mu.Unlock(); return f.starts[pid] }
func (f *fakeProc) CommandLine(pid int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cmds[pid]
}
func (f *fakeProc) Parents() map[int]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]int{}
	for k, v := range f.parents {
		out[k] = v
	}
	return out
}
func (f *fakeProc) List() []ProcInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.noList {
		return nil
	}
	var out []ProcInfo
	for pid, alive := range f.alive {
		if alive {
			out = append(out, ProcInfo{PID: pid, PPID: f.parents[pid], Stat: f.stats[pid], Command: f.cmds[pid]})
		}
	}
	return out
}
func (f *fakeProc) Terminate(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, pid)
	if !f.alive[pid] {
		return false
	}
	if !f.ignoreTerm[pid] {
		f.alive[pid] = false
	}
	return true
}
func (f *fakeProc) Kill(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killed = append(f.killed, pid)
	if !f.alive[pid] {
		return false
	}
	if !f.surviveKill[pid] {
		f.alive[pid] = false
	}
	return true
}
