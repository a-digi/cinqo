package portal

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/db"
)

const listingA = `container: ".job"
fields:
  - label: title
    selector: h2
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.next
  maxPages: 5
`

const listingB = `container: ".result"
fields:
  - label: title
    selector: h3
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.more
  maxPages: 3
`

const listingC = `container: "li.item"
fields:
  - label: title
    selector: span
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.n
  maxPages: 2
`

const listingD = `container: "article"
fields:
  - label: title
    selector: h1
  - label: url
    selector: a
    attribute: href
pagination:
  nextSelector: a.x
  maxPages: 2
`

const detailX = `fields:
  - label: description
    selector: .body
`

func link(id, instructions, detail, lastCrawled string) portalLink {
	return portalLink{ID: id, Title: "link " + id, CrawlInstructions: instructions, JobDetailCrawlInstructions: detail, LastCrawledAt: lastCrawled}
}

func ids(cs []InheritCandidate) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.LinkID)
	}
	return out
}

func TestPickInheritCandidates_ExcludesNewLinkAndUnusableOnes(t *testing.T) {
	links := []portalLink{
		link("new", listingA, "", ""),         // the new link itself
		link("empty", "", "", ""),             // no instructions
		link("blank", "  \n ", "", ""),        // blank instructions
		link("broken", "not: [valid", "", ""), // hand-edited, fails validation
		link("good", listingB, detailX, ""),
	}
	got := pickInheritCandidates(links, "new")
	if !reflect.DeepEqual(ids(got), []string{"good"}) {
		t.Fatalf("got %v", ids(got))
	}
	if got[0].Listing != listingB || got[0].JobDetail != detailX || got[0].Title != "link good" {
		t.Fatalf("candidate = %+v", got[0])
	}
	if len(pickInheritCandidates(nil, "new")) != 0 || len(pickInheritCandidates([]portalLink{link("new", listingA, "", "")}, "new")) != 0 {
		t.Fatal("no siblings must give no candidates")
	}
}

func TestPickInheritCandidates_DropsInvalidJobDetail(t *testing.T) {
	got := pickInheritCandidates([]portalLink{link("a", listingA, "fields: [", "")}, "new")
	if len(got) != 1 || got[0].JobDetail != "" {
		t.Fatalf("invalid detail instructions must be dropped, got %+v", got)
	}
}

func TestPickInheritCandidates_MostRecentlyCrawledFirst(t *testing.T) {
	links := []portalLink{
		link("never", listingA, "", ""),
		link("old", listingB, "", "2026-10-01 10:00:00"),
		link("newest", listingC, "", "2026-10-30T12:00:00Z"), // RFC 3339 also read
		link("mid", listingD, "", "2026-10-15 10:00:00"),
	}
	// "never" sorts last; with the cap of 3 it is the one left out.
	if got := ids(pickInheritCandidates(links, "x")); !reflect.DeepEqual(got, []string{"newest", "mid", "old"}) {
		t.Fatalf("got %v", got)
	}
}

func TestPickInheritCandidates_TiesAndGarbageKeepGivenOrder(t *testing.T) {
	links := []portalLink{
		link("first", listingA, "", "garbage"),
		link("second", listingB, "", ""),
		link("third", listingC, "", "garbage"),
	}
	if got := ids(pickInheritCandidates(links, "x")); !reflect.DeepEqual(got, []string{"first", "second", "third"}) {
		t.Fatalf("got %v", got)
	}
}

func TestPickInheritCandidates_CapsAtThree(t *testing.T) {
	links := []portalLink{
		link("1", listingA, "", "2026-10-04 10:00:00"),
		link("2", listingB, "", "2026-10-03 10:00:00"),
		link("3", listingC, "", "2026-10-02 10:00:00"),
		link("4", listingD, "", "2026-10-01 10:00:00"),
	}
	if got := ids(pickInheritCandidates(links, "x")); !reflect.DeepEqual(got, []string{"1", "2", "3"}) {
		t.Fatalf("got %v", got)
	}
}

