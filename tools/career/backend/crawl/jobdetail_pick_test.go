package crawl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/portal"
	"career-tool-backend/portal/detailpick"
)

// pickLink is a link with four saved jobs (distinct titles, oldest crawled first) and the AI-style instructions.
func pickLink(t *testing.T) (string, []string) {
	t.Helper()
	id := linkCrawlLink(t)
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, detailYAMLWithFields); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"title": "Alpha Developer", "url": "https://board.example/j/1"},
		{"title": "Beta Engineer", "url": "https://board.example/j/2"},
		{"title": "Gamma Analyst", "url": "https://board.example/j/3"},
		{"title": "Delta Manager", "url": "https://board.example/j/4"},
	}
	if _, err := portal.IngestLinkCrawlResults(id, []portal.CrawlResultPage{{URL: listingURL, Items: items}}); err != nil {
		t.Fatal(err)
	}
	targets, err := jobs.ListJobsForDetailCrawl(id)
	if err != nil || len(targets) != 4 {
		t.Fatalf("jobs: %v %v", targets, err)
	}
	// a stable "oldest first" order
	for i, tg := range targets {
		if _, err := db.JobsDB.Exec(`UPDATE jobs SET crawled_at = ? WHERE id = ?`, "2026-01-0"+string(rune('1'+i))+" 00:00:00", tg.ID); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	targets, _ = jobs.ListJobsForDetailCrawl(id)
	for _, tg := range targets {
		ids = append(ids, tg.ID)
	}
	return id, ids
}

// secondPickLink adds another link (with one job) to the SAME database: pickLink resets the database, so a second call would make the
// first link unknown.
func secondPickLink(t *testing.T) (string, []string) {
	t.Helper()
	pid, err := portal.CreatePortal("Other board")
	if err != nil {
		t.Fatal(err)
	}
	id, err := portal.AddPortalLink(pid, listingURL+"/other", "Other jobs")
	if err != nil {
		t.Fatal(err)
	}
	rule := boardRule
	if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodLinks, &rule, false); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{{"title": "Other Role", "url": "https://board.example/jobs/other-1"}}
	if _, err := portal.IngestLinkCrawlResults(id, []portal.CrawlResultPage{{URL: listingURL + "/other", Items: items}}); err != nil {
		t.Fatal(err)
	}
	targets, err := jobs.ListJobsForDetailCrawl(id)
	if err != nil || len(targets) != 1 {
		t.Fatalf("jobs: %v %v", targets, err)
	}
	return id, []string{targets[0].ID}
}

const pickPageJSON = `{"finalUrl":"https://board.example/j/1","title":"Alpha Developer at Acme",
"candidates":{
 "title":[{"selector":"h1.board","unique":true,"value":"18 jobs on the board","tag":"h1","chars":20,"inMain":true,"source":"dom"},{"selector":"h1.job","unique":true,"value":"Alpha Developer","tag":"h1","chars":15,"inMain":true,"source":"dom"}],
 "company":[{"selector":".company","unique":true,"value":"Acme","tag":"div","chars":4,"inMain":true,"source":"dom"}],
 "location":[],"postedAt":[{"selector":"time","unique":true,"value":"2026-10-28","attribute":"datetime","tag":"time","chars":10,"inMain":true,"source":"dom"}],
 "description":[{"selector":"#job-description","unique":true,"chars":3000,"tag":"div","inMain":true,"byAttr":true,"shareOfPage":0.5,"headings":3,"listItems":5,"paragraphs":4,"source":"dom",
   "furniture":[{"selector":".share","unique":true,"kind":"share","tag":"div","chars":14,"value":"Share this job"}],"lineSignals":["similar jobs","apply now"]}]},
"structured":{"jsonld":[{"type":"JobPosting","title":"Alpha Developer","company":"Acme","location":"Berlin","datePosted":"2026-10-28T09:00:00+00:00","descriptionChars":3100,"descriptionPreview":"x"}],"meta":{"ogTitle":"Alpha Developer - Acme"},"microdata":0},
"elements":[{"selector":"h1.job","unique":true,"tag":"h1","text":"Alpha Developer","chars":15}],"pageTextChars":6000,"truncated":false}`

