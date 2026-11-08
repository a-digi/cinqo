package providers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
)

func writeLog(t *testing.T, root, name, content string) string {
	t.Helper()
	m := logFileName.FindStringSubmatch(name)
	if m == nil {
		t.Fatalf("not a log name: %s", name)
	}
	dir := filepath.Join(root, m[1], m[2], m[3])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func firstLine(t *testing.T, p livelog.Provider, ref string) string {
	t.Helper()
	f, err := p.Open(ref)
	if err != nil {
		t.Fatalf("Open(%q): %v", ref, err)
	}
	b, err := livelog.Poll(context.Background(), f, "", 1000, livelog.DefaultLimits())
	if err != nil || len(b.Lines) == 0 {
		t.Fatalf("Poll(%q): %+v %v", ref, b, err)
	}
	return b.Lines[0].Text
}

func TestTheSystemLogIsDescribedAdminOnlyAndLivesBesideTheTraceProvider(t *testing.T) {
	p := NewSystemLog(t.TempDir())
	if p.ID() != "system-log" || p.Title() != "System log" || !reflect.DeepEqual(p.Scopes(), []string{livelog.AdminScope}) {
		t.Errorf("%s %q %v", p.ID(), p.Title(), p.Scopes())
	}
	reg := livelog.NewRegistry()
	if err := reg.Register(NewAITrace(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(p); err != nil {
		t.Fatalf("it must register beside the trace provider: %v", err)
	}
	if got := reg.List([]string{livelog.AdminScope}); fmt.Sprint(got) != "[{ai-trace AI trace logs} {system-log System log}]" {
		t.Errorf("admin sees %v", got)
	}
	if got := reg.List([]string{"cinqo:conversation:use"}); len(got) != 0 {
		t.Errorf("a non-admin sees %v", got)
	}
}

func TestCurrentIsTheNewestFileAcrossYearsMonthsAndDays(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "cinqo_20251231_235959.log", "old year\n")
	writeLog(t, root, "cinqo_20260102_080000.log", "january\n")
	writeLog(t, root, "cinqo_20261103_214937.log", "morning start\n")
	writeLog(t, root, "cinqo_20261103_225318.log", "evening start\n")
	// noise that must never be picked: tool logs beside, a stray file, empty newer folders, odd names
	os.MkdirAll(filepath.Join(root, "tools", "browser"), 0o755)
	os.WriteFile(filepath.Join(root, "2026", "11", "03", "notes.log"), []byte("not ours\n"), 0o644)
	os.WriteFile(filepath.Join(root, "2026", "11", "03", "cinqo_x.log"), []byte("not ours\n"), 0o644)
	os.WriteFile(filepath.Join(root, "2026", "11", "03", "zzz_20261103_235959.log"), []byte("not ours\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "2026", "12", "05"), 0o755) // a newer day with nothing in it
	os.MkdirAll(filepath.Join(root, "2027"), 0o755)             // a newer year with nothing in it
	os.MkdirAll(filepath.Join(root, "202x", "11", "03"), 0o755)
	p := NewSystemLog(root)
	if got := firstLine(t, p, "current"); got != "evening start" {
		t.Errorf("current = %q", got)
	}
	// once a newer file appears, current is that one
	writeLog(t, root, "cinqo_20261104_093015.log", "today\n")
	if got := firstLine(t, p, "current"); got != "today" {
		t.Errorf("current after a new file = %q", got)
	}
}

func TestAnExactNameOpensThatFileAndItsGrowthIsFollowed(t *testing.T) {
	root := t.TempDir()
	path := writeLog(t, root, "cinqo_20261103_225318.log", "[2026-11-03 22:53:18] [INFO] started\n")
	writeLog(t, root, "cinqo_20261104_093015.log", "newer\n")
	p := NewSystemLog(root)
	f, err := p.Open("cinqo_20261103_225318.log")
	if err != nil {
		t.Fatal(err)
	}
	b, err := livelog.Poll(context.Background(), f, "", 10, livelog.DefaultLimits())
	if err != nil || len(b.Lines) != 1 || !strings.Contains(b.Lines[0].Text, "started") {
		t.Fatalf("%+v %v", b, err)
	}
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fh.WriteString("[2026-11-03 22:53:19] [INFO] Request success: GET /api/v1/ping\n")
	fh.Close()
	next, err := livelog.Poll(context.Background(), f, b.Cursor, 10, livelog.DefaultLimits())
	if err != nil || len(next.Lines) != 1 || !strings.Contains(next.Lines[0].Text, "Request success") {
		t.Fatalf("%+v %v", next, err)
	}
}

func TestEveryBadSystemLogReferenceIsTheSameNotFound(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "cinqo_20261103_225318.log", "x\n")
	outside := filepath.Join(t.TempDir(), "secret.log")
	os.WriteFile(outside, []byte("secret\n"), 0o600)
	linkDir := filepath.Join(root, "2026", "11", "04")
	os.MkdirAll(linkDir, 0o755)
	symlinked := true
	if err := os.Symlink(outside, filepath.Join(linkDir, "cinqo_20261104_000000.log")); err != nil {
		symlinked = false
	}
	os.MkdirAll(filepath.Join(root, "2026", "11", "05", "cinqo_20261105_000000.log"), 0o755) // a folder with a log name
	p := NewSystemLog(root)
	cases := map[string]string{
		"empty":                "",
		"a different case":     "CURRENT",
		"current with a slash": "current/",
		"current with a space": "current ",
		"traversal":            "../../etc/passwd",
		"a path":               "2026/11/03/cinqo_20261103_225318.log",
		"upper case name":      "CINQO_20261103_225318.LOG",
		"a missing file":       "cinqo_20261103_225319.log",
		"month 13":             "cinqo_20261301_225318.log",
		"month 00":             "cinqo_20260001_225318.log",
		"day 00":               "cinqo_20261100_225318.log",
		"day 32":               "cinqo_20261132_225318.log",
		"short date":           "cinqo_2026113_225318.log",
		"no extension":         "cinqo_20261103_225318",
		"extra extension":      "cinqo_20261103_225318.log.gz",
		"a folder named so":    "cinqo_20261105_000000.log",
		"nul byte":             "cinqo_20261103_225318.log\x00",
		"newline":              "cinqo_20261103_225318.log\n",
		"very long":            strings.Repeat("a", 5000),
		"windows separator":    `2026\11\03\cinqo_20261103_225318.log`,
	}
	if symlinked {
		cases["a symlink out of the root"] = "cinqo_20261104_000000.log"
	}
	for name, ref := range cases {
		if f, err := p.Open(ref); !errors.Is(err, livelog.ErrNotFound) || f != nil {
			t.Errorf("%s: %v %v", name, f, err)
		}
	}
}

func TestCurrentWithNothingToFindIsNotFound(t *testing.T) {
	if _, err := NewSystemLog(filepath.Join(t.TempDir(), "missing")).Open("current"); !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("a missing root: %v", err)
	}
	empty := t.TempDir()
	os.MkdirAll(filepath.Join(empty, "tools", "browser"), 0o755)
	os.MkdirAll(filepath.Join(empty, "2026", "11", "03"), 0o755)
	if _, err := NewSystemLog(empty).Open("current"); !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("no log files: %v", err)
	}
}

// A tree of thousands of empty folders must not make "current" walk for long: the search reads a bounded number of folders.
func TestCurrentIsBoundedOnAHostileTree(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "cinqo_20200101_000000.log", "the only one, far down\n")
	for m := 1; m <= 12; m++ {
		for d := 1; d <= 28; d++ {
			os.MkdirAll(filepath.Join(root, "2026", fmt.Sprintf("%02d", m), fmt.Sprintf("%02d", d)), 0o755) // 336 empty days newer than the file
		}
	}
	for y := 2021; y <= 2025; y++ {
		for m := 1; m <= 12; m++ {
			for d := 1; d <= 28; d++ {
				os.MkdirAll(filepath.Join(root, fmt.Sprint(y), fmt.Sprintf("%02d", m), fmt.Sprintf("%02d", d)), 0o755)
			}
		}
	}
	start := time.Now()
	_, err := NewSystemLog(root).Open("current")
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
	if !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("a file further down than the folder budget reaches is not found (the bound is %d reads): %v", maxDirReads, err)
	}
	// an exact name still opens it: only "current" is bounded
	if _, err := NewSystemLog(root).Open("cinqo_20200101_000000.log"); err != nil {
		t.Errorf("exact name: %v", err)
	}
}

