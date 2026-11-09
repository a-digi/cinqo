package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	files []File
	data  map[string]string
	fail  map[string]bool
	read  []string
}

func (f *fakeSource) List() []File { return f.files }
func (f *fakeSource) Read(file File, max int) ([]byte, error) {
	f.read = append(f.read, file.Name)
	if f.fail[file.Name] {
		return nil, errors.New("no")
	}
	d := f.data[file.Name]
	if len(d) > max {
		d = d[:max]
	}
	return []byte(d), nil
}

var base = time.Date(2026, 11, 9, 14, 2, 0, 0, time.UTC)

const jetsam = `{"app_name":"JetsamEvent","timestamp":"2026-11-09 15:02:03.12 +0100","bug_type":"298","os_version":"macOS 15"}
{
  "crashReporterKey": "SECRET-KEY-1234",
  "kernel": "Darwin Kernel Version 24.1.0 /Users/andet/secret/path",
  "pageSize": 16384,
  "largestProcess": "Google Chrome Helper (Renderer)",
  "memoryStatus": {"compressorSize": 1, "free": 2},
  "processes": [
    {"name": "Google Chrome Helper (Renderer)", "pid": 100, "rpages": 200000, "states": ["frontmost"]},
    {"name": "llama-server", "pid": 200, "rpages": 180000, "reason": "vm-pageshortage"},
    {"name": "Slack Helper", "pid": 300, "rpages": 50000, "reason": "vm-pageshortage"},
    {"name": "cinqo-pro", "pid": 400, "rpages": 4000},
    {"name": "tiny", "pid": 500, "rpages": 10},
    {"name": "", "pid": 600, "rpages": 999999, "reason": "x"}
  ]
}`

const crash = `{"app_name":"cinqo-pro","timestamp":"2026-11-09 15:01:40.00 +0100","bug_type":"309"}
{"exception":{"type":"EXC_BAD_ACCESS","signal":"SIGSEGV"},"termination":{"indicator":"Segmentation fault: 11"},"procPath":"/Users/andet/private/cinqo-pro","threads":[{"frames":["secret"]}]}`

func src() *fakeSource {
	return &fakeSource{
		files: []File{
			{Name: "JetsamEvent-2026-11-09-150203.ips"},
			{Name: "cinqo-pro-2026-11-09-150140.ips"},
			{Name: "Google Chrome Helper-2026-11-09-150100.ips"},
			{Name: "notes.txt"},
			{Name: "llama-server-2026-11-01-100000.ips"},
		},
		data: map[string]string{
			"JetsamEvent-2026-11-09-150203.ips":          jetsam,
			"cinqo-pro-2026-11-09-150140.ips":            crash,
			"Google Chrome Helper-2026-11-09-150100.ips": crash,
			"llama-server-2026-11-01-100000.ips":         `{"app_name":"llama-server","timestamp":"2026-11-01 10:00:00.00 +0100"}` + "\n{}",
		},
		fail: map[string]bool{},
	}
}

func TestAMemoryKillReportIsReadForTheProgramsItStoppedAndTheBiggest(t *testing.T) {
	got := Collect(src(), base.Add(-time.Minute), base.Add(5*time.Minute), []string{"cinqo-pro", "llama-server"})
	var j *Finding
	for i := range got {
		if got[i].Kind == KindJetsam {
			j = &got[i]
		}
	}
	if j == nil {
		t.Fatalf("%+v", got)
	}
	if !j.At.Equal(time.Date(2026, 11, 9, 14, 2, 3, 0, time.UTC)) {
		t.Errorf("the header time (with its offset) wins: %v", j.At)
	}
	if strings.Join(j.Stopped, "|") != "llama-server|Slack Helper" || !j.Ours || j.Reason != "vm-pageshortage" || j.Program != "Google Chrome Helper (Renderer)" {
		t.Errorf("%+v", j)
	}
	if len(j.Largest) != 3 || j.Largest[0].Name != "Google Chrome Helper (Renderer)" || j.Largest[0].Bytes != 200000*16384 || j.Largest[1].Name != "llama-server" {
		t.Errorf("%+v", j.Largest)
	}
}