func postPick(t *testing.T, h http.HandlerFunc, path, body string, cookie bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if cookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-186"})
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec.Code, rec.Body.String()
}

func TestPickCandidatesReturnsRankedSuggestionsForTheOldestJob(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod") // D1: no Katana needed
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	code, out := postPick(t, JobDetailCandidatesHandler, "/portal-links/job-detail/candidates", `{"portalLinkId":"`+id+`"}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	var resp pickCandidatesResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.URL != "https://board.example/j/1" || len(resp.Samples) != 3 || resp.Samples[0].JobID != ids[0] || resp.Samples[2].Title != "Gamma Analyst" {
		t.Errorf("sample page and samples: %q %+v", resp.URL, resp.Samples)
	}
	if got := resp.Ranked["title"]; len(got) != 2 || got[0].Selector != "h1.job" {
		t.Errorf("the job heading must rank above the board heading: %+v", got)
	}
	if got := resp.Ranked["postedAt"]; len(got) != 1 || got[0].Attribute != "datetime" {
		t.Errorf("date: %+v", got)
	}
	for _, k := range []string{"title", "company", "location", "postedAt", "description"} {
		if resp.Ranked[k] == nil {
			t.Errorf("ranked[%s] must be a list, not null", k)
		}
	}
	h := resp.DescriptionHelp["#job-description"]
	if len(h.Excludes) != 1 || h.Excludes[0].Selector != ".share" || len(h.CutAfter) != 1 || h.CutAfter[0] != "Similar jobs" {
		t.Errorf("description help: %+v", h)
	}
	if len(resp.StructuredHints) == 0 || len(resp.Structured.JSONLD) != 1 || len(resp.Elements) != 1 {
		t.Errorf("structured: %+v", resp)
	}
	if resp.Current.Choices == nil || resp.Current.Choices.DescriptionSelector != ".body" || resp.Current.YAML != detailYAMLWithFields {
		t.Errorf("current instructions: %+v", resp.Current)
	}
	portalID, _ := portal.GetPortalIDForLink(id)
	if len(f.detailBodies) != 1 || f.detailBodies[0]["url"] != "https://board.example/j/1" || portalID == "" || f.detailBodies[0]["rateLimitKey"] != portalID {
		t.Errorf("the Browser call: %v", f.detailBodies)
	}
	if f.settingsCalls != 0 {
		t.Errorf("no Katana gate: the engine setting was read %d times", f.settingsCalls)
	}
}

func TestPickSamplesAreThreeDifferentJobsOldestFirst(t *testing.T) {
	id, ids := pickLink(t)
	// the second-oldest job has the same title as the oldest: it is not offered
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET title = 'alpha developer ' WHERE id = ?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	samples, err := listPickSamples(id, maxPickSamples)
	if err != nil || len(samples) != 3 || samples[0].JobID != ids[0] || samples[1].JobID != ids[2] || samples[2].JobID != ids[3] {
		t.Fatalf("samples %+v (%v), want jobs %v", samples, err, []string{ids[0], ids[2], ids[3]})
	}
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET description = 'abc' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if samples, _ = listPickSamples(id, 1); len(samples) != 1 || samples[0].DescriptionChars != 3 {
		t.Errorf("a limit of one: %+v", samples)
	}
}

func TestPickCandidatesChoosesThePageByJobOrAddressAndChecksTheSite(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "katana")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	ok := func(body string) {
		t.Helper()
		if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 200 {
			t.Errorf("%s: %d %s", body, code, out)
		}
	}
	bad := func(name, body string) {
		t.Helper()
		before := len(f.detailBodies)
		if code, _ := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 400 {
			t.Errorf("%s: status %d, want 400", name, code)
		}
		if len(f.detailBodies) != before {
			t.Errorf("%s: the Browser was asked for a refused page", name)
		}
	}
	ok(`{"portalLinkId":"` + id + `","jobId":"` + ids[2] + `"}`)
	if f.detailBodies[len(f.detailBodies)-1]["url"] != "https://board.example/j/3" {
		t.Errorf("job page: %v", f.detailBodies[len(f.detailBodies)-1])
	}
	ok(`{"portalLinkId":"` + id + `","url":"https://board.example/careers/x"}`)
	bad("another site", `{"portalLinkId":"`+id+`","url":"https://evil.example/x"}`)
	bad("private address", `{"portalLinkId":"`+id+`","url":"http://127.0.0.1/x"}`)
	bad("not http", `{"portalLinkId":"`+id+`","url":"file:///etc/passwd"}`)
	bad("credentials in the address", `{"portalLinkId":"`+id+`","url":"https://user:pw@board.example/x"}`)
	bad("both job and url", `{"portalLinkId":"`+id+`","jobId":"`+ids[0]+`","url":"https://board.example/x"}`)
	bad("unknown job", `{"portalLinkId":"`+id+`","jobId":"nope"}`)
	_, otherIDs := secondPickLink(t)
	bad("a job of another link", `{"portalLinkId":"`+id+`","jobId":"`+otherIDs[0]+`"}`)
	bad("unknown field", `{"portalLinkId":"`+id+`","useLoginWindow":true}`)
	bad("no link", `{}`)
}

func TestPickCandidatesNeverLoadsASavedJobOfAnotherSite(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET source_url = 'https://evil.example/x' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	// as the default sample and as the chosen one
	for _, body := range []string{`{"portalLinkId":"` + id + `"}`, `{"portalLinkId":"` + id + `","jobId":"` + ids[0] + `"}`} {
		if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 400 || !strings.Contains(out, "own site") {
			t.Errorf("%s: %d %s", body, code, out)
		}
	}
	if len(f.detailBodies) != 0 {
		t.Error("the foreign page was loaded")
	}
}

func TestPickCandidatesNeedsAJobOrAnAddressWhenTheLinkHasNoJobs(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	if code, out := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != 400 || !strings.Contains(out, "no saved jobs") {
		t.Errorf("%d %s", code, out)
	}
	if code, _ := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","url":"https://board.example/j/9"}`, true); code != 200 {
		t.Errorf("an address works without jobs: %d", code)
	}
}

