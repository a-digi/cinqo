package portal

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal/linkrule"
)

func ruleFor(host string, include ...string) *linkrule.Rule {
	return &linkrule.Rule{Host: host, Include: include}
}

func newLinkOn(t *testing.T, url string) string {
	t.Helper()
	pid, _ := CreatePortal("Board")
	id, err := AddPortalLink(pid, url, "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestANewLinkIsCrawledByInstructionsWithNoRule(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	got, err := GetLinkCrawlSettings(id)
	if err != nil || got.CrawlMethod != CrawlMethodInstructions || got.Rules != nil {
		t.Errorf("%+v %v", got, err)
	}
	if l := loginLink(t, id); l.CrawlMethod != CrawlMethodInstructions || l.LinkRules != nil {
		t.Errorf("the portals list: %q %v", l.CrawlMethod, l.LinkRules)
	}
}

func TestSavingARuleAndTheLinksMethodShowsUpEverywhere(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	rule := &linkrule.Rule{Host: "Board.Example", Include: []string{"board.example/jobs/*"}, Exclude: []string{"board.example/jobs/featured"}}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, rule, false); err != nil {
		t.Fatal(err)
	}
	want := &linkrule.Rule{Host: "board.example", Include: []string{"board.example/jobs/*"}, Exclude: []string{"board.example/jobs/featured"}} // host stored lower-case
	got, _ := GetLinkCrawlSettings(id)
	if got.CrawlMethod != CrawlMethodLinks || got.Rules == nil || got.Rules.Host != want.Host || len(got.Rules.Include) != 1 || got.Rules.Exclude[0] != want.Exclude[0] {
		t.Errorf("%+v %+v", got, got.Rules)
	}
	if l := loginLink(t, id); l.CrawlMethod != CrawlMethodLinks || l.LinkRules == nil || l.LinkRules.Include[0] != "board.example/jobs/*" {
		t.Errorf("the portals list: %q %+v", l.CrawlMethod, l.LinkRules)
	}
	// the link's own url, title and instructions are not touched
	if l := loginLink(t, id); l.URL != "https://board.example/jobs" || l.Title != "Jobs" {
		t.Errorf("%+v", l)
	}
}

func TestARuleMustBeForTheLinksOwnHost(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	for name, host := range map[string]string{"another site": "other.example", "www is another host": "www.board.example", "empty": "", "port is part of the host": "board.example:8080"} {
		r := ruleFor(host, "board.example/jobs/*")
		if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, r, false); !errors.Is(err, ErrLinkRulesHost) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a link with a port: the rule carries it
	pid, _ := CreatePortal("Local")
	pl, _ := AddPortalLink(pid, "http://board.example:8080/jobs", "Local")
	if err := SaveLinkCrawlSettings(pl, CrawlMethodLinks, ruleFor("board.example:8080", "board.example:8080/jobs/*"), false); err != nil {
		t.Errorf("host with a port: %v", err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.Rules != nil || got.CrawlMethod != CrawlMethodInstructions {
		t.Errorf("a refused save changed the link: %+v", got)
	}
}

func TestAnInvalidRuleIsRefusedAndNothingIsWritten(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	good := ruleFor("board.example", "board.example/jobs/*")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, good, false); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]*linkrule.Rule{
		"no include":          {Host: "board.example"},
		"bad pattern":         ruleFor("board.example", "board.example/a b"),
		"pattern of a host":   ruleFor("board.example", "other.example/jobs/*"),
		"wildcard host":       ruleFor("board.example", "*.example/jobs"),
		"empty pattern":       ruleFor("board.example", ""),
		"too long a pattern":  ruleFor("board.example", "board.example/"+strings.Repeat("a", linkrule.MaxPatternLen)),
		"too many includes":   ruleFor("board.example", manyPatterns(linkrule.MaxIncludes+1)...),
		"too many exclusions": {Host: "board.example", Include: []string{"board.example/*"}, Exclude: manyPatterns(linkrule.MaxExcludes + 1)},
	} {
		err := SaveLinkCrawlSettings(id, CrawlMethodLinks, bad, false)
		if !errors.Is(err, ErrLinkRulesInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	got, _ := GetLinkCrawlSettings(id)
	if got.Rules == nil || got.Rules.Include[0] != "board.example/jobs/*" {
		t.Errorf("a refused save replaced the stored rule: %+v", got.Rules)
	}
}

func manyPatterns(n int) []string {
	var s []string
	for i := 0; i < n; i++ {
		s = append(s, "board.example/p"+string(rune('a'+i%26))+"/*")
	}
	return s
}

func TestTheLinksMethodNeedsARule(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, nil, false); !errors.Is(err, ErrLinkRulesNeeded) {
		t.Errorf("no rule at all: %v", err)
	}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, nil, true); !errors.Is(err, ErrLinkRulesNeeded) {
		t.Errorf("clearing while choosing links: %v", err)
	}
	// with a stored rule the method can be switched on without sending it again
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, nil, false); err != nil {
		t.Errorf("stored rule, method links: %v", err)
	}
	got, _ := GetLinkCrawlSettings(id)
	if got.CrawlMethod != CrawlMethodLinks || got.Rules == nil {
		t.Errorf("%+v", got)
	}
}

