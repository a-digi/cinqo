package deletedjobs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func call(method, target string) (int, string) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest(method, target, strings.NewReader("")))
	return rec.Code, rec.Body.String()
}

type listBody struct {
	Available bool   `json:"available"`
	Total     int    `json:"total"`
	Items     []Item `json:"items"`
}

func list(t *testing.T, target string) listBody {
	t.Helper()
	code, body := call(http.MethodGet, target)
	if code != 200 {
		t.Fatalf("GET %s: %d %s", target, code, body)
	}
	var l listBody
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func seed(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		at(t, fmt.Sprintf("2026-06-%02d 10:00:00", i+1))
		e := entry(fmt.Sprintf("https://x.example/jobs/%d", i))
		e.Title = fmt.Sprintf("Job %d", i)
		if i%2 == 0 {
			e.Reason = ReasonLowMatch
			s := 10 + i
			e.MatchScore = &s
		}
		if _, err := Remember(e); err != nil {
			t.Fatal(err)
		}
	}
}

// The newest deletion first, every field of the snapshot, the score only where there is one, the total of all.
func TestTheListIsNewestFirstWithTheWholeSnapshot(t *testing.T) {
	setup(t)
	seed(t, 3)
	l := list(t, "/jobs/deleted")
	if !l.Available || l.Total != 3 || len(l.Items) != 3 {
		t.Fatalf("%+v", l)
	}
	if l.Items[0].Title != "Job 2" || l.Items[1].Title != "Job 1" || l.Items[2].Title != "Job 0" {
		t.Errorf("order: %s, %s, %s", l.Items[0].Title, l.Items[1].Title, l.Items[2].Title)
	}
	first := l.Items[0]
	if first.ID == 0 || first.SourceURL != "https://x.example/jobs/2" || first.Company != "Acme" || first.Location != "Hamburg" || first.PortalName != "StepStone" ||
		first.PostedAt != "today" || first.Reason != "low_match" || first.DeletedAt != "2026-06-03 10:00:00" || first.MatchScore == nil || *first.MatchScore != 12 {
		t.Errorf("%+v", first)
	}
	if l.Items[1].MatchScore != nil || l.Items[1].Reason != "deleted" {
		t.Errorf("job 1: %+v", l.Items[1])
	}
}

func TestTheListLimit(t *testing.T) {
	setup(t)
	seed(t, 5)
	for target, want := range map[string]int{"/jobs/deleted?limit=2": 2, "/jobs/deleted?limit=1": 1, "/jobs/deleted?limit=0": 5, "/jobs/deleted?limit=999": 5, "/jobs/deleted": 5} {
		l := list(t, target)
		if len(l.Items) != want || l.Total != 5 {
			t.Errorf("%s: %d items, total %d, want %d of 5", target, len(l.Items), l.Total, want)
		}
	}
	if code, _ := call(http.MethodGet, "/jobs/deleted?limit=abc"); code != 400 {
		t.Errorf("limit=abc: %d", code)
	}
	if code, _ := call(http.MethodGet, "/jobs/deleted?limit=-1"); code != 400 {
		t.Errorf("limit=-1: %d", code)
	}
	if DefaultListLimit != 50 || MaxListLimit != 200 {
		t.Errorf("limits changed: %d, %d", DefaultListLimit, MaxListLimit)
	}
}

