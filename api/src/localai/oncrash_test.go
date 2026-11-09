package localai

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A program that dies on its own while ready tells whoever listens (plan 223, step 4): the model and its last words, once, outside the lock.
func TestACrashOfAReadyProgramIsReportedOnceWithItsLastWords(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_CRASH_MS=150")
	type call struct{ model, reason string }
	got := make(chan call, 4)
	s.onCrash = func(model, reason string) {
		// the callback runs outside the lock: asking the supervisor from inside it must not block
		_ = s.Status()
		got <- call{model, reason}
	}
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case c := <-got:
		if c.model != "tiny" || !strings.Contains(c.reason, "stopped unexpectedly (exit code 7)") || !strings.Contains(c.reason, "out of memory") {
			t.Errorf("%+v", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the crash must be reported")
	}
	select {
	case c := <-got:
		t.Errorf("reported twice: %+v", c)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAStopOnPurposeIsNotACrash(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	var called atomic.Int32
	s.onCrash = func(string, string) { called.Add(1) }
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	s.unload(false)
	s.Stop()
	time.Sleep(300 * time.Millisecond)
	if called.Load() != 0 {
		t.Errorf("%d reports for stops the person or the app asked for", called.Load())
	}
}

func TestACrashWithNoListenerIsStillAFailedState(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_CRASH_MS=150")
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	waitUntil(t, "the crash", func() bool { return s.view().state == EngineFailed })
}

// A program that dies while it is still loading is the start's failure, not a crash of a ready program: nobody is told.
func TestAProgramThatExitsWhileLoadingIsNotReportedAsACrash(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_MODE=exit")
	var called atomic.Int32
	s.onCrash = func(string, string) { called.Add(1) }
	if _, _, _, err := s.ensure(context.Background(), testSpec(t), 0); err == nil {
		t.Fatal("the start must fail")
	}
	time.Sleep(300 * time.Millisecond)
	if called.Load() != 0 {
		t.Errorf("%d crash reports for a start that never got ready", called.Load())
	}
}