func TestPickRoutesRefuseLoginLinksSessionlessCallsAndWrongMethods(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	choices := `{"descriptionSelector":"#d"}`
	for name, c := range map[string]struct {
		h    http.HandlerFunc
		body string
		auth bool
	}{
		"candidates": {JobDetailCandidatesHandler, `{"portalLinkId":"` + id + `"}`, true},
		"test":       {JobDetailTestHandler, `{"portalLinkId":"` + id + `","choices":` + choices + `}`, true},
	} {
		if code, _ := postPick(t, c.h, "/x", c.body, false); code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d", name, code)
		}
		if rec := httptest.NewRecorder(); true {
			c.h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s GET: %d", name, rec.Code)
			}
		}
	}
	for name, h := range map[string]http.HandlerFunc{"preview": JobDetailPreviewHandler, "save": JobDetailSaveHandler} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s GET: %d", name, rec.Code)
		}
	}
	// a login link: nothing of the feature is offered
	if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := portal.UpdatePortalLinkLogin(id, &yes, nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		h    http.HandlerFunc
		body string
	}{
		"candidates": {JobDetailCandidatesHandler, `{"portalLinkId":"` + id + `"}`},
		"preview":    {JobDetailPreviewHandler, `{"portalLinkId":"` + id + `","choices":` + choices + `}`},
		"test":       {JobDetailTestHandler, `{"portalLinkId":"` + id + `","choices":` + choices + `,"jobIds":["` + ids[0] + `"]}`},
		"save":       {JobDetailSaveHandler, `{"portalLinkId":"` + id + `","choices":` + choices + `}`},
	} {
		if code, out := postPick(t, c.h, "/x", c.body, true); code != 400 || !strings.Contains(out, "needs a login") {
			t.Errorf("%s on a login link: %d %s", name, code, out)
		}
	}
	if n := len(f.detailBodies) + len(f.paginatedBodies); n != 0 {
		t.Errorf("the Browser was asked %d times for a refused call", n)
	}
}

