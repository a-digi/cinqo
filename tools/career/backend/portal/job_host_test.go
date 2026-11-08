package portal

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal/linkrule"
)

func TestJobHostsAreNormalisedAndOnlyPublicHostNamesPass(t *testing.T) {
	const link = "https://www.board.example/jobs"
	good := map[string]string{
		"jobs.other.example":                     "jobs.other.example",
		"  Jobs.Other.EXAMPLE ":                  "jobs.other.example",
		"jobs.other.example:8443":                "jobs.other.example:8443",
		"www.xn--jobbrse-stellenangebote-blc.de": "www.xn--jobbrse-stellenangebote-blc.de",
		"a-b.c1.example.org":                     "a-b.c1.example.org",
		"":                                       "",
		"   ":                                    "",
		"board.example":                          "board.example", // another name than www.board.example
		"www.board.example:8080":                 "www.board.example:8080",
	}
	for in, want := range good {
		got, err := NormalizeJobHost(in, link)
		if err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	bad := []string{
		"www.board.example", "WWW.Board.Example", // the link's own host
		"localhost", "foo.localhost", "printer.local", "db.internal", "x.example.local", // local names
		"10.0.0.1", "127.0.0.1", "169.254.169.254", "8.8.8.8", "[::1]", "::1", "2001:db8::1", "[2001:db8::1]:80", // addresses, even public ones
		"jobs", "intranet", ".example.com", "example.com.", "a..example.com", // not a public name
		"https://x.example.com", "x.example.com/path", "x.example.com?q=1", "x.example.com#f", "user@x.example.com", "a b.example.com", "*.example.com", "x.exam\nple.com", "x%2eexample.com", `x.example.com\`,
		"x.example.com:", "x.example.com:0", "x.example.com:99999", "x.example.com:abc", "x.example.com:-1",
		"-x.example.com", "x-.example.com", "x_y.example.com", "é.example.com", strings.Repeat("a", 64) + ".example.com", strings.Repeat("a.", 130) + "com",
	}
	for _, in := range bad {
		if got, err := NormalizeJobHost(in, link); !errors.Is(err, ErrInvalidJobHost) || got != "" {
			t.Errorf("%q must be refused: got %q %v", in, got, err)
		}
	}
}

func TestAJobHostIsSetReadAndShownInTheList(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if h, err := LinkJobHost(id); err != nil || h != "" {
		t.Fatalf("a new link has none: %q %v", h, err)
	}
	if err := SetLinkJobHost(id, "Jobs.Other.example"); err != nil {
		t.Fatal(err)
	}
	own, job, err := LinkSiteHosts(id)
	if err != nil || own != "board.example" || job != "jobs.other.example" {
		t.Errorf("site hosts: %q %q %v", own, job, err)
	}
	if l := loginLink(t, id); l.JobHost != "jobs.other.example" {
		t.Errorf("the portals list: %q", l.JobHost)
	}
	if err := SetLinkJobHost(id, ""); err != nil {
		t.Fatal(err)
	}
	if h, _ := LinkJobHost(id); h != "" {
		t.Errorf("cleared: %q", h)
	}
	if l := loginLink(t, id); l.JobHost != "" {
		t.Errorf("the portals list after clearing: %q", l.JobHost)
	}
}

func TestABadOrUnknownJobHostChangesNothing(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if err := SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"localhost", "10.0.0.1", "board.example", "x.example/path"} {
		if err := SetLinkJobHost(id, bad); !errors.Is(err, ErrInvalidJobHost) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if h, _ := LinkJobHost(id); h != "jobs.other.example" {
		t.Errorf("a refused value changed the job host: %q", h)
	}
	if err := SetLinkJobHost("no-such-link", "jobs.other.example"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
	if _, err := LinkJobHost("no-such-link"); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link read: %v", err)
	}
}

func TestARuleWithAJobHostIsStoredOnlyForTheConfirmedOne(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	rule := func(jobHost string) *linkrule.Rule {
		return &linkrule.Rule{Host: "board.example", JobHost: jobHost, Include: []string{"jobs.other.example/jobanzeige/#"}}
	}
	// nothing confirmed: refused, whatever wrote the rule, and nothing is stored
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, rule("jobs.other.example"), false); !errors.Is(err, ErrLinkRulesJobHost) {
		t.Fatalf("unconfirmed: %v", err)
	}
	if s, _ := GetLinkCrawlSettings(id); s.Rules != nil || s.CrawlMethod != CrawlMethodInstructions {
		t.Errorf("a refused save stored %+v", s)
	}
	if _, err := ValidateRuleForLink(id, *rule("jobs.other.example")); !errors.Is(err, ErrLinkRulesJobHost) {
		t.Errorf("the preview and test check says the same: %v", err)
	}
	if err := SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	// another host than the confirmed one is refused, the right one is stored (case-insensitive)
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, rule("third.example"), false); !errors.Is(err, ErrLinkRulesJobHost) {
		t.Errorf("a different job host: %v", err)
	}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, rule("JOBS.other.example"), false); err != nil {
		t.Fatal(err)
	}
	s, _ := GetLinkCrawlSettings(id)
	if s.CrawlMethod != CrawlMethodLinks || s.Rules == nil || s.Rules.JobHost != "jobs.other.example" || s.Rules.Host != "board.example" {
		t.Errorf("stored: %+v %+v", s, s.Rules)
	}
	if l := loginLink(t, id); l.LinkRules == nil || l.LinkRules.JobHost != "jobs.other.example" {
		t.Errorf("the portals list carries the rule's job host: %+v", l.LinkRules)
	}
	// the rule's own host must still be the link's host
	bad := &linkrule.Rule{Host: "jobs.other.example", Include: []string{"jobs.other.example/jobanzeige/#"}}
	if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, bad, false); !errors.Is(err, ErrLinkRulesHost) {
		t.Errorf("a rule for the job host as its host: %v", err)
	}
}

func TestRemovingOrChangingTheJobHostTakesAwayTheRuleThatNeededIt(t *testing.T) {
	loginSetup(t)
	withRule := func(id string) {
		t.Helper()
		if err := SetLinkJobHost(id, "jobs.other.example"); err != nil {
			t.Fatal(err)
		}
		r := &linkrule.Rule{Host: "board.example", JobHost: "jobs.other.example", Include: []string{"jobs.other.example/jobanzeige/#"}}
		if err := SaveLinkCrawlSettings(id, CrawlMethodLinks, r, false); err != nil {
			t.Fatal(err)
		}
	}
	// cleared
	a := newLinkOn(t, "https://board.example/a")
	withRule(a)
	if err := SetLinkJobHost(a, ""); err != nil {
		t.Fatal(err)
	}
	if s, _ := GetLinkCrawlSettings(a); s.Rules != nil || s.CrawlMethod != CrawlMethodInstructions {
		t.Errorf("clearing the job host must take its rule away: %+v", s)
	}
	// changed to another host
	b := newLinkOn(t, "https://board.example/b")
	withRule(b)
	if err := SetLinkJobHost(b, "other.example"); err != nil {
		t.Fatal(err)
	}
	if s, _ := GetLinkCrawlSettings(b); s.Rules != nil {
		t.Errorf("changing the job host must take its rule away: %+v", s)
	}
	// the same value again keeps it; a rule that never used the job host survives its removal
	c := newLinkOn(t, "https://board.example/c")
	withRule(c)
	if err := SetLinkJobHost(c, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if s, _ := GetLinkCrawlSettings(c); s.Rules == nil {
		t.Error("the same job host again took the rule away")
	}
	d := newLinkOn(t, "https://board.example/d")
	if err := SaveLinkCrawlSettings(d, CrawlMethodLinks, ruleFor("board.example", "board.example/jobs/*"), false); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkJobHost(d, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if err := SetLinkJobHost(d, ""); err != nil {
		t.Fatal(err)
	}
	if s, _ := GetLinkCrawlSettings(d); s.Rules == nil || s.CrawlMethod != CrawlMethodLinks {
		t.Errorf("a rule for the own host does not depend on the job host: %+v", s)
	}
}

func TestTheJobHostColumnIsInTheSchema(t *testing.T) {
	loginSetup(t)
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('portal_links') WHERE name = 'job_host'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("job_host column: %d %v", n, err)
	}
	id := newLinkOn(t, "https://board.example/jobs")
	var h string
	if err := db.JobsDB.QueryRow(`SELECT job_host FROM portal_links WHERE id = ?`, id).Scan(&h); err != nil || h != "" {
		t.Errorf("the default is empty, never NULL: %q %v", h, err)
	}
}

func TestOnlyAPersonsRunningAttemptCountsAsAttended(t *testing.T) {
	loginSetup(t)
	id := newLinkOn(t, "https://board.example/jobs")
	if RunningAttemptIsManual(id) {
		t.Fatal("no attempt: not attended")
	}
	add := func(attemptID, by, status string) {
		t.Helper()
		if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, triggered_by, status) VALUES (?, ?, 'listing', ?, ?)`, attemptID, id, by, status); err != nil {
			t.Fatal(err)
		}
	}
	add("a1", "manual", "failed") // finished: does not count
	if RunningAttemptIsManual(id) {
		t.Error("a finished manual attempt is not attended")
	}
	add("a2", "auto", "running")
	if RunningAttemptIsManual(id) {
		t.Error("an auto attempt is never attended")
	}
	if _, err := db.JobsDB.Exec(`UPDATE instruction_attempts SET status='failed' WHERE id='a2'`); err != nil {
		t.Fatal(err)
	}
	add("a3", "manual", "running")
	if !RunningAttemptIsManual(id) {
		t.Error("a running manual attempt is attended")
	}
}
