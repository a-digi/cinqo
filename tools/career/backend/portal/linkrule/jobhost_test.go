package linkrule

import (
	"errors"
	"strings"
	"testing"
)

func jobsOnTwoHosts() []Link {
	return []Link{
		{URL: "https://board.example/about", Texts: []string{"About"}},
		{URL: "https://board.example/jobs/own-1", Texts: []string{"Own one"}},
		{URL: "https://jobs.other.example/jobanzeige/1001", Texts: []string{"Posting 1001"}},
		{URL: "https://jobs.other.example/jobanzeige/1002", Texts: []string{"Posting 1002"}},
		{URL: "https://third.example/jobanzeige/9", Texts: []string{"Elsewhere"}},
	}
}

func TestARuleMayNameOneJobHostForTheJobLinks(t *testing.T) {
	rule := Rule{Host: "board.example", JobHost: "Jobs.Other.example", Include: []string{"jobs.other.example/jobanzeige/#"}}
	if err := ValidateRule(rule); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(rule, jobsOnTwoHosts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 2 || len(res.Items) != 2 || res.Items[0].Title != "Posting 1001" {
		t.Errorf("the postings of the job host: %+v", res)
	}
	// the own host's links and the third host's links are not postings of this rule; only a host that is neither is "another host"
	if res.OtherHost != 1 {
		t.Errorf("only the third host is another host: %d", res.OtherHost)
	}
}

func TestPatternsMayBeForEitherOfTheTwoHostsAndNoOther(t *testing.T) {
	both := Rule{Host: "board.example", JobHost: "jobs.other.example", Include: []string{"board.example/jobs/*", "jobs.other.example/jobanzeige/#"}}
	res, err := Apply(both, jobsOnTwoHosts())
	if err != nil || res.Matched != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	third := Rule{Host: "board.example", JobHost: "jobs.other.example", Include: []string{"third.example/jobanzeige/#"}}
	if err := ValidateRule(third); !errors.Is(err, ErrHostMismatch) || !strings.Contains(err.Error(), "jobs.other.example") {
		t.Errorf("a pattern for a third host: %v", err)
	}
	excl := Rule{Host: "board.example", JobHost: "jobs.other.example", Include: []string{"jobs.other.example/jobanzeige/#"}, Exclude: []string{"third.example/x"}}
	if err := ValidateRule(excl); !errors.Is(err, ErrHostMismatch) {
		t.Errorf("an exclude pattern for a third host: %v", err)
	}
	// without a job host a pattern for the second host is still refused: what every rule did before
	plain := Rule{Host: "board.example", Include: []string{"jobs.other.example/jobanzeige/#"}}
	if err := ValidateRule(plain); !errors.Is(err, ErrHostMismatch) {
		t.Errorf("no job host: %v", err)
	}
}

func TestTheJobHostItselfMustBeOneOtherPlainHost(t *testing.T) {
	for _, jh := range []string{"board.example", "BOARD.example", "a*.example", "x.example/path", "x .example", "x.example?q", "x.example#f"} {
		rule := Rule{Host: "board.example", JobHost: jh, Include: []string{"board.example/jobs/*"}}
		if err := ValidateRule(rule); !errors.Is(err, ErrBadJobHost) {
			t.Errorf("job host %q: %v", jh, err)
		}
	}
	if err := ValidateRule(Rule{Host: "board.example", JobHost: "  ", Include: []string{"board.example/jobs/*"}}); err != nil {
		t.Errorf("a blank job host means none: %v", err)
	}
}

func TestThePagesOfTheListStayOnTheListingHost(t *testing.T) {
	onJobHost := Rule{Host: "board.example", JobHost: "jobs.other.example", Include: []string{"jobs.other.example/jobanzeige/#"},
		Pagination: &Pagination{Kind: PagesLink, Pattern: "jobs.other.example/jobsuche/jobs/?page=#", MaxPages: 3}}
	if err := ValidateRule(onJobHost); err == nil {
		t.Error("a pagination pattern on the job host must be refused: the pages of the list are on the listing host")
	}
	onListing := onJobHost
	onListing.Pagination = &Pagination{Kind: PagesLink, Pattern: "board.example/jobs?page=#", MaxPages: 3}
	if err := ValidateRule(onListing); err != nil {
		t.Errorf("the listing host's pagination with a job host: %v", err)
	}
}

func TestTheJobHostIsKeptWhenARuleIsEncodedAndAsUnchangedRulesGainNothing(t *testing.T) {
	// a rule written before plan 203 has no job host and is read as it always was
	old := Rule{Host: "board.example", Include: []string{"board.example/jobs/*"}}
	if old.JobHost != "" {
		t.Fatal("a rule without a job host has none")
	}
	res, err := Apply(old, jobsOnTwoHosts())
	if err != nil || res.Matched != 1 || res.OtherHost != 3 {
		t.Errorf("an old rule sees the own host only: %+v %v", res, err)
	}
}

func TestAConfirmedJobHostIsNotAnotherSiteInTheSummary(t *testing.T) {
	var links []Link
	for i := 0; i < 5; i++ {
		links = append(links, Link{URL: "https://jobs.other.example/jobanzeige/" + string(rune('1'+i)) + "000"})
	}
	own, others := SummarizeHosts(links, "board.example")
	if _, ok := PostingsOnAnotherSite(own, others); !ok {
		t.Fatal("unconfirmed: the postings are on another site")
	}
	marked := MarkAllowed(others, "Jobs.Other.example")
	if !marked[0].Allowed || others[0].Allowed {
		t.Errorf("only the copy is marked: %+v %+v", marked, others)
	}
	if _, ok := PostingsOnAnotherSite(own, marked); ok {
		t.Error("confirmed: they are no longer another site")
	}
	if got := MarkAllowed(others, ""); got[0].Allowed {
		t.Error("no job host marks nothing")
	}
	// a bigger list on a host that is NOT confirmed still counts as elsewhere, a smaller one than the confirmed host's does not
	more := append([]Link(nil), links...)
	for i := 0; i < 8; i++ {
		more = append(more, Link{URL: "https://other.example/stellen/" + string(rune('a'+i)) + "-job-1"})
	}
	own, others = SummarizeHosts(more, "board.example")
	marked = MarkAllowed(others, "jobs.other.example")
	if top, ok := PostingsOnAnotherSite(own, marked); !ok || top.Host != "other.example" {
		t.Errorf("a bigger unconfirmed list: %+v %v", top, ok)
	}
}
