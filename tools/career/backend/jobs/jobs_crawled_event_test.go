package jobs

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Plan 163, step 6: save_portal_jobs (the AI's own crawl path) raises jobs_crawled once per call that really brought jobs.
func TestSavePortalJobsPublishesJobsCrawledOnlyWhenJobsArrived(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	remember(t, goneURL)
	var published []string
	old := publishJobsCrawled
	publishJobsCrawled = func(id string) { published = append(published, id) }
	defer func() { publishJobsCrawled = old }()
	s := mcpSession(t)
	batch := func(jobs ...map[string]any) {
		callTool(t, s, "save_portal_jobs", map[string]any{"portalLinkId": "l1", "jobs": jobs})
	}

	batch(map[string]any{"sourceUrl": "https://portal.example/jobs/a", "title": "A", "company": "Acme"}, map[string]any{"sourceUrl": "https://portal.example/jobs/b", "title": "B", "company": "Acme"})
	if len(published) != 1 || published[0] != "l1" {
		t.Fatalf("new jobs: published %v, want exactly one for l1", published)
	}

	published = nil
	batch(map[string]any{"sourceUrl": "https://portal.example/jobs/a", "title": "A", "company": "Acme", "description": "now with a description"})
	if len(published) != 1 {
		t.Errorf("an updated job also counts as jobs arrived: %v", published)
	}

	published = nil
	batch(
		map[string]any{"sourceUrl": goneURL, "title": "Gone", "company": "X"},
		map[string]any{"sourceUrl": "", "title": "no url", "company": "Q"},
		map[string]any{"sourceUrl": "https://other.example/dup", "title": "A", "company": "Acme"}, // same title and company as job A: a duplicate
	)
	if len(published) != 0 {
		t.Errorf("only ignored, skipped and duplicate jobs: nothing arrived, nothing published: %v", published)
	}

	published = nil
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_portal_jobs", Arguments: map[string]any{"portalLinkId": "nope", "jobs": []map[string]any{{"sourceUrl": "https://portal.example/jobs/z", "title": "Z", "company": "Acme"}}}})
	if err != nil || !res.IsError {
		t.Fatalf("an unknown link must be an error: %v %+v", err, res)
	}
	if len(published) != 0 {
		t.Errorf("a failed call publishes nothing: %v", published)
	}
}
