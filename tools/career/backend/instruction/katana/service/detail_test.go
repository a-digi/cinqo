package service_test

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const detailPage = `{"finalUrl":"https://board.example/j/a1","title":"Posting A1 at Acme",
"candidates":{
 "title":[{"selector":"h1.board","unique":true,"value":"18 jobs on the board","tag":"h1","chars":20,"inMain":true,"source":"dom"},{"selector":"h1.job","unique":true,"value":"Posting A1","tag":"h1","chars":10,"inMain":true,"source":"dom"}],
 "company":[{"selector":".company","unique":true,"value":"Acme","tag":"div","chars":4,"inMain":true,"source":"dom"}],
 "location":[],"postedAt":[{"selector":"time","unique":true,"value":"2026-10-28","attribute":"datetime","tag":"time","chars":10,"inMain":true,"source":"dom"}],
 "description":[{"selector":"#job-description","unique":true,"chars":3000,"tag":"div","inMain":true,"byAttr":true,"shareOfPage":0.5,"headings":3,"listItems":5,"paragraphs":4,"source":"dom",
   "furniture":[{"selector":".share","unique":true,"kind":"share","tag":"div","chars":14,"value":"Share this job"}],"lineSignals":["similar jobs","apply now"]}]},
"structured":{"jsonld":[{"type":"JobPosting","title":"Posting A1","company":"Acme","location":"Berlin","datePosted":"2026-10-28T09:00:00+00:00","descriptionChars":3100,"descriptionPreview":"x"}],"meta":{},"microdata":0},
"elements":[{"selector":"h1.job","unique":true,"tag":"h1","text":"Posting A1","chars":10}],"pageTextChars":6000,"truncated":false}`

func extractor(values map[string]string) func(string) string {
	return func(pageURL string) string {
		v, _ := json.Marshal(values)
		return `{"pages":[{"url":"` + pageURL + `","results":` + string(v) + `}]}`
	}
}

func (r *rig) crawledJobs() {
	r.t.Helper()
	items := []map[string]any{
		{"title": "Posting A1", "url": "https://board.example/j/a1"},
		{"title": "Posting B2", "url": "https://board.example/j/b2"},
		{"title": "Posting C3", "url": "https://board.example/j/c3"},
	}
	if _, err := portal.IngestLinkCrawlResults(r.linkID, []portal.CrawlResultPage{{URL: listURL, Items: items}}); err != nil {
		r.t.Fatal(err)
	}
}