func TestPickCandidatesAnswersBrowserFailuresAs502WithTheVerdict(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) {
		return 502, `{"code":"cloudflare_challenge_unresolved","message":"challenge","reason":"turnstile","advice":"open it in a browser"}`
	}
	code, out := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != 502 || !strings.Contains(out, "cloudflare_challenge_unresolved") || !strings.Contains(out, "turnstile") {
		t.Errorf("verdict: %d %s", code, out)
	}
	f.detailPage = func(map[string]any) (int, string) { return 500, "boom" }
	if code, out := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != 502 || !strings.Contains(out, "fetch_failed") {
		t.Errorf("failure: %d %s", code, out)
	}
	f.detailPage = func(map[string]any) (int, string) { return 400, "url is required" }
	if code, _ := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != 400 {
		t.Errorf("a bad request stays a 400: %d", code)
	}
}

func TestPickPreviewIsPureAndSaysWhyChoicesAreNotValid(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	stored := func() string {
		s, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id)
		return s
	}
	before := stored()
	code, out := postPick(t, JobDetailPreviewHandler, "/x", `{"portalLinkId":"`+id+`","choices":{"descriptionSelector":"#d","fields":[{"label":"company","selector":".c"}]}}`, false)
	var resp pickPreviewResponse
	_ = json.Unmarshal([]byte(out), &resp)
	if code != 200 || !resp.Valid || !strings.Contains(resp.YAML, "label: company") || resp.Error != "" {
		t.Fatalf("%d %s", code, out)
	}
	for name, body := range map[string]string{
		"no description": `{"portalLinkId":"` + id + `","choices":{"fields":[{"label":"title","selector":"h1"}]}}`,
		"both":           `{"portalLinkId":"` + id + `","choices":{"descriptionSelector":"#d","description":{"parts":[{"include":"#a"}]}}}`,
		"unknown label":  `{"portalLinkId":"` + id + `","choices":{"descriptionSelector":"#d","fields":[{"label":"salary","selector":".s"}]}}`,
		"a bad rule":     `{"portalLinkId":"` + id + `","choices":{"description":{"parts":[]}}}`,
		"control char":   `{"portalLinkId":"` + id + `","choices":{"descriptionSelector":"#d\u0000"}}`,
	} {
		code, out := postPick(t, JobDetailPreviewHandler, "/x", body, false)
		var r pickPreviewResponse
		_ = json.Unmarshal([]byte(out), &r)
		if code != 200 || r.Valid || r.Error == "" || strings.Contains(r.Error, "invalid crawl instructions") || r.YAML != "" {
			t.Errorf("%s: %d %s", name, code, out)
		}
	}
	for name, body := range map[string]string{
		"unknown field in the body":    `{"portalLinkId":"` + id + `","choices":{"descriptionSelector":"#d"},"jobIds":["x"]}`,
		"unknown field in the choices": `{"portalLinkId":"` + id + `","choices":{"descriptionSelector":"#d","evil":1}}`,
		"no link":                      `{"choices":{"descriptionSelector":"#d"}}`,
		"not json":                     `x`,
	} {
		if code, _ := postPick(t, JobDetailPreviewHandler, "/x", body, false); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if stored() != before {
		t.Error("a preview stored something")
	}
	if len(f.detailBodies)+len(f.paginatedBodies) != 0 {
		t.Error("a preview called the Browser")
	}
}

