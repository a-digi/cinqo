package db

import (
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
