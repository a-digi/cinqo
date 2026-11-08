package jobs

// Data for the Home dashboard's Career widgets. Cinqo's dashboard engine calls
// these routes with the caller's own token (after checking tool:career:jobs
// itself) and validates what comes back against the widget's kind, so the shapes
// here must match the core's "chart" and "list" schemas (api/src/dashboard/kinds.go):
// text fields are shortened to the core's limits, because the engine rejects an
// over-long field outright rather than clipping it. See
// plan/ai/dashboard/home-dashboard.md, step 7.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"career-tool-backend/db"
)

const (
	newJobsDays      = 14
	recentJobsLimit  = 5
	maxTitleRunes    = 120
	maxSubtitleRunes = 200
)

type chartPoint struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
}

type chartSeries struct {
	Name   string       `json:"name"`
	Points []chartPoint `json:"points"`
}

type chartPayload struct {
	Type   string        `json:"type"`
	Series []chartSeries `json:"series"`
}

type listBadge struct {
	Text    string `json:"text"`
	Variant string `json:"variant"`
}

type listItem struct {
	Title    string     `json:"title"`
	Subtitle string     `json:"subtitle,omitempty"`
	Meta     string     `json:"meta,omitempty"`
	Badge    *listBadge `json:"badge,omitempty"`
	Link     string     `json:"link,omitempty"`
}

type listPayload struct {
	Items []listItem `json:"items"`
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// DayCount is how many jobs were added on one day (label like "2 Jan").
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// jobsPerDay counts jobs by the day they were added, for the last `days` days
// ending today (UTC), oldest first, with a zero for days that had none.
func jobsPerDay(now time.Time, days int) ([]DayCount, error) {
	today := now.UTC().Truncate(24 * time.Hour)
	first := today.AddDate(0, 0, -(days - 1))

	rows, err := db.JobsDB.Query(
		`SELECT substr(crawled_at, 1, 10) AS day, COUNT(*) FROM jobs WHERE day >= ? GROUP BY day`,
		first.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		counts[day] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]DayCount, 0, days)
	for i := 0; i < days; i++ {
		d := first.AddDate(0, 0, i)
		out = append(out, DayCount{Day: d.Format("2 Jan"), Count: counts[d.Format("2006-01-02")]})
	}
	return out, nil
}

// newJobsChart is the Home dashboard's "New jobs" chart: jobsPerDay as a bar chart.
func newJobsChart(now time.Time, days int) (chartPayload, error) {
	perDay, err := jobsPerDay(now, days)
	if err != nil {
		return chartPayload{}, err
	}
	points := make([]chartPoint, 0, len(perDay))
	for _, d := range perDay {
		points = append(points, chartPoint{X: d.Day, Y: float64(d.Count)})
	}
	return chartPayload{Type: "bar", Series: []chartSeries{{Name: "New jobs", Points: points}}}, nil
}

// anyJobs reports whether there is at least one job, so a library that has never
// had a job shows the widget's "nothing yet" state instead of 14 empty bars.
func anyJobs() (bool, error) {
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM (SELECT 1 FROM jobs LIMIT 1)`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// recentJobsList is the latest jobs by when they were added.
func recentJobsList(now time.Time, limit int) (listPayload, error) {
	rows, err := db.JobsDB.Query(
		`SELECT id, title, COALESCE(company, ''), COALESCE(location, ''), crawled_at, source
		FROM jobs ORDER BY crawled_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return listPayload{}, err
	}
	defer rows.Close()

	out := listPayload{Items: []listItem{}}
	for rows.Next() {
		var id, title, company, location, crawledAt, source string
		if err := rows.Scan(&id, &title, &company, &location, &crawledAt, &source); err != nil {
			return listPayload{}, err
		}
		item := listItem{
			Title:    truncate(strings.TrimSpace(title), maxTitleRunes),
			Subtitle: truncate(joinNonEmpty(" · ", company, location), maxSubtitleRunes),
			Meta:     relativeAge(now, crawledAt),
			Link:     "/tools/career/job-details?id=" + url.QueryEscape(id),
		}
		if item.Title == "" {
			item.Title = "Untitled job"
		}
		if source == "manual" {
			item.Badge = &listBadge{Text: "Manual", Variant: "gray"}
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

var ageLayouts = []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}

// relativeAge renders a stored UTC timestamp relative to now ("just now", "5m
// ago", "3h ago", "2d ago", a plain date after 30 days), or "" if unparseable.
func relativeAge(now time.Time, stored string) string {
	var then time.Time
	for _, layout := range ageLayouts {
		if t, err := time.ParseInLocation(layout, stored, time.UTC); err == nil {
			then = t
			break
		}
	}
	if then.IsZero() {
		return ""
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	default:
		return then.Format("2 Jan 2006")
	}
}

// NewJobsWidgetHandler serves GET /dashboard/new-jobs: the "New jobs" chart.
func NewJobsWidgetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	has, err := anyJobs()
	if err != nil {
		http.Error(w, "failed to read jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !has {
		db.WriteJSON(w, chartPayload{Type: "bar", Series: []chartSeries{{Name: "New jobs", Points: []chartPoint{}}}})
		return
	}
	chart, err := newJobsChart(time.Now(), newJobsDays)
	if err != nil {
		http.Error(w, "failed to read jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, chart)
}

// RecentJobsWidgetHandler serves GET /dashboard/recent-jobs: the latest jobs.
func RecentJobsWidgetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := recentJobsList(time.Now(), recentJobsLimit)
	if err != nil {
		http.Error(w, "failed to read jobs: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, list)
}
