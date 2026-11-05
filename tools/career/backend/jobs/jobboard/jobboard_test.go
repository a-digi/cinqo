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

// sortFixture: p1's board has four jobs moved in the order j1..j4 (j4 most recently). j1 and j2 have a completed
// match made for p1 (60 and 80), j3's completed match (99) was made for another profile, j4's AI match is running (it carries a score, which must not count).
func sortFixture(t *testing.T) {
	t.Helper()
	setup(t)
	for _, id := range []string{"j1", "j2", "j3", "j4"} {
		addJob(t, id, "Job "+id)
		mustSet(t, "p1", id, NotReviewedID)
	}
	mustSet(t, "p2", "j3", NotReviewedID)
	for _, q := range []string{
		`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j1', 'p1', 60, 'completed')`,
		`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j2', 'p1', 80, 'completed')`,
		`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j3', 'p2', 99, 'completed')`,
		`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j4', 'p1', 90, 'matching')`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func scoresOf(c Column) string {
	var s []string
	for _, j := range c.Jobs {
		if j.MatchScore == nil {
			s = append(s, j.JobID+"=-")
		} else {
			s = append(s, fmt.Sprintf("%s=%d", j.JobID, *j.MatchScore))
		}
	}
	return strings.Join(s, ",")
}

func TestBoardOrdersByMovedOrByMatch(t *testing.T) {
	sortFixture(t)
	moved, err := GetBoardSorted("p1", SortMoved)
	if err != nil {
		t.Fatal(err)
	}
	if got := jobIDs(moved.Columns[0]); got != "j4,j3,j2,j1" {
		t.Errorf("moved order = %s", got)
	}
	byMatch, err := GetBoardSorted("p1", SortMatch)
	if err != nil {
		t.Fatal(err)
	}
	// j2 (80), j1 (60), then no score of this profile's: j4 and j3, most recently moved first
	if got := jobIDs(byMatch.Columns[0]); got != "j2,j1,j4,j3" {
		t.Errorf("match order = %s", got)
	}
	if def, _ := GetBoard("p1"); jobIDs(def.Columns[0]) != "j4,j3,j2,j1" {
		t.Errorf("GetBoard must stay moved-first: %s", jobIDs(def.Columns[0]))
	}
	if empty, _ := GetBoardSorted("p1", ""); jobIDs(empty.Columns[0]) != "j4,j3,j2,j1" {
		t.Errorf("an empty sort is moved-first: %s", jobIDs(empty.Columns[0]))
	}
}

func TestACardShowsOnlyAScoreMadeForThisBoardsProfile(t *testing.T) {
	sortFixture(t)
	b1, _ := GetBoardSorted("p1", SortMoved)
	if got := scoresOf(b1.Columns[0]); got != "j4=-,j3=-,j2=80,j1=60" {
		t.Errorf("p1's cards = %s (j3's 99 belongs to p2, j4 is still matching)", got)
	}
	b2, _ := GetBoardSorted("p2", SortMatch)
	if got := scoresOf(b2.Columns[0]); got != "j3=99" {
		t.Errorf("p2's cards = %s", got)
	}
}

func TestEqualScoresKeepTheMostRecentlyMovedFirst(t *testing.T) {
	sortFixture(t)
	if _, err := db.JobsDB.Exec(`UPDATE job_matches SET score = 80 WHERE job_id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	b, _ := GetBoardSorted("p1", SortMatch)
	// j1 and j2 both 80: j2 was moved later, so it comes first
	if got := jobIDs(b.Columns[0]); got != "j2,j1,j4,j3" {
		t.Errorf("order = %s", got)
	}
	mustSet(t, "p1", "j1", NotReviewedID) // moved again: now the most recent
	b, _ = GetBoardSorted("p1", SortMatch)
	if got := jobIDs(b.Columns[0]); got != "j1,j2,j4,j3" {
		t.Errorf("order after moving j1 = %s", got)
	}
}

func TestAnUnknownBoardSortIsRefused(t *testing.T) {
	sortFixture(t)
	for _, bad := range []string{"newest", "MATCH", "match;DROP TABLE job_board_entries"} {
		if _, err := GetBoardSorted("p1", bad); !errors.Is(err, ErrInvalidSort) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestAddEntryIfAbsentAddsOnlyWhenTheJobIsNotOnTheBoard(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	added, err := AddEntryIfAbsent("p1", "j1", AppliedID)
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	var status string
	if err := db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = 'p1' AND job_id = 'j1'`).Scan(&status); err != nil || status != AppliedID {
		t.Errorf("status=%q err=%v", status, err)
	}
	// the same job on another profile's board is a separate entry
	if added, err := AddEntryIfAbsent("p2", "j1", NotReviewedID); err != nil || !added {
		t.Errorf("other profile: added=%v err=%v", added, err)
	}
}

// A card already on the board keeps its status and its moved-at time: nothing a person did is undone.
func TestAddEntryIfAbsentNeverMovesAnExistingCard(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	custom := mustStatus(t, "p1", "Mine")
	mustSet(t, "p1", "j1", custom.ID)
	var before string
	if err := db.JobsDB.QueryRow(`SELECT updated_at FROM job_board_entries WHERE profile_id = 'p1' AND job_id = 'j1'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	added, err := AddEntryIfAbsent("p1", "j1", DeclinedID)
	if err != nil || added {
		t.Fatalf("added=%v err=%v, want false and no error", added, err)
	}
	var status, after string
	if err := db.JobsDB.QueryRow(`SELECT status_id, updated_at FROM job_board_entries WHERE profile_id = 'p1' AND job_id = 'j1'`).Scan(&status, &after); err != nil {
		t.Fatal(err)
	}
	if status != custom.ID || after != before {
		t.Errorf("the card moved: status=%q updated_at %q -> %q", status, before, after)
	}
}

func TestAddEntryIfAbsentChecksLikeSetEntry(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	others := mustStatus(t, "p2", "Theirs")
	if _, err := AddEntryIfAbsent("p1", "nope", AppliedID); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("unknown job = %v", err)
	}
	if _, err := AddEntryIfAbsent("p1", "j1", "no-such"); !errors.Is(err, ErrUnknownStatus) {
		t.Errorf("unknown status = %v", err)
	}
	if _, err := AddEntryIfAbsent("p1", "j1", others.ID); !errors.Is(err, ErrStatusNotOnBoard) {
		t.Errorf("another profile's status = %v", err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries`).Scan(&n)
	if n != 0 {
		t.Errorf("entries = %d, want none after refusals", n)
	}
}

func TestIsOnBoardIsPerProfileAndAnyStatus(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	addJob(t, "j2", "Job two")
	custom := mustStatus(t, "p1", "Mine")
	mustSet(t, "p1", "j1", custom.ID)
	mustSet(t, "p2", "j2", AppliedID)
	for _, c := range []struct {
		profile, job string
		want         bool
	}{
		{"p1", "j1", true},  // a custom status
		{"p2", "j2", true},  // a fixed one
		{"p2", "j1", false}, // the job is on another profile's board only
		{"p1", "j2", false},
		{"p3", "j1", false}, // a profile without a board
		{"p1", "nope", false},
	} {
		got, err := IsOnBoard(c.profile, c.job)
		if err != nil || got != c.want {
			t.Errorf("IsOnBoard(%s, %s) = %v (%v), want %v", c.profile, c.job, got, err, c.want)
		}
	}
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := IsOnBoard("p1", "j1"); got {
		t.Error("a card taken off the board is not on it")
	}
}

// ---- the memory of removals (step 144) ----

func removals(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_removals`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wasRemoved(t *testing.T, profileID, jobID string) bool {
	t.Helper()
	got, err := WasRemoved(profileID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestARealRemovalIsRemembered(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	addJob(t, "j2", "Job two")
	mustSet(t, "p1", "j1", NotReviewedID)
	mustSet(t, "p1", "j2", AppliedID)
	if wasRemoved(t, "p1", "j1") {
		t.Fatal("nothing is remembered before a removal")
	}
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	if !wasRemoved(t, "p1", "j1") {
		t.Error("a removed card must be remembered")
	}
	if wasRemoved(t, "p1", "j2") {
		t.Error("a card that stays must not be remembered")
	}
	if on, _ := IsOnBoard("p1", "j1"); on {
		t.Error("the card must be off the board")
	}
	if removals(t) != 1 {
		t.Errorf("removals = %d, want 1", removals(t))
	}
}

// Removing something that is not on the board remembers nothing: it was not a decision about a card.
func TestRemovingACardThatIsNotThereRemembersNothing(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p2", "j1", NotReviewedID) // on another profile's board only
	for _, c := range [][2]string{{"p1", "j1"}, {"p1", "nope"}, {"ghost", "j1"}} {
		if err := RemoveEntry(c[0], c[1]); err != nil {
			t.Errorf("RemoveEntry(%s, %s): %v", c[0], c[1], err)
		}
	}
	if removals(t) != 0 {
		t.Errorf("removals = %d, want none", removals(t))
	}
	if on, _ := IsOnBoard("p2", "j1"); !on {
		t.Error("the other profile's card must stay")
	}
}

func TestOneProfilesRemovalSaysNothingAboutAnother(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", NotReviewedID)
	mustSet(t, "p2", "j1", NotReviewedID)
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	if !wasRemoved(t, "p1", "j1") || wasRemoved(t, "p2", "j1") {
		t.Error("only p1 removed the card")
	}
	if on, _ := IsOnBoard("p2", "j1"); !on {
		t.Error("p2's card must stay")
	}
}

// Removing again (the card was put back some other way) refreshes the one row instead of adding a second.
func TestARepeatedRemovalKeepsOneRowAndRefreshesTheTime(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	if _, err := db.JobsDB.Exec(`INSERT INTO job_board_removals (profile_id, job_id, removed_at) VALUES ('p1', 'j1', '2020-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p1', 'j1', 'fixed-not-reviewed')`); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	var at string
	if err := db.JobsDB.QueryRow(`SELECT removed_at FROM job_board_removals WHERE profile_id = 'p1' AND job_id = 'j1'`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if removals(t) != 1 || at == "2020-01-01 00:00:00" {
		t.Errorf("rows = %d, removed_at = %s", removals(t), at)
	}
}

// Writing the memory and removing the card are one transaction: when the memory cannot be written the card stays.
func TestAFailedMemoryWriteKeepsTheCardOnTheBoard(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", NotReviewedID)
	if _, err := db.JobsDB.Exec(`DROP TABLE job_board_removals`); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEntry("p1", "j1"); err == nil {
		t.Fatal("a removal whose memory cannot be written must fail")
	}
	if on, _ := IsOnBoard("p1", "j1"); !on {
		t.Error("the card must still be on the board: the removal was rolled back")
	}
}

// A person putting the job on the board again, or moving it, forgets the removal.
func TestAPersonPuttingTheJobBackForgetsTheRemoval(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", NotReviewedID)
	if err := RemoveEntry("p1", "j1"); err != nil {
		t.Fatal(err)
	}
	mustSet(t, "p1", "j1", AppliedID)
	if wasRemoved(t, "p1", "j1") {
		t.Error("putting the job back must forget the removal")
	}
	if on, _ := IsOnBoard("p1", "j1"); !on {
		t.Error("the job must be on the board")
	}
	// moving a card that was never removed changes nothing in the memory
	mustSet(t, "p1", "j1", DeclinedID)
	if removals(t) != 0 {
		t.Errorf("removals = %d", removals(t))
	}
}

func TestOnlyThatProfilesRemovalIsForgottenWhenItPutsTheJobBack(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", NotReviewedID)
	mustSet(t, "p2", "j1", NotReviewedID)
	_ = RemoveEntry("p1", "j1")
	_ = RemoveEntry("p2", "j1")
	mustSet(t, "p1", "j1", NotReviewedID)
	if wasRemoved(t, "p1", "j1") || !wasRemoved(t, "p2", "j1") {
		t.Error("p1 put the job back, p2 did not")
	}
}

// A put-back that fails (unknown status, a foreign status, an unknown job) must not forget anything.
func TestAFailedPutBackKeepsTheRemoval(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	others := mustStatus(t, "p2", "Theirs")
	mustSet(t, "p1", "j1", NotReviewedID)
	_ = RemoveEntry("p1", "j1")
	if _, err := SetEntry("p1", "j1", "no-such-status"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := SetEntry("p1", "j1", others.ID); err == nil {
		t.Fatal("expected an error")
	}
	if !wasRemoved(t, "p1", "j1") {
		t.Error("a failed put-back must not forget the removal")
	}
	if on, _ := IsOnBoard("p1", "j1"); on {
		t.Error("the job must still be off the board")
	}
}

// Deleting the job (the cascade) and deleting the profile both clear the memory.
func TestADeletedJobAndADeletedProfileClearTheMemory(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	addJob(t, "j2", "Job two")
	for _, p := range []string{"p1", "p2"} {
		for _, j := range []string{"j1", "j2"} {
			mustSet(t, p, j, NotReviewedID)
			_ = RemoveEntry(p, j)
		}
	}
	if removals(t) != 4 {
		t.Fatalf("removals = %d, want 4", removals(t))
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	if removals(t) != 2 || wasRemoved(t, "p1", "j1") || wasRemoved(t, "p2", "j1") {
		t.Errorf("after the job was deleted: %d removals", removals(t))
	}
	if err := PurgeProfile("p1"); err != nil {
		t.Fatal(err)
	}
	if wasRemoved(t, "p1", "j2") || !wasRemoved(t, "p2", "j2") || removals(t) != 1 {
		t.Errorf("after p1 was purged: %d removals (p2's must stay)", removals(t))
	}
}

// A card that leaves a board because its status was deleted does not leave: it moves, and nothing is remembered.
func TestDeletingAStatusRemembersNothing(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	custom := mustStatus(t, "p1", "Mine")
	mustSet(t, "p1", "j1", custom.ID)
	if err := DeleteStatus(custom.ID); err != nil {
		t.Fatal(err)
	}
	if removals(t) != 0 {
		t.Errorf("removals = %d, want none", removals(t))
	}
	if on, _ := IsOnBoard("p1", "j1"); !on {
		t.Error("the card must have moved to Not Reviewed, not left")
	}
}

// ---- the three outcomes of an automatic add ----

func TestAnAutomaticAddHasThreeOutcomes(t *testing.T) {
	setup(t)
	addJob(t, "fresh", "Never on the board")
	addJob(t, "there", "On the board")
	addJob(t, "gone", "Taken off")
	mustSet(t, "p1", "there", AppliedID)
	mustSet(t, "p1", "gone", AppliedID)
	_ = RemoveEntry("p1", "gone")

	for job, want := range map[string]AddOutcome{"fresh": Added, "there": OnBoard, "gone": RemovedByPerson} {
		got, err := AddEntryUnlessRemoved("p1", job, NotReviewedID)
		if err != nil || got != want {
			t.Errorf("%s: %q (%v), want %q", job, got, err, want)
		}
	}
	if on, _ := IsOnBoard("p1", "fresh"); !on {
		t.Error("the fresh job must be on the board")
	}
	if on, _ := IsOnBoard("p1", "gone"); on {
		t.Error("a removed job must stay off the board")
	}
	// a card that is on the board keeps its status and its moved-at time
	var status string
	_ = db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = 'p1' AND job_id = 'there'`).Scan(&status)
	if status != AppliedID {
		t.Errorf("a card already on the board moved to %q", status)
	}
	// the memory is not touched by an automatic add
	if !wasRemoved(t, "p1", "gone") {
		t.Error("an automatic add must not forget a removal")
	}
}

func TestARemovalOfOneProfileDoesNotStopAnotherProfilesAdd(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", NotReviewedID)
	_ = RemoveEntry("p1", "j1")
	got, err := AddEntryUnlessRemoved("p2", "j1", NotReviewedID)
	if err != nil || got != Added {
		t.Errorf("p2: %q (%v), want added", got, err)
	}
}

func TestAnAutomaticAddKeepsItsChecks(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	others := mustStatus(t, "p2", "Theirs")
	if _, err := AddEntryUnlessRemoved("p1", "nope", NotReviewedID); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("unknown job: %v", err)
	}
	if _, err := AddEntryUnlessRemoved("p1", "j1", "no-such"); !errors.Is(err, ErrUnknownStatus) {
		t.Errorf("unknown status: %v", err)
	}
	if _, err := AddEntryUnlessRemoved("p1", "j1", others.ID); !errors.Is(err, ErrStatusNotOnBoard) {
		t.Errorf("another profile's status: %v", err)
	}
	if on, _ := IsOnBoard("p1", "j1"); on {
		t.Error("a refused add wrote something")
	}
}

