package jobs

import (
	"testing"

	"career-tool-backend/db"
)

const (
	dirtyDescription = "<style>#no-js-image-:R2j: { display: none !important; }</style><img src=\"https://x/logo.gif\" alt=\"logo\"/>Software Engineer &amp; more\n.job-details-sct {\n  display: flex !important;\n}\n<script>var a = 1;</script>Apply now"
	cleanDescription = "Software Engineer & more\n\nApply now"
)

// Step 147: a crawled description is stored as plain text, whichever way it arrives.
func TestSaveJobStoresPlainText(t *testing.T) {
	setupJobsDB(t)
	id, _, err := saveJob("https://example.com/jobs/1", "Engineer", "Acme", "Berlin", dirtyDescription, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, id).Description; got != cleanDescription {
		t.Errorf("description %q, want %q", got, cleanDescription)
	}
	// Saving the same posting again refreshes it, cleaned the same way.
	if _, _, err := saveJob("https://example.com/jobs/1", "Engineer", "Acme", "Berlin", "<p>New</p><style>.a{top:0;}</style>text", ""); err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, id).Description; got != "New\ntext" {
		t.Errorf("refreshed description %q", got)
	}
}

func TestSavePortalJobStoresPlainText(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	id, created, duplicate, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Engineer", "Acme", "Berlin", dirtyDescription, "today")
	if err != nil || !created || duplicate {
		t.Fatalf("created=%v duplicate=%v err=%v", created, duplicate, err)
	}
	if got := rowOf(t, id).Description; got != cleanDescription {
		t.Errorf("description %q, want %q", got, cleanDescription)
	}
}

func TestSaveJobDetailExtractionStoresPlainText(t *testing.T) {
	setupJobsDB(t)
	id, _, err := saveJob("https://example.com/jobs/2", "Engineer", "", "", "Listing preview", "")
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := SaveJobDetailExtraction(id, map[string]string{"description": dirtyDescription}); err != nil || !updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	got := rowOf(t, id)
	if got.Description != cleanDescription || got.Status != "success" {
		t.Errorf("%+v", got)
	}
}

// A description that was nothing but markup and styles has no text: the crawl counts as failed and the old description stays.
func TestAMarkupOnlyDetailIsAFailedCrawl(t *testing.T) {
	setupJobsDB(t)
	id, _, err := saveJob("https://example.com/jobs/3", "Engineer", "", "", "Listing preview", "")
	if err != nil {
		t.Fatal(err)
	}
	only := "<style>.a { color: red; }</style><img src=\"x\"/>\n.b {\n top: 0;\n}"
	if updated, err := SaveJobDetailExtraction(id, map[string]string{"description": only}); err != nil || updated {
		t.Fatalf("updated=%v err=%v", updated, err)
	}
	got := rowOf(t, id)
	if got.Description != "Listing preview" || got.Status != "failed" {
		t.Errorf("%+v", got)
	}
}

// A job a person typed is never cleaned: what they wrote is what stays, markup characters included.
func TestAManualJobsTextIsLeftExactlyAsTyped(t *testing.T) {
	setupJobsDB(t)
	typed := "Notes: <b>keep</b> this & that {as is}\n\n\n\nthree blank lines"
	manual := mustAdd(t, ManualJobInput{Title: "Mine", Description: typed})
	if got := rowOf(t, manual.ID).Description; got != typed {
		t.Errorf("manual description changed: %q", got)
	}
	// A crawl that completes a manual job with an empty description does clean what it adds.
	blank := mustAdd(t, ManualJobInput{Title: "Blank", SourceURL: "https://example.com/jobs/4"})
	if _, _, err := saveJob("https://example.com/jobs/4", "x", "", "", dirtyDescription, ""); err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, blank.ID).Description; got != cleanDescription {
		t.Errorf("completed description %q", got)
	}
}

// The text the crawl saves is what the search matches: a word that only lived in a style block is not found any more.
func TestSearchDoesNotFindStyleWords(t *testing.T) {
	setupJobsDB(t)
	if _, _, err := saveJob("https://example.com/jobs/5", "Engineer", "", "", "<style>.zzunique { top: 0; }</style>Real words", ""); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE LOWER(description) LIKE '%zzunique%'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("style words stored: %d (%v)", n, err)
	}
}

// Clean text goes in and comes out the same, so saving it again changes nothing.
func TestCleanDescriptionIsStoredAsIs(t *testing.T) {
	setupJobsDB(t)
	text := "Requirements:\n- Go\n- SQL\n\nWe offer 30 days & a team."
	id, _, err := saveJob("https://example.com/jobs/6", "Engineer", "", "", text, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, id).Description; got != text {
		t.Errorf("clean text changed: %q", got)
	}
}
