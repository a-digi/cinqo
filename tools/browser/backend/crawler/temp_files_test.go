package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"browser-tool-backend/shared"
	"browser-tool-backend/shared/chromeproc"
)

// tempFolder makes a real settings database and cache folder (a temp directory) and moves the tests' Chrome ledger and profile root inside it, like
// production, for the length of the test.
func tempFolder(t *testing.T) string {
	t.Helper()
	useBrowserSettings(t, shared.FetchEngineKatana)
	if err := chromeproc.Configure(filepath.Join(shared.CacheRoot, "chrome")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = chromeproc.Configure(testChromeDir) })
	return shared.CacheRoot
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func age(t *testing.T, path string, d time.Duration) {
	t.Helper()
	past := time.Now().Add(-d)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func part(inv tempInventory, id string) tempPart {
	for _, p := range inv.Parts {
		if p.ID == id {
			return p
		}
	}
	return tempPart{ID: "missing"}
}

func TestTheInventoryMeasuresTheWholeFolderAndItsParts(t *testing.T) {
	root := tempFolder(t)
	if _, err := writeSnapshot(strings.Repeat("h", 1000), snapshotMeta{}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(shared.FetchCacheDir, "a.cache"), 300)
	write(t, filepath.Join(shared.FetchCacheDir, "index.json"), 20)
	dir, err := chromeproc.NewProfileDir(chromeproc.KindRod)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "Default", "Cookies"), 500)
	write(t, filepath.Join(root, "stray.txt"), 7)

	inv := inventoryTempFiles()
	pc, cache, prof, other := part(inv, "page_copies"), part(inv, "page_cache"), part(inv, "chrome_profiles"), part(inv, "other")
	if pc.Files != 2 || pc.Bytes < 1000 || pc.Label != "Page copies" {
		t.Errorf("page copies: %+v", pc)
	}
	if cache.Files != 2 || cache.Bytes != 320 || prof.Files != 1 || prof.Bytes != 500 || prof.InUse != 1 { // the new profile is young: in use
		t.Errorf("cache %+v profiles %+v", cache, prof)
	}
	if other.Bytes < 7 || other.Files < 1 { // the stray file, and the ledger
		t.Errorf("other: %+v", other)
	}
	var sum int64
	var files int
	for _, p := range inv.Parts {
		sum += p.Bytes
		files += p.Files
	}
	if inv.Bytes != sum || inv.Files != files {
		t.Errorf("the parts must add up to the whole folder: %d/%d vs %d/%d", sum, files, inv.Bytes, inv.Files)
	}
	if b, f := measure(root); b != inv.Bytes || f != inv.Files {
		t.Errorf("the size shown must be the size of the folder: %d %d vs %d %d", inv.Bytes, inv.Files, b, f)
	}
}

