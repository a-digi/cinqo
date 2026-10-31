package jobboard

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	// A clock that moves one second per call, so "most recently moved" is unambiguous.
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	old := now
	now = func() string {
		tick = tick.Add(time.Second)
		return tick.Format("2006-01-02T15:04:05.000Z")
	}
	t.Cleanup(func() { now = old })
}

func addJob(t *testing.T, id, title string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company) VALUES (?, ?, ?, 'Acme')`, id, "https://x/"+id, title); err != nil {
		t.Fatal(err)
	}
}

func mustStatus(t *testing.T, profileID, name string) Status {
	t.Helper()
	s, err := CreateStatus(profileID, name)
	if err != nil {
		t.Fatalf("CreateStatus(%q): %v", name, err)
	}
	return s
}

func mustSet(t *testing.T, profileID, jobID, statusID string) {
	t.Helper()
	if _, err := SetEntry(profileID, jobID, statusID); err != nil {
		t.Fatalf("SetEntry: %v", err)
	}
}

func columnNames(b Board) []string {
	names := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		names[i] = c.Name
	}
	return names
}

func jobIDs(c Column) string {
	ids := make([]string, len(c.Jobs))
	for i, j := range c.Jobs {
		ids[i] = j.JobID
	}
	return strings.Join(ids, ",")
}

func TestFixedStatusesAreSeeded(t *testing.T) {
	setup(t)
	b, err := GetBoard("p1")
	if err != nil {
		t.Fatal(err)
	}
	want := "Not Reviewed|Applied|Declined|Stopped application"
	if got := strings.Join(columnNames(b), "|"); got != want {
		t.Errorf("columns = %s, want %s", got, want)
	}
	for _, c := range b.Columns {
		if c.Jobs == nil || len(c.Jobs) != 0 {
			t.Errorf("column %s: jobs must be an empty list, got %#v", c.Name, c.Jobs)
		}
	}
	if b.Columns[0].ID != NotReviewedID || b.Columns[1].ID != AppliedID || b.Columns[2].ID != DeclinedID || b.Columns[3].ID != StoppedID {
		t.Errorf("fixed ids = %+v", b.Columns)
	}
}

func TestColumnOrderPutsCustomStatusesBetween(t *testing.T) {
	setup(t)
	mustStatus(t, "p1", "Interview")
	mustStatus(t, "p1", "Offer")
	mustStatus(t, "p2", "Other profile's")
	b, _ := GetBoard("p1")
	want := "Not Reviewed|Interview|Offer|Applied|Declined|Stopped application"
	if got := strings.Join(columnNames(b), "|"); got != want {
		t.Errorf("columns = %s, want %s", got, want)
	}
}

func TestFixedStatusesCannotChange(t *testing.T) {
	setup(t)
	for _, id := range []string{NotReviewedID, DeclinedID, StoppedID, AppliedID} {
		if _, err := RenameStatus(id, "Whatever"); !errors.Is(err, ErrFixedStatus) {
			t.Errorf("rename %s: %v", id, err)
		}
		if err := DeleteStatus(id); !errors.Is(err, ErrFixedStatus) {
			t.Errorf("delete %s: %v", id, err)
		}
	}
	// The database refuses a fixed-looking status with an owner, and a custom one without.
	if _, err := db.JobsDB.Exec(`INSERT INTO job_statuses (id, profile_id, name, kind) VALUES ('x', 'p1', 'X', 'applied')`); err == nil {
		t.Error("a fixed kind with a profile must be refused")
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_statuses (id, profile_id, name, kind) VALUES ('y', NULL, 'Y', 'custom')`); err == nil {
		t.Error("a custom kind without a profile must be refused")
	}
}

func TestStatusNameRules(t *testing.T) {
	setup(t)
	for _, bad := range []string{"", "   ", strings.Repeat("a", MaxNameLength+1)} {
		if _, err := CreateStatus("p1", bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: %v", bad, err)
		}
	}
	if s, err := CreateStatus("p1", "  "+strings.Repeat("é", MaxNameLength)+"  "); err != nil || s.Name != strings.Repeat("é", MaxNameLength) {
		t.Errorf("a 40-character name (trimmed) must pass: %+v, %v", s, err)
	}
	// Fixed names are taken, ignoring case, for every profile.
	for _, n := range []string{"applied", "NOT REVIEWED", "Declined", "stopped Application"} {
		if _, err := CreateStatus("p1", n); !errors.Is(err, ErrDuplicateName) {
			t.Errorf("fixed name %q: %v", n, err)
		}
	}
	mustStatus(t, "p1", "Interview")
	if _, err := CreateStatus("p1", "interview"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("duplicate ignoring case: %v", err)
	}
	// Another profile may use the same name.
	mustStatus(t, "p2", "Interview")
}

