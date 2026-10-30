package handler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	tfConv  = "11111111-2222-4333-8444-555555555555"
	tfTurnA = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	tfTurnB = "bbbbbbbb-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func TestListTraceFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, tfConv)
	os.MkdirAll(dir, 0o755)
	older := filepath.Join(dir, tfTurnA+".txt")
	newer := filepath.Join(dir, tfTurnB+".txt")
	os.WriteFile(older, []byte("one\ntwo\n"), 0o600)
	os.WriteFile(newer, []byte("x"), 0o600)
	now := time.Now()
	os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(newer, now, now)
	// things that are not trace files: a stray name, another extension, a folder named like one
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("n"), 0o600)
	os.WriteFile(filepath.Join(dir, tfTurnA+".md"), []byte("n"), 0o600)
	os.MkdirAll(filepath.Join(dir, "cccccccc-bbbb-4ccc-8ddd-eeeeeeeeeeee.txt"), 0o755)

	logs, err := listTraceFiles(root, tfConv)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].TurnRunID != tfTurnB || logs[1].TurnRunID != tfTurnA || logs[1].SizeBytes != 8 {
		t.Fatalf("logs = %+v", logs)
	}

	empty, err := listTraceFiles(root, "99999999-2222-4333-8444-555555555555")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("no folder must be an empty (non-nil) list: %v %v", empty, err)
	}
}

func TestIDShape(t *testing.T) {
	for _, bad := range []string{"", "..", "../x", tfConv + "/..", "ABCDEFAB-2222-4333-8444-555555555555", tfConv + "x"} {
		if idShape.MatchString(bad) {
			t.Errorf("%q must not be a valid id", bad)
		}
	}
	if !idShape.MatchString(tfConv) {
		t.Error("a real id was refused")
	}
}
