package crawl

import (
	"testing"

	"career-tool-backend/portal"
)

// The usual summary is exactly the text it always was; only a crawl that found jobs the user deleted before says so.
func TestCrawlSummary(t *testing.T) {
	cases := []struct {
		name string
		in   portal.IngestCrawlResultsResult
		want string
	}{
		{"usual", portal.IngestCrawlResultsResult{JobsSaved: 3, JobsUpdated: 2, JobsSkipped: 1}, "Saved 3 new, updated 2, skipped 1 — visited 4 page(s) (no_next_link)."},
		{"nothing", portal.IngestCrawlResultsResult{}, "Saved 0 new, updated 0, skipped 0 — visited 4 page(s) (no_next_link)."},
		{"ignored", portal.IngestCrawlResultsResult{JobsSaved: 1, JobsUpdated: 5, JobsSkipped: 2, JobsIgnored: 3}, "Saved 1 new, updated 5 (3 of them deleted by you before, not added again), skipped 2 — visited 4 page(s) (no_next_link)."},
	}
	for _, c := range cases {
		if got := crawlSummary(c.in, 4, "no_next_link"); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}
