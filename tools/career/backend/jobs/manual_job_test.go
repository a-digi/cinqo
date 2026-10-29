package jobs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
)

func setupJobsDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

// capturedEvents points the domain-event client at a local server and records
// what is published.
type capturedEvents struct {
	mu     sync.Mutex
	events []map[string]any
}

func captureEvents(t *testing.T) *capturedEvents {
	t.Helper()
	c := &capturedEvents{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev map[string]any
		_ = json.Unmarshal(raw, &ev)
		c.mu.Lock()
		c.events = append(c.events, ev)
		c.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "test-token")
	return c
}

func (c *capturedEvents) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func mustAdd(t *testing.T, in ManualJobInput) job {
	t.Helper()
	j, err := AddManualJob(in)
	if err != nil {
		t.Fatalf("AddManualJob(%+v): %v", in, err)
	}
	return j
}

func TestATitleOnlyJobIsStoredAsManualWithoutAURL(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{Title: "  Backend Engineer  "})

	if j.Title != "Backend Engineer" || j.Source != SourceManual || j.SourceURL != "" || j.Company != "" || j.Description != "" || j.CrawledAt == "" {
		t.Fatalf("job: %+v", j)
	}
	var stored, source string
	if err := db.JobsDB.QueryRow(`SELECT source_url, source FROM jobs WHERE id = ?`, j.ID).Scan(&stored, &source); err != nil {
		t.Fatal(err)
	}
	if stored != manualURLPrefix+j.ID || source != "manual" {
		t.Errorf("stored placeholder %q, source %q", stored, source)
	}
	if events.count() != 0 {
		t.Error("no company means no company event")
	}
	if j.MatchedSkills == nil {
		t.Error("matchedSkills must be [] like any job, never null")
	}
}

func TestAFullJobKeepsEveryFieldAndPublishesTheCompanyEvent(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{
		Title: "Platform Engineer", Company: "Acme GmbH", Location: "Hamburg", Description: "Build things.\nSecond line.",
		PostedAt: "2026-10-29", SourceURL: "https://example.com/jobs/42?ref=mail",
	})
	if j.Company != "Acme GmbH" || j.Location != "Hamburg" || j.Description != "Build things.\nSecond line." || j.PostedAt != "2026-10-29" ||
		j.SourceURL != "https://example.com/jobs/42?ref=mail" || j.Source != SourceManual || j.PortalLinkID != "" {
		t.Fatalf("%+v", j)
	}
	if events.count() != 1 {
		t.Fatalf("events = %d, want 1", events.count())
	}
	ev := events.events[0]
	payload, _ := ev["payload"].(map[string]any)
	if ev["topic"] != "career.job.created" || payload["job_id"] != j.ID || payload["company"] != "Acme GmbH" {
		t.Errorf("event: %v", ev)
	}
}

func TestEveryFieldIsTrimmed(t *testing.T) {
	in := ManualJobInput{Title: " t ", Company: " c ", Location: " l ", Description: " d ", PostedAt: " p ", SourceURL: " https://example.com/x "}
	if err := ValidateManualJob(&in); err != nil {
		t.Fatal(err)
	}
	if in.Title != "t" || in.Company != "c" || in.Location != "l" || in.Description != "d" || in.PostedAt != "p" || in.SourceURL != "https://example.com/x" {
		t.Errorf("%+v", in)
	}
}

func TestValidationRejectsWhatItShould(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	for name, c := range map[string]struct {
		in    ManualJobInput
		field string
	}{
		"no title":                {ManualJobInput{}, "title"},
		"blank title":             {ManualJobInput{Title: "   \n"}, "title"},
		"title too long":          {ManualJobInput{Title: long(maxManualTitle + 1)}, "title"},
		"company too long":        {ManualJobInput{Title: "t", Company: long(maxManualCompany + 1)}, "company"},
		"location too long":       {ManualJobInput{Title: "t", Location: long(maxManualLocation + 1)}, "location"},
		"postedAt too long":       {ManualJobInput{Title: "t", PostedAt: long(maxManualPostedAt + 1)}, "postedAt"},
		"description too long":    {ManualJobInput{Title: "t", Description: long(maxManualDescription + 1)}, "description"},
		"url too long":            {ManualJobInput{Title: "t", SourceURL: "https://example.com/" + long(maxManualURL)}, "sourceUrl"},
		"NUL in the title":        {ManualJobInput{Title: "a\x00b"}, "title"},
		"relative url":            {ManualJobInput{Title: "t", SourceURL: "/jobs/1"}, "sourceUrl"},
		"scheme-less url":         {ManualJobInput{Title: "t", SourceURL: "example.com/jobs/1"}, "sourceUrl"},
		"javascript url":          {ManualJobInput{Title: "t", SourceURL: "javascript:alert(1)"}, "sourceUrl"},
		"data url":                {ManualJobInput{Title: "t", SourceURL: "data:text/html,<script>1</script>"}, "sourceUrl"},
		"ftp url":                 {ManualJobInput{Title: "t", SourceURL: "ftp://example.com/x"}, "sourceUrl"},
		"mailto url":              {ManualJobInput{Title: "t", SourceURL: "mailto:a@b.c"}, "sourceUrl"},
		"url without a host":      {ManualJobInput{Title: "t", SourceURL: "https://"}, "sourceUrl"},
		"url with a space inside": {ManualJobInput{Title: "t", SourceURL: "https://example.com/a b"}, "sourceUrl"},
		"the manual placeholder":  {ManualJobInput{Title: "t", SourceURL: "manual:abc"}, "sourceUrl"},
	} {
		err := ValidateManualJob(&c.in)
		v, ok := IsValidationError(err)
		if !ok || v.Field != c.field {
			t.Errorf("%s: err = %v, want a validation error on %s", name, err, c.field)
		}
	}
}