func TestTheWholeJobDetailRecipe(t *testing.T) {
	r := newRig(t)
	r.core.detail = detailPage
	long := "<p>" + strings.Repeat("A real sentence of the posting. ", 30) + "</p>"
	r.core.extract = extractor(map[string]string{"title": "Posting A1", "company": "Acme", "postedAt": "2026-10-28T09:00:00+00:00", "description": long})

	// before any job was crawled the page cannot be picked: a plain refusal
	if text, isErr := r.call("katana_get_job_page_candidates", map[string]any{}); !isErr || !strings.Contains(text, "no saved jobs") {
		t.Errorf("no crawled job: %v %q", isErr, text)
	}
	r.crawledJobs()
	if _, _, err := portal.StartInstructionAttempt(r.linkID, portal.InstructionKindJobDetail, portal.InstructionTriggerManual, "conv"); err != nil {
		t.Fatal(err)
	}

	// 1. candidates: compact, the right heading first, the suggestion made of the top picks
	text, isErr := r.call("katana_get_job_page_candidates", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var v struct {
		SavedJobs int `json:"savedJobs"`
		Fields    map[string][]struct {
			Selector string `json:"selector"`
		} `json:"fields"`
		Excludes  []struct{ Selector string } `json:"descriptionExcludes"`
		Suggested struct {
			Title               struct{ Selector string } `json:"title"`
			DescriptionSelector string                    `json:"descriptionSelector"`
		} `json:"suggested"`
	}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err, text)
	}
	if v.SavedJobs != 3 || v.Fields["title"][0].Selector != "h1.job" || v.Suggested.Title.Selector != "h1.job" || v.Suggested.DescriptionSelector != "#job-description" {
		t.Fatalf("candidates: %s", text)
	}
	if len(v.Excludes) != 1 || v.Excludes[0].Selector != ".share" {
		t.Errorf("description help: %v", v.Excludes)
	}
	if len(text) > 3500 {
		t.Errorf("the candidates answer is %d bytes", len(text))
	}

	// 2. an unreadable default is refused before it is tested, and a save before any test is refused
	if text, isErr := r.call("katana_save_job_detail_picks", map[string]any{}); !isErr || !strings.Contains(text, "not_tested") {
		t.Errorf("save before the test: %v %q", isErr, text)
	}
	if s, _ := portal.GetPortalLinkJobDetailCrawlInstructions(r.linkID); strings.TrimSpace(s) != "" {
		t.Fatal("a refused save wrote instructions")
	}

	// 3. test the suggestion on the real extraction
	text, isErr = r.call("katana_test_job_detail_picks", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var tv struct {
		Jobs    []struct{ Title string } `json:"jobs"`
		Verdict string                   `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(text), &tv); err != nil || len(tv.Jobs) != 3 || !strings.HasPrefix(tv.Verdict, "ok") {
		t.Fatalf("test: %v %s", err, text)
	}
	if strings.Contains(text, "<p>") {
		t.Errorf("markup in the answer: %s", text)
	}

	// 4. save: the picker's own instructions, origin picked, the attempt ends as a success
	text, isErr = r.call("katana_save_job_detail_picks", map[string]any{})
	if isErr || !strings.Contains(text, `"saved":true`) {
		t.Fatalf("save: %v %s", isErr, text)
	}
	stored, _ := portal.GetPortalLinkJobDetailCrawlInstructions(r.linkID)
	if !strings.Contains(stored, "#job-description") || !strings.Contains(stored, "h1.job") {
		t.Errorf("stored: %q", stored)
	}
	var origin string
	if err := dbQuery(`SELECT job_detail_instructions_origin FROM portal_links WHERE id = ?`, r.linkID, &origin); err != nil || origin != portal.JobDetailOriginPicked {
		t.Errorf("origin %q %v", origin, err)
	}
	if status, err := portal.FinishInstructionAttempt(r.linkID, portal.InstructionKindJobDetail, ""); err != nil || status != portal.InstructionAttemptSucceeded {
		t.Errorf("attempt: %q %v", status, err)
	}
}

func TestPicksWithoutADescriptionAreRefusedAndABlockingTestIsNeverSaved(t *testing.T) {
	r := newRig(t)
	r.core.detail = detailPage
	r.crawledJobs()
	r.core.extract = extractor(map[string]string{"title": "Posting A1", "description": ""})
	r.call("katana_get_job_page_candidates", map[string]any{})

	if text, isErr := r.call("katana_test_job_detail_picks", map[string]any{"title": map[string]any{"selector": "h1.job"}}); !isErr || !strings.Contains(text, "description") {
		t.Errorf("no description pick: %v %q", isErr, text)
	}
	// the real extraction finds an empty description on every job: blocking, so nothing can be saved
	text, _ := r.call("katana_test_job_detail_picks", map[string]any{})
	if !strings.Contains(text, "blocking") {
		t.Fatalf("test: %s", text)
	}
	if text, isErr := r.call("katana_save_job_detail_picks", map[string]any{}); !isErr || !strings.Contains(text, "not_tested") {
		t.Errorf("a blocked test was saved: %v %q", isErr, text)
	}
}

func TestJobDetailToolsRefuseARodRunAndALoginLink(t *testing.T) {
	r := newRig(t)
	r.core.detail = detailPage
	r.crawledJobs()
	r.core.engine = "rod"
	for _, tool := range []string{"katana_get_job_page_candidates", "katana_test_job_detail_picks", "katana_save_job_detail_picks"} {
		if text, isErr := r.call(tool, map[string]any{}); !isErr || !strings.Contains(text, "katana") {
			t.Errorf("%s under Rod: %v %q", tool, isErr, text)
		}
	}
}

// The whole instruction in ONE conversation (the user's request): rule, then the AI crawls the jobs itself and waits, then the job details.
func TestTheAICrawlsTheJobsAndGoesOnToTheJobDetails(t *testing.T) {
	r := newRig(t)
	r.core.detail = detailPage
	long := "<p>" + strings.Repeat("A real sentence of the posting. ", 30) + "</p>"
	r.core.extract = extractor(map[string]string{"title": "Posting A1", "company": "Acme", "postedAt": "2026-10-28T09:00:00+00:00", "description": long})

	// no rule yet: no crawl
	if text, isErr := r.call("katana_crawl_jobs", map[string]any{}); !isErr || !strings.Contains(text, "no_rule") {
		t.Errorf("without a rule: %v %q", isErr, text)
	}
	// the state of a new link says the rule is next; after the save it says to crawl, naming the tool
	r.call("katana_get_page_links", map[string]any{})
	r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"board.example/j/a1", "board.example/j/b2"}})
	r.call("katana_test_link_rule", map[string]any{})
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); isErr {
		t.Fatal(text)
	}
	if step, raw := stateOf(t, r); step != "crawl_jobs" || !strings.Contains(raw, "katana_crawl_jobs") {
		t.Fatalf("after the save: %s %s", step, raw)
	}
	// the job details cannot be picked before the jobs exist
	if text, isErr := r.call("katana_get_job_page_candidates", map[string]any{}); !isErr || !strings.Contains(text, "no saved jobs") {
		t.Errorf("candidates before the crawl: %v %q", isErr, text)
	}

	// the AI crawls and waits
	text, isErr := r.call("katana_crawl_jobs", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var c struct {
		Status string `json:"status"`
		Jobs   int    `json:"jobs"`
		Next   string `json:"next"`
	}
	if err := json.Unmarshal([]byte(text), &c); err != nil || c.Status != "completed" || c.Jobs != 3 || !strings.Contains(c.Next, "katana_get_job_page_candidates") {
		t.Fatalf("crawl: %v %s", err, text)
	}
	// asked again with jobs in place: nothing is crawled twice
	fetchesBefore := len(r.core.fetches)
	if text, _ := r.call("katana_crawl_jobs", map[string]any{}); !strings.Contains(text, "already_crawled") || len(r.core.fetches) != fetchesBefore {
		t.Errorf("second call: %s (fetches %d -> %d)", text, fetchesBefore, len(r.core.fetches))
	}
	// the event's own crawl request is answered, so no second crawl follows the run
	var requested int
	if err := dbQueryInt(`SELECT listing_crawl_requested FROM portal_links WHERE id = ?`, r.linkID, &requested); err != nil || requested != 0 {
		t.Errorf("listing_crawl_requested %d %v", requested, err)
	}

	// the same conversation goes on with the job details; the save answers the event's request for the AI to write them
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET job_detail_instructions_ai_requested = 1 WHERE id = ?`, r.linkID); err != nil {
		t.Fatal(err)
	}
	r.call("katana_get_job_page_candidates", map[string]any{})
	if text, _ := r.call("katana_test_job_detail_picks", map[string]any{}); !strings.Contains(text, `"verdict":"ok`) {
		t.Fatalf("test: %s", text)
	}
	text, isErr = r.call("katana_save_job_detail_picks", map[string]any{})
	if isErr || !strings.Contains(text, "no more tool calls") {
		t.Fatalf("save: %v %s", isErr, text)
	}
	var aiRequested int
	if err := dbQueryInt(`SELECT job_detail_instructions_ai_requested FROM portal_links WHERE id = ?`, r.linkID, &aiRequested); err != nil || aiRequested != 0 {
		t.Errorf("job_detail_instructions_ai_requested %d %v", aiRequested, err)
	}
	if step, _ := stateOf(t, r); step != "done" {
		t.Errorf("at the end: %s", step)
	}
}