func TestPickInheritCandidates_DeduplicatesAndSharesJobDetail(t *testing.T) {
	crlf := strings.ReplaceAll(listingA, "\n", "  \r\n") + "\n\n"
	links := []portalLink{
		link("a1", listingA, "", "2026-10-03 10:00:00"),         // newest, no detail instructions
		link("a2", crlf, detailX, "2026-10-02 10:00:00"),        // same document (line endings, spaces), has detail
		link("b", listingB, "", "2026-10-01 10:00:00"),          // a different document
		link("a3", listingA, "other: x", "2026-09-01 10:00:00"), // same again, invalid detail
	}
	got := pickInheritCandidates(links, "x")
	if !reflect.DeepEqual(ids(got), []string{"a1", "b"}) {
		t.Fatalf("got %v", ids(got))
	}
	if got[0].JobDetail != detailX {
		t.Fatalf("the duplicate's job-detail instructions must be used, got %q", got[0].JobDetail)
	}
	if got[0].Listing != listingA {
		t.Fatal("the first copy's own text is kept")
	}
}

func TestPickInheritCandidates_DuplicatesStillMergeWhenCapIsFull(t *testing.T) {
	links := []portalLink{
		link("1", listingA, "", "2026-10-05 10:00:00"),
		link("2", listingB, "", "2026-10-04 10:00:00"),
		link("3", listingC, "", "2026-10-03 10:00:00"),
		link("dup", listingA, detailX, "2026-10-01 10:00:00"),
	}
	got := pickInheritCandidates(links, "x")
	if len(got) != 3 || got[0].JobDetail != detailX {
		t.Fatalf("got %+v", got)
	}
}

func TestLimitToPages(t *testing.T) {
	req := CrawlRequest{RequestedMaxPages: 5, EffectiveMaxPages: 5, Container: ".job"}
	got := LimitToPages(req, 1)
	if got.RequestedMaxPages != 1 || got.EffectiveMaxPages != 1 || got.Container != ".job" {
		t.Fatalf("got %+v", got)
	}
	if req.RequestedMaxPages != 5 {
		t.Fatal("the original must not change")
	}
	if got := LimitToPages(CrawlRequest{RequestedMaxPages: 1, EffectiveMaxPages: 1}, 3); got.EffectiveMaxPages != 1 {
		t.Fatalf("a smaller request is never raised: %+v", got)
	}
	if got := LimitToPages(CrawlRequest{RequestedMaxPages: 4, EffectiveMaxPages: 4}, 0); got.EffectiveMaxPages != 1 {
		t.Fatalf("n below 1 means 1: %+v", got)
	}
}

func trialItemsPage(n int) CrawlResultPage {
	items := []map[string]any{}
	for i := 0; i < n; i++ {
		items = append(items, map[string]any{"title": "Job " + string(rune('A'+i)), "url": "https://x.example/" + string(rune('a'+i))})
	}
	return CrawlResultPage{URL: "https://x.example/list", Items: items}
}