func TestACrashReportOfOurProgramGivesItsExceptionAndNothingElse(t *testing.T) {
	got := Collect(src(), base.Add(-time.Minute), base.Add(5*time.Minute), []string{"cinqo-pro", "llama-server"})
	var c *Finding
	for i := range got {
		if got[i].Kind == KindCrash {
			c = &got[i]
		}
	}
	if c == nil || c.Program != "cinqo-pro" || c.Reason != "EXC_BAD_ACCESS SIGSEGV" || !c.At.Equal(time.Date(2026, 11, 9, 14, 1, 40, 0, time.UTC)) {
		t.Fatalf("%+v", c)
	}
	// a report of someone else's program is never read, and the old report is outside the window
	if len(got) != 2 {
		t.Errorf("%d findings: %+v", len(got), got)
	}
	s := src()
	Collect(s, base.Add(-time.Minute), base.Add(5*time.Minute), []string{"cinqo-pro", "llama-server"})
	for _, n := range s.read {
		if strings.HasPrefix(n, "Google Chrome") || n == "notes.txt" {
			t.Errorf("%s must not be read", n)
		}
	}
}

func TestNothingOfTheReportsContentLeavesThePackage(t *testing.T) {
	got := Collect(src(), base.Add(-time.Minute), base.Add(5*time.Minute), []string{"cinqo-pro"})
	all := ""
	for _, f := range got {
		all += f.Program + "|" + f.Reason + "|" + strings.Join(f.Stopped, ",")
		for _, p := range f.Largest {
			all += "|" + p.Name
		}
	}
	for _, secret := range []string{"SECRET-KEY", "/Users/andet", "Darwin Kernel", "frames", "procPath"} {
		if strings.Contains(all, secret) {
			t.Errorf("%q leaked: %s", secret, all)
		}
	}
}

