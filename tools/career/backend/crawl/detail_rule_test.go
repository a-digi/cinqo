package crawl

import (
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const ruleOnlyYAML = `descriptionRule:
  format: structured
  parts:
    - include: "#job-description"
      exclude: [".share"]
`

func linkWithJobsAndInstructions(t *testing.T, instructions string) string {
	t.Helper()
	id := linkCrawlLink(t)
	if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodInstructions, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, instructions); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{{"title": "Backend Developer", "url": "https://board.example/j/1"}}
	if _, err := portal.IngestLinkCrawlResults(id, []portal.CrawlResultPage{{URL: listingURL, Items: items}}); err != nil {
		t.Fatal(err)
	}
	return id
}

func descriptionOf(t *testing.T, linkID string) string {
	t.Helper()
	var d string
	if err := db.JobsDB.QueryRow(`SELECT COALESCE(description,'') FROM jobs WHERE portal_link_id = ?`, linkID).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestADetailCrawlSendsTheDescriptionRuleAndSavesItsText(t *testing.T) {
	id := linkWithJobsAndInstructions(t, ruleOnlyYAML)
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"Tasks\n\n- Go\n- SQL"},"description":{"text":"x","chars":16,"fullLength":16,"truncated":false,"limit":60000}}]}`
	}
	status, summary, log := crawlDetails(t, id)
	if status != "completed" || !strings.Contains(summary, "Updated 1 of 1") {
		t.Fatalf("%s %q", status, summary)
	}
	if got := descriptionOf(t, id); got != "Tasks\n\n- Go\n- SQL" {
		t.Errorf("saved description %q", got)
	}
	if len(f.paginatedBodies) != 1 {
		t.Fatalf("browser calls: %d", len(f.paginatedBodies))
	}
	body := f.paginatedBodies[0]
	rule, _ := body["descriptionRule"].(map[string]any)
	parts, _ := rule["parts"].([]any)
	if rule == nil || rule["format"] != "structured" || len(parts) != 1 || parts[0].(map[string]any)["include"] != "#job-description" {
		t.Fatalf("the request did not carry the rule: %v", body)
	}
	if fields, ok := body["fields"].([]any); !ok || len(fields) != 0 {
		t.Errorf("a rule-only instruction sends an empty fields array: %v", body["fields"])
	}
	if strings.Contains(log, "cut at") {
		t.Errorf("nothing was cut, yet the log says so: %s", log)
	}
}

func TestOldInstructionsSendNoRuleAndNothingChanges(t *testing.T) {
	id := linkWithJobsAndInstructions(t, detailYAMLWithFields)
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	crawlDetails(t, id)
	if len(f.paginatedBodies) != 1 {
		t.Fatalf("browser calls: %d", len(f.paginatedBodies))
	}
	if _, has := f.paginatedBodies[0]["descriptionRule"]; has {
		t.Errorf("an old instruction must not send a rule: %v", f.paginatedBodies[0])
	}
}

func TestACutDescriptionIsSaidInTheRunLog(t *testing.T) {
	// with a rule: its limit
	id := linkWithJobsAndInstructions(t, ruleOnlyYAML)
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"cut text"},"truncated":["description"],"description":{"text":"cut text","chars":3000,"fullLength":8614,"truncated":true,"limit":3000}}]}`
	}
	_, _, log := crawlDetails(t, id)
	if !strings.Contains(log, "the description was cut at its rule's limit: 3000 of 8614 characters kept") || !strings.Contains(log, "Backend Developer") {
		t.Errorf("log: %s", log)
	}
	if got := descriptionOf(t, id); got != "cut text" {
		t.Errorf("the cut text is still saved: %q", got)
	}
}

func TestAPlainFieldCutAtTheOldLimitIsSaidInTheRunLog(t *testing.T) {
	id := linkWithJobsAndInstructions(t, detailYAMLWithFields)
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"cut at the field limit"},"truncated":["description"]}]}`
	}
	_, _, log := crawlDetails(t, id)
	if !strings.Contains(log, "cut at 5,000 characters") || !strings.Contains(log, "description rule") {
		t.Errorf("log: %s", log)
	}
}

func TestACutCompanyFieldIsNotReportedAsACutDescription(t *testing.T) {
	id := linkWithJobsAndInstructions(t, detailYAMLWithFields)
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"fine","company":"x"},"truncated":["company"]}]}`
	}
	_, _, log := crawlDetails(t, id)
	if strings.Contains(log, "cut at") {
		t.Errorf("log: %s", log)
	}
}

func TestDescriptionCutNote(t *testing.T) {
	cases := []struct {
		name string
		page portal.CrawlResultPage
		want string
	}{
		{"nothing", portal.CrawlResultPage{}, ""},
		{"other label", portal.CrawlResultPage{Truncated: []string{"company"}}, ""},
		{"rule flag without the cut", portal.CrawlResultPage{Truncated: []string{"description"}, Description: &portal.DescriptionReport{Truncated: false}}, "5,000"},
		{"rule cut", portal.CrawlResultPage{Truncated: []string{"description"}, Description: &portal.DescriptionReport{Truncated: true, Chars: 10, FullLength: 99}}, "10 of 99 characters kept"},
	}
	for _, c := range cases {
		got := descriptionCutNote(c.page)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want something with %q", c.name, got, c.want)
		}
	}
}

func TestAStoredRuleSurvivesTheRoundTripThroughTheDatabase(t *testing.T) {
	id := linkCrawlLink(t)
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, ruleOnlyYAML); err != nil {
		t.Fatal(err)
	}
	got, err := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	if err != nil || got != ruleOnlyYAML {
		t.Fatalf("stored %q (%v)", got, err)
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, "descriptionRule:\n  parts: []\n"); err == nil {
		t.Error("a bad rule was stored")
	}
	if again, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id); again != ruleOnlyYAML {
		t.Errorf("a refused save changed the stored instructions: %q", again)
	}
}