func TestPickSaveStoresThroughTheValidatedFunction(t *testing.T) {
	id, _ := pickLink(t)
	before, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	code, out := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"`+id+`","choices":{"description":{"format":"structured","parts":[{"include":"#job-description","exclude":[".share"]}]},"fields":[{"label":"postedAt","selector":"time","attribute":"datetime"}]}}`, false)
	var resp pickPreviewResponse
	_ = json.Unmarshal([]byte(out), &resp)
	if code != 200 || !resp.Valid {
		t.Fatalf("%d %s", code, out)
	}
	got, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	if got != resp.YAML || got == before || !strings.Contains(got, "descriptionRule") || !strings.Contains(got, "attribute: datetime") {
		t.Errorf("stored:\n%s", got)
	}
	// the stored text is what the detail crawl reads
	if _, err := portal.BuildJobDetailCrawlRequest(id); err != nil {
		t.Errorf("the saved instructions cannot be run: %v", err)
	}
	// a refused save changes nothing
	if code, _ := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"`+id+`","choices":{"fields":[]}}`, false); code != 400 {
		t.Errorf("refused: %d", code)
	}
	if again, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id); again != got {
		t.Error("a refused save changed the stored instructions")
	}
	if code, out := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"nope","choices":{"descriptionSelector":"#d"}}`, false); code != 400 || !strings.Contains(out, "unknown") {
		t.Errorf("unknown link: %d %s", code, out)
	}
}

// ---- the real extraction on sample jobs ----

func pickExtractPage(values map[string]string, report string) func(string) (int, string) {
	return func(pageURL string) (int, string) {
		v, _ := json.Marshal(values)
		out := `{"pages":[{"url":"` + pageURL + `","results":` + string(v)
		if report != "" {
			out += `,"truncated":["description"],"description":` + report
		}
		return 200, out + `}]}`
	}
}

const pickChoices = `{"descriptionSelector":".body","fields":[{"label":"title","selector":"h1"},{"label":"company","selector":".company"},{"label":"postedAt","selector":"time","attribute":"datetime"}]}`

func longDesc(tag string) string {
	return "<p>" + strings.Repeat("A real sentence of the posting about "+tag+". ", 20) + "</p><script>x=1</script>"
}

func TestPickTestRunsTheRealExtractionOnThreeJobsAndFlagsWhatLooksOff(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET description = ? WHERE id = ?`, "Old line one\nOld line two", ids[0]); err != nil {
		t.Fatal(err)
	}
	f.paginated = func(pageURL string) (int, string) {
		n := pageURL[len(pageURL)-1:]
		return pickExtractPage(map[string]string{
			"title": "Backend Developer " + n, "company": "Acme", "postedAt": "2026-10-28T09:00:00+00:00", "description": longDesc("job " + n),
		}, "")(pageURL)
	}
	code, out := postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+pickChoices+`}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	var resp pickTestResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Jobs) != 3 || resp.Jobs[0].Title != "Alpha Developer" || !strings.Contains(resp.YAML, "label: company") {
		t.Fatalf("jobs: %+v", resp.Jobs)
	}
	j := resp.Jobs[0]
	if strings.Contains(j.Values["description"], "<p>") || strings.Contains(j.Values["description"], "x=1") || !strings.HasPrefix(j.Values["description"], "A real sentence of the posting about job 1.") {
		t.Errorf("the description must show as it would be saved (markup out): %q", j.Values["description"][:80])
	}
	if j.DescriptionChars == 0 || j.PostedAtClean != "2026-10-28" || j.StoredChars != len("Old line one\nOld line two") {
		t.Errorf("sizes and date: %+v", j)
	}
	if j.Diff == nil || j.Diff.Unchanged || j.Diff.Removed != 2 || j.Diff.Added == 0 {
		t.Errorf("diff against the stored text: %+v", j.Diff)
	}
	// the Browser got the real instruction shape, with no request id and no pacing key (never a visible window, no 30-second pacing)
	if len(f.paginatedBodies) != 3 {
		t.Fatalf("Browser calls: %d", len(f.paginatedBodies))
	}
	b := f.paginatedBodies[0]
	if b["url"] != "https://board.example/j/1" || b["requestId"] != nil || b["rateLimitKey"] != nil || b["effectiveMaxPages"] != float64(1) {
		t.Errorf("request: %v", b)
	}
	if fl, _ := b["fields"].([]any); len(fl) != 4 {
		t.Errorf("fields: %v", b["fields"])
	}
	// the company is the same everywhere and the titles differ from the saved ones: information only, nothing blocks or warns
	got := map[string]detailpick.Severity{}
	for _, fl := range resp.Flags {
		got[fl.Code+"|"+string(fl.Field)] = fl.Severity
		if fl.Severity != detailpick.Info {
			t.Errorf("only information expected: %+v", fl)
		}
	}
	if got["identical_on_all|company"] != detailpick.Info || got["title_differs|title"] != detailpick.Info || len(got) != 2 {
		t.Errorf("flags: %+v", resp.Flags)
	}
	// lists are never null in the answer (the screen maps over them)
	if strings.Contains(out, `"flags":null`) || strings.Contains(out, `"jobs":null`) {
		t.Errorf("null list in the answer: %s", out)
	}
	// nothing was saved
	if s, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id); s != detailYAMLWithFields {
		t.Errorf("a test changed the stored instructions: %q", s)
	}
	if desc := descriptionOf(t, id); strings.Contains(desc, "A real sentence") {
		t.Error("a test saved a description")
	}
	if f.settingsCalls != 0 {
		t.Error("no Katana gate")
	}
}

func TestPickTestWithNothingToFlagAnswersAnEmptyListNotNull(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.paginated = func(pageURL string) (int, string) {
		return pickExtractPage(map[string]string{"title": "Alpha Developer", "company": "Acme", "postedAt": "2026-10-28", "description": longDesc("one")}, "")(pageURL)
	}
	_, out := postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+pickChoices+`,"jobIds":["`+ids[0]+`"]}`, true)
	if !strings.Contains(out, `"flags":[]`) {
		t.Errorf("no flags must be an empty list: %s", out)
	}
	// and when every page failed
	f.paginated = func(string) (int, string) { return 502, `{"code":"x","message":"m"}` }
	_, out = postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+pickChoices+`,"jobIds":["`+ids[0]+`"]}`, true)
	if !strings.Contains(out, `"flags":[]`) {
		t.Errorf("all failed: %s", out)
	}
}

func TestPickTestFlagsAnEmptyDescriptionAsBlockingAndACutAsAWarning(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.paginated = pickExtractPage(map[string]string{"title": "x", "description": "  "}, "")
	body := `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["` + ids[0] + `","` + ids[1] + `"]}`
	_, out := postPick(t, JobDetailTestHandler, "/x", body, true)
	var resp pickTestResponse
	_ = json.Unmarshal([]byte(out), &resp)
	blocking := false
	for _, fl := range resp.Flags {
		if fl.Code == "description_empty" && fl.Severity == detailpick.Blocking {
			blocking = true
		}
	}
	if !blocking {
		t.Errorf("an empty description on every sample blocks: %+v", resp.Flags)
	}
	f.paginated = pickExtractPage(map[string]string{"description": longDesc("cut")}, `{"chars":3000,"fullLength":8000,"truncated":true,"limit":3000,"parts":[{"index":1,"matches":1,"chars":3000,"overlap":true}]}`)
	_, out = postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":{"description":{"parts":[{"include":"#d"}]}},"jobIds":["`+ids[0]+`"]}`, true)
	resp = pickTestResponse{}
	_ = json.Unmarshal([]byte(out), &resp)
	codes := map[string]bool{}
	for _, fl := range resp.Flags {
		codes[fl.Code] = true
	}
	if !codes["description_cut_at_limit"] || !codes["description_parts_overlap"] || resp.Jobs[0].Report == nil || !resp.Jobs[0].Report.Truncated || len(resp.Jobs[0].Truncated) != 1 {
		t.Errorf("flags %v job %+v", codes, resp.Jobs[0])
	}
	// the rule went to the Browser as the rule
	last := f.paginatedBodies[len(f.paginatedBodies)-1]
	if r, _ := last["descriptionRule"].(map[string]any); r == nil {
		t.Errorf("the rule was not sent: %v", last)
	}
}

