package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/evidence"
)

var at = time.Date(2026, 11, 9, 14, 2, 3, 0, time.UTC)

func jet(ours bool, stopped ...string) evidence.Finding {
	return evidence.Finding{Kind: evidence.KindJetsam, At: at, Program: "Google Chrome Helper", Stopped: stopped, Ours: ours, Reason: "vm-pageshortage",
		Largest: []evidence.Process{{Name: "Google Chrome Helper", Bytes: 2 << 30}, {Name: "llama-server", Bytes: 1 << 30}}}
}

func TestAReportOfOurOwnProgramCrashingBeatsEverythingElse(t *testing.T) {
	crash := evidence.Finding{Kind: evidence.KindCrash, Program: "cinqo-pro", At: at, Reason: "EXC_BAD_ACCESS SIGSEGV"}
	v, ev := VerdictWith(hb(16<<30, 8<<30, 0, "normal"), "2026-11-09T13:00:00Z", "2026-11-09T14:02:00Z", []evidence.Finding{jet(true, "llama-server"), crash})
	if !strings.HasPrefix(v, "Cinqo-pro crashed at ") || !strings.Contains(v, "(EXC_BAD_ACCESS SIGSEGV)") || !strings.Contains(v, "macOS wrote a crash report.") {
		t.Errorf("%s", v)
	}
	if got := findFact(ev, "System report: crash"); !strings.HasPrefix(got, "2026-11-09T14:02:03Z: cinqo-pro (EXC_BAD_ACCESS SIGSEGV)") {
		t.Errorf("%q", got)
	}
	if findFact(ev, "Last seen") == "<missing>" || findFact(ev, "Memory free") == "<missing>" {
		t.Error("the heartbeat's own facts stay beside the report's")
	}
}

func TestAMemoryKillThatListsOneOfOursSaysTheSystemStoppedCinqoForMemory(t *testing.T) {
	v, ev := VerdictWith(hb(16<<30, 8<<30, 0, "normal"), "", "", []evidence.Finding{jet(true, "llama-server", "Slack Helper")})
	for _, want := range []string{"macOS stopped Cinqo's programs because memory ran out at ", "lists llama-server, Slack Helper as stopped", "The biggest were Google Chrome Helper 2 GB, llama-server 1 GB."} {
		if !strings.Contains(v, want) {
			t.Errorf("%q missing in %q", want, v)
		}
	}
	if strings.Contains(v, "Memory looked fine") {
		t.Errorf("a report replaces the guess from the heartbeat: %s", v)
	}
	if got := findFact(ev, "System report: memory ran out"); got != "2026-11-09T14:02:03Z: stopped llama-server, Slack Helper (vm-pageshortage)" {
		t.Errorf("%q", got)
	}
}

func TestAMemoryKillThatDoesNotNameCinqoIsOnlyAHint(t *testing.T) {
	v, _ := VerdictWith(nil, "", "", []evidence.Finding{jet(false, "Slack Helper")})
	if !strings.Contains(v, "stopped other programs (Slack Helper)") || !strings.Contains(v, "probably why Cinqo stopped too, but its own name is not in the report") {
		t.Errorf("%s", v)
	}
	v, _ = VerdictWith(nil, "", "", []evidence.Finding{jet(false)})
	if !strings.Contains(v, "(none named)") {
		t.Errorf("%s", v)
	}
}

func TestNoFindingsLeavesTheHeartbeatVerdictUntouched(t *testing.T) {
	h := hb(16<<30, 8<<30, 0, "normal")
	a, ae := Verdict(h, "2026-11-09T13:00:00Z", "2026-11-09T14:00:00Z")
	b, be := VerdictWith(h, "2026-11-09T13:00:00Z", "2026-11-09T14:00:00Z", nil)
	if a != b || len(ae) != len(be) {
		t.Errorf("%q vs %q", a, b)
	}
	c, ce := VerdictWith(h, "2026-11-09T13:00:00Z", "2026-11-09T14:00:00Z", []evidence.Finding{})
	if a != c || len(ae) != len(ce) {
		t.Error("an empty list is no list")
	}
}

func TestTheLocalModelSentenceFollowsAReportVerdict(t *testing.T) {
	h := hb(16<<30, 8<<30, 0, "normal")
	h.LocalAi = entity.HeartbeatAgent{State: "working", Name: "phi-4-mini"}
	v, _ := VerdictWith(h, "", "", []evidence.Finding{jet(true, "llama-server")})
	if !strings.HasSuffix(v, " Phi-4-mini was loaded (working).") {
		t.Errorf("%s", v)
	}
	if ReportTail(nil) != "" || ReportTail(hb(1, 1, 0, "")) != "" {
		t.Error("no model, no sentence")
	}
}

