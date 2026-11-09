package system

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/system/memory"
)

func fakeTop(now *time.Time, calls *int, rows []memory.ProcRow) *TopReader {
	return &TopReader{
		table:  func() []memory.ProcRow { *calls++; return rows },
		total:  func() uint64 { return 16 << 30 },
		ownPID: func() int { return 100 },
		now:    func() time.Time { return *now },
	}
}

var sampleRows = []memory.ProcRow{
	{PID: 1, PPID: 0, RSSKB: 100, Command: "/sbin/launchd"},
	{PID: 100, PPID: 1, RSSKB: 60000, Command: "./app/cinqo-pro"},
	{PID: 101, PPID: 100, RSSKB: 40000, Command: "/data/tools/browser/backend/tool"},
	{PID: 200, PPID: 1, RSSKB: 300000, Command: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
	{PID: 201, PPID: 200, RSSKB: 200000, Command: "/Applications/Google Chrome.app/Contents/Frameworks/H.app/Contents/MacOS/H"},
	{PID: 300, PPID: 1, RSSKB: 150000, Command: "claude"},
}

func TestTheTopReadingHasTheTotalTheAppsAndTheCinqoRow(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 700, time.UTC)
	calls := 0
	got := fakeTop(&now, &calls, sampleRows).Read()
	if got.TotalBytes != 16<<30 || !got.SampledAt.Equal(time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)) || got.Note != "" {
		t.Errorf("%+v", got)
	}
	names := []string{}
	for _, a := range got.Apps {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "Google Chrome,claude,Cinqo,launchd" {
		t.Errorf("%v", names)
	}
	if c := got.Apps[0]; c.Processes != 2 || c.PID != 200 || c.RSSBytes != 500000*1024 || c.Cinqo {
		t.Errorf("%+v", c)
	}
	if c := got.Apps[2]; !c.Cinqo || c.Processes != 2 || c.PID != 100 {
		t.Errorf("Cinqo's own processes are one row: %+v", c)
	}
}

func TestTheTopListIsCutToTenApplications(t *testing.T) {
	var rows []memory.ProcRow
	for i := 1; i <= 25; i++ {
		rows = append(rows, memory.ProcRow{PID: 1000 + i, PPID: 1, RSSKB: uint64(i) * 10, Command: "/bin/app" + string(rune('a'+i))})
	}
	now := time.Now()
	calls := 0
	got := fakeTop(&now, &calls, rows).Read()
	if TopCount != 10 || len(got.Apps) != 10 || got.Apps[0].RSSBytes != 250*1024 || got.Apps[9].RSSBytes != 160*1024 {
		t.Errorf("%d apps: %+v", len(got.Apps), got.Apps)
	}
}

func TestTheTopReadingIsKeptForOneSecond(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	calls := 0
	r := fakeTop(&now, &calls, sampleRows)
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
	now = now.Add(-time.Hour)
	r.Read()
	if calls != 3 {
		t.Errorf("%d reads after the clock went back, want 3", calls)
	}
}

func TestAnUnreadableProcessTableIsAnEmptyListWithANoteNotAnError(t *testing.T) {
	now := time.Now()
	r := &TopReader{table: func() []memory.ProcRow { return nil }, total: func() uint64 { return 8 << 30 }, ownPID: func() int { return 1 }, now: func() time.Time { return now }}
	raw, err := json.Marshal(r.Read())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"apps":[]`, `"totalBytes":8589934592`, `"note":"The list of running programs cannot be read on this system."`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s missing in %s", want, raw)
		}
	}
}

func TestAReadableButEmptyTableHasNoNote(t *testing.T) {
	now := time.Now()
	r := &TopReader{table: func() []memory.ProcRow { return []memory.ProcRow{} }, total: func() uint64 { return 1 }, ownPID: func() int { return 1 }, now: func() time.Time { return now }}
	got := r.Read()
	raw, _ := json.Marshal(got)
	if got.Note != "" || !strings.Contains(string(raw), `"apps":[]`) {
		t.Errorf("%s", raw)
	}
}

func TestTheJSONShapeOfAnAppIsWhatThePageReads(t *testing.T) {
	now := time.Now()
	calls := 0
	raw, _ := json.Marshal(fakeTop(&now, &calls, sampleRows).Read().Apps[0])
	for _, want := range []string{`"name":"Google Chrome"`, `"pid":200`, `"processes":2`, `"rssBytes":512000000`, `"cinqo":false`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s missing in %s", want, raw)
		}
	}
	for _, secret := range []string{"Applications", "Contents", "/"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("an executable path must never leave: %q in %s", secret, raw)
		}
	}
}

func TestTheRealTopReaderAnswersOnThisComputer(t *testing.T) {
	got := NewTopReader().Read()
	if got.TotalBytes == 0 || len(got.Apps) == 0 {
		t.Skipf("this system does not say: %+v", got)
	}
	if len(got.Apps) > TopCount {
		t.Errorf("%d apps", len(got.Apps))
	}
	for i, a := range got.Apps {
		if a.Name == "" || a.PID <= 0 || a.RSSBytes == 0 || (i > 0 && a.RSSBytes > got.Apps[i-1].RSSBytes) {
			t.Errorf("row %d: %+v", i, a)
		}
	}
}

// A route without a handler fails when the server starts: every executor of the route file must be registered in routes.go.
func TestEveryExecutorOfTheRouteFileIsRegisteredInTheHandlerMap(t *testing.T) {
	yaml, err := os.ReadFile("../../config/routes/route-system.yaml")
	if err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile("../../config/routes/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, m := range regexp.MustCompile(`(?m)^\s*executor:\s*(\S+)\s*$`).FindAllStringSubmatch(string(yaml), -1) {
		found++
		if !strings.Contains(string(code), `"`+m[1]+`":`) {
			t.Errorf("executor %s has no handler in routes.go", m[1])
		}
	}
	if found != 2 {
		t.Errorf("%d executors, want 2", found)
	}
}