func TestSwitchingBackAndClearingAreAlwaysAllowedAndKeepWhatTheyShould(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	// back to instructions: the rule stays stored
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, nil, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodInstructions || got.Rules == nil {
		t.Errorf("%+v", got)
	}
	// clear
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, nil, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.Rules != nil || got.CrawlMethod != CrawlMethodInstructions {
		t.Errorf("%+v", got)
	}
	// a rule and a clear together is a contradiction
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, ruleFor("board.example", "board.example/*"), true); !errors.Is(err, ErrLinkRulesInvalid) {
		t.Errorf("both: %v", err)
	}
}

func TestUnknownMethodsAndLinksAreRefused(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	for _, m := range []string{"", "Links", "colly", "instructions ", "ai"} {
		if err := SaveLinkCrawlSettings(id, m, nil, false); !errors.Is(err, ErrInvalidCrawlMethod) {
			t.Errorf("method %q: %v", m, err)
		}
	}
	if err := SaveLinkCrawlSettings("nobody", CrawlMethodInstructions, nil, false); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
	if _, err := GetLinkCrawlSettings("nobody"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("get unknown link: %v", err)
	}
}

func TestTheRuleIsBoundedInSize(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	// 20 includes + 20 excludes of 300 characters stay inside the limit
	long := func(i int) string {
		return "board.example/" + strings.Repeat(string(rune('a'+i%26)), linkrule.MaxPatternLen-len("board.example/"))
	}
	var inc, exc []string
	for i := 0; i < linkrule.MaxIncludes; i++ {
		inc = append(inc, long(i))
	}
	for i := 0; i < linkrule.MaxExcludes; i++ {
		exc = append(exc, long(i+7))
	}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, &linkrule.Rule{Host: "board.example", Include: inc, Exclude: exc}, false); err != nil {
		t.Errorf("the biggest valid rule: %v", err)
	}
	// the worst case for the JSON size: characters that JSON escapes (< becomes \u003c, six bytes)
	worst := "board.example/" + strings.Repeat("<", linkrule.MaxPatternLen-len("board.example/"))
	var winc, wexc []string
	for i := 0; i < linkrule.MaxIncludes; i++ {
		winc = append(winc, worst)
	}
	for i := 0; i < linkrule.MaxExcludes; i++ {
		wexc = append(wexc, worst)
	}
	id2 := newLinkOn(t, "https://board.example/other")
	if err := SaveLinkCrawlSettings(id2, CrawlMethodLinks, &linkrule.Rule{Host: "board.example", Include: winc, Exclude: wexc}, false); err != nil {
		t.Errorf("the biggest valid rule in JSON-escaped characters: %v", err)
	}
	if MaxLinkRulesJSON < (linkrule.MaxIncludes+linkrule.MaxExcludes)*linkrule.MaxPatternLen*6 {
		t.Errorf("MaxLinkRulesJSON %d cannot hold the biggest valid rule", MaxLinkRulesJSON)
	}
}

