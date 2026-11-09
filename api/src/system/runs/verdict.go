package runs

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/evidence"
)

// The words of the verdict for an unexpected stop, from the last thing the app knew about itself. It is a guess shown beside its evidence, never a
// certainty: a program that is killed cannot report why. The limits are those of the memory pill in the shell.
const (
	redAvailable    = 0.10
	orangeAvailable = 0.25
	redSwap         = 3 << 30
	orangeSwap      = 1 << 30
)

// Verdict says, in plain words, what the last heartbeat suggests, and lists the facts it rests on. A nil heartbeat is a run that stopped before its first.
func Verdict(h *entity.Heartbeat, startedAt, lastSeenAt string) (string, []entity.EvidenceItem) {
	return VerdictWith(h, startedAt, lastSeenAt, nil)
}

// VerdictWith is Verdict with what the system itself wrote down about the stop (plan 223, step 4). A report beats a guess: a memory-kill report that
// lists a program of ours says the system stopped it for memory; a crash report of one of our programs says it crashed. A memory-kill report that
// lists only other programs says memory ran out at that time, which is a hint and is worded as one.
func VerdictWith(h *entity.Heartbeat, startedAt, lastSeenAt string, findings []evidence.Finding) (string, []entity.EvidenceItem) {
	verdict, ev := heartbeatVerdict(h, startedAt, lastSeenAt)
	if len(findings) == 0 {
		return verdict, ev
	}
	ev = append(ev, findingEvidence(findings)...)
	if v := findingVerdict(findings); v != "" {
		return v + ReportTail(h), ev
	}
	return verdict, ev
}

// ReportTail is the sentence about the local model that follows a verdict built from a report.
func ReportTail(h *entity.Heartbeat) string {
	if h == nil || h.LocalAi.State == "" || h.LocalAi.State == "none" {
		return ""
	}
	name := h.LocalAi.Name
	if name == "" {
		name = "the local model"
	}
	return fmt.Sprintf(" %s was loaded (%s).", capitalise(name), h.LocalAi.State)
}

// findingVerdict is the verdict of the strongest finding: a crash of ours, then a memory kill that stopped ours, then a memory kill of others.
func findingVerdict(fs []evidence.Finding) string {
	for _, f := range fs {
		if f.Kind == evidence.KindCrash {
			why := ""
			if f.Reason != "" {
				why = " (" + f.Reason + ")"
			}
			return fmt.Sprintf("%s crashed at %s%s. macOS wrote a crash report.", capitalise(f.Program), reportClock(f), why)
		}
	}
	for _, f := range fs {
		if f.Kind == evidence.KindJetsam && f.Ours {
			return fmt.Sprintf("macOS stopped Cinqo's programs because memory ran out at %s: its own memory-pressure report lists %s as stopped.%s", reportClock(f), strings.Join(f.Stopped, ", "), biggest(f))
		}
	}
	for _, f := range fs {
		if f.Kind == evidence.KindJetsam {
			return fmt.Sprintf("macOS ran out of memory at %s and stopped other programs (%s). That is probably why Cinqo stopped too, but its own name is not in the report.%s", reportClock(f), orNone(f.Stopped), biggest(f))
		}
	}
	return ""
}

func biggest(f evidence.Finding) string {
	if len(f.Largest) == 0 {
		return ""
	}
	parts := make([]string, 0, len(f.Largest))
	for _, p := range f.Largest {
		parts = append(parts, fmt.Sprintf("%s %s", p.Name, FormatBytes(p.Bytes)))
	}
	return " The biggest were " + strings.Join(parts, ", ") + "."
}

func orNone(names []string) string {
	if len(names) == 0 {
		return "none named"
	}
	return strings.Join(names, ", ")
}

func reportClock(f evidence.Finding) string { return f.At.Local().Format("15:04") }

// findingEvidence lists each report as one fact: what it was and when, never its contents.
func findingEvidence(fs []evidence.Finding) []entity.EvidenceItem {
	out := make([]entity.EvidenceItem, 0, len(fs))
	for _, f := range fs {
		label := "System report: crash"
		value := f.Program
		if f.Kind == evidence.KindJetsam {
			label = "System report: memory ran out"
			value = "stopped " + orNone(f.Stopped)
		}
		if f.Reason != "" {
			value += " (" + f.Reason + ")"
		}
		out = append(out, entity.EvidenceItem{Label: label, Value: f.At.UTC().Format(time.RFC3339) + ": " + value})
	}
	return out
}

