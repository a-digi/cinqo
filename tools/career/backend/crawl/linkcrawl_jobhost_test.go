package crawl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const jobHostCandidates = `[{"url":"https://jobs.other.example/jobanzeige/1001","texts":["Posting 1001"]},
{"url":"https://jobs.other.example/jobanzeige/1002","texts":["Posting 1002"]},
{"url":"https://jobs.other.example/jobanzeige/1003","texts":["Posting 1003"]},
{"url":"https://duapune.com/jobs/7-own","texts":["Own"]}]`

// putRulesOnFakeCore is putRules without its own core address, so the Browser tool's fake (Katana selected) answers.
func putRulesOnFakeCore(t *testing.T, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/portal-links/link-crawl/rules", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-5"})
	rec := httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, req)
	return rec.Code, rec.Body.String()
}

func TestLearnOnLinksOfTheConfirmedJobHostMakesARuleThatNamesIt(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	picks := []string{"https://jobs.other.example/jobanzeige/1001", "https://jobs.other.example/jobanzeige/1002"}

	// not confirmed: refused, and the message says what the way on is
	code, resp := post(t, LinkCrawlLearnHandler, "/x", learnReq(id, picks, nil, jobHostCandidates), true)
	if code != http.StatusBadRequest || !strings.Contains(resp, "own site (duapune.com)") || strings.Contains(resp, "confirmed job site") {
		t.Fatalf("unconfirmed: %d %s", code, resp)
	}
	if err := portal.SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	code, resp = post(t, LinkCrawlLearnHandler, "/x", learnReq(id, picks, nil, jobHostCandidates), true)
	if code != 200 {
		t.Fatalf("confirmed: %d %s", code, resp)
	}
	var got learnResponse
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if got.Rule.Host != "duapune.com" || got.Rule.JobHost != "jobs.other.example" || len(got.Rule.Include) != 1 || !strings.HasPrefix(got.Rule.Include[0], "jobs.other.example/jobanzeige/") || got.MatchCount != 3 {
		t.Errorf("the rule: %+v (%d matches)", got.Rule, got.MatchCount)
	}
	// picks on the own site still make an ordinary rule with no job host
	code, resp = post(t, LinkCrawlLearnHandler, "/x", learnReq(id, []string{"https://duapune.com/jobs/7-own", "https://duapune.com/jobs/8-own"}, nil, "[]"), true)
	var own learnResponse
	if err := json.Unmarshal([]byte(resp), &own); code != 200 || err != nil || own.Rule.JobHost != "" || own.Rule.Host != "duapune.com" {
		t.Errorf("own site picks: %d %v %+v", code, err, own.Rule)
	}
	// a third host, and a mix of the two hosts, are still refused (a rule's picks are on one host)
	for name, p := range map[string][]string{
		"third host": {"https://third.example/jobs/1", "https://third.example/jobs/2"},
		"mixed":      {"https://duapune.com/jobs/7-own", "https://jobs.other.example/jobanzeige/1001"},
	} {
		code, resp := post(t, LinkCrawlLearnHandler, "/x", learnReq(id, p, nil, "[]"), true)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, resp)
		}
	}
	code, resp = post(t, LinkCrawlLearnHandler, "/x", learnReq(id, []string{"https://third.example/jobs/1", "https://third.example/jobs/2"}, nil, "[]"), true)
	if !strings.Contains(resp, "confirmed job site (jobs.other.example)") {
		t.Errorf("the refusal names the confirmed site: %s", resp)
	}
}

func TestTheRulesRouteStoresARuleWithAJobHostOnlyForTheConfirmedOne(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	rule := `{"host":"duapune.com","jobHost":"jobs.other.example","include":["jobs.other.example/jobanzeige/#"]}`
	body := `{"portalLinkId":"` + id + `","crawlMethod":"links","rules":` + rule + `}`
	if code, resp := putRulesOnFakeCore(t, body); code != http.StatusBadRequest {
		t.Fatalf("unconfirmed: %d %s", code, resp)
	}
	if s, _ := portal.GetLinkCrawlSettings(id); s.Rules != nil {
		t.Fatal("a refused save stored a rule")
	}
	if err := portal.SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if code, resp := putRulesOnFakeCore(t, body); code != 200 {
		t.Fatalf("confirmed: %d %s", code, resp)
	}
	if s, _ := portal.GetLinkCrawlSettings(id); s.Rules == nil || s.Rules.JobHost != "jobs.other.example" || s.Rules.Host != "duapune.com" {
		t.Errorf("stored: %+v", s.Rules)
	}
	// the same rule for another job host than the confirmed one is refused again
	other := `{"portalLinkId":"` + id + `","crawlMethod":"links","rules":{"host":"duapune.com","jobHost":"third.example","include":["third.example/jobanzeige/#"]}}`
	if code, _ := putRulesOnFakeCore(t, other); code != http.StatusBadRequest {
		t.Errorf("a job host that is not the confirmed one: %d", code)
	}
}