func TestTheTimeWindowHasAMinuteOfSlackOnBothSides(t *testing.T) {
	s := &fakeSource{files: []File{{Name: "cinqo-pro-2026-11-09-120000.ips"}}, data: map[string]string{"cinqo-pro-2026-11-09-120000.ips": `{"app_name":"cinqo-pro","timestamp":"2026-11-09 12:00:00.00 +0000"}` + "\n{}"}}
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		from, to time.Time
		want     int
	}{
		{at, at, 1},
		{at.Add(time.Minute), at.Add(2 * time.Minute), 1},           // exactly a minute after: still in
		{at.Add(time.Minute + time.Second), at.Add(time.Hour), 0},   // a second more: out
		{at.Add(-2 * time.Minute), at.Add(-time.Minute), 1},         // exactly a minute before
		{at.Add(-time.Hour), at.Add(-time.Minute - time.Second), 0}, // out
	} {
		if got := len(Collect(s, tc.from, tc.to, []string{"cinqo-pro"})); got != tc.want {
			t.Errorf("%v..%v: %d findings, want %d", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestNewestFirstAndAnUnreadableOrOddReportStillGivesItsHeader(t *testing.T) {
	s := &fakeSource{
		files: []File{{Name: "cinqo-pro-2026-11-09-150000.ips"}, {Name: "cinqo-pro-2026-11-09-150100.ips"}, {Name: "cinqo-pro-2026-11-09-150200.ips"}, {Name: "JetsamEvent-2026-11-09-150300.ips"}},
		data: map[string]string{
			"cinqo-pro-2026-11-09-150000.ips":   `{"app_name":"cinqo-pro","timestamp":"2026-11-09 15:00:00.00 +0100"}` + "\n{not json at all",
			"cinqo-pro-2026-11-09-150100.ips":   "garbage with no newline",
			"JetsamEvent-2026-11-09-150300.ips": `{"timestamp":"2026-11-09 15:03:00.00 +0100"}` + "\n" + `{"processes":"not a list"}`,
		},
		fail: map[string]bool{"cinqo-pro-2026-11-09-150200.ips": true},
	}
	got := Collect(s, time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC), time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC), []string{"cinqo-pro"})
	if len(got) != 3 {
		t.Fatalf("%d findings: %+v", len(got), got)
	}
	if got[0].Kind != KindJetsam || got[1].At.Before(got[2].At) {
		t.Errorf("newest first: %+v", got)
	}
	if got[0].Reason != "" || len(got[0].Stopped) != 0 {
		t.Errorf("an odd shape gives no invented facts: %+v", got[0])
	}
}

func TestANameAndAReasonAreOneShortPrintableLine(t *testing.T) {
	if clean("a\x00b\nc\x7fd", 80) != "abcd" || clean("  x  ", 80) != "x" || len([]rune(clean(strings.Repeat("é", 200), 80))) != 80 || len(clean(strings.Repeat("x", 81), 80)) != 80 || len(clean(strings.Repeat("x", 80), 80)) != 80 {
		t.Error("clean")
	}
}

func TestTheTimestampFormatsReportsUse(t *testing.T) {
	for _, s := range []string{"2026-11-06 08:46:19.00 +0100", "2026-11-06 08:46:19.123 +0100", "2026-11-06 08:46:19 +0100"} {
		got, ok := parseTimestamp(s)
		if !ok || !got.Equal(time.Date(2026, 11, 6, 7, 46, 19, 0, time.UTC)) {
			t.Errorf("%q: %v %v", s, got, ok)
		}
	}
	if _, ok := parseTimestamp("yesterday"); ok {
		t.Error("garbage")
	}
}

func TestTheNameOfAFileGivesItsTimeWhenTheHeaderCannot(t *testing.T) {
	f := File{Name: "cinqo-pro-2026-11-09-150203.ips", ModTime: time.Unix(5, 0)}
	if got := fileTime(f); got.Year() != 2026 || got.Hour() != 15 || got.Minute() != 2 || got.Second() != 3 {
		t.Errorf("%v", got)
	}
	if got := fileTime(File{Name: "odd.ips", ModTime: time.Unix(5, 0)}); !got.Equal(time.Unix(5, 0)) {
		t.Errorf("%v", got)
	}
}

func TestADirectorySourceListsOnlyIpsFilesAndReadsAtMostTheLimit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.ips"), []byte(strings.Repeat("x", 100)), 0o600)
	os.WriteFile(filepath.Join(dir, "b.diag"), []byte("x"), 0o600)
	os.Mkdir(filepath.Join(dir, "c.ips"), 0o700)
	s := DirSource{Dirs: []string{dir, "/definitely/not/here"}}
	files := s.List()
	if len(files) != 1 || files[0].Name != "a.ips" || files[0].Dir != dir {
		t.Fatalf("%+v", files)
	}
	b, err := s.Read(files[0], 10)
	if err != nil || len(b) != 10 {
		t.Errorf("%d %v", len(b), err)
	}
	if _, err := s.Read(File{Name: "missing.ips", Dir: dir}, 10); err == nil {
		t.Error("a missing file is an error")
	}
	if dirs := DefaultDirs("/Users/x"); len(dirs) != 2 || dirs[0] != "/Users/x/Library/Logs/DiagnosticReports" || dirs[1] != "/Library/Logs/DiagnosticReports" {
		t.Errorf("%v", dirs)
	}
	if dirs := DefaultDirs(""); len(dirs) != 1 {
		t.Errorf("%v", dirs)
	}
}