func heartbeatVerdict(h *entity.Heartbeat, startedAt, lastSeenAt string) (string, []entity.EvidenceItem) {
	ev := []entity.EvidenceItem{
		{Label: "Last seen", Value: lastSeenAt},
		{Label: "Running for", Value: runningFor(startedAt, lastSeenAt)},
	}
	if h == nil {
		return "Cinqo stopped without saying goodbye, within its first seconds, before it could write down how the computer was doing.", ev
	}
	m := h.Memory
	free := ""
	if m.TotalBytes > 0 {
		free = fmt.Sprintf("%s of %s", FormatBytes(m.AvailableBytes), FormatBytes(m.TotalBytes))
	}
	ev = append(ev,
		entity.EvidenceItem{Label: "Memory free", Value: orUnknown(free)},
		entity.EvidenceItem{Label: "Memory pressure", Value: orUnknown(pressureOrEmpty(m.Pressure))},
		entity.EvidenceItem{Label: "Swap in use", Value: FormatBytes(m.SwapUsedBytes)},
		entity.EvidenceItem{Label: "Cinqo's own memory", Value: FormatBytes(h.AppByte)},
		entity.EvidenceItem{Label: "Local model", Value: localAiText(h.LocalAi)},
	)

	level := tightness(m)
	model := ""
	if h.LocalAi.State != "" && h.LocalAi.State != "none" {
		name := h.LocalAi.Name
		if name == "" {
			name = "the local model"
		}
		model = fmt.Sprintf(" %s was loaded (%s).", capitalise(name), h.LocalAi.State)
	}
	why := swapNote(m)
	if free != "" {
		why = free + " free" + why
	} else {
		why = strings.TrimPrefix(why, ", ")
	}
	switch level {
	case "red":
		return fmt.Sprintf("Cinqo stopped while the computer's memory was very tight (%s). The system may have stopped it to free memory.%s", why, model), ev
	case "orange":
		return fmt.Sprintf("Cinqo stopped while the computer's memory was getting tight (%s). That may be why, but it may also be a crash or a force quit.%s", why, model), ev
	}
	return "Cinqo stopped without saying goodbye (a crash, a force quit, or the computer was shut down). Memory looked fine in its last seconds." + model, ev
}

// swapNote adds the swap to what is said about tight memory when the swap is part of the reason (more than the orange limit), so "7 GB of 16 GB
// free" is never offered as the reason for memory being tight on its own.
func swapNote(m entity.HeartbeatMemory) string {
	if m.SwapUsedBytes > orangeSwap {
		return ", " + FormatBytes(m.SwapUsedBytes) + " of swap in use"
	}
	return ""
}

// tightness classifies the last readings: "red", "orange" or "" (nothing unusual, or nothing known).
func tightness(m entity.HeartbeatMemory) string {
	if m.TotalBytes == 0 {
		return ""
	}
	free := float64(m.AvailableBytes) / float64(m.TotalBytes)
	if m.AvailableBytes == 0 && m.Pressure == "unknown" {
		return "" // an unreadable figure is not a shortage
	}
	if m.Pressure == "critical" || free < redAvailable || m.SwapUsedBytes > redSwap {
		return "red"
	}
	if m.Pressure == "warn" || free < orangeAvailable || m.SwapUsedBytes > orangeSwap {
		return "orange"
	}
	return ""
}

func pressureOrEmpty(p string) string {
	if p == "unknown" {
		return ""
	}
	return p
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func localAiText(a entity.HeartbeatAgent) string {
	if a.State == "" || a.State == "none" {
		return "not running"
	}
	name := a.Name
	if name == "" {
		name = "local model"
	}
	return fmt.Sprintf("%s, %s (%s)", name, a.State, FormatBytes(a.Bytes))
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

func runningFor(startedAt, lastSeenAt string) string {
	a, err1 := time.Parse(time.RFC3339, startedAt)
	b, err2 := time.Parse(time.RFC3339, lastSeenAt)
	if err1 != nil || err2 != nil || b.Before(a) {
		return "unknown"
	}
	d := b.Sub(a).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d/time.Minute))
	}
	return fmt.Sprintf("%d h %d min", int(d/time.Hour), int(d%time.Hour/time.Minute))
}

// FormatBytes writes a size the way the rest of the app does: 1024-based, one decimal below 100.
func FormatBytes(b uint64) string {
	const k = 1024.0
	v := float64(b)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= k && i < len(units)-1 {
		v /= k
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	if v >= 100 || v == float64(int64(v)) {
		return fmt.Sprintf("%d %s", int64(v+0.5), units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// a path starts a word (or follows a quote, a bracket, = or :), so "3/4" is not one; a path with spaces in it is replaced up to its first space
var pathLike = regexp.MustCompile(`(^|[\s'"(=:])((?:/[^\s'"():,;]+)+)`)

// CleanReason makes the text a program gave as its last words safe to show: file paths (they hold user names) are replaced, control characters are
// dropped, and it is cut to one line of at most 200 characters.
func CleanReason(s string) string {
	s = pathLike.ReplaceAllString(s, "${1}<path>")
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(out); len(r) > 200 {
		out = string(r[:200])
	}
	return out
}

// LocalAiVerdict is the verdict of an event for the model program stopping on its own while the app lives: its last words, and how tight the
// memory was at that moment (a common cause), with the facts.
func LocalAiVerdict(name, reason string, m entity.HeartbeatMemory) (string, []entity.EvidenceItem) {
	if name == "" {
		name = "The local model"
	} else {
		name = "The local model (" + name + ")"
	}
	why := CleanReason(reason)
	verdict := name + " stopped unexpectedly."
	if why != "" {
		verdict += " Its last words: " + why + "."
	}
	free := ""
	if m.TotalBytes > 0 {
		free = fmt.Sprintf("%s of %s", FormatBytes(m.AvailableBytes), FormatBytes(m.TotalBytes))
	}
	switch tightness(m) {
	case "red":
		verdict += fmt.Sprintf(" The computer's memory was very tight at that moment (%s free%s), which is a common reason.", free, swapNote(m))
	case "orange":
		verdict += fmt.Sprintf(" The computer's memory was getting tight at that moment (%s free%s).", free, swapNote(m))
	}
	ev := []entity.EvidenceItem{
		{Label: "Its last words", Value: orUnknown(why)},
		{Label: "Memory free", Value: orUnknown(free)},
		{Label: "Memory pressure", Value: orUnknown(pressureOrEmpty(m.Pressure))},
		{Label: "Swap in use", Value: FormatBytes(m.SwapUsedBytes)},
	}
	return verdict, ev
}
