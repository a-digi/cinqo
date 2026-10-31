package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
)

func boardCall(t *testing.T, h http.HandlerFunc, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func boardFixture(t *testing.T) (p1, p2 string) {
	t.Helper()
	setupHandlerDB(t)
	var err error
	if p1, err = profile.CreateProfile("Ada", "Lovelace"); err != nil {
		t.Fatal(err)
	}
	if p2, err = profile.CreateProfile("Grace", "Hopper"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, ?)`, id, "https://x/"+id, "Job "+id); err != nil {
			t.Fatal(err)
		}
	}
	return p1, p2
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d (want %d): %s", rec.Code, want, rec.Body.String())
	}
}

func TestJobBoardRoutesEndToEnd(t *testing.T) {
	p1, p2 := boardFixture(t)

	// The board of a new profile: the four fixed columns.
	rec := boardCall(t, jobBoardHandler, "GET", "/job-board?profileId="+p1, "")
	wantStatus(t, rec, 200)
	var board jobboard.Board
	if err := json.Unmarshal(rec.Body.Bytes(), &board); err != nil || len(board.Columns) != 4 || board.Columns[0].Name != "Not Reviewed" {
		t.Fatalf("board = %s", rec.Body.String())
	}

	// A custom status.
	rec = boardCall(t, jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"`+p1+`","name":" Interview "}`)
	wantStatus(t, rec, 201)
	var st jobboard.Status
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Name != "Interview" || st.Kind != "custom" || st.ID == "" {
		t.Fatalf("status = %+v", st)
	}
	rec = boardCall(t, jobStatusesHandler, "PUT", "/job-statuses", `{"id":"`+st.ID+`","name":"Interviews"}`)
	wantStatus(t, rec, 200)

	// Add and move.
	wantStatus(t, boardCall(t, jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"`+p1+`","jobId":"j1","statusId":"`+jobboard.NotReviewedID+`"}`), 200)
	wantStatus(t, boardCall(t, jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"`+p1+`","jobId":"j1","statusId":"`+st.ID+`"}`), 200)
	wantStatus(t, boardCall(t, jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"`+p2+`","jobId":"j1","statusId":"`+jobboard.AppliedID+`"}`), 200)

	// Usage names both profiles and their different statuses.
	rec = boardCall(t, jobBoardUsageHandler, "GET", "/job-board-usage?jobId=j1", "")
	wantStatus(t, rec, 200)
	var usage struct {
		JobID    string `json:"jobId"`
		Profiles []struct {
			ProfileID  string `json:"profileId"`
			FirstName  string `json:"firstName"`
			LastName   string `json:"lastName"`
			StatusName string `json:"statusName"`
			Kind       string `json:"kind"`
		} `json:"profiles"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &usage)
	got := map[string]string{}
	for _, u := range usage.Profiles {
		got[u.FirstName+" "+u.LastName] = u.StatusName + "/" + u.Kind
	}
	if len(got) != 2 || got["Ada Lovelace"] != "Interviews/custom" || got["Grace Hopper"] != "Applied/applied" {
		t.Errorf("usage = %s", rec.Body.String())
	}

	// The jobs list and the single read carry the count.
	list := boardCall(t, jobsHandler, "GET", "/jobs", "")
	one := boardCall(t, jobsHandler, "GET", "/jobs?id=j1", "")
	if !strings.Contains(one.Body.String(), `"boardProfileCount":2`) {
		t.Errorf("single job: %s", one.Body.String())
	}
	var l struct {
		Jobs []struct {
			ID    string `json:"id"`
			Count int    `json:"boardProfileCount"`
		} `json:"jobs"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &l)
	counts := map[string]int{}
	for _, j := range l.Jobs {
		counts[j.ID] = j.Count
	}
	if len(counts) != 2 || counts["j1"] != 2 || counts["j2"] != 0 {
		t.Errorf("counts = %v (%s)", counts, list.Body.String())
	}

	// Remove from board; deleting the status moves the rest to Not Reviewed.
	wantStatus(t, boardCall(t, jobBoardEntriesHandler, "DELETE", "/job-board-entries?profileId="+p2+"&jobId=j1", ""), 204)
	wantStatus(t, boardCall(t, jobStatusesHandler, "DELETE", "/job-statuses?id="+st.ID, ""), 204)
	rec = boardCall(t, jobBoardHandler, "GET", "/job-board?profileId="+p1, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &board)
	if len(board.Columns) != 4 || len(board.Columns[0].Jobs) != 1 || board.Columns[0].Jobs[0].JobID != "j1" {
		t.Errorf("after delete: %s", rec.Body.String())
	}
}

func TestJobBoardRouteErrors(t *testing.T) {
	p1, p2 := boardFixture(t)
	mine := func() string {
		s, err := jobboard.CreateStatus(p1, "Interview")
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}()

	cases := []struct {
		name   string
		h      http.HandlerFunc
		method string
		url    string
		body   string
		want   int
	}{
		{"board without profile", jobBoardHandler, "GET", "/job-board", "", 400},
		{"board of unknown profile", jobBoardHandler, "GET", "/job-board?profileId=nope", "", 404},
		{"board wrong method", jobBoardHandler, "POST", "/job-board", "", 405},
		{"status for unknown profile", jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"nope","name":"X"}`, 404},
		{"status without profile", jobStatusesHandler, "POST", "/job-statuses", `{"name":"X"}`, 400},
		{"status blank name", jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"` + p1 + `","name":"  "}`, 400},
		{"status fixed name", jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"` + p1 + `","name":"applied"}`, 400},
		{"status duplicate", jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"` + p1 + `","name":"INTERVIEW"}`, 400},
		{"status bad json", jobStatusesHandler, "POST", "/job-statuses", `{`, 400},
		{"rename fixed", jobStatusesHandler, "PUT", "/job-statuses", `{"id":"` + jobboard.AppliedID + `","name":"X"}`, 400},
		{"rename unknown", jobStatusesHandler, "PUT", "/job-statuses", `{"id":"nope","name":"X"}`, 404},
		{"rename without id", jobStatusesHandler, "PUT", "/job-statuses", `{"name":"X"}`, 400},
		{"delete fixed", jobStatusesHandler, "DELETE", "/job-statuses?id=" + jobboard.DeclinedID, "", 400},
		{"delete unknown is a no-op", jobStatusesHandler, "DELETE", "/job-statuses?id=nope", "", 204},
		{"delete without id", jobStatusesHandler, "DELETE", "/job-statuses", "", 400},
		{"statuses wrong method", jobStatusesHandler, "PATCH", "/job-statuses", "", 405},
		{"entry other profile's status", jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"` + p2 + `","jobId":"j1","statusId":"` + mine + `"}`, 400},
		{"entry unknown job", jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"` + p1 + `","jobId":"nope","statusId":"` + mine + `"}`, 404},
		{"entry unknown status", jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"` + p1 + `","jobId":"j1","statusId":"nope"}`, 404},
		{"entry unknown profile", jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"nope","jobId":"j1","statusId":"` + jobboard.AppliedID + `"}`, 404},
		{"entry missing field", jobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"` + p1 + `","jobId":"j1"}`, 400},
		{"entry remove missing params", jobBoardEntriesHandler, "DELETE", "/job-board-entries?profileId=" + p1, "", 400},
		{"entry remove absent is a no-op", jobBoardEntriesHandler, "DELETE", "/job-board-entries?profileId=" + p1 + "&jobId=j1", "", 204},
		{"entries wrong method", jobBoardEntriesHandler, "GET", "/job-board-entries", "", 405},
		{"usage without job", jobBoardUsageHandler, "GET", "/job-board-usage", "", 400},
		{"usage wrong method", jobBoardUsageHandler, "POST", "/job-board-usage?jobId=j1", "", 405},
	}
	for _, c := range cases {
		if rec := boardCall(t, c.h, c.method, c.url, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
	}

	// A job with no board: an empty list, never null.
	rec := boardCall(t, jobBoardUsageHandler, "GET", "/job-board-usage?jobId=j2", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"profiles":[]`) {
		t.Errorf("empty usage: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCustomStatusLimitIsA409(t *testing.T) {
	p1, _ := boardFixture(t)
	for i := 0; i < jobboard.MaxCustomStatuses; i++ {
		name := string(rune('A' + i))
		wantStatus(t, boardCall(t, jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"`+p1+`","name":"`+name+`"}`), 201)
	}
	wantStatus(t, boardCall(t, jobStatusesHandler, "POST", "/job-statuses", `{"profileId":"`+p1+`","name":"Extra"}`), 409)
}

