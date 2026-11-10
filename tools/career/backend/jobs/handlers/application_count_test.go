package jobshandlers

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/profile"
)

func countOf(t *testing.T) int {
	t.Helper()
	rec := handlertest.BoardCall(t, ApplicationCountHandler, "GET", "/application-count", "")
	handlertest.WantStatus(t, rec, 200)
	var out struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return out.Total
}

func TestNoProfilesAndNoApplicationsCountZero(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	rec := handlertest.BoardCall(t, ApplicationCountHandler, "GET", "/application-count", "")
	handlertest.WantStatus(t, rec, 200)
	if strings.TrimSpace(rec.Body.String()) != `{"total":0}` {
		t.Errorf("the exact answer: %s", rec.Body.String())
	}
	handlertest.BoardFixture(t)
	if n := countOf(t); n != 0 {
		t.Errorf("two profiles and two jobs, nothing applied: %d", n)
	}
}

func TestAnAppliedJobAndAJobWithAProtocolEntryEachCountOnce(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t) // jobs j1, j2
	handlertest.MustSet(t, p1, "j1", jobboard.AppliedID)
	if n := countOf(t); n != 1 {
		t.Fatalf("an Applied job: %d", n)
	}
	addEntry(t, p1, "j2", "sent", "2026-11-03")
	if n := countOf(t); n != 2 {
		t.Fatalf("plus a job with only a protocol entry: %d", n)
	}
	// the same job Applied and with entries is still one application
	addEntry(t, p1, "j1", "received", "2026-11-04")
	addEntry(t, p1, "j1", "note", "2026-11-05")
	if n := countOf(t); n != 2 {
		t.Errorf("a job that is Applied and has entries counts once: %d", n)
	}
}

func TestADeclinedJobIsNotAnApplicationButItsProtocolEntryMakesItOne(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	handlertest.MustSet(t, p1, "j1", jobboard.DeclinedID)
	handlertest.MustSet(t, p1, "j2", jobboard.NotReviewedID)
	if n := countOf(t); n != 0 {
		t.Fatalf("Declined and Not Reviewed are not applications: %d", n)
	}
	addEntry(t, p1, "j1", "received", "2026-11-04")
	if n := countOf(t); n != 1 {
		t.Errorf("an entry in the protocol makes the Declined job an application: %d", n)
	}
}

func TestEveryProfilesApplicationsAreCountedAndTheSameJobOnTwoProfilesCountsTwice(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	handlertest.MustSet(t, p1, "j1", jobboard.AppliedID)
	handlertest.MustSet(t, p2, "j1", jobboard.AppliedID)
	addEntry(t, p2, "j2", "sent", "2026-11-03")
	if n := countOf(t); n != 3 {
		t.Errorf("j1 for each profile and j2 for the second: %d", n)
	}
}

func TestTheCountIsTheSumOfTheApplicationsPageForEveryProfile(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	for _, id := range []string{"j3", "j4"} {
		insertJobRow(t, id)
	}
	handlertest.MustSet(t, p1, "j1", jobboard.AppliedID)
	handlertest.MustSet(t, p1, "j2", jobboard.DeclinedID)
	addEntry(t, p1, "j3", "sent", "2026-11-03")
	addEntry(t, p2, "j3", "note", "2099-01-01")
	handlertest.MustSet(t, p2, "j4", jobboard.AppliedID)
	a, _ := applicationsOf(t, p1)
	b, _ := applicationsOf(t, p2)
	if got, want := countOf(t), len(a)+len(b); got != want || want != 4 {
		t.Errorf("count %d, the page lists %d and %d", got, len(a), len(b))
	}
}

func TestADeletedJobAndADeletedProfileAreNotCounted(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	handlertest.MustSet(t, p1, "j1", jobboard.AppliedID)
	addEntry(t, p2, "j2", "sent", "2026-11-03")
	if n := countOf(t); n != 2 {
		t.Fatalf("%d", n)
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	if n := countOf(t); n != 1 {
		t.Errorf("a deleted job: %d", n)
	}
	if err := profile.DeleteProfile(p2); err != nil {
		t.Fatal(err)
	}
	if n := countOf(t); n != 0 {
		t.Errorf("a deleted profile: %d", n)
	}
}

func TestOnlyGETIsAllowedAndAnUnreadableDatabaseIsAPlain500(t *testing.T) {
	handlertest.BoardFixture(t)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if rec := handlertest.BoardCall(t, ApplicationCountHandler, m, "/application-count", ""); rec.Code != 405 {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	if _, err := db.CareerDB.Exec(`DROP TABLE profiles`); err != nil {
		t.Fatal(err)
	}
	rec := handlertest.BoardCall(t, ApplicationCountHandler, "GET", "/application-count", "")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "no such table") || !strings.Contains(rec.Body.String(), "failed to count the applications") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