func TestListingTrialPasses(t *testing.T) {
	cases := []struct {
		name  string
		pages []CrawlResultPage
		want  bool
	}{
		{"many items", []CrawlResultPage{trialItemsPage(5)}, true},
		{"exactly three items (no warning)", []CrawlResultPage{trialItemsPage(3)}, true},
		{"two items warn", []CrawlResultPage{trialItemsPage(2)}, false},
		{"one item warns", []CrawlResultPage{trialItemsPage(1)}, false},
		{"nothing extracted", []CrawlResultPage{{URL: "u"}}, false},
		{"no pages", nil, false},
		{"one item without a url", []CrawlResultPage{{Items: []map[string]any{
			{"title": "A", "url": "https://x/a"}, {"title": "B", "url": "https://x/b"}, {"title": "C", "url": "https://x/c"}, {"title": "D"},
		}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ListingTrialPasses(ClassifyTrialPages(c.pages)); got != c.want {
				t.Fatalf("got %v, want %v (%+v)", got, c.want, ClassifyTrialPages(c.pages))
			}
		})
	}
}

func TestTrialJobURLs(t *testing.T) {
	pages := []CrawlResultPage{{Items: []map[string]any{
		{"title": "A", "url": "https://x/a"},
		{"title": "", "url": "https://x/skipped"},  // no title: not extractable
		{"title": "A again", "url": "https://x/a"}, // repeated url
		{"title": "B", "url": "https://x/b"},
		{"title": "C", "url": "https://x/c"},
	}}}
	if got := TrialJobURLs(pages, 2); !reflect.DeepEqual(got, []string{"https://x/a", "https://x/b"}) {
		t.Fatalf("got %v", got)
	}
	if got := TrialJobURLs(pages, 10); !reflect.DeepEqual(got, []string{"https://x/a", "https://x/b", "https://x/c"}) {
		t.Fatalf("got %v", got)
	}
	if got := TrialJobURLs(nil, 2); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	// Several pages are read in order.
	two := []CrawlResultPage{
		{Items: []map[string]any{{"title": "A", "url": "https://x/a"}}},
		{Items: []map[string]any{{"title": "B", "url": "https://x/b"}}},
	}
	if got := TrialJobURLs(two, 2); !reflect.DeepEqual(got, []string{"https://x/a", "https://x/b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTrialJobURLs_ParallelArraysForm(t *testing.T) {
	pages := []CrawlResultPage{{Results: map[string]any{
		"title": []any{"A", "B", "C"},
		"url":   []any{"https://x/a", "", "https://x/c"},
	}}}
	if got := TrialJobURLs(pages, 5); !reflect.DeepEqual(got, []string{"https://x/a", "https://x/c"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDetailTrialsPass(t *testing.T) {
	// The plan's numbers, written out: 100 runes pass, 99 do not.
	long := strings.Repeat("a", 100)
	short := strings.Repeat("a", 99)
	cases := []struct {
		name   string
		values []map[string]string
		want   bool
	}{
		{"no job opened", nil, false},
		{"empty list", []map[string]string{}, false},
		{"one long enough", []map[string]string{{"description": long}}, true},
		{"one runes short", []map[string]string{{"description": short}}, false},
		{"both long enough", []map[string]string{{"description": long}, {"description": long + "b"}}, true},
		{"one of two too short", []map[string]string{{"description": long}, {"description": short}}, false},
		{"no description field", []map[string]string{{"title": long}}, false},
		{"only spaces around", []map[string]string{{"description": "  " + short + "  "}}, false},
		{"runes, not bytes", []map[string]string{{"description": strings.Repeat("é", 100)}}, true},
		{"blank", []map[string]string{{"description": "   "}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetailTrialsPass(c.values); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestInheritCopies(t *testing.T) {
	cases := []struct {
		listing       bool
		detail        string
		wantL, wantJD bool
	}{
		{true, DetailOK, true, true},
		{true, DetailFailed, true, false},
		{true, DetailNoInstructions, true, false},
		{true, DetailNotTried, true, false},
		{false, DetailOK, false, false},
		{false, DetailFailed, false, false},
		{false, DetailNotTried, false, false},
	}
	for _, c := range cases {
		l, jd := InheritCopies(c.listing, c.detail)
		if l != c.wantL || jd != c.wantJD {
			t.Errorf("InheritCopies(%v, %q) = %v, %v; want %v, %v", c.listing, c.detail, l, jd, c.wantL, c.wantJD)
		}
	}
}

// The factored-out builders must give exactly what the id-based ones always gave.
func TestBuildRequestFromYAML_MatchesTheLinkBasedBuilders(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	lid, err := AddPortalLink(pid, "https://a.example", "A")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkCrawlInstructions(lid, listingA); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkJobDetailCrawlInstructions(lid, detailX); err != nil {
		t.Fatal(err)
	}

	byID, err := BuildCrawlRequest(lid)
	if err != nil {
		t.Fatal(err)
	}
	byText, err := BuildCrawlRequestFromYAML(listingA)
	if err != nil || !reflect.DeepEqual(byID, byText) {
		t.Fatalf("listing: %+v vs %+v (%v)", byID, byText, err)
	}
	if byText.Container != ".job" || byText.NextSelector != "a.next" || byText.RequestedMaxPages != 5 || len(byText.Fields) != 2 {
		t.Fatalf("listing request = %+v", byText)
	}

	jdByID, err := BuildJobDetailCrawlRequest(lid)
	if err != nil {
		t.Fatal(err)
	}
	jdByText, err := BuildJobDetailCrawlRequestFromYAML(detailX)
	if err != nil || !reflect.DeepEqual(jdByID, jdByText) {
		t.Fatalf("detail: %+v vs %+v (%v)", jdByID, jdByText, err)
	}
	if jdByText.EffectiveMaxPages != 1 || jdByText.NextSelector != "" || len(jdByText.Fields) != 1 {
		t.Fatalf("detail request = %+v", jdByText)
	}
}

func TestBuildRequestFromYAML_RefusesEmptyAndInvalidText(t *testing.T) {
	if _, err := BuildCrawlRequestFromYAML(""); !errors.Is(err, ErrNoCrawlInstructions) {
		t.Fatalf("empty listing: %v", err)
	}
	if _, err := BuildCrawlRequestFromYAML("fields: ["); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Fatalf("invalid listing: %v", err)
	}
	if _, err := BuildJobDetailCrawlRequestFromYAML(""); !errors.Is(err, ErrNoJobDetailCrawlInstructions) {
		t.Fatalf("empty detail: %v", err)
	}
	if _, err := BuildJobDetailCrawlRequestFromYAML("fields: ["); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Fatalf("invalid detail: %v", err)
	}
	// A link with nothing stored still reports "no instructions", as before the refactor.
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	lid, _ := AddPortalLink(pid, "https://a.example", "A")
	if _, err := BuildCrawlRequest(lid); !errors.Is(err, ErrNoCrawlInstructions) {
		t.Fatalf("link without instructions: %v", err)
	}
	if _, err := BuildJobDetailCrawlRequest(lid); !errors.Is(err, ErrNoJobDetailCrawlInstructions) {
		t.Fatalf("link without detail instructions: %v", err)
	}
}

func TestListingTrialFailureReason(t *testing.T) {
	cases := []struct {
		name  string
		pages []CrawlResultPage
		want  string
	}{
		{"passes", []CrawlResultPage{trialItemsPage(5)}, ""},
		{"nothing", []CrawlResultPage{{URL: "u"}}, "no items found"},
		{"no pages", nil, "no items found"},
		{"two items", []CrawlResultPage{trialItemsPage(2)}, "only 2 item(s) found"},
		{"one skipped", []CrawlResultPage{{Items: []map[string]any{
			{"title": "A", "url": "https://x/a"}, {"title": "B", "url": "https://x/b"}, {"title": "C", "url": "https://x/c"}, {"title": "D"},
		}}}, "1 of 4 items had no title or url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ListingTrialFailureReason(ClassifyTrialPages(c.pages)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The candidates come from the new link's own portal only.
func TestInheritCandidatesFor_OwnPortalOnly(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("Mine")
	other, _ := CreatePortal("Other")
	newLink, _ := AddPortalLink(pid, "https://mine.example/new", "New")
	sib, _ := AddPortalLink(pid, "https://mine.example/sib", "Sibling")
	stranger, _ := AddPortalLink(other, "https://other.example/x", "Stranger")
	bare, _ := AddPortalLink(pid, "https://mine.example/bare", "Bare")
	for id, text := range map[string]string{sib: listingA, stranger: listingB} {
		if err := UpdatePortalLinkCrawlInstructions(id, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := UpdatePortalLinkJobDetailCrawlInstructions(sib, detailX); err != nil {
		t.Fatal(err)
	}
	_ = bare

	got, err := InheritCandidatesFor(newLink)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LinkID != sib || got[0].Title != "Sibling" || got[0].Listing != listingA || got[0].JobDetail != detailX {
		t.Fatalf("got %+v", got)
	}
	if got, err := InheritCandidatesFor(stranger); err != nil || len(got) != 0 {
		t.Fatalf("the other portal has no other instructed link: %+v %v", got, err)
	}
	if _, err := InheritCandidatesFor("nope"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Fatalf("unknown link: %v", err)
	}
}

func storedFor(t *testing.T, id string) (listing, detail string) {
	t.Helper()
	l, err := GetPortalLinkCrawlInstructions(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := GetPortalLinkJobDetailCrawlInstructions(id)
	if err != nil {
		t.Fatal(err)
	}
	return l, d
}

func TestSaveInheritedInstructions(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	mk := func(title string) string {
		id, err := AddPortalLink(pid, "https://x.example/"+title, title)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// An empty link gets both.
	both := mk("both")
	ls, ds, err := SaveInheritedInstructions(both, listingA, detailX)
	if err != nil || !ls || !ds {
		t.Fatalf("both: %v %v %v", ls, ds, err)
	}
	if l, d := storedFor(t, both); l != listingA || d != detailX {
		t.Fatalf("stored %q / %q", l, d)
	}

	// Listing only when no job-detail text is offered.
	only := mk("only")
	if ls, ds, err := SaveInheritedInstructions(only, listingA, ""); err != nil || !ls || ds {
		t.Fatalf("listing only: %v %v %v", ls, ds, err)
	}
	if l, d := storedFor(t, only); l != listingA || d != "" {
		t.Fatalf("stored %q / %q", l, d)
	}

	// A link that already has listing instructions is left completely alone.
	taken := mk("taken")
	if err := UpdatePortalLinkCrawlInstructions(taken, listingB); err != nil {
		t.Fatal(err)
	}
	if ls, ds, err := SaveInheritedInstructions(taken, listingA, detailX); err != nil || ls || ds {
		t.Fatalf("taken: %v %v %v", ls, ds, err)
	}
	if l, d := storedFor(t, taken); l != listingB || d != "" {
		t.Fatalf("a link with instructions was changed: %q / %q", l, d)
	}

	// Existing job-detail instructions are kept; the listing is still saved.
	keep := mk("keep")
	if err := UpdatePortalLinkJobDetailCrawlInstructions(keep, "fields:\n  - label: description\n    selector: .mine\n"); err != nil {
		t.Fatal(err)
	}
	if ls, ds, err := SaveInheritedInstructions(keep, listingA, detailX); err != nil || !ls || ds {
		t.Fatalf("keep: %v %v %v", ls, ds, err)
	}
	if l, d := storedFor(t, keep); l != listingA || !strings.Contains(d, ".mine") {
		t.Fatalf("stored %q / %q", l, d)
	}

	// Whitespace-only text counts as empty.
	blank := mk("blank")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = '  ' || char(10) || char(9) || ' ' WHERE id = ?`, blank); err != nil {
		t.Fatal(err)
	}
	if ls, _, err := SaveInheritedInstructions(blank, listingA, ""); err != nil || !ls {
		t.Fatalf("blank: %v %v", ls, err)
	}

	// A pending "does this page list jobs?" question is settled by the save.
	asked := mk("asked")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET jobs_question_asked_at = datetime('now'), jobs_question_note = 'n' WHERE id = ?`, asked); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SaveInheritedInstructions(asked, listingA, ""); err != nil {
		t.Fatal(err)
	}
	var q any
	if err := db.JobsDB.QueryRow(`SELECT jobs_question_asked_at FROM portal_links WHERE id = ?`, asked).Scan(&q); err != nil || q != nil {
		t.Fatalf("question not settled: %v %v", q, err)
	}
}

func TestSaveInheritedInstructions_RefusesBadTextAndUnknownLinks(t *testing.T) {
	setupPortalDB(t)
	pid, _ := CreatePortal("P")
	id, _ := AddPortalLink(pid, "https://x.example/a", "A")

	if _, _, err := SaveInheritedInstructions(id, "fields: [", ""); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Fatalf("invalid listing: %v", err)
	}
	if _, _, err := SaveInheritedInstructions(id, listingA, "fields: ["); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Fatalf("invalid detail: %v", err)
	}
	if _, _, err := SaveInheritedInstructions("nope", listingA, ""); !errors.Is(err, ErrUnknownPortalLink) {
		t.Fatalf("unknown link: %v", err)
	}
	// Nothing was written by any of the refused calls.
	if l, d := storedFor(t, id); l != "" || d != "" {
		t.Fatalf("a refused call wrote %q / %q", l, d)
	}
}
