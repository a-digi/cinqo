package jobs

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	"career-tool-backend/db"
)

// seedRaw puts a job straight into the table, the way old crawls stored it (no cleaning).
func seedRaw(t *testing.T, id, source, description string) {
	t.Helper()
	var desc any = description
	if description == "\x00" {
		desc = nil
	}
	if _, err := db.JobsDB.Exec(
		`INSERT INTO jobs (id, source_url, title, description, source, crawled_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		id, "https://example.com/"+id, "Job "+id, desc, source,
	); err != nil {
		t.Fatal(err)
	}
}

func descriptionOf(t *testing.T, id string) string {
	t.Helper()
	var d sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT description FROM jobs WHERE id = ?`, id).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d.String
}

func fixRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM data_fixes WHERE name = ?`, descriptionCleanFix).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCleanStoredDescriptionsRewritesOnlyWhatChanges(t *testing.T) {
	setupJobsDB(t)
	manualDirty := "My notes <b>kept</b>\n.a { top: 0; }"
	seedRaw(t, "dirty", "crawled", dirtyDescription)
	seedRaw(t, "clean", "crawled", "Plain text\n\nstays")
	seedRaw(t, "manual", "manual", manualDirty)
	seedRaw(t, "markup-only", "crawled", "<style>.a { top: 0; }</style><img src=\"x\"/>")
	seedRaw(t, "empty", "crawled", "")
	seedRaw(t, "null", "crawled", "\x00")

	res, err := CleanStoredDescriptions()
	if err != nil {
		t.Fatal(err)
	}
	if res.AlreadyDone || res.Scanned != 3 || res.Changed != 2 || res.Backup == "" {
		t.Errorf("result %+v", res)
	}
	if got := descriptionOf(t, "dirty"); got != cleanDescription {
		t.Errorf("dirty: %q", got)
	}
	if got := descriptionOf(t, "clean"); got != "Plain text\n\nstays" {
		t.Errorf("clean: %q", got)
	}
	if got := descriptionOf(t, "manual"); got != manualDirty {
		t.Errorf("a manual job must not be touched: %q", got)
	}
	if got := descriptionOf(t, "markup-only"); got != "" {
		t.Errorf("markup only becomes empty: %q", got)
	}
	if got := descriptionOf(t, "empty"); got != "" {
		t.Errorf("empty: %q", got)
	}
	if fixRows(t) != 1 {
		t.Error("the marker was not written")
	}
}

func TestTheBackupHoldsTheOldText(t *testing.T) {
	setupJobsDB(t)
	seedRaw(t, "dirty", "crawled", dirtyDescription)
	res, err := CleanStoredDescriptions()
	if err != nil || res.Backup == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(res.Backup); err != nil {
		t.Fatal(err)
	}
	bak, err := sql.Open("sqlite", res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	defer bak.Close()
	var old string
	if err := bak.QueryRow(`SELECT description FROM jobs WHERE id = 'dirty'`).Scan(&old); err != nil || old != dirtyDescription {
		t.Errorf("the backup has %q (%v)", old, err)
	}
	if got := descriptionOf(t, "dirty"); got == dirtyDescription {
		t.Error("the live row was not rewritten")
	}
}

func TestItIsDoneOnce(t *testing.T) {
	setupJobsDB(t)
	seedRaw(t, "dirty", "crawled", dirtyDescription)
	if _, err := CleanStoredDescriptions(); err != nil {
		t.Fatal(err)
	}
	// A dirty row that shows up later is not this repair's business: the marker says it was done.
	seedRaw(t, "later", "crawled", dirtyDescription)
	res, err := CleanStoredDescriptions()
	if err != nil || !res.AlreadyDone || res.Changed != 0 || res.Backup != "" {
		t.Errorf("second run %+v %v", res, err)
	}
	if descriptionOf(t, "later") != dirtyDescription {
		t.Error("a second run rewrote a row")
	}
	if fixRows(t) != 1 {
		t.Error("one marker")
	}
}

func TestNothingToCleanStillMarksItDoneWithoutABackup(t *testing.T) {
	setupJobsDB(t)
	seedRaw(t, "clean", "crawled", "Plain text")
	called := false
	old := backupJobsDB
	backupJobsDB = func(string) (string, error) { called = true; return "x", nil }
	t.Cleanup(func() { backupJobsDB = old })

	res, err := CleanStoredDescriptions()
	if err != nil || res.Changed != 0 || res.Scanned != 1 || res.Backup != "" || called {
		t.Errorf("%+v %v backup called=%v", res, err, called)
	}
	if fixRows(t) != 1 {
		t.Error("marked done")
	}
}

func TestADatabaseWithoutJobsIsFine(t *testing.T) {
	setupJobsDB(t)
	res, err := CleanStoredDescriptions()
	if err != nil || res.Scanned != 0 || res.Changed != 0 || fixRows(t) != 1 {
		t.Errorf("%+v %v", res, err)
	}
}

// When the copy cannot be made nothing is rewritten and the repair is not marked, so the next start tries again.
func TestNoBackupNoRewrite(t *testing.T) {
	setupJobsDB(t)
	seedRaw(t, "dirty", "crawled", dirtyDescription)
	old := backupJobsDB
	backupJobsDB = func(string) (string, error) { return "", errors.New("disk full") }
	t.Cleanup(func() { backupJobsDB = old })

	if _, err := CleanStoredDescriptions(); err == nil {
		t.Fatal("expected the backup error")
	}
	if descriptionOf(t, "dirty") != dirtyDescription || fixRows(t) != 0 {
		t.Error("nothing may change without a backup")
	}
	backupJobsDB = old
	if res, err := CleanStoredDescriptions(); err != nil || res.Changed != 1 {
		t.Errorf("the next run does it: %+v %v", res, err)
	}
}

// A write that fails half way leaves every row and the marker as they were.
func TestAFailedRewriteChangesNothing(t *testing.T) {
	setupJobsDB(t)
	seedRaw(t, "a", "crawled", dirtyDescription)
	seedRaw(t, "b", "crawled", dirtyDescription)
	seedRaw(t, "c", "crawled", dirtyDescription)
	if _, err := db.JobsDB.Exec(`CREATE TRIGGER stop_b BEFORE UPDATE OF description ON jobs WHEN NEW.id = 'b' BEGIN SELECT RAISE(ABORT, 'stop'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := CleanStoredDescriptions(); err == nil {
		t.Fatal("expected the failed write")
	}
	for _, id := range []string{"a", "b", "c"} {
		if descriptionOf(t, id) != dirtyDescription {
			t.Errorf("%s was changed by a failed run", id)
		}
	}
	if fixRows(t) != 0 {
		t.Error("a failed run must not mark the repair done")
	}
	if _, err := db.JobsDB.Exec(`DROP TRIGGER stop_b`); err != nil {
		t.Fatal(err)
	}
	if res, err := CleanStoredDescriptions(); err != nil || res.Changed != 3 {
		t.Errorf("after the fix: %+v %v", res, err)
	}
}
