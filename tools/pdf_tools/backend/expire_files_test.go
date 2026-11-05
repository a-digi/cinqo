package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pdf-tools-backend/generate"
)

const (
	oldID   = "11111111-1111-4111-8111-111111111111"
	youngID = "22222222-2222-4222-8222-222222222222"
	edgeID  = "33333333-3333-4333-8333-333333333333"
)

func put(t *testing.T, dir, name string, age time.Duration, now time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExpireFilesRemovesOnlyOldGeneratedPDFs(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	maxAge := 30 * 24 * time.Hour
	old := put(t, dir, oldID+".pdf", maxAge+time.Hour, now)
	young := put(t, dir, youngID+".pdf", maxAge-time.Hour, now)
	edge := put(t, dir, edgeID+".pdf", maxAge, now)
	notPDF := put(t, dir, oldID+".txt", maxAge*2, now)
	notUUID := put(t, dir, "notes.pdf", maxAge*2, now)
	upper := put(t, dir, "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA.pdf", maxAge*2, now)
	subdir := filepath.Join(dir, "44444444-4444-4444-8444-444444444444.pdf")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	old2 := now.Add(-maxAge * 2)
	if err := os.Chtimes(subdir, old2, old2); err != nil {
		t.Fatal(err)
	}
	outside := put(t, t.TempDir(), "target.pdf", maxAge*2, now)
	link := filepath.Join(dir, "55555555-5555-4555-8555-555555555555.pdf")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	n, err := generate.ExpireFiles(dir, maxAge, now)
	if err != nil || n != 1 {
		t.Fatalf("removed %d, err %v; want exactly 1", n, err)
	}
	if exists(old) {
		t.Error("the old generated file was kept")
	}
	for name, p := range map[string]string{"young": young, "exactly at the limit": edge, "not a pdf": notPDF, "not a uuid": notUUID, "capital letters": upper, "folder": subdir, "link": link, "link target": outside} {
		if !exists(p) {
			t.Errorf("%s was removed", name)
		}
	}
}

func TestExpireFilesOnAMissingFolderIsAnError(t *testing.T) {
	if _, err := generate.ExpireFiles(filepath.Join(t.TempDir(), "nope"), time.Hour, time.Now()); err == nil {
		t.Error("a missing folder must be reported")
	}
}

func TestTheSweeperRunsAtOnceAndStops(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := put(t, dir, oldID+".pdf", 31*24*time.Hour, now)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { sweepExpiredFiles(dir, generate.GeneratedFileMaxAge, time.Hour, stop); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for exists(old) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if exists(old) {
		t.Error("the first sweep did not run at start")
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("the sweeper did not stop")
	}
}

func TestTheSweeperRepeats(t *testing.T) {
	dir := t.TempDir()
	stop := make(chan struct{})
	defer close(stop)
	go sweepExpiredFiles(dir, generate.GeneratedFileMaxAge, 20*time.Millisecond, stop)
	time.Sleep(60 * time.Millisecond)
	old := put(t, dir, oldID+".pdf", 31*24*time.Hour, time.Now())
	deadline := time.Now().Add(3 * time.Second)
	for exists(old) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if exists(old) {
		t.Error("a later sweep did not remove a file that expired after start")
	}
}