func TestCandidatesNameTheConfirmedJobHostAndMarkItAllowed(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = `{"finalUrl":"https://duapune.com/search","title":"Listing","truncated":false,"droppedPrivate":0,"links":[
 {"url":"https://duapune.com/","texts":["home"]},
 {"url":"https://jobs.other.example/jobanzeige/1001","texts":["A"]},{"url":"https://jobs.other.example/jobanzeige/1002","texts":["B"]},{"url":"https://jobs.other.example/jobanzeige/1003","texts":["C"]}]}`
	read := func() (jobHost string, allowed bool) {
		code, resp := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
		if code != 200 {
			t.Fatalf("%d %s", code, resp)
		}
		var got struct {
			JobHost    string `json:"jobHost"`
			OtherHosts []struct {
				Host    string `json:"host"`
				Allowed bool   `json:"allowed"`
			} `json:"otherHosts"`
		}
		if err := json.Unmarshal([]byte(resp), &got); err != nil || len(got.OtherHosts) != 1 {
			t.Fatalf("%v %s", err, resp)
		}
		return got.JobHost, got.OtherHosts[0].Allowed
	}
	if jh, al := read(); jh != "" || al {
		t.Errorf("unconfirmed: %q %v", jh, al)
	}
	if err := portal.SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if jh, al := read(); jh != "jobs.other.example" || !al {
		t.Errorf("confirmed: %q %v", jh, al)
	}
}

func TestAJobPageMayBeOnTheOwnHostOrTheConfirmedJobHostOnly(t *testing.T) {
	const link = "https://board.example/jobs"
	ok := []string{"https://board.example/job/1", "https://jobs.other.example/jobanzeige/9", "http://JOBS.other.example/x"}
	bad := []string{"https://third.example/x", "https://jobs.other.example.evil.example/x", "https://evil.example/https://jobs.other.example/",
		"https://user:pw@jobs.other.example/x", "ftp://jobs.other.example/x", "javascript:alert(1)", "//jobs.other.example/x", "https://jobs.other.example:8443/x"}
	for _, u := range ok {
		if _, err := jobSitePage(link, "jobs.other.example", u); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
	for _, u := range bad {
		if _, err := jobSitePage(link, "jobs.other.example", u); err == nil {
			t.Errorf("%q must be refused", u)
		}
	}
	// without a confirmed job host the job host's page is refused, and the own host still passes: what it always did
	if _, err := jobSitePage(link, "", "https://jobs.other.example/jobanzeige/9"); err == nil {
		t.Error("unconfirmed job host")
	}
	if _, err := jobSitePage(link, "", "https://board.example/job/1"); err != nil {
		t.Errorf("own host: %v", err)
	}
	// the pages of the LIST are still the own host only
	if _, err := ownSitePage(link, "https://jobs.other.example/jobsuche/"); err == nil {
		t.Error("a list page on the job host must be refused")
	}
}

func TestPickCandidatesLoadsASavedJobOnTheConfirmedJobHostAndStillRefusesAnyOther(t *testing.T) {
	id, ids := pickLink(t)
	f := newFakeCore(t, "rod")
	f.detailPage = func(map[string]any) (int, string) { return 200, pickPageJSON }
	setSource := func(u string) {
		t.Helper()
		if _, err := db.JobsDB.Exec(`UPDATE jobs SET source_url = ? WHERE id = ?`, u, ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	body := `{"portalLinkId":"` + id + `","jobId":"` + ids[0] + `"}`
	setSource("https://jobs.other.example/jobanzeige/1")
	if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 400 || !strings.Contains(out, "own site") {
		t.Fatalf("unconfirmed: %d %s", code, out)
	}
	if len(f.detailBodies) != 0 {
		t.Fatal("an unconfirmed host's page was loaded")
	}
	if err := portal.SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 200 {
		t.Fatalf("confirmed: %d %s", code, out)
	}
	if len(f.detailBodies) != 1 {
		t.Errorf("the job host's page was not loaded: %d", len(f.detailBodies))
	}
	setSource("https://evil.example/x")
	if code, out := postPick(t, JobDetailCandidatesHandler, "/x", body, true); code != 400 || !strings.Contains(out, "own site") {
		t.Errorf("another host after confirming one: %d %s", code, out)
	}
}
