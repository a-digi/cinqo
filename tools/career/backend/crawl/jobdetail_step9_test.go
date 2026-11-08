package crawl

import (
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/portal"
)

// linkWithOrigin is a link on the instructions method with one saved job and detail instructions of the given origin.
func linkWithOrigin(t *testing.T, yamlText, origin string) string {
	t.Helper()
	id := linkCrawlLink(t)
	if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodInstructions, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructionsFrom(id, yamlText, origin); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{{"title": "Backend Developer", "url": "https://board.example/j/1"}}
	if _, err := portal.IngestLinkCrawlResults(id, []portal.CrawlResultPage{{URL: listingURL, Items: items}}); err != nil {
		t.Fatal(err)
	}
	return id
}

func filledAfterACrawl(t *testing.T, id string) [3]string {
	t.Helper()
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	crawlDetails(t, id)
	return jobFields(t, id)["Backend Developer"]
}

// Decision D3: only instructions a person picked fill the company, location and date of the jobs of a link that is not on the link crawler.
func TestOnlyPickedInstructionsFillTheJobsOfAnInstructionLink(t *testing.T) {
	want := [3]string{"2RM Lab", "Tirana, Albania", "23-11-2026"}
	for origin, filled := range map[string]bool{
		portal.JobDetailOriginPicked:    true,
		portal.JobDetailOriginAI:        false,
		portal.JobDetailOriginManual:    false,
		portal.JobDetailOriginInherited: false,
		"":                              false,
	} {
		id := linkWithOrigin(t, detailYAMLWithFields, origin)
		got := filledAfterACrawl(t, id)
		switch {
		case filled && got != want:
			t.Errorf("origin %q: %v, want %v", origin, got, want)
		case !filled && got != [3]string{}:
			t.Errorf("origin %q must fill nothing, got %v", origin, got)
		}
	}
}

func TestPickedInstructionsStillNeverTouchAJobAddedByHand(t *testing.T) {
	id := linkWithOrigin(t, detailYAMLWithFields, portal.JobDetailOriginPicked)
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET source = 'manual' WHERE portal_link_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if got := filledAfterACrawl(t, id); got != [3]string{} {
		t.Errorf("a manual job was filled: %v", got)
	}
}

func TestLinkCrawlerLinksFillWhateverTheOrigin(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodLinks) // saved with the plain function: no origin
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	crawlDetails(t, id)
	if got := jobFields(t, id)["Backend Developer"]; got == [3]string{} {
		t.Errorf("the link crawler's own rule changed: %v", got)
	}
}

// ---- decision D2: the title ----

const titleDetailYAML = "fields:\n  - label: description\n    selector: .body\n  - label: title\n    selector: h1\n"

func titlePage(title string) func(string) (int, string) {
	return pickExtractPage(map[string]string{"title": title, "description": longDesc("x")}, "")
}

func jobTitles(t *testing.T, linkID string) []string {
	t.Helper()
	rows, err := db.JobsDB.Query(`SELECT title FROM jobs WHERE portal_link_id = ? ORDER BY title`, linkID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestTheTitleIsOnlyReplacedWhenTheInstructionsSayAndTheRunLogSaysSo(t *testing.T) {
	// off (the default): the page's title is extracted and ignored
	id := linkWithOrigin(t, titleDetailYAML, portal.JobDetailOriginPicked)
	f := newFakeCore(t, "katana")
	f.paginated = titlePage("A Much Better Title")
	_, _, log := crawlDetails(t, id)
	if got := jobTitles(t, id); len(got) != 1 || got[0] != "Backend Developer" || strings.Contains(log, "title changed") {
		t.Fatalf("default off: %v\n%s", got, log)
	}
	// on
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructionsFrom(id, titleDetailYAML+"replaceTitle: true\n", portal.JobDetailOriginPicked); err != nil {
		t.Fatal(err)
	}
	_, summary, log := crawlDetails(t, id)
	if got := jobTitles(t, id); len(got) != 1 || got[0] != "A Much Better Title" {
		t.Fatalf("on: %v", got)
	}
	if !strings.Contains(log, `title changed: "Backend Developer" → "A Much Better Title"`) || !strings.Contains(summary, "Updated 1 of 1") {
		t.Errorf("the run log must say it (never silent): %s | %s", log, summary)
	}
	// the same title again changes nothing and logs nothing new about it
	_, _, log = crawlDetails(t, id)
	if strings.Count(log, "title changed") != 0 {
		t.Errorf("an unchanged title was reported: %s", log)
	}
}

func TestAReplacedTitleIsCleanedLikeEveryListingTitleAndBadOnesAreIgnored(t *testing.T) {
	id := linkWithOrigin(t, titleDetailYAML+"replaceTitle: true\n", portal.JobDetailOriginPicked)
	f := newFakeCore(t, "katana")
	f.paginated = titlePage("Java 17: Developer")
	crawlDetails(t, id)
	if got := jobTitles(t, id)[0]; strings.ContainsAny(got, "0123456789:") || !strings.Contains(got, "Developer") {
		t.Errorf("a replaced title is cleaned like a listing title: %q", got)
	}
	before := jobTitles(t, id)[0]
	for _, bad := range []string{"", "   ", "Line one\nLine two", strings.Repeat("x", 400)} {
		f.paginated = titlePage(bad)
		crawlDetails(t, id)
		if got := jobTitles(t, id)[0]; got != before {
			t.Errorf("title %q changed the job to %q", bad, got)
		}
	}
}

func TestAManualJobKeepsItsTitleWhateverTheInstructionsSay(t *testing.T) {
	id := linkWithOrigin(t, titleDetailYAML+"replaceTitle: true\n", portal.JobDetailOriginPicked)
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET source = 'manual' WHERE portal_link_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	f := newFakeCore(t, "katana")
	f.paginated = titlePage("Other Title")
	crawlDetails(t, id)
	if got := jobTitles(t, id); got[0] != "Backend Developer" {
		t.Errorf("a manual job's title changed: %v", got)
	}
}

func TestReplaceJobTitle(t *testing.T) {
	id := linkWithOrigin(t, titleDetailYAML, portal.JobDetailOriginPicked)
	targets, err := jobs.ListJobsForDetailCrawl(id)
	if err != nil || len(targets) != 1 {
		t.Fatalf("%v %v", targets, err)
	}
	jid := targets[0].ID
	if old, changed, err := jobs.ReplaceJobTitle(jid, "  New Name  "); err != nil || !changed || old != "Backend Developer" {
		t.Fatalf("replace: %q %v %v", old, changed, err)
	}
	if _, changed, _ := jobs.ReplaceJobTitle(jid, "New Name"); changed {
		t.Error("the same title is no change")
	}
	for _, bad := range []string{"", "  ", "a\nb", "a\rb", strings.Repeat("é", 301)} {
		if _, changed, err := jobs.ReplaceJobTitle(jid, bad); changed || err != nil {
			t.Errorf("%q: changed=%v err=%v", bad, changed, err)
		}
	}
	if _, changed, err := jobs.ReplaceJobTitle(jid, strings.Repeat("é", 300)); !changed || err != nil {
		t.Errorf("exactly the limit is allowed: %v %v", changed, err)
	}
	if _, _, err := jobs.ReplaceJobTitle("nope", "x"); err == nil {
		t.Error("an unknown job")
	}
}
