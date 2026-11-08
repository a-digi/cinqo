package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/a-digi/cinqo/src/dashboard"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// maxMediaBars is how many tools get a bar; the rest are folded into "Other".
const maxMediaBars = 6

// MediaUsage is "cinqo.media_usage": storage used by uploaded files, per tool
// (and the core app's own "cinqo" bucket), as a bar chart in megabytes. Admin-only,
// like the Media page it summarises.
type MediaUsage struct {
	db *sql.DB
}

func NewMediaUsage(db *sql.DB) *MediaUsage { return &MediaUsage{db: db} }

func (*MediaUsage) Definition() dashboard.Definition {
	return dashboard.Definition{
		Provider:       "cinqo",
		Key:            "media_usage",
		Title:          "Media storage",
		Description:    "Space used by uploaded files, per tool.",
		Kind:           dashboard.KindChart,
		Size:           dashboard.Size{W: 2, H: 1},
		RefreshSeconds: 300,
		RequiredScope:  dashboard.AdminScope,
	}
}

func (m *MediaUsage) Data(_ context.Context, _ dashboard.Caller) ([]byte, error) {
	usage, err := media_query.NewMediaQueryRepo(m.db).UsageByTool()
	if err != nil {
		return nil, fmt.Errorf("media usage: %w", err)
	}
	return json.Marshal(mediaChart(usage))
}

func megabytes(b int64) float64 {
	return math.Round(float64(b)/(1024*1024)*100) / 100
}

// mediaChart makes the bar chart: the largest tools by size, then one "Other"
// bar for whatever is left. No media at all gives an empty series, which the
// engine reports as "empty".
func mediaChart(usage []media_query.ToolUsage) dashboard.ChartData {
	points := []dashboard.ChartPoint{}
	var other int64
	for i, u := range usage {
		if i < maxMediaBars {
			points = append(points, dashboard.ChartPoint{X: truncate(u.ToolSlug, 40), Y: megabytes(u.Bytes)})
			continue
		}
		other += u.Bytes
	}
	if other > 0 {
		points = append(points, dashboard.ChartPoint{X: "Other", Y: megabytes(other)})
	}
	return dashboard.ChartData{
		Type:   "bar",
		Series: []dashboard.ChartSeries{{Name: "Size (MB)", Points: points}},
	}
}