// The boolean form (used until its callers distinguish the outcomes) is true only for a real add.
func TestAddEntryIfAbsentIsTrueOnlyForARealAdd(t *testing.T) {
	setup(t)
	addJob(t, "fresh", "F")
	addJob(t, "there", "T")
	addJob(t, "gone", "G")
	mustSet(t, "p1", "there", AppliedID)
	mustSet(t, "p1", "gone", AppliedID)
	_ = RemoveEntry("p1", "gone")
	for job, want := range map[string]bool{"fresh": true, "there": false, "gone": false} {
		got, err := AddEntryIfAbsent("p1", job, NotReviewedID)
		if err != nil || got != want {
			t.Errorf("%s: %v (%v), want %v", job, got, err, want)
		}
	}
}

func TestWasRemovedOfAnUnknownJobOrProfileIsFalse(t *testing.T) {
	setup(t)
	if wasRemoved(t, "nobody", "nothing") {
		t.Error("nothing is remembered about unknown ids")
	}
}

func TestTheRemovalTableRefusesAnUnknownJobByItself(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO job_board_removals (profile_id, job_id) VALUES ('p1', 'no-such-job')`); err == nil {
		t.Error("the foreign key must refuse an unknown job")
	}
	addJob(t, "j1", "Job one")
	for i := 0; i < 2; i++ {
		_, err := db.JobsDB.Exec(`INSERT INTO job_board_removals (profile_id, job_id) VALUES ('p1', 'j1')`)
		if (i == 0) != (err == nil) {
			t.Errorf("insert %d: err=%v (the primary key must refuse the second)", i, err)
		}
	}
}

// A card on the board wins over a stale memory row (it should not happen, but a card that is there is on the board).
func TestACardOnTheBoardIsOnTheBoardEvenWithAStaleMemoryRow(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	mustSet(t, "p1", "j1", AppliedID)
	if _, err := db.JobsDB.Exec(`INSERT INTO job_board_removals (profile_id, job_id) VALUES ('p1', 'j1')`); err != nil {
		t.Fatal(err)
	}
	got, err := AddEntryUnlessRemoved("p1", "j1", NotReviewedID)
	if err != nil || got != OnBoard {
		t.Errorf("%q (%v), want on_board", got, err)
	}
}

// A memory that cannot be read is an error, never "nothing remembered": the job is not added blindly.
func TestAMemoryThatCannotBeReadStopsTheAdd(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job one")
	if _, err := db.JobsDB.Exec(`DROP TABLE job_board_removals`); err != nil {
		t.Fatal(err)
	}
	if got, err := AddEntryUnlessRemoved("p1", "j1", NotReviewedID); err == nil {
		t.Errorf("outcome %q, want an error", got)
	}
	if on, _ := IsOnBoard("p1", "j1"); on {
		t.Error("the job must not have been added")
	}
	if _, err := WasRemoved("p1", "j1"); err == nil {
		t.Error("WasRemoved must report a memory it cannot read")
	}
}

// ---- AddEntry with a choice about removals ----

func TestAddEntryWithIncludeRemovedAddsARemovedJobAndForgetsTheRemoval(t *testing.T) {
	setup(t)
	addJob(t, "gone", "Taken off")
	addJob(t, "there", "On the board")
	addJob(t, "fresh", "Never on the board")
	mustSet(t, "p1", "gone", AppliedID)
	mustSet(t, "p1", "there", AppliedID)
	_ = RemoveEntry("p1", "gone")

	for job, want := range map[string]AddOutcome{"gone": Added, "there": OnBoard, "fresh": Added} {
		got, err := AddEntry("p1", job, NotReviewedID, true)
		if err != nil || got != want {
			t.Errorf("%s: %q (%v), want %q", job, got, err, want)
		}
	}
	if on, _ := IsOnBoard("p1", "gone"); !on {
		t.Error("the removed job must be on the board when the person asked for it")
	}
	if wasRemoved(t, "p1", "gone") {
		t.Error("adding a removed job on request must forget the removal")
	}
	var status string
	_ = db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = 'p1' AND job_id = 'there'`).Scan(&status)
	if status != AppliedID {
		t.Errorf("a card that was on the board moved to %q", status)
	}
}

