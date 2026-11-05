package crawl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const detailYAMLWithFields = `fields:
  - label: description
    selector: .body
  - label: company
    selector: .company
  - label: location
    selector: .place
  - label: postedAt
    selector: .date
`

func fullDetailPage(pageURL string) (int, string) {
	return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"The full text of the posting at ` + pageURL + `","company":"  2RM   Lab ","location":"Tirana, Albania","postedAt":"Posted 23-11-2026"}}]}`
}

// crawlDetails runs "Crawl job details now" for a link and waits for the run.
func crawlDetails(t *testing.T, linkID string) (status, summary, log string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/portal-links/crawl-job-details-now", strings.NewReader(`{"portalLinkId":"`+linkID+`"}`))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-7b"})
	rec := httptest.NewRecorder()
	CrawlJobDetailsNowHandler(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	st, sum, errMsg, lg := waitRun(t, linkID)
	if errMsg != "" {
		t.Fatalf("run error: %s", errMsg)
	}
	return st, sum, lg
}

func jobFields(t *testing.T, linkID string) map[string][3]string {
	t.Helper()
	rows, err := db.JobsDB.Query(`SELECT title, COALESCE(company,''), COALESCE(location,''), COALESCE(posted_at,'') FROM jobs WHERE portal_link_id = ?`, linkID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][3]string{}
	for rows.Next() {
		var title, company, location, postedAt string
		if err := rows.Scan(&title, &company, &location, &postedAt); err != nil {
			t.Fatal(err)
		}
		out[title] = [3]string{company, location, postedAt}
	}
	return out
}

func linkWithJobs(t *testing.T, method string) string {
	t.Helper()
	id := linkCrawlLink(t) // method links with the board rule
	if method == portal.CrawlMethodInstructions {
		if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodInstructions, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, detailYAMLWithFields); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"title": "Backend Developer", "url": "https://board.example/j/1"},
		{"title": "Data Engineer", "url": "https://board.example/j/3"},
	}
	if _, err := portal.IngestLinkCrawlResults(id, []portal.CrawlResultPage{{URL: listingURL, Items: items}}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTheDetailStepCompletesTheJobsOfTheLinkCrawler(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodLinks)
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	before := jobFields(t, id)
	if before["Backend Developer"] != [3]string{} {
		t.Fatalf("a link-crawled job starts without company, location and date: %v", before)
	}

	status, summary, _ := crawlDetails(t, id)
	if status != "completed" || !strings.Contains(summary, "Updated 2 of 2") {
		t.Fatalf("%s %q", status, summary)
	}
	after := jobFields(t, id)
	want := [3]string{"2RM Lab", "Tirana, Albania", "23-11-2026"} // whitespace collapsed; "Posted " removed like a listing's date
	for title, got := range after {
		if got != want {
			t.Errorf("%s: %v, want %v", title, got, want)
		}
	}
	if len(after) != 2 {
		t.Errorf("jobs %v", after)
	}
	// the description arrived through the existing path
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE portal_link_id = ? AND description LIKE 'The full text%'`, id).Scan(&n); err != nil || n != 2 {
		t.Errorf("descriptions %d %v", n, err)
	}
}

func TestTheDetailStepNeverFillsTheJobsOfAnInstructionLink(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodInstructions)
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	status, summary, _ := crawlDetails(t, id)
	if status != "completed" || !strings.Contains(summary, "Updated 2 of 2") { // the description is saved as it always was
		t.Fatalf("%s %q", status, summary)
	}
	for title, got := range jobFields(t, id) {
		if got != [3]string{} {
			t.Errorf("%s was filled although its link is on the instructions method: %v", title, got)
		}
	}
}

func TestTheDetailStepKeepsWhatAJobAlreadyHas(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodLinks)
	f := newFakeCore(t, "katana")
	f.paginated = fullDetailPage
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET company = 'Known GmbH', location = 'Berlin' WHERE title = 'Backend Developer'`); err != nil {
		t.Fatal(err)
	}
	crawlDetails(t, id)
	got := jobFields(t, id)
	if got["Backend Developer"] != [3]string{"Known GmbH", "Berlin", "23-11-2026"} {
		t.Errorf("existing values were replaced: %v", got["Backend Developer"])
	}
	if got["Data Engineer"] != [3]string{"2RM Lab", "Tirana, Albania", "23-11-2026"} {
		t.Errorf("%v", got["Data Engineer"])
	}
}

func TestADetailPageWithOnlyACompanyStillCountsAsAnUpdate(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodLinks)
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"company":"Acme"}}]}`
	}
	_, summary, log := crawlDetails(t, id)
	// no description: the job's detail status is "failed" as before, but the company was completed, so it is not "skipped"
	if !strings.Contains(summary, "Updated 2 of 2") {
		t.Errorf("summary %q log %q", summary, log)
	}
	if got := jobFields(t, id)["Data Engineer"]; got[0] != "Acme" {
		t.Errorf("%v", got)
	}
}

func TestADetailStepWithoutTheExtraLabelsChangesNothingElse(t *testing.T) {
	id := linkWithJobs(t, portal.CrawlMethodLinks)
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(id, trialDetail); err != nil { // description only, as every link had before
		t.Fatal(err)
	}
	f := newFakeCore(t, "katana")
	f.paginated = func(pageURL string) (int, string) {
		return 200, `{"pages":[{"url":"` + pageURL + `","results":{"description":"Body of ` + pageURL + `"}}]}`
	}
	if status, summary, _ := crawlDetails(t, id); status != "completed" || !strings.Contains(summary, "Updated 2 of 2") {
		t.Fatalf("%s %q", status, summary)
	}
	for title, got := range jobFields(t, id) {
		if got != [3]string{} {
			t.Errorf("%s: %v", title, got)
		}
	}
}
