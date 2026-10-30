package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"career-tool-backend/db"
)

func exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestFoldPlatforms(t *testing.T) {
	in := []PlatformCount{{"A", 9}, {"B", 8}, {"C", 7}, {"D", 6}, {"E", 5}, {"F", 4}, {"G", 3}}
	got := foldPlatforms(in, 5)
	if len(got) != 6 || got[5].Name != "Other" || got[5].Count != 7 || got[0].Name != "A" {
		t.Errorf("fold = %+v", got)
	}
	if same := foldPlatforms(in[:3], 5); len(same) != 3 {
		t.Errorf("a short list must pass through: %+v", same)
	}
	// An existing "Other" inside the top is merged, not duplicated.
	merged := foldPlatforms([]PlatformCount{{"A", 9}, {"Other", 8}, {"C", 7}, {"D", 1}}, 3)
	if len(merged) != 3 || merged[1].Name != "Other" || merged[1].Count != 9 {
		t.Errorf("merge = %+v", merged)
	}
	if len(in) != 7 {
		t.Error("the input was modified")
	}
}

func TestComputeStatsOnAnEmptyLibrary(t *testing.T) {
	setupJobsDB(t)
	s, err := ComputeStats(dashboardNow)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 0 || s.Manual != 0 || s.Companies != 0 || s.Matched != 0 || s.AvgMatchScore != nil || s.WithCv != 0 {
		t.Errorf("stats = %+v", s)
	}
	if len(s.PerDay) != statsDays || s.AddedLast7Days != 0 || s.ByPlatform == nil || len(s.ByPlatform) != 0 {
		t.Errorf("per day %d, platforms %#v", len(s.PerDay), s.ByPlatform)
	}
}

func TestComputeStats(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme'), ('c2', 'Globex')`)
	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Alpha'), ('p2', 'Beta')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://a.example'), ('l2', 'p2', 'https://b.example')`)

	add := func(id, at, source, company, link string) {
		var c, l any
		if company != "" {
			c = company
		}
		if link != "" {
			l = link
		}
		exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at, source, company_id, portal_link_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, "https://x.example/"+id, id, at, source, c, l)
	}
	// now = 2026-10-30 12:00 UTC: "last 7 days" is 24-30 Oct, "previous 7" is 17-23 Oct.
	add("j1", "2026-10-30 09:00:00", "crawled", "c1", "l1")
	add("j2", "2026-10-30T10:00:00Z", "crawled", "c1", "l1")
	add("j3", "2026-10-24 00:00:00", "crawled", "c2", "l2")
	add("j4", "2026-10-23 23:59:59", "crawled", "", "l2") // previous week
	add("j5", "2026-10-17 00:00:00", "crawled", "", "")   // previous week, no platform
	add("j6", "2026-10-16 10:00:00", "crawled", "", "")   // outside both windows
	add("j7", "2026-10-29 10:00:00", "manual", "", "")

	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES
		('j1', 'pr', 80, 'completed'), ('j2', 'pr', 71, 'completed'), ('j3', 'pr', 50, 'failed'),
		('j4', 'pr', NULL, 'matching'), ('j5', 'pr', 90, 'completed')`)
	exec(t, `INSERT INTO job_cv_pdfs (job_id, profile_id, status) VALUES ('j1', 'pr', 'completed'), ('j2', 'pr', 'generating'), ('j3', 'pr', 'failed')`)

	s, err := ComputeStats(dashboardNow)
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 7 || s.Manual != 1 || s.Companies != 2 {
		t.Errorf("totals: %+v", s)
	}
	if s.AddedLast7Days != 4 { // j1, j2, j3, j7
		t.Errorf("last 7 days = %d, want 4", s.AddedLast7Days)
	}
	if s.AddedPrev7Days != 2 { // j4, j5
		t.Errorf("previous 7 days = %d, want 2", s.AddedPrev7Days)
	}
	if s.Matched != 3 || s.AvgMatchScore == nil || *s.AvgMatchScore != 80 { // (80 + 71 + 90) / 3 = 80.33
		t.Errorf("matched %d, avg %v", s.Matched, s.AvgMatchScore)
	}
	if s.WithCv != 1 {
		t.Errorf("with CV = %d, want 1", s.WithCv)
	}
	if len(s.PerDay) != statsDays || s.PerDay[statsDays-1].Day != "30 Oct" || s.PerDay[statsDays-1].Count != 2 {
		t.Errorf("per day tail = %+v", s.PerDay[statsDays-1])
	}
	names := map[string]int{}
	for _, p := range s.ByPlatform {
		names[p.Name] = p.Count
	}
	// Alpha 2 (j1, j2), Beta 2 (j3, j4), Other 2 (j5, j6: crawled with no platform), Manual 1 (j7).
	if names["Alpha"] != 2 || names["Beta"] != 2 || names["Other"] != 2 || names["Manual"] != 1 || len(names) != 4 {
		t.Errorf("by platform = %+v", s.ByPlatform)
	}
	if s.ByPlatform[0].Count < s.ByPlatform[len(s.ByPlatform)-1].Count {
		t.Errorf("not biggest first: %+v", s.ByPlatform)
	}
}

func TestAMatchedJobThatIsDeletedStopsCounting(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://x/1', 'a'), ('j2', 'https://x/2', 'b')`)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j1', 'p', 60, 'completed'), ('j2', 'p', 100, 'completed')`)
	exec(t, `INSERT INTO job_cv_pdfs (job_id, profile_id, status) VALUES ('j1', 'p', 'completed'), ('j2', 'p', 'completed')`)
	if err := DeleteJob("j2"); err != nil {
		t.Fatal(err)
	}
	s, err := ComputeStats(dashboardNow)
	if err != nil || s.Matched != 1 || s.AvgMatchScore == nil || *s.AvgMatchScore != 60 || s.WithCv != 1 {
		t.Errorf("%+v, %v", s, err)
	}
}

func TestStatsHandler(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at) VALUES ('j1', 'https://x/1', 'a', ?)`, time.Now().UTC().Format("2006-01-02 15:04:05"))

	rec := httptest.NewRecorder()
	StatsHandler(rec, httptest.NewRequest(http.MethodGet, "/jobs/stats", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatalf("%d %q: %v", rec.Code, rec.Body.String(), err)
	}
	if got["total"] != float64(1) || got["addedLast7Days"] != float64(1) || got["avgMatchScore"] != nil {
		t.Errorf("payload = %v", got)
	}
	if per, _ := got["perDay"].([]any); len(per) != statsDays {
		t.Errorf("perDay = %v", got["perDay"])
	}
	if plat, ok := got["byPlatform"].([]any); !ok || len(plat) != 1 {
		t.Errorf("byPlatform = %v", got["byPlatform"])
	}
	for _, m := range []string{http.MethodPost, http.MethodDelete} {
		rec := httptest.NewRecorder()
		StatsHandler(rec, httptest.NewRequest(m, "/jobs/stats", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
}