func TestTheListCapsAtTheMaximum(t *testing.T) {
	setup(t)
	tx, _ := db.DeletedJobsDB.Begin()
	for i := 0; i < 230; i++ {
		if _, err := tx.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, reason) VALUES (?, 1, 'u', 't', 'deleted')`, fmt.Sprintf("h%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tx.Commit()
	if l := list(t, "/jobs/deleted?limit=500"); len(l.Items) != 200 || l.Total != 230 {
		t.Errorf("%d items, total %d", len(l.Items), l.Total)
	}
	if l := list(t, "/jobs/deleted"); len(l.Items) != 50 {
		t.Errorf("default: %d", len(l.Items))
	}
}

func TestAnEmptyListIsAnEmptyArray(t *testing.T) {
	setup(t)
	_, body := call(http.MethodGet, "/jobs/deleted")
	if !strings.Contains(body, `"items":[]`) || !strings.Contains(body, `"available":true`) || !strings.Contains(body, `"total":0`) {
		t.Errorf("%s", body)
	}
}

func TestWithoutTheDatabaseTheListSaysSo(t *testing.T) {
	setup(t)
	saved := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	defer func() { db.DeletedJobsDB = saved }()
	code, body := call(http.MethodGet, "/jobs/deleted")
	if code != 200 || !strings.Contains(body, `"available":false`) || !strings.Contains(body, `"items":[]`) {
		t.Errorf("%d %s", code, body)
	}
	if code, _ := call(http.MethodDelete, "/jobs/deleted?all=true"); code != http.StatusServiceUnavailable {
		t.Errorf("delete all: %d", code)
	}
	if code, _ := call(http.MethodDelete, "/jobs/deleted?id=1"); code != http.StatusServiceUnavailable {
		t.Errorf("delete one: %d", code)
	}
}

// Allowing one job again removes only that row, and the next crawl is no longer turned away for it.
func TestAllowOneAgain(t *testing.T) {
	setup(t)
	seed(t, 3)
	l := list(t, "/jobs/deleted")
	target := l.Items[1] // Job 1
	if code, _ := call(http.MethodDelete, fmt.Sprintf("/jobs/deleted?id=%d", target.ID)); code != http.StatusNoContent {
		t.Fatalf("status %d", code)
	}
	after := list(t, "/jobs/deleted")
	if after.Total != 2 || len(after.Items) != 2 || after.Items[0].Title != "Job 2" || after.Items[1].Title != "Job 0" {
		t.Errorf("%+v", after.Items)
	}
	if has, _ := Has(target.SourceURL); has {
		t.Error("still remembered")
	}
	if has, _ := Has("https://x.example/jobs/2"); !has {
		t.Error("another job was released")
	}
	// the same id again, and an id that never existed, are 404
	if code, _ := call(http.MethodDelete, fmt.Sprintf("/jobs/deleted?id=%d", target.ID)); code != http.StatusNotFound {
		t.Errorf("again: %d", code)
	}
	if code, _ := call(http.MethodDelete, "/jobs/deleted?id=99999"); code != http.StatusNotFound {
		t.Errorf("unknown: %d", code)
	}
}

func TestAllowAllAgain(t *testing.T) {
	setup(t)
	seed(t, 4)
	code, body := call(http.MethodDelete, "/jobs/deleted?all=true")
	if code != 200 || !strings.Contains(body, `"released":4`) {
		t.Fatalf("%d %s", code, body)
	}
	if l := list(t, "/jobs/deleted"); l.Total != 0 || len(l.Items) != 0 {
		t.Errorf("%+v", l)
	}
	if _, body := call(http.MethodDelete, "/jobs/deleted?all=true"); !strings.Contains(body, `"released":0`) {
		t.Errorf("empty: %s", body)
	}
}

func TestDeleteNeedsExactlyOneOfIdOrAll(t *testing.T) {
	setup(t)
	seed(t, 2)
	for _, target := range []string{"/jobs/deleted", "/jobs/deleted?all=false", "/jobs/deleted?id=1&all=true", "/jobs/deleted?id=abc", "/jobs/deleted?id=0", "/jobs/deleted?id=-3", "/jobs/deleted?id=1.5", "/jobs/deleted?all=yes"} {
		if code, _ := call(http.MethodDelete, target); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", target, code)
		}
	}
	if l := list(t, "/jobs/deleted"); l.Total != 2 {
		t.Errorf("a refused delete removed something: %d left", l.Total)
	}
}

func TestOtherMethodsAreRefused(t *testing.T) {
	setup(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		if code, _ := call(m, "/jobs/deleted"); code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, code)
		}
	}
}

func TestListAndForgetFunctionsWithoutTheDatabase(t *testing.T) {
	setup(t)
	saved := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	defer func() { db.DeletedJobsDB = saved }()
	if _, _, err := List(10); err != ErrNoMemory {
		t.Errorf("List: %v", err)
	}
	if _, err := ForgetByID(1); err != ErrNoMemory {
		t.Errorf("ForgetByID: %v", err)
	}
	if _, err := ForgetAll(); err != ErrNoMemory {
		t.Errorf("ForgetAll: %v", err)
	}
}

// Jobs deleted in the same second come out in the opposite order of their ids (the later row first), so the list is
// stable and the newest deletion is always at the top.
func TestJobsDeletedInTheSameSecondComeNewestRowFirst(t *testing.T) {
	setup(t)
	at(t, "2026-06-30 12:00:00")
	for i := 0; i < 3; i++ {
		e := entry(fmt.Sprintf("https://x.example/same/%d", i))
		e.Title = fmt.Sprintf("Same %d", i)
		if _, err := Remember(e); err != nil {
			t.Fatal(err)
		}
	}
	l := list(t, "/jobs/deleted")
	if l.Items[0].Title != "Same 2" || l.Items[1].Title != "Same 1" || l.Items[2].Title != "Same 0" {
		t.Errorf("%s, %s, %s", l.Items[0].Title, l.Items[1].Title, l.Items[2].Title)
	}
}
