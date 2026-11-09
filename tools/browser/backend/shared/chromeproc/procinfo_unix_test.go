//go:build unix

package chromeproc

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestTheRealReaderSeesARealProcessComeAndGo(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start sleep")
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	var p realProc
	if !p.Alive(pid) {
		t.Fatal("a running process must be alive")
	}
	if !p.Alive(os.Getpid()) {
		t.Error("this process must be alive")
	}
	start := p.StartTime(pid)
	if start == "" {
		t.Skip("ps cannot tell the start time here")
	}
	if p.StartTime(pid) != start {
		t.Error("the start time of one process must not change")
	}
	if other := p.StartTime(os.Getpid()); other == "" {
		t.Error("this process has a start time too")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for p.Alive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Alive(pid) {
		t.Error("a killed and reaped process must not be alive")
	}
	if p.StartTime(pid) != "" {
		t.Error("a process that is gone has no start time")
	}
}

func TestTheRealReaderRefusesIdsThatAreNotProcesses(t *testing.T) {
	var p realProc
	if p.Alive(0) || p.Alive(-1) {
		t.Error("0 and negative ids are never alive (kill(0) would signal our whole group)")
	}
	if p.StartTime(0) != "" || p.StartTime(-4) != "" {
		t.Error("no start time for a non-process")
	}
}
