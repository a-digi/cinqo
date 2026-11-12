package system

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/system/device"
	"github.com/a-digi/cinqo/src/system/memory"
)

func fakeReader(now *time.Time, calls *int) *Reader {
	return &Reader{
		memory: func() memory.MemoryInfo {
			*calls++
			return memory.MemoryInfo{TotalBytes: 8 << 30, AvailableBytes: 1 << 30, SwapUsedBytes: 2 << 30, Pressure: memory.PressureWarn}
		},
		resident: func(pid int) uint64 {
			switch pid {
			case 4242:
				return 400 << 20
			case 777:
				return 2900 << 20
			}
			return 1
		},
		pid:      func() int { return 4242 },
		localPid: func() int { return 777 },
		load:     func() (float64, int) { return 3.5, 8 },
		agent: func() AgentStatus {
			return AgentStatus{Name: "Phi-4-mini-instruct", State: AgentIdle, UnloadsInSeconds: 540}
		},
		device: func() device.Info {
			return device.Info{Model: "MacBook Air", Chip: "Apple M1", OS: "macOS", OSVersion: "15.1"}
		},
		disk: func() (uint64, uint64) { return 120 << 30, 500 << 30 },
		now:  func() time.Time { return *now },
	}
}

func TestReadFillsEveryFieldFromTheSources(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 900, time.UTC)
	calls := 0
	got := fakeReader(&now, &calls).Read()
	if got.Memory.TotalBytes != 8<<30 || got.Memory.AvailableBytes != 1<<30 || got.Memory.SwapUsedBytes != 2<<30 || got.Memory.Pressure != "warn" {
		t.Errorf("memory %+v", got.Memory)
	}
	if got.Cinqo.AppBytes != 400<<20 {
		t.Errorf("app bytes %d: the pid of this program must be asked, and only that", got.Cinqo.AppBytes)
	}
	if got.Cinqo.LocalAiBytes != 2900<<20 {
		t.Errorf("local AI bytes %d: the pid of the model program must be asked", got.Cinqo.LocalAiBytes)
	}
	if got.CPU.Load1 != 3.5 || got.CPU.Cores != 8 || got.Disk.FreeBytes != 120<<30 || got.Disk.TotalBytes != 500<<30 {
		t.Errorf("cpu %+v disk %+v", got.CPU, got.Disk)
	}
	if len(got.Agents) != 1 || got.Agents[0] != (AgentBody{Kind: "local-ai", Name: "Phi-4-mini-instruct", State: "idle", MemoryBytes: 2900 << 20, UnloadsInSeconds: 540}) {
		t.Errorf("agents %+v", got.Agents)
	}
	if got.Device.Model != "MacBook Air" || got.Device.Chip != "Apple M1" || got.Device.OS != "macOS" || got.Device.OSVersion != "15.1" {
		t.Errorf("device %+v", got.Device)
	}
	if !got.SampledAt.Equal(time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("sampledAt %v", got.SampledAt)
	}
}

func TestTheReadingIsKeptForOneSecondAndThenRenewed(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	calls := 0
	r := fakeReader(&now, &calls)
	r.Read()
	now = now.Add(999 * time.Millisecond)
	r.Read()
	if calls != 1 {
		t.Errorf("%d reads within one second, want 1", calls)
	}
	now = now.Add(time.Millisecond)
	r.Read()
	if calls != 2 {
		t.Errorf("%d reads after one second, want 2", calls)
	}
	now = now.Add(-time.Hour) // the clock went back: never serve a reading from the future
	r.Read()
	if calls != 3 {
		t.Errorf("%d reads after the clock went back, want 3", calls)
	}
}

