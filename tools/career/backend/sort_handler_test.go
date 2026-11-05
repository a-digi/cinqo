package main

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/profile"
)

// sortHandlerFixture: three jobs (a oldest, c newest), a scored on a board's profile, b scored for another profile.
func sortHandlerFixture(t *testing.T) (profileID string) {
	t.Helper()
	setupHandlerDB(t)
	var err error
	if profileID, err = profile.CreateProfile("Ada", "Test"); err != nil {
		t.Fatal(err)
	}
	other, _ := profile.CreateProfile("Grace", "Test")
	for i, id := range []string{"a", "b", "c"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, crawled_at) VALUES (?, ?, ?, ?)`, id, "https://x.example/"+id, "Job "+id, "2026-01-0"+string(rune('1'+i))+" 10:00:00"); err != nil {
			t.Fatal(err)
		}
		if _, err := jobboard.SetEntry(profileID, id, jobboard.NotReviewedID); err != nil {
			t.Fatal(err)
		}
		// explicit move times, so "most recently moved" never depends on the clock
		if _, err := db.JobsDB.Exec(`UPDATE job_board_entries SET updated_at = ? WHERE profile_id = ? AND job_id = ?`, "2026-02-0"+string(rune('1'+i))+"T10:00:00.000Z", profileID, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []struct {
		id, profile string
		score       int
	}{{"a", profileID, 70}, {"b", other, 95}} {
		if _, err := db.JobsDB.Exec(`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES (?, ?, ?, 'completed')`, q.id, q.profile, q.score); err != nil {
			t.Fatal(err)
		}
	}
	return profileID
}

func jobIDsFrom(t *testing.T, body string) string {
	t.Helper()
	var res struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	var ids string
	for _, j := range res.Jobs {
		ids += j.ID
	}
	return ids
}

func TestJobsListSortParameter(t *testing.T) {
	sortHandlerFixture(t)
	for sort, want := range map[string]string{"": "cba", "newest": "cba", "match": "bac"} {
		rec := matchCall(t, jobsHandler, "GET", "/jobs?sort="+sort, "")
		wantStatus(t, rec, 200)
		if got := jobIDsFrom(t, rec.Body.String()); got != want {
			t.Errorf("sort=%q gave %s, want %s", sort, got, want)
		}
	}
	rec := matchCall(t, jobsHandler, "GET", "/jobs", "")
	if got := jobIDsFrom(t, rec.Body.String()); got != "cba" {
		t.Errorf("no sort = %s", got)
	}
	for _, bad := range []string{"oldest", "MATCH", "score"} {
		if rec := matchCall(t, jobsHandler, "GET", "/jobs?sort="+bad, ""); rec.Code != 400 {
			t.Errorf("sort=%s: status %d", bad, rec.Code)
		}
	}
}

func TestJobBoardSortParameter(t *testing.T) {
	profileID := sortHandlerFixture(t)
	read := func(sort string) (string, string) {
		rec := matchCall(t, jobBoardHandler, "GET", "/job-board?profileId="+profileID+"&sort="+sort, "")
		wantStatus(t, rec, 200)
		var b struct {
			Columns []struct {
				Jobs []struct {
					JobID      string `json:"jobId"`
					MatchScore *int   `json:"matchScore"`
				} `json:"jobs"`
			} `json:"columns"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		var ids, scores []string
		for _, j := range b.Columns[0].Jobs {
			ids = append(ids, j.JobID)
			if j.MatchScore != nil {
				scores = append(scores, j.JobID)
			}
		}
		return strings.Join(ids, ""), strings.Join(scores, "")
	}
	ids, scored := read("moved")
	if ids != "cba" || scored != "a" {
		t.Errorf("moved: %s, scored %s (b's 95 belongs to another profile)", ids, scored)
	}
	ids, _ = read("")
	if ids != "cba" {
		t.Errorf("default: %s", ids)
	}
	ids, scored = read("match")
	if ids != "acb" || scored != "a" {
		t.Errorf("match: %s, scored %s", ids, scored)
	}
	for _, bad := range []string{"newest", "MATCH", "score"} {
		if rec := matchCall(t, jobBoardHandler, "GET", "/job-board?profileId="+profileID+"&sort="+bad, ""); rec.Code != 400 {
			t.Errorf("sort=%s: status %d", bad, rec.Code)
		}
	}
}
