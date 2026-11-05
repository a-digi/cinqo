package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"career-tool-backend/applicationlog"
	"career-tool-backend/db"
	"career-tool-backend/jobboard"
)

type appRow struct {
	JobID           string `json:"jobId"`
	Title           string `json:"title"`
	Status          *struct{ Name, Kind string }
	Protocol        *applicationlog.Summary `json:"protocol"`
	ActivityAt      string                  `json:"activityAt"`
	WaitingForReply bool                    `json:"waitingForReply"`
}

func applicationsOf(t *testing.T, profileID string) ([]appRow, string) {
	t.Helper()
	rec := boardCall(t, jobApplicationsHandler, "GET", "/job-applications?profileId="+profileID, "")
	wantStatus(t, rec, 200)
	var out struct {
		Applications []appRow `json:"applications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return out.Applications, rec.Body.String()
}

func insertJobRow(t *testing.T, id string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, ?)`, id, "https://x/"+id, "Job "+id); err != nil {
		t.Fatal(err)
	}
}

func moveAt(t *testing.T, profileID, jobID, at string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`UPDATE job_board_entries SET updated_at = ? WHERE profile_id = ? AND job_id = ?`, at, profileID, jobID); err != nil {
		t.Fatal(err)
	}
}

func addEntry(t *testing.T, p, j, dir, at string) {
	t.Helper()
	if _, err := applicationlog.Add(p, j, applicationlog.Input{Direction: dir, Channel: "email", OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
}

func TestTheApplicationsOverviewListsAppliedJobsAndJobsWithAProtocolMostRecentFirst(t *testing.T) {
	p1, p2 := boardFixture(t) // jobs j1, j2
	for _, id := range []string{"j3", "j4", "j5", "j6", "j7"} {
		insertJobRow(t, id)
	}
	// j1: Applied, moved 1 Nov, a message sent 3 Nov -> activity 3 Nov, waiting for a reply.
	mustSet(t, p1, "j1", jobboard.AppliedID)
	moveAt(t, p1, "j1", "2026-11-01T08:00:00.000Z")
	addEntry(t, p1, "j1", "sent", "2026-11-03T10:00:00Z")
	// j2: Not Reviewed on the board but has a protocol: they wrote 4 Nov, a call is planned in 2099.
	mustSet(t, p1, "j2", jobboard.NotReviewedID)
	moveAt(t, p1, "j2", "2026-10-01T08:00:00.000Z")
	addEntry(t, p1, "j2", "received", "2026-11-04")
	addEntry(t, p1, "j2", "note", "2099-01-01T10:00:00Z")
	// j3: Applied only, moved 2 Nov (later than nothing): no protocol, not waiting.
	mustSet(t, p1, "j3", jobboard.AppliedID)
	moveAt(t, p1, "j3", "2026-11-02T08:00:00.000Z")
	// j4: protocol only, not on the board, a planned entry only: no activity at all.
	addEntry(t, p1, "j4", "note", "2099-02-02")
	// j5: Declined with no protocol: not an application.
	mustSet(t, p1, "j5", jobboard.DeclinedID)
	// j6: Applied on the OTHER profile's board only: not p1's application.
	mustSet(t, p2, "j6", jobboard.AppliedID)
	// j7: nothing for p1, but the OTHER profile has an entry on it: still not p1's application.
	addEntry(t, p2, "j7", "sent", "2026-11-04")

	got, raw := applicationsOf(t, p1)
	order := []string{}
	byID := map[string]appRow{}
	for _, a := range got {
		order = append(order, a.JobID)
		byID[a.JobID] = a
	}
	if want := "j2,j1,j3,j4"; strings.Join(order, ",") != want {
		t.Fatalf("order %v, want %s", order, want)
	}
	check := func(id, activity string, waiting bool, status string, count, planned int) {
		t.Helper()
		a := byID[id]
		gotStatus := ""
		if a.Status != nil {
			gotStatus = a.Status.Kind
		}
		gotCount, gotPlanned := 0, 0
		if a.Protocol != nil {
			gotCount, gotPlanned = a.Protocol.Count, a.Protocol.Planned
		}
		if a.ActivityAt != activity || a.WaitingForReply != waiting || gotStatus != status || gotCount != count || gotPlanned != planned {
			t.Errorf("%s: got activity %q waiting %v status %q count %d planned %d", id, a.ActivityAt, a.WaitingForReply, gotStatus, gotCount, gotPlanned)
		}
	}
	check("j1", "2026-11-03T10:00:00Z", true, "applied", 1, 0)
	check("j2", "2026-11-04T00:00:00Z", false, "not_reviewed", 2, 1)
	check("j3", "2026-11-02T08:00:00Z", false, "applied", 0, 0)
	check("j4", "", false, "", 1, 1)
	if n := strings.Count(raw, `"status"`); n != 3 {
		t.Errorf("only the jobs on the board carry a status (3), got %d: %s", n, raw)
	}
	if byID["j3"].Protocol != nil || strings.Count(raw, `"protocol"`) != 3 {
		t.Errorf("a job without entries must carry no protocol: %s", raw)
	}
}

func TestMovingTheCardAfterTheLastEntryMakesItTheLatestActivityButNotTheReply(t *testing.T) {
	p1, _ := boardFixture(t)
	mustSet(t, p1, "j1", jobboard.AppliedID)
	addEntry(t, p1, "j1", "sent", "2026-11-03")
	moveAt(t, p1, "j1", "2026-11-09T12:00:00.500Z")
	got, _ := applicationsOf(t, p1)
	if len(got) != 1 || got[0].ActivityAt != "2026-11-09T12:00:00Z" || !got[0].WaitingForReply {
		t.Errorf("%+v", got)
	}
}

func TestTheApplicationsOverviewRefusalsAndEmptyAnswer(t *testing.T) {
	p1, _ := boardFixture(t)
	if rec := boardCall(t, jobApplicationsHandler, "GET", "/job-applications", ""); rec.Code != 400 {
		t.Errorf("no profile: %d", rec.Code)
	}
	if rec := boardCall(t, jobApplicationsHandler, "GET", "/job-applications?profileId=nobody", ""); rec.Code != 404 && rec.Code != 400 {
		t.Errorf("unknown profile: %d", rec.Code)
	}
	if rec := boardCall(t, jobApplicationsHandler, "POST", "/job-applications?profileId="+p1, ""); rec.Code != 405 {
		t.Errorf("POST: %d", rec.Code)
	}
	rec := boardCall(t, jobApplicationsHandler, "GET", "/job-applications?profileId="+p1, "")
	wantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"applications":[]`) {
		t.Errorf("an empty overview must be a list, not null: %s", rec.Body.String())
	}
}

func TestManifestDeclaresTheApplicationsOverviewRoute(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Routes {
		if r["path_suffix"] == "job-applications" && r["method"] == "GET" && r["required_scope"] == "tool:career:jobs" {
			return
		}
	}
	t.Error("manifest.json does not declare GET job-applications with the jobs scope")
}

func TestApplicationsWithTheSameActivityComeByTitleThenById(t *testing.T) {
	p1, _ := boardFixture(t)
	for _, j := range []struct{ id, title string }{{"k1", "Zeta"}, {"k2", "Alpha"}, {"k3", "Same"}, {"k4", "Same"}} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, ?)`, j.id, "https://x/"+j.id, j.title); err != nil {
			t.Fatal(err)
		}
		mustSet(t, p1, j.id, jobboard.AppliedID)
		moveAt(t, p1, j.id, "2026-11-02T08:00:00.000Z")
	}
	got, _ := applicationsOf(t, p1)
	order := []string{}
	for _, a := range got {
		order = append(order, a.JobID)
	}
	if want := "k2,k3,k4,k1"; strings.Join(order, ",") != want {
		t.Errorf("order %v, want %s", order, want)
	}
}
