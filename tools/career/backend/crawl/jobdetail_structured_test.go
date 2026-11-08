package crawl

import (
	"net/http"
	"strings"
	"testing"

	"career-tool-backend/portal"
)

const structuredChoices = `{"descriptionSelector":".body","fields":[{"label":"company","source":"jsonld","path":"hiringOrganization"},{"label":"postedAt","source":"jsonld","path":"datePosted"}]}`
const structuredRuleChoices = `{"description":{"parts":[{"source":"jsonld","path":"description"}]}}`

func newBrowserSays(f *fakeCore) {
	f.capabilities = func() (int, string) { return 200, `{"structuredFields":true,"descriptionRule":true}` }
}

func TestCandidatesSayWhetherTheBrowserCanReadStructuredData(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	supported := func() bool {
		t.Helper()
		code, out := postPick(t, JobDetailCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
		if code != 200 {
			t.Fatalf("%d %s", code, out)
		}
		return strings.Contains(out, `"structuredSupported":true`)
	}
	if supported() {
		t.Error("an older Browser (no capabilities route) must read as not supported")
	}
	for name, fn := range map[string]func() (int, string){
		"empty answer":            func() (int, string) { return 200, `{}` },
		"garbage":                 func() (int, string) { return 200, `not json` },
		"server error":            func() (int, string) { return 500, `boom` },
		"unauthorised":            func() (int, string) { return 401, `` },
		"flag is false":           func() (int, string) { return 200, `{"structuredFields":false}` },
		"an error that says yes":  func() (int, string) { return 500, `{"structuredFields":true}` },
		"not found that says yes": func() (int, string) { return 404, `{"structuredFields":true}` },
	} {
		f.capabilities = fn
		if supported() {
			t.Errorf("%s read as supported", name)
		}
	}
	newBrowserSays(f)
	if !supported() {
		t.Error("a Browser that says so is supported")
	}
}

func TestSavingStructuredPicksNeedsAnUpToDateBrowser(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	before, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	for _, choices := range []string{structuredChoices, structuredRuleChoices} {
		body := `{"portalLinkId":"` + id + `","choices":` + choices + `}`
		code, out := postPick(t, JobDetailSaveHandler, "/x", body, true)
		if code != http.StatusConflict || !strings.Contains(out, "browser_too_old") || !strings.Contains(out, "update the Browser tool") {
			t.Fatalf("an older Browser: %d %s", code, out)
		}
		if got, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id); got != before {
			t.Fatal("a refused save changed the stored instructions")
		}
	}
	// no session: the probe cannot be made, so nothing is saved
	if code, _ := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+structuredChoices+`}`, false); code != http.StatusUnauthorized {
		t.Errorf("no session: %d", code)
	}
	newBrowserSays(f)
	code, out := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+structuredChoices+`}`, true)
	if code != 200 {
		t.Fatalf("a current Browser: %d %s", code, out)
	}
	got, _ := portal.GetPortalLinkJobDetailCrawlInstructions(id)
	if !strings.Contains(got, "source: jsonld") || !strings.Contains(got, "path: hiringOrganization") {
		t.Errorf("stored:\n%s", got)
	}
}

func TestPlainPicksNeverAskTheBrowserWhatItCanDo(t *testing.T) {
	id, _ := pickLink(t)
	f := newFakeCore(t, "rod")
	if code, out := postPick(t, JobDetailSaveHandler, "/x", `{"portalLinkId":"`+id+`","choices":{"descriptionSelector":".body"}}`, true); code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	if f.capsCalls != 0 {
		t.Errorf("a save without structured sources asked the Browser %d times", f.capsCalls)
	}
	// and the preview is pure even for structured choices
	if code, out := postPick(t, JobDetailPreviewHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+structuredChoices+`}`, false); code != 200 || !strings.Contains(out, `"valid":true`) {
		t.Errorf("preview: %d %s", code, out)
	}
	if f.capsCalls != 0 {
		t.Errorf("a preview asked the Browser")
	}
}

func TestTestingStructuredPicksNeedsAnUpToDateBrowserAndSendsTheSources(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.paginated = pickExtractPage(map[string]string{"company": "Acme GmbH", "postedAt": "2026-10-28T09:00:00+00:00", "description": longDesc("x")}, "")
	body := `{"portalLinkId":"` + id + `","choices":` + structuredChoices + `,"jobIds":["` + ids[0] + `"]}`
	code, out := postPick(t, JobDetailTestHandler, "/x", body, true)
	if code != http.StatusConflict || !strings.Contains(out, "browser_too_old") {
		t.Fatalf("an older Browser: %d %s", code, out)
	}
	if len(f.paginatedBodies) != 0 {
		t.Fatal("the extraction was asked for although the Browser cannot do it")
	}
	newBrowserSays(f)
	code, out = postPick(t, JobDetailTestHandler, "/x", body, true)
	if code != 200 {
		t.Fatalf("a current Browser: %d %s", code, out)
	}
	if len(f.paginatedBodies) != 1 {
		t.Fatalf("Browser calls: %d", len(f.paginatedBodies))
	}
	fields, _ := f.paginatedBodies[0]["fields"].([]any)
	var sawCompany bool
	for _, raw := range fields {
		fld, _ := raw.(map[string]any)
		if fld["label"] == "company" {
			sawCompany = true
			if fld["source"] != "jsonld" || fld["path"] != "hiringOrganization" || fld["selector"] != "" {
				t.Errorf("company field sent as %v", fld)
			}
		}
	}
	if !sawCompany {
		t.Errorf("fields: %v", fields)
	}
	// the date is cleaned like any other
	if !strings.Contains(out, `"postedAtClean":"2026-10-28"`) {
		t.Errorf("date: %s", out)
	}
	// a rule whose part is structured goes the same way
	f.capabilities = nil
	code, out = postPick(t, JobDetailTestHandler, "/x", `{"portalLinkId":"`+id+`","choices":`+structuredRuleChoices+`,"jobIds":["`+ids[0]+`"]}`, true)
	if code != http.StatusConflict || !strings.Contains(out, "browser_too_old") {
		t.Errorf("a structured rule part on an older Browser: %d %s", code, out)
	}
}

func TestADetailCrawlSendsTheStructuredSourcesAndSavesWhatTheyGave(t *testing.T) {
	yamlText := "fields:\n  - label: company\n    source: jsonld\n    path: hiringOrganization\n  - label: postedAt\n    source: jsonld\n    path: datePosted\ndescriptionRule:\n  parts:\n    - source: jsonld\n      path: description\n"
	id := linkWithOrigin(t, yamlText, portal.JobDetailOriginPicked)
	f := newFakeCore(t, "katana")
	f.paginated = pickExtractPage(map[string]string{"company": "Acme GmbH", "postedAt": "2026-10-28T09:00:00+00:00", "description": "Build things.\n\n- Go\n- SQL"}, "")
	crawlDetails(t, id)
	if got := jobFields(t, id)["Backend Developer"]; got != [3]string{"Acme GmbH", "", "2026-10-28"} {
		t.Errorf("filled %v", got)
	}
	if d := descriptionOf(t, id); d != "Build things.\n\n- Go\n- SQL" {
		t.Errorf("description %q", d)
	}
	if len(f.paginatedBodies) != 1 {
		t.Fatalf("calls %d", len(f.paginatedBodies))
	}
	rule, _ := f.paginatedBodies[0]["descriptionRule"].(map[string]any)
	parts, _ := rule["parts"].([]any)
	part, _ := parts[0].(map[string]any)
	if part["source"] != "jsonld" || part["path"] != "description" || part["include"] != nil {
		t.Errorf("part %v", part)
	}
}