func TestCleanReasonDropsPathsControlCharactersAndLength(t *testing.T) {
	cases := map[string]string{
		"failed to load model '/Users/andet/Library/Application Support/Cinqo/m.gguf'": "failed to load model '<path> Support/Cinqo/m.gguf'",
		"out of memory":           "out of memory",
		"a\x00b\nc\td":            "a b c d",
		"  spaced   out  ":        "spaced out",
		"see /var/log/x.log now":  "see <path> now",
		"ratio 3/4 is not a path": "ratio 3/4 is not a path",
		"":                        "",
	}
	for in, want := range cases {
		if got := CleanReason(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if got := CleanReason(strings.Repeat("x", 201)); len(got) != 200 {
		t.Errorf("%d", len(got))
	}
	if got := CleanReason(strings.Repeat("x", 200)); len(got) != 200 {
		t.Errorf("%d", len(got))
	}
	if got := CleanReason(strings.Repeat("é", 300)); len([]rune(got)) != 200 {
		t.Errorf("%d", len([]rune(got)))
	}
}

func TestTheLocalModelsLastWordsAndTheMemoryAtThatMoment(t *testing.T) {
	m := entity.HeartbeatMemory{TotalBytes: 8 << 30, AvailableBytes: 200 << 20, SwapUsedBytes: 2 << 30, Pressure: "critical"}
	v, ev := LocalAiVerdict("Phi-4-mini", "the model program stopped unexpectedly (exit code 7): out of memory", m)
	for _, want := range []string{"The local model (Phi-4-mini) stopped unexpectedly.", "Its last words: the model program stopped unexpectedly (exit code 7) out of memory.", "very tight at that moment (200 MB of 8 GB free, 2 GB of swap in use), which is a common reason."} {
		if !strings.Contains(strings.ReplaceAll(v, "(exit code 7):", "(exit code 7)"), strings.ReplaceAll(want, "(exit code 7):", "(exit code 7)")) && !strings.Contains(v, want) {
			t.Errorf("%q missing in %q", want, v)
		}
	}
	if findFact(ev, "Memory free") != "200 MB of 8 GB" || findFact(ev, "Memory pressure") != "critical" || findFact(ev, "Swap in use") != "2 GB" || !strings.Contains(findFact(ev, "Its last words"), "out of memory") {
		t.Errorf("%v", ev)
	}
	v, _ = LocalAiVerdict("", "", entity.HeartbeatMemory{TotalBytes: 16 << 30, AvailableBytes: 8 << 30, Pressure: "normal"})
	if v != "The local model stopped unexpectedly." {
		t.Errorf("no words, fine memory, no name: %q", v)
	}
	v, ev = LocalAiVerdict("X", "boom", entity.HeartbeatMemory{TotalBytes: 16 << 30, AvailableBytes: 3 << 30, Pressure: "normal"})
	if !strings.Contains(v, "getting tight at that moment (3 GB of 16 GB free).") || strings.Contains(v, "common reason") {
		t.Errorf("%s", v)
	}
	if findFact(ev, "Its last words") != "boom" {
		t.Errorf("%v", ev)
	}
	_, ev = LocalAiVerdict("X", "", entity.HeartbeatMemory{})
	if findFact(ev, "Its last words") != "unknown" || findFact(ev, "Memory free") != "unknown" {
		t.Errorf("%v", ev)
	}
}

func TestTheRecorderAsksForReportsOfTheStopAndWritesThemIntoTheEvent(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-7", 1, "", "2026-11-09T13:00:00Z", "2026-11-09T13:59:50Z", hbJSON(entity.Heartbeat{Memory: entity.HeartbeatMemory{TotalBytes: 8 << 30, AvailableBytes: 4 << 30, Pressure: "normal"}}))
	var from, to time.Time
	o := w.opts()
	o.Evidence = func(f, t time.Time) []evidence.Finding {
		from, to = f, t
		return []evidence.Finding{jet(true, "cinqo-pro")}
	}
	rec, _ := Start(o)
	defer rec.Close()
	if !from.Equal(time.Date(2026, 11, 9, 13, 59, 50, 0, time.UTC)) || !to.Equal(w.clk.now()) {
		t.Errorf("the window is from the last heartbeat to now: %v .. %v", from, to)
	}
	e := events(t, w.db)[0]
	if !strings.Contains(e.Verdict, "macOS stopped Cinqo's programs because memory ran out") || findFact(e.Evidence, "System report: memory ran out") == "<missing>" {
		t.Errorf("%+v", e)
	}
}

func TestARunWithAnUnreadableLastSeenAsksForNoReports(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-8", 1, "", "2026-11-09T13:00:00Z", "not a time", "")
	asked := false
	o := w.opts()
	o.Evidence = func(time.Time, time.Time) []evidence.Finding { asked = true; return nil }
	rec, _ := Start(o)
	defer rec.Close()
	if asked {
		t.Error("without a time there is no window to look in")
	}
	if len(events(t, w.db)) != 1 {
		t.Error("the event is still written")
	}
}

func TestAddEventWritesAnEventNowAndAFailureIsReported(t *testing.T) {
	w := newWorld(t)
	rec, _ := Start(w.opts())
	defer rec.Close()
	w.clk.add(5 * time.Minute)
	rec.AddEvent(entity.KindLocalAiStopped, "The local model stopped.", []entity.EvidenceItem{{Label: "a", Value: "b"}})
	ev := events(t, w.db)
	if len(ev) != 1 || ev[0].Kind != entity.KindLocalAiStopped || ev[0].At != "2026-11-09T14:05:00Z" || ev[0].Verdict != "The local model stopped." || len(ev[0].Evidence) != 1 {
		t.Errorf("%+v", ev)
	}
	w.db.Exec(`DROP TABLE system_events`)
	rec.AddEvent(entity.KindLocalAiStopped, "x", nil)
	if len(w.logs) == 0 {
		t.Error("a failed write is reported")
	}
}