func TestCustomStatusLimit(t *testing.T) {
	setup(t)
	for i := 0; i < MaxCustomStatuses; i++ {
		mustStatus(t, "p1", fmt.Sprintf("S%d", i))
	}
	if _, err := CreateStatus("p1", "One too many"); !errors.Is(err, ErrTooManyStatuses) {
		t.Errorf("13th status: %v", err)
	}
	mustStatus(t, "p2", "Fine") // the limit is per profile
}

func TestRenameStatus(t *testing.T) {
	setup(t)
	a := mustStatus(t, "p1", "Interview")
	mustStatus(t, "p1", "Offer")
	if s, err := RenameStatus(a.ID, "Interview"); err != nil || s.Name != "Interview" {
		t.Errorf("same name: %+v, %v", s, err)
	}
	if s, err := RenameStatus(a.ID, "  INTERVIEW  "); err != nil || s.Name != "INTERVIEW" {
		t.Errorf("recasing its own name: %+v, %v", s, err)
	}
	if _, err := RenameStatus(a.ID, "offer"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("onto another status: %v", err)
	}
	if _, err := RenameStatus(a.ID, "Applied"); !errors.Is(err, ErrDuplicateName) {
		t.Errorf("onto a fixed name: %v", err)
	}
	if _, err := RenameStatus(a.ID, " "); !errors.Is(err, ErrInvalidName) {
		t.Errorf("blank: %v", err)
	}
	if _, err := RenameStatus("nope", "X"); !errors.Is(err, ErrUnknownStatus) {
		t.Errorf("unknown: %v", err)
	}
}

func TestSetEntryAddsThenMoves(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Go dev")
	e, err := SetEntry("p1", "j1", NotReviewedID)
	if err != nil || e.StatusID != NotReviewedID {
		t.Fatalf("add: %+v, %v", e, err)
	}
	mustSet(t, "p1", "j1", AppliedID)
	b, _ := GetBoard("p1")
	if jobIDs(b.Columns[0]) != "" || jobIDs(b.Columns[1]) != "j1" {
		t.Errorf("after the move: %v / %v", b.Columns[0].Jobs, b.Columns[1].Jobs)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries`).Scan(&n)
	if n != 1 {
		t.Errorf("a move must update the row, found %d rows", n)
	}
	card := b.Columns[1].Jobs[0]
	if card.Title != "Go dev" || card.Company != "Acme" || card.Location != "" {
		t.Errorf("card = %+v", card)
	}
}

func TestSetEntryRejections(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Go dev")
	mine := mustStatus(t, "p1", "Interview")
	if _, err := SetEntry("p1", "missing", NotReviewedID); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("unknown job: %v", err)
	}
	if _, err := SetEntry("p1", "j1", "missing"); !errors.Is(err, ErrUnknownStatus) {
		t.Errorf("unknown status: %v", err)
	}
	if _, err := SetEntry("p2", "j1", mine.ID); !errors.Is(err, ErrStatusNotOnBoard) {
		t.Errorf("another profile's status: %v", err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries`).Scan(&n)
	if n != 0 {
		t.Errorf("a rejected call must write nothing, found %d rows", n)
	}
}

func TestColumnListsMostRecentlyMovedFirst(t *testing.T) {
	setup(t)
	addJob(t, "j1", "A")
	addJob(t, "j2", "B")
	addJob(t, "j3", "C")
	mustSet(t, "p1", "j1", NotReviewedID)
	mustSet(t, "p1", "j2", NotReviewedID)
	mustSet(t, "p1", "j3", NotReviewedID)
	b, _ := GetBoard("p1")
	if got := jobIDs(b.Columns[0]); got != "j3,j2,j1" {
		t.Errorf("order = %s", got)
	}
	mustSet(t, "p1", "j1", AppliedID)
	mustSet(t, "p1", "j2", AppliedID)
	b, _ = GetBoard("p1")
	if got := jobIDs(b.Columns[1]); got != "j2,j1" {
		t.Errorf("applied order = %s", got)
	}
}