func TestAnUnreadableComputerIsZeroAndUnknownNotAnError(t *testing.T) {
	now := time.Now()
	r := &Reader{
		memory:   func() memory.MemoryInfo { return memory.MemoryInfo{Pressure: memory.PressureUnknown} },
		resident: func(int) uint64 { return 0 },
		pid:      func() int { return 1 },
		now:      func() time.Time { return now },
	}
	raw, err := json.Marshal(r.Read())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"totalBytes":0`, `"availableBytes":0`, `"pressure":"unknown"`, `"appBytes":0`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s missing in %s", want, raw)
		}
	}
}

func TestNoModelProgramIsZeroNotTheShellsOwnMemory(t *testing.T) {
	now := time.Now()
	r := &Reader{
		memory:   func() memory.MemoryInfo { return memory.MemoryInfo{} },
		resident: func(pid int) uint64 { return uint64(pid) * 10 },
		pid:      func() int { return 5 },
		now:      func() time.Time { return now },
	}
	if got := r.Read().Cinqo.LocalAiBytes; got != 0 {
		t.Errorf("no source: %d, want 0", got)
	}
	r.at = time.Time{}
	r.localPid = func() int { return 0 }
	if got := r.Read().Cinqo; got.LocalAiBytes != 0 || got.AppBytes != 50 {
		t.Errorf("source says 0 (not running): %+v", got)
	}
}

func TestTheModuleHandsOverItsPidAndTheRealReaderUsesIt(t *testing.T) {
	old := localAiSource.Load()
	t.Cleanup(func() { localAiSource.Store(old) })
	localAiSource.Store(nil)
	if localAiPid() != 0 {
		t.Error("without a source the pid is 0")
	}
	SetLocalAiPid(func() int { return 31337 })
	if got := localAiPid(); got != 31337 {
		t.Errorf("got %d", got)
	}
	if NewReader().localPid() != 31337 {
		t.Error("the real reader must read the pid through the source")
	}
}

func TestNoSourcesIsUnknownNotZeroLoad(t *testing.T) {
	r := &Reader{memory: func() memory.MemoryInfo { return memory.MemoryInfo{} }, resident: func(int) uint64 { return 0 }, pid: func() int { return 1 }, now: time.Now}
	got := r.Read()
	if got.Device != (device.Info{}) {
		t.Errorf("no source: %+v", got.Device)
	}
	if got.CPU.Load1 != -1 || got.CPU.Cores != 0 || got.Disk.FreeBytes != 0 || got.Disk.TotalBytes != 0 {
		t.Errorf("cpu %+v disk %+v", got.CPU, got.Disk)
	}
}

func TestDataDirIsKeptForTheDiskFigures(t *testing.T) {
	old := dataDir.Load()
	t.Cleanup(func() { dataDir.Store(old) })
	dataDir.Store(nil)
	if dataDirValue() != "" {
		t.Error("unset means empty")
	}
	SetDataDir("/some/dir")
	if dataDirValue() != "/some/dir" {
		t.Error("the folder must be kept")
	}
}

func TestNoRunningAgentIsAnEmptyListNotNull(t *testing.T) {
	now := time.Now()
	base := func() *Reader {
		return &Reader{memory: func() memory.MemoryInfo { return memory.MemoryInfo{} }, resident: func(pid int) uint64 { return uint64(pid) * 5 }, pid: func() int { return 1 }, localPid: func() int { return 9 }, now: func() time.Time { return now }}
	}
	for name, agent := range map[string]func() AgentStatus{
		"no source":   nil,
		"stopped":     func() AgentStatus { return AgentStatus{State: AgentNone} },
		"empty state": func() AgentStatus { return AgentStatus{} },
	} {
		r := base()
		r.agent = agent
		raw, err := json.Marshal(r.Read())
		if err != nil || !strings.Contains(string(raw), `"agents":[]`) {
			t.Errorf("%s: %s %v", name, raw, err)
		}
	}
	r := base()
	r.agent = func() AgentStatus { return AgentStatus{Name: "m", State: AgentStarting} }
	if got := r.Read().Agents; len(got) != 1 || got[0].State != "starting" || got[0].MemoryBytes != 45 {
		t.Errorf("a starting agent is listed with the memory of its process: %+v", got)
	}
}

func TestTheStatusIsHandedOverByTheModule(t *testing.T) {
	old := localAiStatusSource.Load()
	t.Cleanup(func() { localAiStatusSource.Store(old) })
	localAiStatusSource.Store(nil)
	if localAiStatus().State != AgentNone {
		t.Error("without a source nothing runs")
	}
	SetLocalAiStatus(func() AgentStatus { return AgentStatus{State: AgentWorking} })
	if localAiStatus().State != AgentWorking || NewReader().agent().State != AgentWorking {
		t.Error("the real reader must read the status through the source")
	}
}

func TestTheRealReaderAnswersOnThisComputer(t *testing.T) {
	got := NewReader().Read()
	if got.Memory.TotalBytes == 0 || got.Cinqo.AppBytes == 0 {
		t.Skipf("this system does not say: %+v", got)
	}
	if got.Memory.AvailableBytes > got.Memory.TotalBytes {
		t.Errorf("available %d above total %d", got.Memory.AvailableBytes, got.Memory.TotalBytes)
	}
}

func TestTheScopesAreRegisteredAsTheRoutesNeedThem(t *testing.T) {
	if ScopeGroup.ID != "cinqo:system" || len(ScopeGroup.Scopes) != 2 || ScopeGroup.Scopes[0].ID != "cinqo:system:read" || ScopeGroup.Scopes[1].ID != "cinqo:system:manage" {
		t.Errorf("%+v", ScopeGroup)
	}
	for _, sc := range ScopeGroup.Scopes {
		if sc.Description == "" {
			t.Errorf("%s has no description", sc.ID)
		}
	}
}

// The routes are authenticated; the reads need the system read scope or super admin, marking a stop as seen (a change) needs the manage scope or super admin.
func TestTheRoutesAreAuthenticatedAndScoped(t *testing.T) {
	raw, err := os.ReadFile("../../config/routes/route-system.yaml")
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(string(raw), "  - path:")[1:]
	type want struct{ path, method, executor, scope string }
	wants := []want{
		{"/api/v1/system/resources", "GET", "SystemResourcesGet", "cinqo:system:read"},
		{"/api/v1/system/memory-top", "GET", "SystemMemoryTopGet", "cinqo:system:read"},
		{"/api/v1/system/events", "GET", "SystemEventsGet", "cinqo:system:read"},
		{"/api/v1/system/events/summary", "GET", "SystemEventsSummaryGet", "cinqo:system:read"},
		{"/api/v1/system/events/{id}", "GET", "SystemEventGet", "cinqo:system:read"},
		{"/api/v1/system/events/acknowledge-all", "POST", "SystemEventsAcknowledgeAll", "cinqo:system:manage"},
		{"/api/v1/system/events/{id}/acknowledge", "POST", "SystemEventAcknowledge", "cinqo:system:manage"},
	}
	if len(blocks) != len(wants) {
		t.Fatalf("%d routes, want %d", len(blocks), len(wants))
	}
	for i, w := range wants {
		b := blocks[i]
		for _, must := range []string{w.path, "method: " + w.method, "executor: " + w.executor, "security: authenticated", w.scope, "cinqo:super:admin"} {
			if !strings.Contains(b, must) {
				t.Errorf("%q missing:\n%s", must, b)
			}
		}
		if n := len(regexp.MustCompile(`cinqo:[a-z]+:[a-z]+`).FindAllString(b, -1)); n != 2 {
			t.Errorf("%s: %d scopes, want exactly 2", w.path, n)
		}
		if w.method == "GET" && (strings.Contains(b, "cinqo:system:manage") || strings.Contains(b, "method: POST")) {
			t.Errorf("%s is a read: it must not need or allow a change", w.path)
		}
		if w.method == "POST" && strings.Contains(b, "cinqo:system:read") {
			t.Errorf("%s changes data: the read scope alone must not allow it", w.path)
		}
	}
}
