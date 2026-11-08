package crawler

import (
	"path/filepath"
	"strings"
	"testing"

	"browser-tool-backend/shared"
)

func TestLogRefForGivesTheReadersRelativePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "data", "logs", "tools", "browser")
	good := map[string]string{
		filepath.Join(root, "crawl_logs", "0f1e2d3c-aaaa-bbbb-cccc-1234567890ab.json"):        "crawl_logs/0f1e2d3c-aaaa-bbbb-cccc-1234567890ab.json",
		filepath.Join(root, "challenge_logs", "run-1", "challenge_detector_1794004203_1.txt"): "challenge_logs/run-1/challenge_detector_1794004203_1.txt",
	}
	for path, want := range good {
		if got := logRefFor(root, path); got != want {
			t.Errorf("%s: got %q want %q", path, got, want)
		}
	}
	bad := []string{
		"",   // no path
		root, // the folder itself
		filepath.Join(root, "..", "other", "a.json"), // outside the root
		filepath.Join(string(filepath.Separator), "elsewhere", "a.json"),
		filepath.Join(root, "crawl_logs", ".hidden.json"),          // a hidden file is not readable
		filepath.Join(root, "crawl_logs", "a b.json"),              // a space is not an accepted character
		filepath.Join(root, "a", "b", "c", "d", "e", "f", "g.txt"), // seven parts
		filepath.Join(root, "crawl_logs", strings.Repeat("x", 129)+".json"),
	}
	for _, path := range bad {
		if got := logRefFor(root, path); got != "" {
			t.Errorf("%q must give no reference, got %q", path, got)
		}
	}
	if logRefFor("", filepath.Join(root, "crawl_logs", "a.json")) != "" {
		t.Error("an unknown root gives no reference")
	}
}

func TestTheCrawlLogListCarriesTheRefOnlyForFilesUnderTheLogsFolder(t *testing.T) {
	useBrowserSettings(t, "katana") // a real database and logs folders in a temp directory
	inside := filepath.Join(shared.CrawlLogsDir, "11111111-1111-1111-1111-111111111111.json")
	outside := filepath.Join(t.TempDir(), "22222222-2222-2222-2222-222222222222.json")
	for id, path := range map[string]string{"11111111-1111-1111-1111-111111111111": inside, "22222222-2222-2222-2222-222222222222": outside} {
		if _, err := shared.DB.Exec(`INSERT INTO crawl_logs (id, created_at, stopped_reason, pages_visited, not_found_count, log_file_path) VALUES (?, ?, 'no_next_link', 1, 0, ?)`,
			id, "2026-11-07 10:00:00.000", path); err != nil {
			t.Fatal(err)
		}
	}
	list, err := listCrawlLogs()
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %+v", err, list)
	}
	got := map[string]string{}
	for _, s := range list {
		got[s.ID] = s.LogRef
	}
	if got["11111111-1111-1111-1111-111111111111"] != "crawl_logs/11111111-1111-1111-1111-111111111111.json" {
		t.Errorf("inside: %q", got["11111111-1111-1111-1111-111111111111"])
	}
	if got["22222222-2222-2222-2222-222222222222"] != "" {
		t.Errorf("a file outside the logs folder has no reference: %q", got["22222222-2222-2222-2222-222222222222"])
	}
}