func TestDeletingAProfileRemovesItsBoardButNotOthers(t *testing.T) {
	p1, p2 := boardFixture(t)
	s, _ := jobboard.CreateStatus(p1, "Interview")
	_, _ = jobboard.SetEntry(p1, "j1", s.ID)
	_, _ = jobboard.SetEntry(p2, "j1", jobboard.AppliedID)

	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries WHERE profile_id = ?`, p1).Scan(&n)
	var m int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_statuses WHERE profile_id = ?`, p1).Scan(&m)
	if n != 0 || m != 0 {
		t.Errorf("%d entries and %d statuses of the deleted profile are left", n, m)
	}
	rec := boardCall(t, jobBoardUsageHandler, "GET", "/job-board-usage?jobId=j1", "")
	if strings.Contains(rec.Body.String(), "Ada") || !strings.Contains(rec.Body.String(), "Grace") {
		t.Errorf("usage after the delete: %s", rec.Body.String())
	}
	wantStatus(t, boardCall(t, jobBoardHandler, "GET", "/job-board?profileId="+p1, ""), 404)
	wantStatus(t, boardCall(t, jobBoardHandler, "GET", "/job-board?profileId="+p2, ""), 200)
}

func TestUsageSkipsAProfileThatNoLongerExists(t *testing.T) {
	p1, _ := boardFixture(t)
	_, _ = jobboard.SetEntry(p1, "j1", jobboard.AppliedID)
	// A leftover entry (the purge is best effort): delete only the career.db row.
	if _, err := db.CareerDB.Exec(`DELETE FROM profiles WHERE id = ?`, p1); err != nil {
		t.Fatal(err)
	}
	rec := boardCall(t, jobBoardUsageHandler, "GET", "/job-board-usage?jobId=j1", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"profiles":[]`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestListStatusesAnswersFixedThenEveryProfilesOwn(t *testing.T) {
	p1, p2 := boardFixture(t)
	a, _ := jobboard.CreateStatus(p1, "Interview")
	b, _ := jobboard.CreateStatus(p2, "Offer")
	c, _ := jobboard.CreateStatus(p1, "Phone screen")

	rec := boardCall(t, jobStatusesHandler, "GET", "/job-statuses", "")
	wantStatus(t, rec, 200)
	var resp struct {
		Statuses []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			ProfileID string `json:"profileId"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range resp.Statuses {
		got = append(got, s.Name+"/"+s.Kind+"/"+s.ProfileID)
	}
	want := []string{
		"Not Reviewed/not_reviewed/", "Declined/declined/", "Stopped application/stopped/", "Applied/applied/",
		"Interview/custom/" + p1, "Offer/custom/" + p2, "Phone screen/custom/" + p1,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("statuses = %v\nwant     %v", got, want)
	}
	_ = a
	_ = b
	_ = c
}