func TestPickTestOneFailingPageDoesNotSpoilTheOthers(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	f.paginated = func(pageURL string) (int, string) {
		if strings.HasSuffix(pageURL, "/2") {
			return 502, `{"code":"cloudflare_challenge_unresolved","message":"challenge","reason":"turnstile"}`
		}
		return pickExtractPage(map[string]string{"title": "T", "description": longDesc("ok")}, "")(pageURL)
	}
	code, out := postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+pickChoices+`}`, true)
	var resp pickTestResponse
	_ = json.Unmarshal([]byte(out), &resp)
	if code != 200 || len(resp.Jobs) != 3 || resp.Jobs[1].Error == "" || resp.Jobs[0].Error != "" || resp.Jobs[2].Error != "" {
		t.Fatalf("%d %+v", code, resp.Jobs)
	}
	if !strings.Contains(resp.Jobs[1].Error, "Cloudflare") {
		t.Errorf("error text: %q", resp.Jobs[1].Error)
	}
	for _, fl := range resp.Flags {
		for _, jid := range fl.JobIDs {
			if jid == resp.Jobs[1].JobID {
				t.Errorf("a failed job must not be in the flags: %+v", fl)
			}
		}
	}
}

func TestPickTestRefusals(t *testing.T) {
	id, ids := pickLink(t)
	_, otherIDs := secondPickLink(t)
	f := newFakeCore(t, "rod")
	f.paginated = pickExtractPage(map[string]string{"description": longDesc("x")}, "")
	for name, body := range map[string]string{
		"four jobs":        `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["` + ids[0] + `","` + ids[1] + `","` + ids[2] + `","` + ids[3] + `"]}`,
		"a job twice":      `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["` + ids[0] + `","` + ids[0] + `"]}`,
		"a job of another": `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["` + otherIDs[0] + `"]}`,
		"an unknown job":   `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["nope"]}`,
		"invalid choices":  `{"portalLinkId":"` + id + `","choices":{"fields":[]}}`,
		"unknown field":    `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"requestId":"x"}`,
		"no link":          `{"choices":` + pickChoices + `}`,
	} {
		if code, _ := postPick(t, JobDetailTestHandler, "/x", body, true); code != 400 {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}
	if len(f.paginatedBodies) != 0 {
		t.Errorf("a refused test reached the Browser %d times", len(f.paginatedBodies))
	}
	pidE, _ := portal.CreatePortal("Empty board")
	empty, _ := portal.AddPortalLink(pidE, listingURL+"/empty", "Empty")
	if code, out := postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+empty+`","choices":`+pickChoices+`}`, true); code != 400 || !strings.Contains(out, "no saved jobs") {
		t.Errorf("a link without jobs: %d %s", code, out)
	}
	// a saved job whose page is on another site is never loaded
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET source_url = 'https://evil.example/x' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if code, out := postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+pickChoices+`,"jobIds":["`+ids[0]+`"]}`, true); code != 400 || !strings.Contains(out, "own site") {
		t.Errorf("foreign page: %d %s", code, out)
	}
	if len(f.paginatedBodies) != 0 {
		t.Error("the foreign page was loaded")
	}
}

func TestPickTestRunsOneAtATimePerLink(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	f.paginated = func(pageURL string) (int, string) {
		once.Do(func() { close(started) })
		<-release
		return pickExtractPage(map[string]string{"description": longDesc("x")}, "")(pageURL)
	}
	body := `{"portalLinkId":"` + id + `","choices":` + pickChoices + `,"jobIds":["` + ids[0] + `"]}`
	done := make(chan int, 1)
	go func() {
		code, _ := postPick(t, JobDetailTestHandler, "/x", body, true)
		done <- code
	}()
	<-started
	code, out := postPick(t, JobDetailTestHandler, "/x", body, true)
	if code != http.StatusConflict || !strings.Contains(out, "already running") {
		t.Errorf("second test: %d %s", code, out)
	}
	close(release)
	select {
	case c := <-done:
		if c != 200 {
			t.Errorf("first test: %d", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first test never finished")
	}
}
