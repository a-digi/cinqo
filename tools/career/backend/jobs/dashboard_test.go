package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
)

var dashboardNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func insertJob(t *testing.T, id, title, company, location, crawledAt, source string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(
		`INSERT INTO jobs (id, source_url, title, company, location, crawled_at, source) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, "https://example.com/"+id, title, company, location, crawledAt, source,
	); err != nil {
		t.Fatal(err)
	}
}

func TestNewJobsChart(t *testing.T) {
	setupJobsDB(t)
	insertJob(t, "a", "A", "", "", "2026-10-30 08:00:00", "crawled")
	insertJob(t, "b", "B", "", "", "2026-10-30T09:30:00Z", "crawled") // RFC3339 is stored too
	insertJob(t, "c", "C", "", "", "2026-10-28 23:59:59", "crawled")
	insertJob(t, "d", "D", "", "", "2026-10-17 00:00:00", "crawled") // the day before the 14-day window
	insertJob(t, "e", "E", "", "", "2026-10-17 00:00:01", "crawled")
	insertJob(t, "f", "F", "", "", "2026-10-16 10:00:00", "crawled") // outside

	chart, err := newJobsChart(dashboardNow, 14)
	if err != nil {
		t.Fatal(err)
	}
	if chart.Type != "bar" || len(chart.Series) != 1 || chart.Series[0].Name != "New jobs" {
		t.Fatalf("chart = %+v", chart)
	}
	p := chart.Series[0].Points
	if len(p) != 14 {
		t.Fatalf("points = %d, want 14", len(p))
	}
	if p[0].X != "17 Oct" || p[13].X != "30 Oct" {
		t.Errorf("window = %s … %s, want 17 Oct … 30 Oct", p[0].X, p[13].X)
	}
	counts := map[string]float64{}
	for _, pt := range p {
		counts[pt.X] = pt.Y
	}
	if counts["30 Oct"] != 2 || counts["28 Oct"] != 1 || counts["17 Oct"] != 2 || counts["29 Oct"] != 0 {
		t.Errorf("counts = %v", counts)
	}
	for _, pt := range p {
		if pt.X == "16 Oct" {
			t.Error("a job outside the window was counted")
		}
	}
}

func TestRecentJobsList(t *testing.T) {
	setupJobsDB(t)
	insertJob(t, "old", "Old job", "Acme", "Berlin", "2026-10-01 10:00:00", "crawled")
	insertJob(t, "new", "New job", "", "", "2026-10-30 11:00:00", "manual")
	insertJob(t, "mid", strings.Repeat("T", 300), strings.Repeat("c", 300), "Remote", "2026-10-29 12:00:00", "crawled")
	insertJob(t, "x1", "x1", "", "", "2026-09-01 10:00:00", "crawled")
	insertJob(t, "x2", "x2", "", "", "2026-09-02 10:00:00", "crawled")
	insertJob(t, "x3", "x3", "", "", "2026-09-03 10:00:00", "crawled")

	list, err := recentJobsList(dashboardNow, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(list.Items))
	}
	first := list.Items[0]
	if first.Title != "New job" || first.Meta != "1h ago" || first.Link != "/tools/career/job-details?id=new" ||
		first.Badge == nil || first.Badge.Text != "Manual" || first.Subtitle != "" {
		t.Errorf("first = %+v", first)
	}
	second := list.Items[1]
	if n := len([]rune(second.Title)); n != maxTitleRunes {
		t.Errorf("long title = %d characters, want %d", n, maxTitleRunes)
	}
	if n := len([]rune(second.Subtitle)); n > maxSubtitleRunes {
		t.Errorf("long subtitle = %d characters, over %d", n, maxSubtitleRunes)
	}
	if second.Badge != nil || second.Meta != "1d ago" {
		t.Errorf("second = %+v", second)
	}
	if third := list.Items[2]; third.Title != "Old job" || third.Subtitle != "Acme · Berlin" || third.Meta != "29d ago" {
		t.Errorf("third = %+v", third)
	}
}

func TestWidgetHandlers(t *testing.T) {
	setupJobsDB(t)
	call := func(h http.HandlerFunc, method string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(method, "/x", nil))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	// No jobs yet: an empty series (the core shows "nothing yet") and an empty list, never null.
	code, out := call(NewJobsWidgetHandler, http.MethodGet)
	series := out["series"].([]any)
	if code != 200 || out["type"] != "bar" || len(series[0].(map[string]any)["points"].([]any)) != 0 {
		t.Errorf("no jobs chart: %d %v", code, out)
	}
	code, out = call(RecentJobsWidgetHandler, http.MethodGet)
	if items, ok := out["items"].([]any); code != 200 || !ok || len(items) != 0 {
		t.Errorf("no jobs list: %d %v", code, out)
	}

	insertJob(t, "a", "A", "Acme", "", time.Now().UTC().Format("2006-01-02 15:04:05"), "crawled")
	code, out = call(NewJobsWidgetHandler, http.MethodGet)
	pts := out["series"].([]any)[0].(map[string]any)["points"].([]any)
	if code != 200 || len(pts) != newJobsDays {
		t.Errorf("chart with a job: %d, %d points", code, len(pts))
	}
	if code, _ = call(NewJobsWidgetHandler, http.MethodPost); code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", code)
	}
	if code, _ = call(RecentJobsWidgetHandler, http.MethodPost); code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", code)
	}
}
