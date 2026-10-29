package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstructionAttemptsSchema covers step 86's table on a fresh
// install, an idempotent restart, and its constraints.
func TestInstructionAttemptsSchema(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatalf("fresh InitDatabases: %v", err)
	}
	// A second start against the existing files must also succeed.
	JobsDB.Close()
	CareerDB.Close()
	if err := InitDatabases(); err != nil {
		t.Fatalf("restart InitDatabases: %v", err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := JobsDB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`)
	mustExec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`)

	mustExec(`INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('a1', 'l1', 'listing')`)

	var status, triggeredBy string
	if err := JobsDB.QueryRow(`SELECT status, triggered_by FROM instruction_attempts WHERE id = 'a1'`).Scan(&status, &triggeredBy); err != nil || status != "running" || triggeredBy != "manual" {
		t.Fatalf("defaults: status=%q triggered_by=%q err=%v", status, triggeredBy, err)
	}

	// At most one running attempt per link and kind.
	if _, err := JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('a2', 'l1', 'listing')`); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("second running attempt for same link+kind must be rejected, got %v", err)
	}
	// ...but another kind may run, and a finished one frees the slot.
	mustExec(`INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('a3', 'l1', 'job_detail')`)
	mustExec(`UPDATE instruction_attempts SET status = 'failed' WHERE id = 'a1'`)
	mustExec(`INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('a4', 'l1', 'listing')`)

	for name, q := range map[string]string{
		"bad kind":     `INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('b1', 'l1', 'other')`,
		"bad status":   `INSERT INTO instruction_attempts (id, portal_link_id, kind, status) VALUES ('b2', 'l1', 'listing', 'weird')`,
		"bad trigger":  `INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by) VALUES ('b3', 'l1', 'listing', 'robot')`,
		"unknown link": `INSERT INTO instruction_attempts (id, portal_link_id, kind) VALUES ('b4', 'nope', 'listing')`,
	} {
		if _, err := JobsDB.Exec(q); err == nil {
			t.Errorf("%s: expected a constraint error", name)
		}
	}

	// Deleting the link cascades.
	mustExec(`DELETE FROM portal_links WHERE id = 'l1'`)
	var n int
	if err := JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cascade delete left %d rows (err=%v)", n, err)
	}
}

func TestFailureReasonColumnOnAFreshInstall(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	if _, err := JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"not_saved", "error", "interrupted"} {
		if _, err := JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, failure_reason) VALUES (?, 'l1', 'listing', 'failed', ?)`, reason, reason); err != nil {
			t.Errorf("%s: %v", reason, err)
		}
	}
	if _, err := JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, failure_reason) VALUES ('bad', 'l1', 'listing', 'failed', 'whatever')`); err == nil {
		t.Error("an unknown failure_reason must be rejected by the CHECK constraint")
	}
}

// A database created by step 86 has no failure_reason column; the
// migration must add it and classify the failures already recorded.
func TestMigrateInstructionAttemptsFailureReasonBackfills(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Exec(`CREATE TABLE instruction_attempts (
		id TEXT PRIMARY KEY, portal_link_id TEXT NOT NULL, kind TEXT NOT NULL, triggered_by TEXT NOT NULL DEFAULT 'manual',
		status TEXT NOT NULL DEFAULT 'running', conversation_id TEXT, started_at TEXT NOT NULL DEFAULT (datetime('now')),
		finished_at TEXT, saved_at TEXT, error_message TEXT)`); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ id, status, msg string }{
		{"gave-up", "failed", "The AI finished without saving instructions."},
		{"interrupted", "failed", "Interrupted: the app or tab closed before the run finished."},
		{"turn-error", "failed", "provider returned 500"},
		{"ok", "succeeded", ""},
		{"running", "running", ""},
	}
	for _, r := range rows {
		var msg any
		if r.msg != "" {
			msg = r.msg
		}
		if _, err := database.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, error_message) VALUES (?, 'l1', 'listing', ?, ?)`, r.id, r.status, msg); err != nil {
			t.Fatal(err)
		}
	}

	if err := migrateInstructionAttemptsFailureReason(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := map[string]*string{"gave-up": ptr("not_saved"), "interrupted": ptr("interrupted"), "turn-error": ptr("error"), "ok": nil, "running": nil}
	for id, w := range want {
		var got *string
		if err := database.QueryRow(`SELECT failure_reason FROM instruction_attempts WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (w == nil) || (got != nil && *got != *w) {
			t.Errorf("%s: failure_reason = %v, want %v", id, deref(got), deref(w))
		}
	}

	// Idempotent, and a later start never rewrites rows.
	if _, err := database.Exec(`UPDATE instruction_attempts SET failure_reason = 'error' WHERE id = 'gave-up'`); err != nil {
		t.Fatal(err)
	}
	if err := migrateInstructionAttemptsFailureReason(database); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := database.QueryRow(`SELECT failure_reason FROM instruction_attempts WHERE id = 'gave-up'`).Scan(&got); err != nil || got != "error" {
		t.Errorf("a second run must not backfill again: %q, %v", got, err)
	}

	// No table at all (a fresh install at migration time) is a no-op.
	empty, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	if err := migrateInstructionAttemptsFailureReason(empty); err != nil {
		t.Errorf("missing table must be a no-op: %v", err)
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// The full startup path on an install that already has step 86's table
// (migrations run before the base schema): the column must appear.
func TestInitDatabasesUpgradesAStepEightySixInstall(t *testing.T) {
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE instruction_attempts (
		id TEXT PRIMARY KEY, portal_link_id TEXT NOT NULL, kind TEXT NOT NULL, triggered_by TEXT NOT NULL DEFAULT 'manual',
		status TEXT NOT NULL DEFAULT 'running', conversation_id TEXT, started_at TEXT NOT NULL DEFAULT (datetime('now')),
		finished_at TEXT, saved_at TEXT, error_message TEXT);
		INSERT INTO instruction_attempts (id, portal_link_id, kind, status, error_message) VALUES ('a', 'l1', 'listing', 'failed', 'The AI finished without saving instructions.')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatalf("upgrade start: %v", err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	var reason string
	if err := JobsDB.QueryRow(`SELECT failure_reason FROM instruction_attempts WHERE id = 'a'`).Scan(&reason); err != nil || reason != "not_saved" {
		t.Fatalf("upgraded row: %q, %v", reason, err)
	}
}

func TestCrawlRunsListingOutcomeColumnsOnFreshAndUpgradedInstalls(t *testing.T) {
	columns := []string{"jobs_saved", "jobs_updated", "jobs_skipped", "pages_visited", "stopped_reason", "not_found_fields"}

	// Fresh install: from the base schema.
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	for _, c := range columns {
		if _, has, err := tableHasColumn(JobsDB, "crawl_runs", c); err != nil || !has {
			t.Errorf("fresh install lacks crawl_runs.%s (%v)", c, err)
		}
	}
	JobsDB.Close()
	CareerDB.Close()

	// Upgrade: an existing crawl_runs table without them, with a row.
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE crawl_runs (id TEXT PRIMARY KEY, portal_link_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'running',
		started_at TEXT NOT NULL DEFAULT (datetime('now')), finished_at TEXT, log TEXT NOT NULL DEFAULT '', result_summary TEXT,
		error_message TEXT, phase TEXT, kind TEXT NOT NULL DEFAULT 'listing', triggered_by TEXT NOT NULL DEFAULT 'manual');
		INSERT INTO crawl_runs (id, portal_link_id, status, result_summary) VALUES ('old', 'l1', 'completed', 'Saved 0 new, updated 0, skipped 0')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatalf("upgrade start: %v", err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	for _, c := range columns {
		if _, has, err := tableHasColumn(JobsDB, "crawl_runs", c); err != nil || !has {
			t.Errorf("upgraded install lacks crawl_runs.%s (%v)", c, err)
		}
	}
	// No guessing: the pre-existing run has no structured data.
	var saved *int
	if err := JobsDB.QueryRow(`SELECT jobs_saved FROM crawl_runs WHERE id = 'old'`).Scan(&saved); err != nil || saved != nil {
		t.Errorf("an old run must stay NULL, got %v (%v)", saved, err)
	}
}

func TestPortalLinksEmptyCrawlAcknowledgedAtOnFreshAndUpgradedInstalls(t *testing.T) {
	// Fresh install: from the base schema.
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if _, has, err := tableHasColumn(JobsDB, "portal_links", "empty_crawl_acknowledged_at"); err != nil || !has {
		t.Errorf("fresh install lacks portal_links.empty_crawl_acknowledged_at (%v)", err)
	}
	JobsDB.Close()
	CareerDB.Close()

	// Upgrade: a portal_links table without it (the migration only adds it).
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, UNIQUE(portal_id, url));
		INSERT INTO portals VALUES ('p1', 'P'); INSERT INTO portal_links VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	if err := migratePortalLinksEmptyCrawlAcknowledgedAt(legacy); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := migratePortalLinksEmptyCrawlAcknowledgedAt(legacy); err != nil {
		t.Fatalf("migrate must be idempotent: %v", err)
	}
	var ack *string
	if err := legacy.QueryRow(`SELECT empty_crawl_acknowledged_at FROM portal_links WHERE id = 'l1'`).Scan(&ack); err != nil || ack != nil {
		t.Errorf("existing links must start unacknowledged: %v %v", ack, err)
	}
	legacy.Close()
}

func TestPortalLinksJobsQuestionOnFreshAndUpgradedInstalls(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"jobs_question_asked_at", "jobs_question_note"} {
		if _, has, err := tableHasColumn(JobsDB, "portal_links", col); err != nil || !has {
			t.Errorf("fresh install lacks portal_links.%s (%v)", col, err)
		}
	}
	JobsDB.Close()
	CareerDB.Close()

	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, UNIQUE(portal_id, url));
		INSERT INTO portals VALUES ('p1', 'P'); INSERT INTO portal_links VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := migratePortalLinksJobsQuestion(legacy); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var asked, note *string
	if err := legacy.QueryRow(`SELECT jobs_question_asked_at, jobs_question_note FROM portal_links WHERE id = 'l1'`).Scan(&asked, &note); err != nil || asked != nil || note != nil {
		t.Errorf("existing links must start with nothing pending: %v %v %v", asked, note, err)
	}
}

func TestJobsSourceColumnOnFreshAndUpgradedInstalls(t *testing.T) {
	// Fresh install: from the base schema, with its CHECK.
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if _, has, err := tableHasColumn(JobsDB, "jobs", "source"); err != nil || !has {
		t.Fatalf("fresh install lacks jobs.source (%v)", err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://x/1', 'T')`); err != nil {
		t.Fatal(err)
	}
	var src string
	if err := JobsDB.QueryRow(`SELECT source FROM jobs WHERE id = 'j1'`).Scan(&src); err != nil || src != "crawled" {
		t.Errorf("a row without a source must default to crawled: %q %v", src, err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, source) VALUES ('j2', 'https://x/2', 'T', 'weird')`); err == nil {
		t.Error("an unknown source must be rejected by the CHECK")
	}
	if _, err := JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, source) VALUES ('j3', 'manual:j3', 'T', 'manual')`); err != nil {
		t.Errorf("manual must be accepted: %v", err)
	}
	JobsDB.Close()
	CareerDB.Close()

	// Upgrade: a jobs table from before this step keeps its rows as 'crawled'.
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(`CREATE TABLE jobs (id TEXT PRIMARY KEY, source_url TEXT NOT NULL UNIQUE, title TEXT NOT NULL);
		INSERT INTO jobs VALUES ('old', 'https://x/old', 'Old job')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := migrateJobsSource(legacy); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	if err := legacy.QueryRow(`SELECT source FROM jobs WHERE id = 'old'`).Scan(&src); err != nil || src != "crawled" {
		t.Errorf("an existing job must read as crawled: %q %v", src, err)
	}
	// And no jobs table at all is not an error.
	empty, _ := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	defer empty.Close()
	if err := migrateJobsSource(empty); err != nil {
		t.Errorf("no table: %v", err)
	}
}
