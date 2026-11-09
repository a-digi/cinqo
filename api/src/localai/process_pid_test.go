package localai

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
)

// The resource bar asks the supervisor which process is the model program: its pid while it runs, 0 before the first start and after the stop.
func TestThePidIsTheRunningProgramAndZeroOtherwise(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	if got := s.pid(); got != 0 {
		t.Fatalf("before any start: %d, want 0", got)
	}
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	pid := s.pid()
	if pid <= 1 || pid == os.Getpid() {
		t.Fatalf("running: pid %d, want the model program's own pid", pid)
	}
	if !processAlive(pid) {
		t.Errorf("pid %d is not a live process", pid)
	}
	s.Stop()
	if got := s.pid(); got != 0 {
		t.Errorf("after the stop: %d, want 0", got)
	}
}