// A name that is not exactly ours is refused even when a file of that very name exists on disk: the shape rule is the gate, not the file
// system.
func TestOnlyExactNamesWithAPossibleDateOpenEvenWhenTheFileExists(t *testing.T) {
	root := t.TempDir()
	put := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("2026/00/01/cinqo_20260001_000000.log")  // month 00
	put("2026/13/01/cinqo_20261301_000000.log")  // month 13
	put("2026/11/00/cinqo_20261100_000000.log")  // day 00
	put("2026/11/32/cinqo_20261132_000000.log")  // day 32
	put("2026/11/03/xcinqo_20261103_225318.log") // a prefix
	put("2026/11/03/cinqo_20261103_225318.log.gz")
	put("2026/11/03/cinqo_20261103_225318.logx")
	put("2026/11/03/cinqo_20261103_225318.log") // the one good file
	p := NewSystemLog(root)
	for _, ref := range []string{"cinqo_20260001_000000.log", "cinqo_20261301_000000.log", "cinqo_20261100_000000.log", "cinqo_20261132_000000.log", "xcinqo_20261103_225318.log", "cinqo_20261103_225318.log.gz", "cinqo_20261103_225318.logx"} {
		if f, err := p.Open(ref); !errors.Is(err, livelog.ErrNotFound) || f != nil {
			t.Errorf("%q opened an existing file it must not: %v %v", ref, f, err)
		}
	}
	if got := firstLine(t, p, "cinqo_20261103_225318.log"); got != "content" {
		t.Errorf("the good file: %q", got)
	}
}

// "current" looks only at folders and files of the shapes the logger writes: a folder or file with a longer number, or a folder with a log's name,
// never wins.
func TestCurrentIgnoresOddFoldersAndFolderNamedLikeALog(t *testing.T) {
	root := t.TempDir()
	writeLog(t, root, "cinqo_20261103_225318.log", "the real one\n")
	os.MkdirAll(filepath.Join(root, "2026", "11", "03", "cinqo_20261103_235959.log"), 0o755) // a folder with a later log name, same day
	for _, rel := range []string{"99999/01/01", "2026/123/01", "2026/11/031"} {              // longer numbers, sorting after the real ones
		dir := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "cinqo_20270101_000000.log"), []byte("a decoy\n"), 0o644)
	}
	if got := firstLine(t, NewSystemLog(root), "current"); got != "the real one" {
		t.Errorf("current = %q", got)
	}
}
