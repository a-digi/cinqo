// Package evidence reads what macOS itself wrote down about a program that stopped (plan 223, step 4): the memory-kill reports ("JetsamEvent") and the
// crash reports of Cinqo's own programs in the DiagnosticReports folders. Only an allow-list of facts is read: the names of the programs, the time, the
// reason, and the largest memory figures; never stack traces, paths, user names, or any content. A report that cannot be read, or that has a shape this
// code does not know, still gives what its header says (the kind and the time) and nothing else.
package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Finding is what one report says.
type Finding struct {
	Kind    string    // "jetsam" (the system stopped programs because memory ran out) or "crash" (a program of ours crashed)
	Program string    // for a crash: the program; for a memory kill: the largest process the system named
	At      time.Time // when the report says it happened
	Reason  string    // a crash's exception and signal, or the reasons the system gave for stopping programs
	Stopped []string  // for a memory kill: the programs it stopped (names only), at most 8
	Largest []Process // for a memory kill: the biggest programs by memory, at most 3
	Ours    bool      // for a memory kill: one of the stopped programs is one of ours
}

// Process is a program and the memory it held.
type Process struct {
	Name  string
	Bytes uint64
}

const (
	KindJetsam = "jetsam"
	KindCrash  = "crash"

	maxReadBytes = 4 << 20 // a memory-kill report lists every process: read at most 4 MB of it
	maxListed    = 8
	maxName      = 80
	maxReason    = 120
)

// File is one report file.
type File struct {
	Name    string
	ModTime time.Time
	Dir     string
}

// Source lists and reads report files; an interface so the tests need no real folder.
type Source interface {
	List() []File
	Read(f File, max int) ([]byte, error)
}

// DirSource reads real folders. On a system that has none (Linux) the folders do not exist and nothing is found.
type DirSource struct{ Dirs []string }

func (d DirSource) List() []File {
	var out []File
	for _, dir := range d.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".ips") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, File{Name: e.Name(), ModTime: info.ModTime(), Dir: dir})
		}
	}
	return out
}

func (d DirSource) Read(f File, max int) ([]byte, error) {
	h, err := os.Open(filepath.Join(f.Dir, f.Name))
	if err != nil {
		return nil, err
	}
	defer h.Close()
	buf := make([]byte, max)
	n, err := h.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	return nil, err
}

// DefaultDirs are the folders macOS writes reports to: the person's own and the system-wide one. home may be "".
func DefaultDirs(home string) []string {
	dirs := []string{"/Library/Logs/DiagnosticReports"}
	if home != "" {
		dirs = append([]string{filepath.Join(home, "Library", "Logs", "DiagnosticReports")}, dirs...)
	}
	return dirs
}

var nameDate = regexp.MustCompile(`^(.*)-(\d{4}-\d{2}-\d{2}-\d{6})\.ips$`)