// A real report of this computer, when there is one: the shape this code reads is the shape macOS writes.
func TestARealCrashReportOfThisComputerIsReadByItsShape(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Library", "Logs", "DiagnosticReports")
	files := DirSource{Dirs: []string{dir}}.List()
	var pick *File
	for i := range files {
		if strings.HasSuffix(files[i].Name, ".ips") && nameDate.MatchString(files[i].Name) {
			pick = &files[i]
			break
		}
	}
	if pick == nil {
		t.Skip("no crash report on this computer")
	}
	name := nameDate.FindStringSubmatch(pick.Name)[1]
	got := Collect(DirSource{Dirs: []string{dir}}, time.Now().Add(-365*24*time.Hour), time.Now().Add(time.Hour), []string{name})
	if len(got) == 0 {
		t.Fatalf("the report %s must be found", pick.Name)
	}
	c := got[0]
	if c.Kind != KindCrash || c.Program == "" || c.At.IsZero() || c.Reason == "" {
		t.Errorf("a real report must give a program, a time and a reason: %+v", c)
	}
}

type partialSource struct {
	files []File
	data  map[string]string
}

func (p partialSource) List() []File { return p.files }
func (p partialSource) Read(f File, max int) ([]byte, error) {
	return []byte(p.data[f.Name]), errors.New("a read that returned data and then an error")
}

func TestAReadThatReturnedSomeDataBeforeFailingIsStillUsed(t *testing.T) {
	s := partialSource{files: []File{{Name: "cinqo-pro-2026-11-09-150000.ips"}}, data: map[string]string{"cinqo-pro-2026-11-09-150000.ips": `{"app_name":"cinqo-pro","timestamp":"2026-11-09 15:00:00.00 +0100"}` + "\n" + `{"exception":{"type":"EXC_CRASH","signal":"SIGABRT"}}`}}
	got := Collect(s, time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC), time.Date(2026, 11, 9, 14, 5, 0, 0, time.UTC), []string{"cinqo-pro"})
	if len(got) != 1 || got[0].Reason != "EXC_CRASH SIGABRT" {
		t.Errorf("%+v", got)
	}
}

func TestACrashReportWithOnlyATerminationIndicatorGivesThatAsItsReason(t *testing.T) {
	s := &fakeSource{files: []File{{Name: "cinqo-pro-2026-11-09-150000.ips"}}, data: map[string]string{"cinqo-pro-2026-11-09-150000.ips": `{"app_name":"cinqo-pro","timestamp":"2026-11-09 15:00:00.00 +0100"}` + "\n" + `{"termination":{"indicator":"Abort trap: 6"}}`}}
	got := Collect(s, time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC), time.Date(2026, 11, 9, 14, 5, 0, 0, time.UTC), []string{"cinqo-pro"})
	if len(got) != 1 || got[0].Reason != "Abort trap: 6" {
		t.Errorf("%+v", got)
	}
}

func TestAMemoryKillWithoutAPageSizeAssumesTheAppleSiliconPageAndNamesItsProgramFromTheHeader(t *testing.T) {
	long := strings.Repeat("n", 120)
	body := `{"app_name":"JetsamEvent","timestamp":"2026-11-09 15:00:00.00 +0100"}` + "\n" + `{"processes":[{"name":"` + long + `","rpages":10,"reason":"x"}]}`
	s := &fakeSource{files: []File{{Name: "JetsamEvent-2026-11-09-150000.ips"}}, data: map[string]string{"JetsamEvent-2026-11-09-150000.ips": body}}
	got := Collect(s, time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC), time.Date(2026, 11, 9, 14, 5, 0, 0, time.UTC), nil)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	f := got[0]
	if f.Program != "JetsamEvent" {
		t.Errorf("with no largest process the header's name is used: %q", f.Program)
	}
	if len(f.Largest) != 1 || f.Largest[0].Bytes != 10*16384 {
		t.Errorf("a page is 16384 bytes when the report does not say: %+v", f.Largest)
	}
	if len(f.Stopped) != 1 || len([]rune(f.Stopped[0])) != 80 {
		t.Errorf("a name is cut to 80 characters: %d", len([]rune(f.Stopped[0])))
	}
}
