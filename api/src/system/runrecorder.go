package system

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/a-digi/cinqo/src/system/memory"
	"github.com/a-digi/cinqo/src/system/runs"
	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/evidence"
)

// sampleHeartbeat is what the app writes down about itself every few seconds: the computer's memory, its own memory and the local model, read
// from the same sources as the resource bar.
func sampleHeartbeat() entity.Heartbeat {
	m := memory.Memory()
	st := localAiStatus()
	hb := entity.Heartbeat{
		Memory:  entity.HeartbeatMemory{TotalBytes: m.TotalBytes, AvailableBytes: m.AvailableBytes, SwapUsedBytes: m.SwapUsedBytes, Pressure: string(m.Pressure)},
		AppByte: memory.Resident(os.Getpid()),
		LocalAi: entity.HeartbeatAgent{State: st.State, Name: st.Name},
	}
	if st.State != AgentNone && st.State != "" {
		if pid := localAiPid(); pid > 0 {
			hb.LocalAi.Bytes = memory.Resident(pid)
		}
	}
	return hb
}

// StartRunRecorder starts the flight recorder of this run (plan 223, step 3): at the next start an unexpected stop of this run is found and written
// as an event. The returned function says the app is stopping properly; register it as a shutdown hook. A recorder that cannot start (no
// database) never stops the app: the function is then a no-op.
func StartRunRecorder(db *sql.DB, logf func(format string, args ...any)) func() {
	rec, err := runs.Start(runs.Options{DB: db, PID: os.Getpid(), Sample: sampleHeartbeat, Evidence: systemReports(), Log: logf})
	if err != nil {
		if logf != nil {
			logf("run recorder: not started: %v", err)
		}
		return func() {}
	}
	recorder.Store(rec)
	return func() {
		recorder.Store(nil)
		rec.Close()
	}
}

var recorder atomic.Pointer[runs.Recorder]

// ownProgramNames are the names Cinqo's own programs go by in a crash report: the running program and the model program.
func ownProgramNames() []string {
	names := []string{"llama-server"}
	if exe, err := os.Executable(); err == nil {
		names = append(names, filepath.Base(exe))
	}
	return names
}

// systemReports reads macOS's own reports about a stop (memory-kill and crash reports); on other systems there is nothing to read.
func systemReports() func(from, to time.Time) []evidence.Finding {
	if runtime.GOOS != "darwin" {
		return nil
	}
	home, _ := os.UserHomeDir()
	src := evidence.DirSource{Dirs: evidence.DefaultDirs(home)}
	return func(from, to time.Time) []evidence.Finding { return evidence.Collect(src, from, to, ownProgramNames()) }
}

// ReportLocalAiStopped records that the model program stopped on its own while the app lives (plan 223, step 4), so that it is still known after a
// restart. name is the model's name, reason the program's last words. Without a running recorder it does nothing.
func ReportLocalAiStopped(name, reason string) {
	rec := recorder.Load()
	if rec == nil {
		return
	}
	m := memory.Memory()
	verdict, facts := runs.LocalAiVerdict(name, reason, entity.HeartbeatMemory{TotalBytes: m.TotalBytes, AvailableBytes: m.AvailableBytes, SwapUsedBytes: m.SwapUsedBytes, Pressure: string(m.Pressure)})
	rec.AddEvent(entity.KindLocalAiStopped, verdict, facts)
}