func TestClearingDeletesEverythingNotInUseAndKeepsTheLedger(t *testing.T) {
	root := tempFolder(t)
	chromeDir := filepath.Join(root, "chrome")
	if _, err := writeSnapshot("<p>", snapshotMeta{}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(shared.PageSnapshotsDir, "notes.txt"), 5) // not named like a copy: still part of the folder
	write(t, filepath.Join(shared.FetchCacheDir, "a.cache"), 100)
	write(t, filepath.Join(shared.FetchCacheDir, "index.json"), 10)
	write(t, filepath.Join(root, "stray.txt"), 7)
	write(t, filepath.Join(root, "strange", "deep", "file.bin"), 9)
	write(t, filepath.Join(chromeDir, "chrome-pids.previous.json.damaged"), 3)
	write(t, filepath.Join(chromeDir, ".chrome-pids-123.tmp"), 2)

	idle, _ := chromeproc.NewProfileDir(chromeproc.KindRod) // old and unregistered: goes
	write(t, filepath.Join(idle, "Default", "x"), 40)
	age(t, idle, 2*time.Hour)
	young, _ := chromeproc.NewProfileDir(chromeproc.KindKatana) // a moment old, not registered yet: stays
	write(t, filepath.Join(young, "Default", "y"), 40)
	running, _ := chromeproc.NewProfileDir(chromeproc.KindSharedHeadless) // registered and alive: stays whatever its age
	write(t, filepath.Join(running, "Default", "z"), 40)
	age(t, running, 2*time.Hour)
	pid := os.Getpid()
	chromeproc.Register(chromeproc.Launch{Kind: chromeproc.KindSharedHeadless, PID: pid, ProfileDir: running})
	t.Cleanup(func() { chromeproc.Release(pid) })

	ledger := filepath.Join(chromeDir, "chrome-pids.json")
	if !exists(ledger) {
		t.Fatal("the ledger must exist for this test")
	}

	res := ClearTemporaryFiles()
	for _, gone := range []string{filepath.Join(shared.PageSnapshotsDir, "notes.txt"), filepath.Join(shared.FetchCacheDir, "a.cache"), filepath.Join(shared.FetchCacheDir, "index.json"),
		filepath.Join(root, "stray.txt"), filepath.Join(root, "strange"), filepath.Join(chromeDir, "chrome-pids.previous.json.damaged"), idle} {
		if exists(gone) {
			t.Errorf("%s must be deleted", gone)
		}
	}
	for _, kept := range []string{ledger, filepath.Join(chromeDir, ".chrome-pids-123.tmp"), young, running, shared.PageSnapshotsDir, shared.FetchCacheDir, chromeproc.ProfileRoot()} {
		if !exists(kept) {
			t.Errorf("%s must be kept", kept)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(shared.PageSnapshotsDir, "*")); len(left) != 0 {
		t.Errorf("the page copies must all go: %v", left)
	}
	if res.DeletedFiles < 8 || res.DeletedBytes < 5+100+10+7+9+3+40 {
		t.Errorf("what was deleted must be reported: %+v", res)
	}
	if want, _ := measure(root); res.KeptBytes != want || res.KeptBytes < 80 {
		t.Errorf("kept bytes %d, folder is %d", res.KeptBytes, want)
	}
	if !strings.Contains(res.KeptReason, "2 browser session(s)") {
		t.Errorf("the reason must say what was kept: %q", res.KeptReason)
	}
	// nothing left to delete the second time
	if again := ClearTemporaryFiles(); again.DeletedFiles != 0 || again.DeletedBytes != 0 {
		t.Errorf("a second clear finds nothing: %+v", again)
	}
}

func TestAFolderOnlyNothingElseIsKept(t *testing.T) {
	root := tempFolder(t)
	write(t, filepath.Join(root, "a", "b.txt"), 4)
	res := ClearTemporaryFiles()
	if res.KeptReason != "" || exists(filepath.Join(root, "a")) {
		t.Errorf("%+v", res)
	}
}

func TestALinkIsRemovedAsALinkAndItsTargetIsNeverTouched(t *testing.T) {
	root := tempFolder(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "precious.txt"), 100)
	write(t, filepath.Join(outside, "sub", "precious2.txt"), 100)
	for _, link := range []string{filepath.Join(shared.PageSnapshotsDir, "link-to-dir"), filepath.Join(shared.FetchCacheDir, "link-to-file"), filepath.Join(root, "link-at-top"), filepath.Join(root, "chrome", "link-in-chrome")} {
		target := outside
		if strings.Contains(link, "to-file") {
			target = filepath.Join(outside, "precious.txt")
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skip("symbolic links are not available here")
		}
	}
	before := inventoryTempFiles()
	if before.Bytes > 1000 {
		t.Errorf("a link must not be followed when measuring: %d", before.Bytes)
	}
	res := ClearTemporaryFiles()
	for _, link := range []string{filepath.Join(shared.PageSnapshotsDir, "link-to-dir"), filepath.Join(shared.FetchCacheDir, "link-to-file"), filepath.Join(root, "link-at-top"), filepath.Join(root, "chrome", "link-in-chrome")} {
		if exists(link) {
			t.Errorf("the link %s must be removed", link)
		}
	}
	for _, kept := range []string{filepath.Join(outside, "precious.txt"), filepath.Join(outside, "sub", "precious2.txt")} {
		if !exists(kept) {
			t.Errorf("%s is outside the folder and must never be touched", kept)
		}
	}
	if res.DeletedBytes > 1000 {
		t.Errorf("the target of a link must not be counted as deleted: %+v", res)
	}
}

func TestNothingIsTouchedWhenTheFolderIsNotSetUpProperly(t *testing.T) {
	tempFolder(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "keep.txt"), 5)
	orig := shared.CacheRoot
	t.Cleanup(func() { shared.CacheRoot = orig })
	// the guard is tested on its own for every unsafe value, including the root of the file system: a clear is never run on those, so a broken guard could
	// never delete anything while this test runs
	for _, bad := range []string{"", "relative/path", "/", filepath.VolumeName(outside) + string(filepath.Separator)} {
		shared.CacheRoot = bad
		if got := tempRoot(); got != "" {
			t.Errorf("%q must not be a folder this tool clears, got %q", bad, got)
		}
	}
	// and the two harmless ones (nothing exists there) prove that an unset folder is measured and cleared as nothing
	for _, bad := range []string{"", "no-such-relative-folder-for-this-test"} {
		shared.CacheRoot = bad
		if inv := inventoryTempFiles(); inv.Bytes != 0 || len(inv.Parts) != 0 {
			t.Errorf("%q: %+v", bad, inv)
		}
		if res := ClearTemporaryFiles(); res.DeletedFiles != 0 {
			t.Errorf("%q: %+v", bad, res)
		}
	}
	shared.CacheRoot = outside // a proper absolute folder is cleared: the guard lets it through
	if got := tempRoot(); got != outside {
		t.Errorf("a proper folder must be accepted, got %q", got)
	}
}

