package jobs

import (
	"errors"
	"testing"

	"career-tool-backend/db"
)

func byURLSetup(t *testing.T) string {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	for _, q := range []string{
		`INSERT INTO portals (id, name) VALUES ('p1', 'Board')`,
		`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://board.example/jobs')`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return "l1"
}

// SavePortalJobWith keeps merging a second job of the same title and company; SavePortalJobByURL identifies a job by its address alone.
func TestSavePortalJobByURLDoesNotMergeJobsOfTheSameTitle(t *testing.T) {
	link := byURLSetup(t)

	// the existing path: same title and (empty) company = the second one is a duplicate, not a new job
	id1, created, dup, err := SavePortalJobWith(NewCriteria(), link, "https://board.example/j/1", "Software Engineer", "", "", "", "")
	if err != nil || !created || dup {
		t.Fatalf("first: %v %v %v", created, dup, err)
	}
	id2, created, dup, err := SavePortalJobWith(NewCriteria(), link, "https://board.example/j/2", "Software Engineer", "", "", "", "")
	if err != nil || created || !dup || id2 != id1 {
		t.Fatalf("the existing check must stay as it is: created=%v dup=%v id2=%s id1=%s err=%v", created, dup, id2, id1, err)
	}

	// by address: a different address with the same title is a new job
	id3, created, dup, err := SavePortalJobByURL(NewCriteria(), link, "https://board.example/j/3", "Software Engineer", "", "", "", "")
	if err != nil || !created || dup || id3 == id1 {
		t.Fatalf("by address: created=%v dup=%v err=%v", created, dup, err)
	}
	// the same address again is a refresh of that job, not a new one
	id4, created, dup, err := SavePortalJobByURL(NewCriteria(), link, "https://board.example/j/3", "Software Engineer", "", "", "", "")
	if err != nil || created || dup || id4 != id3 {
		t.Fatalf("same address: created=%v dup=%v id=%s want %s err=%v", created, dup, id4, id3, err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil || n != 2 {
		t.Errorf("jobs: %d %v", n, err)
	}
}

func TestSavePortalJobByURLStillNeverInsertsADeletedAddressAndStillAsksTheCriteria(t *testing.T) {
	link := byURLSetup(t)
	id, created, _, err := SavePortalJobByURL(NewCriteria(), link, "https://board.example/j/9", "Designer", "", "", "", "")
	if err != nil || !created {
		t.Fatal(err)
	}
	if err := DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := SavePortalJobByURL(NewCriteria(), link, "https://board.example/j/9", "Designer", "", "", "", ""); err != ErrDeletedBefore {
		t.Errorf("a deleted address came back: %v", err)
	}
	// a criteria that refuses everything refuses a NEW job by address as well
	refuse := &Criteria{matchers: []CriteriaMatcher{refuseAll{}}}
	if _, _, _, err := SavePortalJobByURL(refuse, link, "https://board.example/j/10", "Cook", "", "", "", ""); !errors.Is(err, ErrFilteredOut) {
		t.Errorf("the crawler criteria were not asked: %v", err)
	}
}

type refuseAll struct{}

func (refuseAll) Name() string                     { return "refuse-all" }
func (refuseAll) Match(Candidate) (Verdict, error) { return Verdict{Keep: false, Reason: "no"}, nil }
