package jobs

// Statistics for the Jobs page's dashboard strip: totals, what was added recently,
// how many jobs were matched and what the average match is, how many have a CV,
// and where the jobs come from. One read-only query set, computed on request.

import (
	"math"
	"net/http"
	"time"

	"career-tool-backend/db"
)

const (
	statsDays        = 14
	maxStatsPlatform = 5
)

// PlatformCount is how many jobs one platform (portal) contributed.
type PlatformCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Stats is the whole dashboard strip's data.
type Stats struct {
	Total     int `json:"total"`
	Manual    int `json:"manual"`
	Companies int `json:"companies"`
	// Jobs added in the last 7 days (today included) and in the 7 days before that.
	AddedLast7Days int `json:"addedLast7Days"`
	AddedPrev7Days int `json:"addedPrev7Days"`
	// Matched counts jobs with a completed match; AvgMatchScore is their average
	// (a whole percent), null when there are none.
	Matched       int  `json:"matched"`
	AvgMatchScore *int `json:"avgMatchScore"`
	// WithCv counts jobs whose CV is completed.
	WithCv int `json:"withCv"`
	// PerDay is the last statsDays days, oldest first, zeros included.
	PerDay []DayCount `json:"perDay"`
	// ByPlatform is the biggest sources first; anything beyond the top few is folded
	// into "Other".
	ByPlatform []PlatformCount `json:"byPlatform"`
}

// foldPlatforms keeps the biggest `max` platforms and sums the rest into "Other"
// (merging with an existing "Other"). Input is already sorted biggest first.
func foldPlatforms(all []PlatformCount, max int) []PlatformCount {
	if len(all) <= max {
		return all
	}
	out := append([]PlatformCount(nil), all[:max]...)
	other := 0
	for _, p := range all[max:] {
		other += p.Count
	}
	for i := range out {
		if out[i].Name == "Other" {
			out[i].Count += other
			return out
		}
	}
	return append(out, PlatformCount{Name: "Other", Count: other})
}

// ComputeStats reads the numbers. now decides what "the last 7 days" means.
func ComputeStats(now time.Time) (Stats, error) {
	var s Stats
	if err := db.JobsDB.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN source = 'manual' THEN 1 ELSE 0 END), 0), COUNT(DISTINCT company_id) FROM jobs`,
	).Scan(&s.Total, &s.Manual, &s.Companies); err != nil {
		return Stats{}, err
	}

	perDay, err := jobsPerDay(now, statsDays)
	if err != nil {
		return Stats{}, err
	}
	s.PerDay = perDay
	for i, d := range perDay {
		if i >= len(perDay)-7 {
			s.AddedLast7Days += d.Count
		} else if i >= len(perDay)-14 {
			s.AddedPrev7Days += d.Count
		}
	}

	var avg *float64
	if err := db.JobsDB.QueryRow(
		`SELECT COUNT(*), AVG(m.score) FROM job_matches m JOIN jobs j ON j.id = m.job_id WHERE m.status = 'completed' AND m.score IS NOT NULL`,
	).Scan(&s.Matched, &avg); err != nil {
		return Stats{}, err
	}
	if s.Matched > 0 && avg != nil {
		rounded := int(math.Round(*avg))
		s.AvgMatchScore = &rounded
	}

	if err := db.JobsDB.QueryRow(
		`SELECT COUNT(*) FROM job_cv_pdfs c JOIN jobs j ON j.id = c.job_id WHERE c.status = 'completed'`,
	).Scan(&s.WithCv); err != nil {
		return Stats{}, err
	}

	rows, err := db.JobsDB.Query(
		`SELECT COALESCE(p.name, CASE WHEN j.source = 'manual' THEN 'Manual' ELSE 'Other' END) AS platform, COUNT(*) AS n
		FROM jobs j
		LEFT JOIN portal_links pl ON pl.id = j.portal_link_id
		LEFT JOIN portals p ON p.id = pl.portal_id
		GROUP BY platform ORDER BY n DESC, platform ASC`,
	)
	if err != nil {
		return Stats{}, err
	}
	defer rows.Close()
	all := []PlatformCount{}
	for rows.Next() {
		var p PlatformCount
		if err := rows.Scan(&p.Name, &p.Count); err != nil {
			return Stats{}, err
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return Stats{}, err
	}
	s.ByPlatform = foldPlatforms(all, maxStatsPlatform)
	return s, nil
}

// StatsHandler serves GET /jobs/stats.
func StatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s, err := ComputeStats(time.Now())
	if err != nil {
		http.Error(w, "failed to compute job statistics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, s)
}