func TestAnOldRulesJobsDoNotCountAfterANewRuleIsSaved(t *testing.T) {
	r := newRig(t)
	r.crawledJobs() // jobs of an earlier rule
	r.call("katana_get_page_links", map[string]any{})
	r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"board.example/j/a1", "board.example/j/b2"}})
	r.call("katana_test_link_rule", map[string]any{})
	r.call("katana_save_link_rule", map[string]any{})
	before := len(r.core.fetches)
	text, _ := r.call("katana_crawl_jobs", map[string]any{})
	if strings.Contains(text, "already_crawled") || len(r.core.fetches) == before {
		t.Errorf("the new rule was not crawled: %s", text)
	}
}

// The live Indeed run: Cloudflare answered 403 for the job page. The AI is told to STOP, and the event's request for the AI to write the job details
// is answered so that no second run meets the same block.
func TestABlockedJobPageStopsTheAIAndAnswersTheRequest(t *testing.T) {
	r := newRig(t)
	r.crawledJobs()
	r.core.detailStatus = 502
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET job_detail_instructions_ai_requested = 1 WHERE id = ?`, r.linkID); err != nil {
		t.Fatal(err)
	}
	text, isErr := r.call("katana_get_job_page_candidates", map[string]any{})
	if !isErr || !strings.Contains(text, "STOP") || !strings.Contains(text, "challenge") {
		t.Fatalf("%v %q", isErr, text)
	}
	var requested int
	if err := dbQueryInt(`SELECT job_detail_instructions_ai_requested FROM portal_links WHERE id = ?`, r.linkID, &requested); err != nil || requested != 0 {
		t.Errorf("the request was not answered: %d %v", requested, err)
	}
	// a page that merely cannot be analysed (not a block) keeps the request
	r.core.detailStatus = 0
	r.core.detail = "not json"
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET job_detail_instructions_ai_requested = 1 WHERE id = ?`, r.linkID); err != nil {
		t.Fatal(err)
	}
	r.call("katana_get_job_page_candidates", map[string]any{})
	if err := dbQueryInt(`SELECT job_detail_instructions_ai_requested FROM portal_links WHERE id = ?`, r.linkID, &requested); err != nil || requested != 1 {
		t.Errorf("an unreadable page cleared the request: %d %v", requested, err)
	}
}