func TestALoginLinkNeverGetsTheLinkCrawler(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkLogin(id, ptr(true), nil); err != nil {
		t.Fatal(err)
	}
	r := ruleFor("board.example", "board.example/jobs/*")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, r, false); !errors.Is(err, ErrLinkRulesLogin) {
		t.Errorf("method links: %v", err)
	}
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, r, false); !errors.Is(err, ErrLinkRulesLogin) {
		t.Errorf("storing a rule: %v", err)
	}
	// going back to instructions and clearing stay possible
	if err := SaveLinkCrawlSettings(id, CrawlMethodInstructions, nil, true); err != nil {
		t.Errorf("clearing on a login link: %v", err)
	}
}

func TestSwitchingTheLoginOnPutsALinkBackToInstructions(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkLogin(id, ptr(true), nil); err != nil {
		t.Fatal(err)
	}
	got, _ := GetLinkCrawlSettings(id)
	if got.CrawlMethod != CrawlMethodInstructions || got.Rules == nil {
		t.Errorf("a login link must be back on instructions, its rule kept: %+v", got)
	}
	// switching the login off again does not silently re-enable the link crawler
	if err := UpdatePortalLinkLogin(id, ptr(false), nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodInstructions {
		t.Errorf("%+v", got)
	}
	// a login URL change alone does not touch the method
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkLogin(id, nil, ptr("https://board.example/login")); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodLinks {
		t.Errorf("a login page change alone must not change the method: %+v", got)
	}
}

func TestMovingALinkToAnotherHostTakesItsRuleAway(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	// the same host, another path (and only a title change): the rule stays
	if err := UpdatePortalLink(id, ptr("https://board.example/careers"), nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodLinks || got.Rules == nil {
		t.Errorf("same host: %+v", got)
	}
	if err := UpdatePortalLink(id, nil, ptr("Renamed")); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodLinks || got.Rules == nil {
		t.Errorf("title only: %+v", got)
	}
	// a host written in other letters is the same host
	if err := UpdatePortalLink(id, ptr("https://BOARD.example/x"), nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.Rules == nil {
		t.Errorf("same host in capitals: %+v", got)
	}
	// another site: rule and method go
	if err := UpdatePortalLink(id, ptr("https://elsewhere.example/jobs"), nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetLinkCrawlSettings(id); got.CrawlMethod != CrawlMethodInstructions || got.Rules != nil {
		t.Errorf("another host: %+v", got)
	}
	// a link without a rule can be moved freely
	other := newLinkOn(t, "https://a.example/x")
	if err := UpdatePortalLink(other, ptr("https://b.example/x"), nil); err != nil {
		t.Errorf("a link without a rule: %v", err)
	}
}

func TestDeletingTheLinkLeavesNothingOfItsRule(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	if err := RemovePortalLink(id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM portal_links WHERE id = ?`, id).Scan(&n); err != nil || n != 0 {
		t.Errorf("the link is still there: %d %v", n, err)
	}
	if _, err := GetLinkCrawlSettings(id); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("%v", err)
	}
}

func TestACorruptStoredRuleReadsAsNoRule(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET link_rules = 'not json' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	got, err := GetLinkCrawlSettings(id)
	if err != nil || got.Rules != nil {
		t.Errorf("%+v %v", got, err)
	}
	if l := loginLink(t, id); l.LinkRules != nil {
		t.Errorf("the list must not fail on a corrupt rule: %+v", l.LinkRules)
	}
	// choosing links with only a corrupt rule stored needs a new rule
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, nil, false); !errors.Is(err, ErrLinkRulesNeeded) {
		t.Errorf("%v", err)
	}
}

func TestTheNewErrorsAreCallerMistakes(t *testing.T) {
	// WritePortalLinkAwareError maps them to 400, never to a server error.
	for _, e := range []error{ErrInvalidCrawlMethod, ErrLinkRulesNeeded, ErrLinkRulesLogin, ErrLinkRulesHost, ErrLinkRulesInvalid, ErrLinkRulesTooLarge} {
		rec := httptest.NewRecorder()
		WritePortalLinkAwareError(rec, "x", e)
		if rec.Code != 400 {
			t.Errorf("%v -> %d", e, rec.Code)
		}
	}
}
