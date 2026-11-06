package service_test

import (
	"encoding/json"
	"strings"
	"testing"

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
