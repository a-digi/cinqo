package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/system/runs/entity"
)

func TestTheOwnProgramNamesAreTheRunningProgramAndTheModelProgram(t *testing.T) {
	names := ownProgramNames()
	exe, _ := os.Executable()
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
	if !has("llama-server") || !has(filepath.Base(exe)) {
		t.Errorf("%v (running program %s)", names, filepath.Base(exe))
	}
}

func TestReportingTheLocalModelWithoutARecorderDoesNothing(t *testing.T) {
	recorder.Store(nil)
	ReportLocalAiStopped("Phi", "boom") // must not panic or block
}

func TestARecorderWithoutADatabaseNeverStopsTheApp(t *testing.T) {
	var logs []string
	stop := StartRunRecorder(nil, func(f string, a ...any) { logs = append(logs, f) })
	if stop == nil {
		t.Fatal("always a function to call")
	}
	stop()
	if len(logs) != 1 || !strings.Contains(logs[0], "not started") {
		t.Errorf("the reason must be reported: %v", logs)
	}
}

func TestTheHeartbeatSampleHasTheMemoryTheProgramAndTheLocalModel(t *testing.T) {
	old := localAiStatusSource.Load()
	oldPid := localAiSource.Load()
	t.Cleanup(func() { localAiStatusSource.Store(old); localAiSource.Store(oldPid) })

	localAiStatusSource.Store(nil)
	localAiSource.Store(nil)
	hb := sampleHeartbeat()
	if hb.Memory.TotalBytes == 0 || hb.AppByte == 0 {
		t.Skipf("this system does not say: %+v", hb)
	}
	if hb.Memory.AvailableBytes > hb.Memory.TotalBytes || hb.Memory.Pressure == "" {
		t.Errorf("%+v", hb.Memory)
	}
	if hb.LocalAi.State != AgentNone || hb.LocalAi.Bytes != 0 {
		t.Errorf("no model: %+v", hb.LocalAi)
	}

	// a running model: its state, its name and the memory of its process (this process stands in for the model program)
	SetLocalAiStatus(func() AgentStatus { return AgentStatus{Name: "Phi-4-mini", State: AgentWorking} })
	SetLocalAiPid(os.Getpid)
	hb = sampleHeartbeat()
	if hb.LocalAi.State != AgentWorking || hb.LocalAi.Name != "Phi-4-mini" || hb.LocalAi.Bytes == 0 {
		t.Errorf("a running model: %+v", hb.LocalAi)
	}

	// a model that is not running holds no memory, even if a process id is known
	SetLocalAiStatus(func() AgentStatus { return AgentStatus{State: AgentNone} })
	if hb = sampleHeartbeat(); hb.LocalAi.Bytes != 0 {
		t.Errorf("none running: %+v", hb.LocalAi)
	}
	SetLocalAiStatus(func() AgentStatus { return AgentStatus{} })
	if hb = sampleHeartbeat(); hb.LocalAi.Bytes != 0 {
		t.Errorf("empty state: %+v", hb.LocalAi)
	}
}

func TestAStartedRecorderTakesTheModelsCrashAndAStoppedOneDoesNot(t *testing.T) {
	db := openMainDB(t)
	stop := StartRunRecorder(db, nil)
	ReportLocalAiStopped("Phi-4-mini", "the model program stopped unexpectedly (exit code 7): out of memory")
	ev := readEvents(t, db)
	if len(ev) != 1 {
		t.Fatalf("%d events", len(ev))
	}
	for _, e := range ev {
		if e.Kind != entity.KindLocalAiStopped || !strings.Contains(e.Verdict, "The local model (Phi-4-mini) stopped unexpectedly.") || !strings.Contains(e.Verdict, "out of memory") {
			t.Errorf("%+v", e)
		}
	}
	stop()
	ReportLocalAiStopped("Phi-4-mini", "again")
	if n := len(readEvents(t, db)); n != 1 {
		t.Errorf("after the app said it was stopping nothing more is written: %d events", n)
	}
}