func TestAddEntryWithoutIncludeRemovedIsAddEntryUnlessRemoved(t *testing.T) {
	setup(t)
	addJob(t, "gone", "Taken off")
	mustSet(t, "p1", "gone", AppliedID)
	_ = RemoveEntry("p1", "gone")
	got, err := AddEntry("p1", "gone", NotReviewedID, false)
	if err != nil || got != RemovedByPerson || !wasRemoved(t, "p1", "gone") {
		t.Errorf("%q (%v), memory kept: %v", got, err, wasRemoved(t, "p1", "gone"))
	}
}

// Including a removed job only forgets THIS profile's removal.
func TestIncludingARemovedJobForgetsOnlyThatProfilesRemoval(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job")
	for _, p := range []string{"p1", "p2"} {
		mustSet(t, p, "j1", AppliedID)
		_ = RemoveEntry(p, "j1")
	}
	if got, err := AddEntry("p1", "j1", NotReviewedID, true); err != nil || got != Added {
		t.Fatalf("%q %v", got, err)
	}
	if wasRemoved(t, "p1", "j1") || !wasRemoved(t, "p2", "j1") {
		t.Error("p1's removal is forgotten, p2's stays")
	}
}

// A refused include (unknown status) must leave the removal alone.
func TestARefusedIncludeKeepsTheRemoval(t *testing.T) {
	setup(t)
	addJob(t, "j1", "Job")
	mustSet(t, "p1", "j1", AppliedID)
	_ = RemoveEntry("p1", "j1")
	if _, err := AddEntry("p1", "j1", "no-such-status", true); !errors.Is(err, ErrUnknownStatus) {
		t.Fatalf("err = %v", err)
	}
	if !wasRemoved(t, "p1", "j1") {
		t.Error("a refused include must not forget the removal")
	}
}