func TestOneJobOnTwoBoardsWithDifferentStatuses(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Go dev")
	interview := mustStatus(t, "p1", "Interview")
	mustSet(t, "p1", "j1", interview.ID)
	mustSet(t, "p2", "j1", DeclinedID)

	b1, _ := GetBoard("p1")
	b2, _ := GetBoard("p2")
	if jobIDs(b1.Columns[1]) != "j1" || jobIDs(b2.Columns[0]) != "" || jobIDs(b2.Columns[2]) != "j1" {
		t.Errorf("boards = %+v / %+v", b1.Columns, b2.Columns)
	}
	// Another profile's board has no trace of p1's custom status.
	if len(b2.Columns) != 4 {
		t.Errorf("p2 sees %v", columnNames(b2))
	}

	usage, err := GetUsage("j1")
	if err != nil || len(usage) != 2 {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
	byProfile := map[string]Usage{}
	for _, u := range usage {
		byProfile[u.ProfileID] = u
	}
	if u := byProfile["p1"]; u.StatusName != "Interview" || u.Kind != KindCustom || u.StatusID != interview.ID {
		t.Errorf("p1 usage = %+v", u)
	}
	if u := byProfile["p2"]; u.StatusName != "Declined" || u.Kind != KindDeclined {
		t.Errorf("p2 usage = %+v", u)
	}
	if u, err := GetUsage("nobody"); err != nil || u == nil || len(u) != 0 {
		t.Errorf("a job with no board: %#v, %v", u, err)
	}
}

func TestRemoveEntry(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Go dev")
	mustSet(t, "p1", "j1", AppliedID)
	mustSet(t, "p2", "j1", AppliedID)
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Errorf("removing twice must be a no-op: %v", err)
	}
	if u, _ := GetUsage("j1"); len(u) != 1 || u[0].ProfileID != "p2" {
		t.Errorf("usage = %+v", u)
	}
}

func TestDeleteStatusMovesItsJobsToNotReviewed(t *testing.T) {
	setup(t)
	addJob(t, "j1", "A")
	addJob(t, "j2", "B")
	addJob(t, "j3", "C")
	s := mustStatus(t, "p1", "Interview")
	mustSet(t, "p1", "j1", s.ID)
	mustSet(t, "p1", "j2", s.ID)
	mustSet(t, "p1", "j3", AppliedID)

	if err := DeleteStatus(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteStatus(s.ID); err != nil {
		t.Errorf("deleting an unknown status must be a no-op: %v", err)
	}
	b, _ := GetBoard("p1")
	// Both moved at the same instant, so they list by title.
	if len(b.Columns) != 4 || jobIDs(b.Columns[0]) != "j1,j2" || jobIDs(b.Columns[1]) != "j3" {
		t.Errorf("columns = %+v", b.Columns)
	}
}

func TestStatusWithEntriesCannotBeDeletedBehindOurBack(t *testing.T) {
	setup(t)
	addJob(t, "j1", "A")
	s := mustStatus(t, "p1", "Interview")
	mustSet(t, "p1", "j1", s.ID)
	if _, err := db.JobsDB.Exec(`DELETE FROM job_statuses WHERE id = ?`, s.ID); err == nil {
		t.Error("ON DELETE RESTRICT must stop a raw delete of a status in use")
	}
}

func TestDeletingAJobRemovesItsEntries(t *testing.T) {
	setup(t)
	addJob(t, "j1", "A")
	mustSet(t, "p1", "j1", AppliedID)
	mustSet(t, "p2", "j1", NotReviewedID)
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries`).Scan(&n)
	if n != 0 {
		t.Errorf("%d entries left after the job was deleted", n)
	}
}

func TestPurgeProfile(t *testing.T) {
	setup(t)
	addJob(t, "j1", "A")
	s := mustStatus(t, "p1", "Interview")
	mustSet(t, "p1", "j1", s.ID)
	mustSet(t, "p2", "j1", AppliedID)
	mustStatus(t, "p2", "Keep")

	if err := PurgeProfile("p1"); err != nil {
		t.Fatal(err)
	}
	if err := PurgeProfile("p1"); err != nil {
		t.Errorf("purging twice must be a no-op: %v", err)
	}
	if u, _ := GetUsage("j1"); len(u) != 1 || u[0].ProfileID != "p2" {
		t.Errorf("usage = %+v", u)
	}
	b2, _ := GetBoard("p2")
	if len(b2.Columns) != 5 || jobIDs(b2.Columns[2]) != "j1" {
		t.Errorf("p2's board changed: %v", columnNames(b2))
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_statuses WHERE profile_id = 'p1'`).Scan(&n)
	if n != 0 {
		t.Errorf("%d of p1's statuses left", n)
	}
	var fixed int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_statuses WHERE profile_id IS NULL`).Scan(&fixed)
	if fixed != 4 {
		t.Errorf("the fixed statuses must survive, found %d", fixed)
	}
}

func TestSeedIsIdempotentAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	for i := 0; i < 2; i++ {
		if err := db.InitDatabases(); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_statuses`).Scan(&n)
		if n != 4 {
			t.Errorf("start %d: %d statuses, want 4", i+1, n)
		}
		db.JobsDB.Close()
		db.CareerDB.Close()
	}
}