func TestValidationAcceptsTheLimitsAndRealURLs(t *testing.T) {
	long := func(n int) string { return strings.Repeat("é", n) } // multi-byte: counted in characters
	in := ManualJobInput{
		Title: long(maxManualTitle), Company: long(maxManualCompany), Location: long(maxManualLocation),
		PostedAt: long(maxManualPostedAt), Description: long(maxManualDescription),
	}
	if err := ValidateManualJob(&in); err != nil {
		t.Errorf("exactly at every limit must pass: %v", err)
	}
	for _, u := range []string{"http://example.com", "https://example.com/jobs/1?a=b&c=d#frag", "https://jobs.example.co.uk:8443/x", "HTTPS://EXAMPLE.COM/X"} {
		in := ManualJobInput{Title: "t", SourceURL: u}
		if err := ValidateManualJob(&in); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
}

func TestADuplicateURLIsRefusedWithTheExistingId(t *testing.T) {
	setupJobsDB(t)
	first := mustAdd(t, ManualJobInput{Title: "A", SourceURL: "https://example.com/jobs/1"})
	_, err := AddManualJob(ManualJobInput{Title: "Different title", SourceURL: " https://example.com/jobs/1 "})
	d, ok := IsDuplicate(err)
	if !ok || d.ExistingID != first.ID || d.Reason != "source_url" {
		t.Fatalf("%v", err)
	}
	// A crawled job with that URL counts too.
	if _, _, err := saveJob("https://example.com/jobs/2", "Crawled", "Co", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AddManualJob(ManualJobInput{Title: "x", SourceURL: "https://example.com/jobs/2"}); err == nil {
		t.Error("a crawled job's URL is a duplicate as well")
	}
}

func TestADuplicateTitleAndCompanyIsRefusedCaseInsensitively(t *testing.T) {
	setupJobsDB(t)
	first := mustAdd(t, ManualJobInput{Title: "Backend Engineer", Company: "Acme"})
	_, err := AddManualJob(ManualJobInput{Title: "  backend ENGINEER ", Company: "ACME  "})
	d, ok := IsDuplicate(err)
	if !ok || d.ExistingID != first.ID || d.Reason != "title_company" {
		t.Fatalf("%v", err)
	}
	// Across sources: a crawled job with the same title and company.
	if _, _, err := saveJob("https://example.com/c", "Data Engineer", "Globex", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := AddManualJob(ManualJobInput{Title: "data engineer", Company: "globex"}); err == nil {
		t.Error("a crawled job with the same title and company is a duplicate too")
	}
}

func TestTheDuplicateCheckOnlyRunsWhenThereIsSomethingToCompare(t *testing.T) {
	setupJobsDB(t)
	mustAdd(t, ManualJobInput{Title: "Same title"})
	mustAdd(t, ManualJobInput{Title: "Same title"}) // no company, no URL: two different postings
	mustAdd(t, ManualJobInput{Title: "Same title", Company: "A"})
	mustAdd(t, ManualJobInput{Title: "Same title", Company: "B"}) // another company
	mustAdd(t, ManualJobInput{Title: "Other title", Company: "A"})
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(DISTINCT source_url) FROM jobs`).Scan(&n); err != nil || n != 5 {
		t.Errorf("five jobs, five distinct placeholders: %d (%v)", n, err)
	}
}

func TestThePlaceholderNeverAppearsInAnyRead(t *testing.T) {
	setupJobsDB(t)
	j := mustAdd(t, ManualJobInput{Title: "No URL here", Company: "Acme"})
	byID, err := GetJobByID(j.ID)
	if err != nil || byID.SourceURL != "" {
		t.Fatalf("get: %q %v", byID.SourceURL, err)
	}
	list, err := listJobs("", "", "", 50, 0)
	if err != nil || len(list.Jobs) != 1 || list.Jobs[0].SourceURL != "" || list.Jobs[0].Source != SourceManual {
		t.Fatalf("list: %+v %v", list, err)
	}
	found, err := SearchJobs("No URL", "", nil, "", "", 50, 0)
	if err != nil || len(found.Jobs) != 1 || found.Jobs[0].SourceURL != "" {
		t.Fatalf("search: %+v %v", found, err)
	}
	raw, _ := json.Marshal(found)
	if strings.Contains(string(raw), "manual:") {
		t.Errorf("the placeholder leaked into JSON: %s", raw)
	}
	if !strings.Contains(string(raw), `"source":"manual"`) {
		t.Errorf("the job must say it is manual: %s", raw)
	}
}

func TestCrawledJobsKeepTheirURLAndSource(t *testing.T) {
	setupJobsDB(t)
	id, _, err := saveJob("https://example.com/crawled/1", "Crawled", "Co", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	j, err := GetJobByID(id)
	if err != nil || j.Source != SourceCrawled || j.SourceURL != "https://example.com/crawled/1" {
		t.Fatalf("%+v %v", j, err)
	}
}

func TestAManualJobCanBeDeletedAndMatchedLikeAnyOther(t *testing.T) {
	setupJobsDB(t)
	j := mustAdd(t, ManualJobInput{Title: "Deletable"})
	if err := RequireJobExists(j.ID); err != nil {
		t.Errorf("match and CV look the job up by id: %v", err)
	}
	if err := DeleteJob(j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetJobByID(j.ID); err != ErrUnknownJob {
		t.Errorf("deleted: %v", err)
	}
}
