//go:build unix

package chromeproc

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type realProc struct{}

// Alive: signal 0 only asks whether the process exists (EPERM means it exists but belongs to someone else).
func (realProc) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// psCandidates are the places `ps` lives; an absolute path is always used, never a search of the path.
var psCandidates = []string{"/bin/ps", "/usr/bin/ps"}

// StartTime is the process's start time as `ps -o lstart=` prints it ("Mon Nov  9 14:00:00 2026"): fixed per process, so a reused id differs.
func (realProc) StartTime(pid int) string {
	if pid <= 0 {
		return ""
	}
	for _, ps := range psCandidates {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, ps, "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
		cancel()
		if err == nil {
			return strings.Join(strings.Fields(string(out)), " ")
		}
	}
	return ""
}

// CommandLine is `ps -ww -o command= -p pid`: the full command line, however long.
func (realProc) CommandLine(pid int) string {
	if pid <= 0 {
		return ""
	}
	for _, ps := range psCandidates {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, ps, "-ww", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		cancel()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// Parents reads the whole process table once: `ps -axo pid=,ppid=`.
func (realProc) Parents() map[int]int {
	for _, ps := range psCandidates {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, ps, "-axo", "pid=,ppid=").Output()
		cancel()
		if err != nil {
			continue
		}
		table := map[int]int{}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 {
				continue
			}
			pid, err1 := strconv.Atoi(f[0])
			ppid, err2 := strconv.Atoi(f[1])
			if err1 == nil && err2 == nil {
				table[pid] = ppid
			}
		}
		return table
	}
	return nil
}

func (realProc) Terminate(pid int) bool { return pid > 1 && syscall.Kill(pid, syscall.SIGTERM) == nil }
func (realProc) Kill(pid int) bool      { return pid > 1 && syscall.Kill(pid, syscall.SIGKILL) == nil }