func TestOnlyWhatLiesInsideTheFolderIsTouchedEvenWhenTheChromeFoldersAreElsewhere(t *testing.T) {
	useBrowserSettings(t, shared.FetchEngineKatana)
	elsewhere := t.TempDir()
	if err := chromeproc.Configure(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = chromeproc.Configure(testChromeDir) })
	idle, _ := chromeproc.NewProfileDir(chromeproc.KindRod)
	age(t, idle, 2*time.Hour)
	write(t, filepath.Join(elsewhere, "other.txt"), 5)
	ClearTemporaryFiles()
	if !exists(idle) || !exists(filepath.Join(elsewhere, "other.txt")) || !exists(filepath.Join(elsewhere, "chrome-pids.json")) {
		t.Error("a ledger or profile folder outside the temporary folder must not be touched by it")
	}
}

func TestClearingTheCacheWaitsForTheCacheLockAndALookupThenMisses(t *testing.T) {
	tempFolder(t)
	if err := fetchCacheStore("https://example.com/a", crawlResponse{HTML: "<p>a</p>", Title: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fetchCacheLookup("https://example.com/a"); !ok {
		t.Fatal("the entry must be served before the clear")
	}
	fetchCacheMu.Lock() // a crawl is writing the cache
	done := make(chan struct{})
	go func() { ClearTemporaryFiles(); close(done) }()
	select {
	case <-done:
		fetchCacheMu.Unlock()
		t.Fatal("the clear must wait for the cache's own lock")
	case <-time.After(150 * time.Millisecond):
	}
	fetchCacheMu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the clear must go on once the lock is free")
	}
	if _, ok := fetchCacheLookup("https://example.com/a"); ok {
		t.Error("after a clear the lookup is an ordinary miss")
	}
	if err := fetchCacheStore("https://example.com/b", crawlResponse{HTML: "<p>b</p>"}); err != nil { // and the cache works again
		t.Fatal(err)
	}
	if _, ok := fetchCacheLookup("https://example.com/b"); !ok {
		t.Error("the cache must work again after a clear")
	}
}

func TestOnlyTheNewestPageCopyIsKept(t *testing.T) {
	tempFolder(t)
	var last snapshotInfo
	for i := 0; i < 3; i++ {
		info, err := writeSnapshot("<p>"+strings.Repeat("x", i)+"</p>", snapshotMeta{})
		if err != nil {
			t.Fatal(err)
		}
		last = info
	}
	write(t, filepath.Join(shared.PageSnapshotsDir, "notes.txt"), 3) // not a copy: left alone by this rule
	if _, err := writeSnapshot("<p>new</p>", snapshotMeta{}); err != nil {
		t.Fatal(err)
	}
	count, _, _ := snapshotStats()
	if count != 1 {
		t.Errorf("one copy at a time, got %d", count)
	}
	if exists(filepath.Join(shared.PageSnapshotsDir, last.ID+".html")) || exists(filepath.Join(shared.PageSnapshotsDir, last.ID+".json")) {
		t.Error("the earlier copy and its meta must be gone")
	}
	if !exists(filepath.Join(shared.PageSnapshotsDir, "notes.txt")) {
		t.Error("the one-copy rule deletes copies only")
	}
}

func TestPageCopiesMadeAtTheSameTimeLeaveExactlyOne(t *testing.T) {
	tempFolder(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := writeSnapshot("<p>x</p>", snapshotMeta{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count, _, _ := snapshotStats(); count != 1 {
		t.Errorf("exactly one copy must be left, got %d", count)
	}
	entries, _ := os.ReadDir(shared.PageSnapshotsDir)
	if len(entries) != 2 { // its html and its json
		t.Errorf("no half copies: %d entries", len(entries))
	}
}

func TestTheTempFilesRoute(t *testing.T) {
	tempFolder(t)
	write(t, filepath.Join(shared.FetchCacheDir, "a.cache"), 64)
	rec := httptest.NewRecorder()
	TempFilesHandler(rec, httptest.NewRequest(http.MethodGet, "/temp-files", nil))
	var inv tempInventory
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &inv) != nil || part(inv, "page_cache").Bytes != 64 || inv.Bytes < 64 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), `"parts"`) {
		t.Errorf("headers %v body %s", rec.Header(), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	TempFilesHandler(rec, httptest.NewRequest(http.MethodDelete, "/temp-files", nil))
	var res tempClearResult
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.DeletedFiles < 1 || res.DeletedBytes < 64 || !strings.Contains(rec.Body.String(), `"deletedBytes"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if exists(filepath.Join(shared.FetchCacheDir, "a.cache")) {
		t.Error("DELETE must clear the folder")
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		rec = httptest.NewRecorder()
		TempFilesHandler(rec, httptest.NewRequest(m, "/temp-files", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	TempFilesHandler(rec, httptest.NewRequest(http.MethodDelete, "/temp-files?path=/etc", nil))
	if !exists("/etc/hosts") && !exists("/etc") {
		t.Error("a path in the request must be ignored")
	}
}

func TestTheLogLineOfAnAutomaticClear(t *testing.T) {
	if SummarizeTempClear("at start", tempClearResult{}) != "" {
		t.Error("nothing deleted: no line")
	}
	got := SummarizeTempClear("at start", tempClearResult{DeletedFiles: 3, DeletedBytes: 3 * 1024 * 1024})
	if got != "temporary files cleared at start: 3 file(s), 3.0 MB" {
		t.Errorf("%q", got)
	}
}
