package localai

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/system"
)

func TestAgentStatusSaysWhatTheProgramDoes(t *testing.T) {
	cases := []struct {
		name string
		v    agentView
		want system.AgentStatus
	}{
		{"stopped", agentView{state: entity.EngineStopped}, system.AgentStatus{State: system.AgentNone}},
		{"starting", agentView{state: entity.EngineStarting, modelID: "qwen2.5-1.5b-instruct-q4_k_m"}, system.AgentStatus{Name: "Qwen2.5-1.5B-Instruct", State: system.AgentStarting}},
		{"working", agentView{state: entity.EngineReady, modelID: "qwen2.5-1.5b-instruct-q4_k_m", busy: true, unloadsIn: time.Minute}, system.AgentStatus{Name: "Qwen2.5-1.5B-Instruct", State: system.AgentWorking, UnloadsInSeconds: 60}},
		{"idle", agentView{state: entity.EngineReady, modelID: "qwen2.5-1.5b-instruct-q4_k_m", unloadsIn: 90*time.Second + time.Millisecond}, system.AgentStatus{Name: "Qwen2.5-1.5B-Instruct", State: system.AgentIdle, UnloadsInSeconds: 91}},
		{"idle, never unloads", agentView{state: entity.EngineReady, modelID: "x"}, system.AgentStatus{Name: "x", State: system.AgentIdle}},
		{"failed", agentView{state: entity.EngineFailed, modelID: "x"}, system.AgentStatus{Name: "x", State: system.AgentFailed}},
	}
	for _, c := range cases {
		got := agentStatus(c.v)
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestModelNameIsTheNameThenTheIdThenNothing(t *testing.T) {
	if got := modelName(""); got != "" {
		t.Errorf("no id: %q", got)
	}
	if got := modelName("not-in-any-list"); got != "not-in-any-list" {
		t.Errorf("unknown id: %q", got)
	}
}

func TestTheViewFollowsTheProgramThroughItsLife(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_DELAY_MS=300")
	if v := s.view(); v.state != EngineStopped || v.busy || v.unloadsIn != 0 {
		t.Fatalf("before any start: %+v", v)
	}
	done := make(chan func())
	go func() {
		_, _, release, err := s.ensure(context.Background(), testSpec(t), 100) // 100 idle units = 6 s
		if err != nil {
			t.Error(err)
			release = func() {}
		}
		done <- release
	}()
	waitUntil(t, "starting", func() bool { return s.view().state == EngineStarting })
	release := <-done
	if v := s.view(); v.state != EngineReady || !v.busy || v.modelID != "tiny" || v.unloadsIn != 0 {
		t.Fatalf("a request is under way: %+v (no unload countdown while busy)", v)
	}
	release()
	v := s.view()
	if v.state != EngineReady || v.busy || v.unloadsIn <= 5*time.Second || v.unloadsIn > 6*time.Second {
		t.Fatalf("idle with a 6 s idle time: %+v", v)
	}
	s.Stop()
	if v := s.view(); v.state != EngineStopped || v.unloadsIn != 0 {
		t.Fatalf("after the stop: %+v", v)
	}
}

func TestAnIdleTimeOfZeroHasNoCountdownAndACrashIsFailed(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if v := s.view(); v.state != EngineReady || v.unloadsIn != 0 {
		t.Fatalf("never unloads: %+v", v)
	}
	s.Stop()

	crash := newSup(t, &starts, "FAKE_LLAMA_CRASH_MS=150")
	_, _, rel, err := crash.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	waitUntil(t, "the crash", func() bool { return crash.view().state == EngineFailed })
}

func TestAnIdleStopClearsTheCountdown(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 1) // 60 ms
	if err != nil {
		t.Fatal(err)
	}
	release()
	waitUntil(t, "the idle stop", func() bool { return s.view().state == EngineStopped })
	if v := s.view(); v.unloadsIn != 0 {
		t.Errorf("%+v", v)
	}
}
