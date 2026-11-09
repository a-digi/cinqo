package runs

import (
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/system/runs/entity"
)

func hb(total, avail, swap uint64, pressure string) *entity.Heartbeat {
	return &entity.Heartbeat{Memory: entity.HeartbeatMemory{TotalBytes: total, AvailableBytes: avail, SwapUsedBytes: swap, Pressure: pressure}}
}

func TestTheVerdictFollowsHowTightTheMemoryWas(t *testing.T) {
	const GB = 1 << 30
	cases := []struct {
		name string
		h    *entity.Heartbeat
		want string // a phrase the verdict must contain
		not  string
	}{
		{"critical pressure", hb(16*GB, 10*GB, 0, "critical"), "very tight", ""},
		{"under 10 % free", hb(16*GB, 1*GB, 0, "normal"), "very tight", ""},
		{"exactly 10 % free is not red", hb(10*GB, 1*GB, 0, "normal"), "getting tight", "very tight"},
		{"more than 3 GB of swap", hb(16*GB, 8*GB, 3*GB+1, "normal"), "very tight", ""},
		{"exactly 3 GB of swap is not red", hb(16*GB, 8*GB, 3*GB, "normal"), "getting tight", "very tight"},
		{"pressure warn", hb(16*GB, 10*GB, 0, "warn"), "getting tight", "very tight"},
		{"under 25 % free", hb(16*GB, 3*GB, 0, "normal"), "getting tight", "very tight"},
		{"exactly 25 % free is fine", hb(8*GB, 2*GB, 0, "normal"), "Memory looked fine", "tight"},
		{"more than 1 GB of swap", hb(16*GB, 8*GB, 1*GB+1, "normal"), "getting tight", "very tight"},
		{"exactly 1 GB of swap is fine", hb(16*GB, 8*GB, 1*GB, "normal"), "Memory looked fine", "tight"},
		{"plenty of memory", hb(16*GB, 8*GB, 0, "normal"), "Memory looked fine", "tight"},
		{"no total known", hb(0, 0, 0, "unknown"), "Memory looked fine", "tight"},
		{"an unreadable figure is not a shortage", hb(16*GB, 0, 0, "unknown"), "Memory looked fine", "tight"},
		{"zero free with a known pressure IS a shortage", hb(16*GB, 0, 0, "normal"), "very tight", ""},
	}
	for _, c := range cases {
		v, _ := Verdict(c.h, "2026-11-09T13:00:00Z", "2026-11-09T13:10:00Z")
		if !strings.Contains(v, c.want) || (c.not != "" && strings.Contains(v, c.not)) {
			t.Errorf("%s: %q (want %q, not %q)", c.name, v, c.want, c.not)
		}
	}
}

func TestAnOrangeVerdictSaysItMayStillBeACrash(t *testing.T) {
	v, _ := Verdict(hb(16<<30, 3<<30, 0, "normal"), "2026-11-09T13:00:00Z", "2026-11-09T13:10:00Z")
	if !strings.Contains(v, "may also be a crash or a force quit") || !strings.Contains(v, "3 GB of 16 GB free") {
		t.Errorf("%s", v)
	}
}

func TestTheLocalModelIsNamedOnlyWhenItWasLoaded(t *testing.T) {
	h := hb(16<<30, 8<<30, 0, "normal")
	h.LocalAi = entity.HeartbeatAgent{State: "idle", Name: "qwen2.5-1.5b", Bytes: 1 << 30}
	if v, _ := Verdict(h, "", ""); !strings.Contains(v, "Qwen2.5-1.5b was loaded (idle).") {
		t.Errorf("%s", v)
	}
	h.LocalAi = entity.HeartbeatAgent{State: "starting"}
	if v, _ := Verdict(h, "", ""); !strings.Contains(v, "The local model was loaded (starting).") {
		t.Errorf("%s", v)
	}
	for _, state := range []string{"", "none"} {
		h.LocalAi = entity.HeartbeatAgent{State: state}
		if v, ev := Verdict(h, "", ""); strings.Contains(v, "loaded") || findFact(ev, "Local model") != "not running" {
			t.Errorf("state %q: %s / %v", state, v, ev)
		}
	}
}

func findFact(ev []entity.EvidenceItem, label string) string {
	for _, e := range ev {
		if e.Label == label {
			return e.Value
		}
	}
	return "<missing>"
}