// Collect returns the findings whose report time lies between from and to (a minute of slack on both sides: the clocks of the app and of the report
// writer are not the same). own lists the base names of Cinqo's programs (cinqo-pro, llama-server, ...); a memory-kill report is always read, a crash
// report only when it is named after one of them. Newest first.
func Collect(src Source, from, to time.Time, own []string) []Finding {
	ownSet := map[string]bool{}
	for _, n := range own {
		ownSet[strings.ToLower(n)] = true
	}
	lo, hi := from.Add(-time.Minute), to.Add(time.Minute)
	var out []Finding
	for _, f := range src.List() {
		kind, program := classify(f.Name, ownSet)
		if kind == "" {
			continue
		}
		raw, err := src.Read(f, maxReadBytes)
		if err != nil && len(raw) == 0 {
			continue
		}
		find := parse(kind, program, raw, f)
		if find.At.Before(lo) || find.At.After(hi) {
			continue
		}
		find.Ours = find.Ours || (kind == KindJetsam && anyOwn(find.Stopped, ownSet))
		out = append(out, find)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func classify(name string, own map[string]bool) (kind, program string) {
	if strings.HasPrefix(name, "JetsamEvent-") {
		return KindJetsam, ""
	}
	m := nameDate.FindStringSubmatch(name)
	if m == nil {
		return "", ""
	}
	if own[strings.ToLower(m[1])] {
		return KindCrash, m[1]
	}
	return "", ""
}

func anyOwn(names []string, own map[string]bool) bool {
	for _, n := range names {
		if own[strings.ToLower(n)] {
			return true
		}
	}
	return false
}

type header struct {
	AppName   string `json:"app_name"`
	Timestamp string `json:"timestamp"`
}

type crashBody struct {
	Exception struct {
		Type   string `json:"type"`
		Signal string `json:"signal"`
	} `json:"exception"`
	Termination struct {
		Indicator string `json:"indicator"`
	} `json:"termination"`
}

type jetsamBody struct {
	PageSize       uint64 `json:"pageSize"`
	LargestProcess string `json:"largestProcess"`
	Processes      []struct {
		Name   string `json:"name"`
		RPages uint64 `json:"rpages"`
		Reason string `json:"reason"`
	} `json:"processes"`
}

// parse reads a report: the first line is a small JSON header, the rest is one JSON document. Whatever cannot be read is left empty.
func parse(kind, program string, raw []byte, f File) Finding {
	out := Finding{Kind: kind, Program: program, At: fileTime(f)}
	head, body := splitReport(raw)
	var h header
	if json.Unmarshal(head, &h) == nil {
		if t, ok := parseTimestamp(h.Timestamp); ok {
			out.At = t
		}
		if out.Program == "" {
			out.Program = clean(h.AppName, maxName)
		}
	}
	switch kind {
	case KindCrash:
		var b crashBody
		if json.Unmarshal(body, &b) == nil {
			out.Reason = clean(strings.TrimSpace(b.Exception.Type+" "+b.Exception.Signal), maxReason)
			if out.Reason == "" {
				out.Reason = clean(b.Termination.Indicator, maxReason)
			}
		}
	case KindJetsam:
		var b jetsamBody
		if json.Unmarshal(body, &b) == nil {
			applyJetsam(&out, b)
		}
	}
	return out
}

func applyJetsam(out *Finding, b jetsamBody) {
	if p := clean(b.LargestProcess, maxName); p != "" {
		out.Program = p
	}
	page := b.PageSize
	if page == 0 {
		page = 16384
	}
	reasons := map[string]bool{}
	var all []Process
	for _, p := range b.Processes {
		name := clean(p.Name, maxName)
		if name == "" {
			continue
		}
		all = append(all, Process{Name: name, Bytes: p.RPages * page})
		if p.Reason != "" {
			if len(out.Stopped) < maxListed {
				out.Stopped = append(out.Stopped, name)
			}
			reasons[clean(p.Reason, maxReason)] = true
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Bytes > all[j].Bytes })
	if len(all) > 3 {
		all = all[:3]
	}
	out.Largest = all
	var rs []string
	for r := range reasons {
		if r != "" {
			rs = append(rs, r)
		}
	}
	sort.Strings(rs)
	out.Reason = strings.Join(rs, ", ")
}

// splitReport cuts the header line from the body.
func splitReport(raw []byte) (head, body []byte) {
	i := bytes.IndexByte(raw, '\n')
	if i < 0 {
		return raw, nil
	}
	return raw[:i], raw[i+1:]
}

// parseTimestamp reads "2026-11-06 08:46:19.00 +0100", the form reports use.
func parseTimestamp(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05.00 -0700", "2006-01-02 15:04:05.000 -0700", "2006-01-02 15:04:05 -0700"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.Truncate(time.Second), true
		}
	}
	return time.Time{}, false
}

// fileTime is the time a report file's name gives (local time), else its modification time.
func fileTime(f File) time.Time {
	if i := strings.LastIndex(f.Name, "-20"); i >= 0 {
		stamp := strings.TrimSuffix(f.Name[i+1:], ".ips")
		if t, err := time.ParseInLocation("2006-01-02-150405", stamp, time.Local); err == nil {
			return t
		}
	}
	return f.ModTime
}

// clean makes a name or reason one printable line of at most max characters.
func clean(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if r := []rune(out); len(r) > max {
		out = string(r[:max])
	}
	return out
}
