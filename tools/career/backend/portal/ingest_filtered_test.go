package portal

import (
	"testing"

	"career-tool-backend/jobs"
)

// refuseTitled refuses every candidate whose title contains the word, keeps the rest.
type refuseTitled struct{ word string }

func (r refuseTitled) Name() string { return "refuse-" + r.word }

func (r refuseTitled) Match(c jobs.Candidate) (jobs.Verdict, error) {
	if len(r.word) > 0 && containsFold(c.Title, r.word) {
		return jobs.Verdict{Keep: false, Reason: "title has " + r.word}, nil
	}
	return jobs.Verdict{Keep: true}, nil
}

func containsFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 32
		}
		if y >= 'A' && y <= 'Z' {
			y += 32
		}
		if x != y {
			return false
		}
	}
	return true
}

type countingSource struct {
	ms    []jobs.CriteriaMatcher
	calls int
}

func (s *countingSource) Matchers() ([]jobs.CriteriaMatcher, error) {
	s.calls++
	return s.ms, nil
}

// A crawl whose new jobs are refused by the criteria saves the others, counts the refused ones as filtered out and ALSO as updated
// (found, nothing saved: the zero-jobs flag reads saved + updated), in both result shapes, and builds the chain once for the whole crawl.
func TestIngestKeepsOutWhatTheCriteriaRefuse(t *testing.T) {
	setupAttemptsDB(t, "l1")
	src := &countingSource{ms: []jobs.CriteriaMatcher{refuseTitled{word: "sales"}}}
	jobs.UseCriteria(src)
	t.Cleanup(func() { jobs.UseCriteria(nil) })

	pages := []CrawlResultPage{
		{URL: "https://jobs.example/list?page=1", Items: []map[string]any{
			{"title": "Sales Manager", "url": "https://jobs.example/a", "company": "Acme"},    // refused
			{"title": "Backend Engineer", "url": "https://jobs.example/b", "company": "Acme"}, // kept
			{"title": "", "url": "https://jobs.example/skipme"},                               // skipped
		}},
		{URL: "https://jobs.example/list?page=2", Results: map[string]any{
			"title":   []any{"Inside Sales Rep", "Go Developer"},
			"url":     []any{"https://jobs.example/c", "https://jobs.example/d"},
			"company": []any{"Globex", "Initech"},
		}},
	}
	res, err := IngestCrawlResults("l1", pages)
	if err != nil {
		t.Fatal(err)
	}
	want := IngestCrawlResultsResult{JobsSaved: 2, JobsUpdated: 2, JobsSkipped: 1, JobsFilteredOut: 2}
	if res != want {
		t.Errorf("%+v, want %+v", res, want)
	}
	if liveJobs(t) != 2 {
		t.Errorf("%d jobs, want only the two that were kept", liveJobs(t))
	}
	if src.calls != 1 {
		t.Errorf("the source was asked %d times for one crawl", src.calls)
	}

	// the same crawl again: the two kept jobs are refreshed, the two refused ones are looked at again and refused again
	res, err = IngestCrawlResults("l1", pages)
	if err != nil {
		t.Fatal(err)
	}
	if res.JobsSaved != 0 || res.JobsFilteredOut != 2 || res.JobsUpdated != 4 || liveJobs(t) != 2 {
		t.Errorf("second crawl: %+v, %d jobs", res, liveJobs(t))
	}
}

// An existing job is refreshed even if the criteria would now refuse it, and a criteria-free crawl is exactly what it was.
func TestIngestRefreshesExistingJobsAndNoCriteriaChangesNothing(t *testing.T) {
	setupAttemptsDB(t, "l1")
	pages := []CrawlResultPage{{URL: "https://jobs.example/list", Items: []map[string]any{
		{"title": "Sales Manager", "url": "https://jobs.example/a", "company": "Acme"},
		{"title": "Backend Engineer", "url": "https://jobs.example/b", "company": "Acme"},
	}}}
	res, err := IngestCrawlResults("l1", pages)
	if err != nil || res != (IngestCrawlResultsResult{JobsSaved: 2}) {
		t.Fatalf("without criteria: %+v %v", res, err)
	}
	jobs.UseCriteria(&countingSource{ms: []jobs.CriteriaMatcher{refuseTitled{word: "sales"}, refuseTitled{word: "engineer"}}})
	t.Cleanup(func() { jobs.UseCriteria(nil) })
	res, err = IngestCrawlResults("l1", pages)
	if err != nil || res != (IngestCrawlResultsResult{JobsUpdated: 2}) || liveJobs(t) != 2 {
		t.Errorf("existing jobs are refreshed, never filtered: %+v %v (%d jobs)", res, err, liveJobs(t))
	}
}