func TestTheEvidenceListsTheFactsInOrderAndSaysUnknownForWhatIsMissing(t *testing.T) {
	_, ev := Verdict(hb(0, 0, 0, "unknown"), "2026-11-09T13:00:00Z", "2026-11-09T15:05:30Z")
	labels := []string{}
	for _, e := range ev {
		labels = append(labels, e.Label)
	}
	if strings.Join(labels, "|") != "Last seen|Running for|Memory free|Memory pressure|Swap in use|Cinqo's own memory|Local model" {
		t.Errorf("%v", labels)
	}
	if findFact(ev, "Running for") != "2 h 5 min" || findFact(ev, "Memory free") != "unknown" || findFact(ev, "Memory pressure") != "unknown" {
		t.Errorf("%v", ev)
	}
}

func TestRunningForInWords(t *testing.T) {
	cases := map[[2]string]string{
		{"2026-11-09T13:00:00Z", "2026-11-09T13:00:00Z"}: "0 s",
		{"2026-11-09T13:00:00Z", "2026-11-09T13:00:59Z"}: "59 s",
		{"2026-11-09T13:00:00Z", "2026-11-09T13:01:00Z"}: "1 min",
		{"2026-11-09T13:00:00Z", "2026-11-09T13:59:59Z"}: "59 min",
		{"2026-11-09T13:00:00Z", "2026-11-09T14:00:00Z"}: "1 h 0 min",
		{"2026-11-09T13:00:00Z", "2026-11-09T15:05:30Z"}: "2 h 5 min",
		{"2026-11-09T13:00:00Z", "2026-11-09T12:00:00Z"}: "unknown",
		{"garbage", "2026-11-09T12:00:00Z"}:              "unknown",
		{"2026-11-09T13:00:00Z", "garbage"}:              "unknown",
	}
	for in, want := range cases {
		if got := runningFor(in[0], in[1]); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{0: "0 B", 1023: "1023 B", 1024: "1 KB", 1536: "1.5 KB", 70 << 20: "70 MB", 300 << 20: "300 MB", 1 << 30: "1 GB", 3<<30 + 3<<29: "4.5 GB", 8 << 30: "8 GB", 2900 << 20: "2.8 GB", 1 << 40: "1 TB"}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("%d: got %q, want %q", in, got, want)
		}
	}
}

func TestCapitalise(t *testing.T) {
	if capitalise("phi") != "Phi" || capitalise("") != "" || capitalise("Éa") != "Éa" || capitalise("1x") != "1x" {
		t.Error("capitalise")
	}
}

func TestAnUnknownTotalNeverMakesMemoryTightWhateverThePressureSays(t *testing.T) {
	for _, p := range []string{"critical", "warn", "normal", "unknown"} {
		if got := tightness(entity.HeartbeatMemory{Pressure: p, SwapUsedBytes: 9 << 30}); got != "" {
			t.Errorf("pressure %q with no total: %q", p, got)
		}
	}
}

func TestSwapIsNamedInTheVerdictOnlyWhenItIsPartOfTheReason(t *testing.T) {
	const GB = 1 << 30
	// plenty free, but 1.5 GB of swap: orange because of the swap, and the words say so
	v, _ := Verdict(hb(16*GB, 7*GB, 3*GB/2, "normal"), "", "")
	if !strings.Contains(v, "getting tight (7 GB of 16 GB free, 1.5 GB of swap in use)") {
		t.Errorf("%s", v)
	}
	// exactly at the limit the swap is not a reason, so it is not named
	v, _ = Verdict(hb(16*GB, 3*GB, 1*GB, "normal"), "", "")
	if !strings.Contains(v, "getting tight (3 GB of 16 GB free).") || strings.Contains(v, "swap") {
		t.Errorf("%s", v)
	}
	v, _ = Verdict(hb(16*GB, 1*GB, 4*GB, "critical"), "", "")
	if !strings.Contains(v, "very tight (1 GB of 16 GB free, 4 GB of swap in use)") {
		t.Errorf("%s", v)
	}
	lv, _ := LocalAiVerdict("X", "boom", entity.HeartbeatMemory{TotalBytes: 16 * GB, AvailableBytes: 7 * GB, SwapUsedBytes: 2 * GB, Pressure: "normal"})
	if !strings.Contains(lv, "getting tight at that moment (7 GB of 16 GB free, 2 GB of swap in use).") {
		t.Errorf("%s", lv)
	}
}
