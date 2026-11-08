package portal

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func originOf(t *testing.T, linkID string) string {
	t.Helper()
	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == linkID {
				return l.JobDetailInstructionsOrigin
			}
		}
	}
	t.Fatalf("link %s not listed", linkID)
	return ""
}

func originLink(t *testing.T) string {
	t.Helper()
	setupPortalDB(t)
	pid, err := CreatePortal("P")
	if err != nil {
		t.Fatal(err)
	}
	lid, err := AddPortalLink(pid, "https://a.example", "A")
	if err != nil {
		t.Fatal(err)
	}
	return lid
}

func TestTheOriginOfJobDetailInstructionsIsRecorded(t *testing.T) {
	lid := originLink(t)
	if got := originOf(t, lid); got != "" {
		t.Fatalf("a new link has no instructions and no origin, got %q", got)
	}
	for _, origin := range []string{JobDetailOriginPicked, JobDetailOriginAI, JobDetailOriginManual, JobDetailOriginInherited} {
		if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom(lid, detailX, origin); err != nil {
			t.Fatalf("%s: %v", origin, err)
		}
		if got := originOf(t, lid); got != origin {
			t.Errorf("stored %q, listed %q", origin, got)
		}
	}
	// the plain function (callers that do not know who wrote the text) records nothing
	if err := UpdatePortalLinkJobDetailCrawlInstructions(lid, detailX); err != nil {
		t.Fatal(err)
	}
	if got := originOf(t, lid); got != "" {
		t.Errorf("an unknown origin must be empty, got %q", got)
	}
}

func TestAnUnknownOriginOrInvalidTextChangesNothing(t *testing.T) {
	lid := originLink(t)
	if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom(lid, detailX, JobDetailOriginPicked); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom(lid, detailX, "evil"); err == nil {
		t.Error("an unknown origin was accepted")
	}
	err := UpdatePortalLinkJobDetailCrawlInstructionsFrom(lid, "fields: []", JobDetailOriginAI)
	if !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Errorf("invalid text: %v", err)
	}
	if got := originOf(t, lid); got != JobDetailOriginPicked {
		t.Errorf("a refused save changed the origin to %q", got)
	}
	if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom("nope", detailX, JobDetailOriginAI); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
}

func TestInheritedInstructionsRecordTheirOrigin(t *testing.T) {
	lid := originLink(t)
	if _, jd, err := SaveInheritedInstructions(lid, listingA, detailX); err != nil || !jd {
		t.Fatalf("%v %v", jd, err)
	}
	if got := originOf(t, lid); got != JobDetailOriginInherited {
		t.Errorf("origin %q", got)
	}
}

func TestStartFreshClearsTheOrigin(t *testing.T) {
	lid := originLink(t)
	if err := UpdatePortalLinkJobDetailCrawlInstructionsFrom(lid, detailX, JobDetailOriginPicked); err != nil {
		t.Fatal(err)
	}
	if _, err := StartFresh(lid, false); err != nil {
		t.Fatal(err)
	}
	if got := originOf(t, lid); got != JobDetailOriginPicked {
		t.Errorf("starting fresh without clearing the instructions must keep them (and their origin), got %q", got)
	}
	if _, err := StartFresh(lid, true); err != nil {
		t.Fatal(err)
	}
	if got := originOf(t, lid); got != "" {
		t.Errorf("origin after clearing the instructions: %q", got)
	}
	if text, _ := GetPortalLinkJobDetailCrawlInstructions(lid); strings.TrimSpace(text) != "" {
		t.Errorf("instructions not cleared: %q", text)
	}
}

// The AI's tool and the text box reach the one save function with their own origin; the tool runs inside an MCP server a unit test does
// not start, so the call sites are pinned by reading the source (the same way the no-page-loaded guard is).
func TestTheAIToolAndTheTextBoxSaveWithTheirOwnOrigin(t *testing.T) {
	for file, want := range map[string]string{
		"portals.go":                 "UpdatePortalLinkJobDetailCrawlInstructionsFrom(args.PortalLinkID, args.Instructions, JobDetailOriginAI)",
		"handlers/links.go":          "UpdatePortalLinkJobDetailCrawlInstructionsFrom(body.ID, *body.JobDetailCrawlInstructions, portal.JobDetailOriginManual)",
		"../crawl/jobdetail_pick.go": "UpdatePortalLinkJobDetailCrawlInstructionsFrom(pc.linkID, text, portal.JobDetailOriginPicked)",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s no longer saves with the right origin: want %q", file, want)
		}
	}
}

// ---- the title option and the request ----

const withTitleDetail = "fields:\n  - label: description\n    selector: .d\n  - label: title\n    selector: h1\nreplaceTitle: true\n"

func TestReplaceTitleNeedsATitleFieldAndStaysOutOfTheBrowsersRequest(t *testing.T) {
	if err := validateJobDetailCrawlInstructions(withTitleDetail); err != nil {
		t.Fatalf("replaceTitle with a title field: %v", err)
	}
	noTitle := "fields:\n  - label: description\n    selector: .d\nreplaceTitle: true\n"
	if err := validateJobDetailCrawlInstructions(noTitle); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Errorf("replaceTitle without a title field: %v", err)
	}
	req, err := BuildJobDetailCrawlRequestFromYAML(withTitleDetail)
	if err != nil || !req.ReplaceTitle {
		t.Fatalf("%v %+v", err, req)
	}
	raw, _ := jsonMarshal(req)
	if strings.Contains(raw, "replaceTitle") || strings.Contains(raw, "ReplaceTitle") {
		t.Errorf("the Browser must not be sent the option: %s", raw)
	}
	plain, _ := BuildJobDetailCrawlRequestFromYAML(detailX)
	if plain.ReplaceTitle {
		t.Error("old instructions never replace the title")
	}
}

func TestSanitizeTitleIsTheListingsCleaning(t *testing.T) {
	if got := SanitizeTitle("  Java 17: Developer  "); got != sanitizeTitle("  Java 17: Developer  ") || strings.ContainsAny(got, "0123456789:") {
		t.Errorf("got %q", got)
	}
}
