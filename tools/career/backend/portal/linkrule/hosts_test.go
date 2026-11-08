package linkrule

import (
	"fmt"
	"strings"
	"testing"
)

const (
	siteJobboerse = "www.xn--jobbrse-d1a.de"
	hostJobs      = "www.xn--jobbrse-stellenangebote-blc.de"
)

// jobboerseLinks is the page of plan 203 in miniature: menus and filters on the link's own host, the postings (a long token in the path)
// and a few other addresses on the other host, the same posting linked several times.
func jobboerseLinks() []Link {
	var links []Link
	for _, p := range []string{"/", "/jobsuche/jobs/", "/karrieretipps/", "/gehalt/", "/arbeitgeber-stellenangebote-schalten.html"} {
		links = append(links, Link{URL: "https://" + siteJobboerse + p})
	}
	for i := 0; i < 15; i++ {
		links = append(links, Link{URL: fmt.Sprintf("https://%s/jobsuche/jobs/?q=x&l=hamburg&d=%d&qs=1", siteJobboerse, i)})
	}
	// the postings, in the three address shapes the real page mixes: an alias part step5 (13), talet (2) and sprit (2)
	for i := 0; i < 17; i++ {
		alias := "step5"
		switch {
		case i >= 15:
			alias = "sprit"
		case i >= 13:
			alias = "talet"
		}
		u := fmt.Sprintf("https://%s/jobanzeige/job_search/%s/eJx1jktuwzAMRG-THeMabZCmQNCjCPowkm%dyJVCgaQ/0l-00/", hostJobs, alias, i)
		links = append(links, Link{URL: u}, Link{URL: u}) // the title and the logo link to the same posting
	}
	for _, p := range []string{"/stellenangebote-deutschland/", "/gehalt/", "/beruf/", "/branche/", "/karrieretipps/"} {
		links = append(links, Link{URL: "https://" + hostJobs + p})
	}
	return links
}

func TestThePostingsOnAnotherHostAreFoundAndCounted(t *testing.T) {
	own, others := SummarizeHosts(jobboerseLinks(), siteJobboerse)
	if own.Host != siteJobboerse || own.Links != 20 {
		t.Fatalf("own site: %+v", own)
	}
	if own.JobLike != 0 {
		t.Errorf("15 filter links share one address shape but the last part never varies: they are not postings: %+v", own)
	}
	if len(others) != 1 || others[0].Host != hostJobs {
		t.Fatalf("others: %+v", others)
	}
	top := others[0]
	if top.Links != 22 || top.JobLike != 17 || top.Shape != "/jobanzeige/job_search/*/*/*" {
		t.Errorf("the other host: %+v (17 postings in three shapes, each linked twice, count once; the biggest shape is shown)", top)
	}
	if strings.HasPrefix(top.Sample, "http") || !strings.HasPrefix(top.Sample, hostJobs+"/jobanzeige/") || len([]rune(top.Sample)) > maxSampleLen+1 {
		t.Errorf("sample %q", top.Sample)
	}
	if got, ok := PostingsOnAnotherSite(own, others); !ok || got.Host != hostJobs {
		t.Errorf("the postings are on the other site: %+v %v", got, ok)
	}
}

func TestPostingsOnTheOwnSiteAreNotReportedAsElsewhere(t *testing.T) {
	var links []Link
	for i := 0; i < 12; i++ {
		links = append(links, Link{URL: fmt.Sprintf("https://board.example/jobs/%d-engineer", 1000+i)})
	}
	for i := 0; i < 4; i++ {
		links = append(links, Link{URL: fmt.Sprintf("https://ads.example/promo/%d-offer", 5000+i)})
	}
	own, others := SummarizeHosts(links, "board.example")
	if own.JobLike != 12 || len(others) != 1 || others[0].JobLike != 4 {
		t.Fatalf("%+v %+v", own, others)
	}
	if _, ok := PostingsOnAnotherSite(own, others); ok {
		t.Error("a bigger list on the own site must not be reported as elsewhere")
	}
}

func TestNeedsMoreThanTwoSimilarLinksAndMoreThanTheOwnSite(t *testing.T) {
	two := []Link{{URL: "https://a.example/job/1001"}, {URL: "https://a.example/job/1002"}}
	own, others := SummarizeHosts(two, "own.example")
	if others[0].JobLike != 0 {
		t.Errorf("two links are not a list: %+v", others[0])
	}
	if _, ok := PostingsOnAnotherSite(own, others); ok {
		t.Error("two links are not a list of postings")
	}
	three := append(two, Link{URL: "https://a.example/job/1003"})
	own, others = SummarizeHosts(three, "own.example")
	if _, ok := PostingsOnAnotherSite(own, others); !ok {
		t.Error("three similar links on another host and none on the own site are")
	}
	// the same size on the own site: not "elsewhere"
	both := append(three, Link{URL: "https://own.example/job/2001"}, Link{URL: "https://own.example/job/2002"}, Link{URL: "https://own.example/job/2003"})
	own, others = SummarizeHosts(both, "own.example")
	if _, ok := PostingsOnAnotherSite(own, others); ok {
		t.Errorf("equal lists: %+v %+v", own, others)
	}
}

func TestAddressesAreNormalisedAndOddOnesSkipped(t *testing.T) {
	links := []Link{
		{URL: "https://OWN.example:8443/Jobs/1001"}, {URL: "https://own.example:8443/Jobs/1001"}, // the same address twice
		{URL: "https://own.example:8443/Jobs/1002"},
		{URL: "not a url"}, {URL: ""}, {URL: "mailto:x@y.z"}, {URL: "/relative/only"},
		{URL: "https://Other.EXAMPLE/a"},
	}
	own, others := SummarizeHosts(links, "  OWN.example:8443 ")
	if own.Host != "own.example:8443" || own.Links != 2 {
		t.Errorf("own: %+v", own)
	}
	if len(others) != 1 || others[0].Host != "other.example" || others[0].Links != 1 {
		t.Errorf("others: %+v", others)
	}
}

func TestTheOwnSiteIsAlwaysThereAndOthersAreOrderedAndBounded(t *testing.T) {
	own, others := SummarizeHosts(nil, "Site.Example")
	if own.Host != "site.example" || own.Links != 0 || len(others) != 0 {
		t.Fatalf("%+v %+v", own, others)
	}
	var links []Link
	for h := 0; h < 8; h++ {
		for i := 0; i <= h; i++ { // host h has h+1 plain links, none posting-like
			links = append(links, Link{URL: fmt.Sprintf("https://h%d.example/page%c", h, 'a'+i)})
		}
	}
	for i := 0; i < 5; i++ { // a small host with a real list outranks bigger hosts without one
		links = append(links, Link{URL: fmt.Sprintf("https://list.example/job/%d", 7000+i)})
	}
	_, others = SummarizeHosts(links, "site.example")
	if len(others) != MaxOtherHosts || others[0].Host != "list.example" || others[0].JobLike != 5 {
		t.Fatalf("%d hosts, first %+v", len(others), others[0])
	}
	if others[1].Links < others[2].Links {
		t.Errorf("hosts without a list are ordered by their links: %+v", others)
	}
}

func TestShapesKeepTheirMeaning(t *testing.T) {
	for in, want := range map[string]string{
		"1234": "#", "9e107d9d-372b-4f22-9f64-3f2a0b1c4d5e": "#", "eJx1jktuwzAMRG-THeMabZCmQNCjCPowkmyJVCgaQm8fbbLscmbwHiaptv6zLGOMc1dsXZnwHHCZoRQkSxEdKwJ0pMwCnR86rCCSjuz3grPyaaC0g3aFhzA": "*",
		"job_search": "job_search", "step5": "*", "0l-00": "*", "a-b-c": "*", "jobs": "jobs", "": "",
	} {
		if got := SegmentShape(in); got != want {
			t.Errorf("SegmentShape(%q) = %q, want %q", in, got, want)
		}
	}
	shape, varies := ShapeOf("https://x.example/jobs/12345?utm=1&b=2")
	if shape != "x.example/jobs/#?b&utm" || !varies {
		t.Errorf("%q %v", shape, varies)
	}
	if shape, varies := ShapeOf("https://x.example/jobs"); shape != "x.example/jobs" || varies {
		t.Errorf("%q %v", shape, varies)
	}
	if shape, _ := ShapeOf("::bad"); shape != "" {
		t.Errorf("a bad address has no shape: %q", shape)
	}
}

func TestOneFamilyOfPostingsInSeveralShapesCountsAsOneList(t *testing.T) {
	var links []Link
	for i := 0; i < 3; i++ { // the smaller shape comes FIRST on the page
		links = append(links, Link{URL: fmt.Sprintf("https://x.example/jobanzeige/b/c-d-e%d", i)})
	}
	for i := 0; i < 6; i++ { // the bigger one second: it is the one shown
		links = append(links, Link{URL: fmt.Sprintf("https://x.example/jobanzeige/a/%d-x/item%d", i, i)})
	}
	links = append(links, Link{URL: "https://x.example/jobanzeige/"}) // the family's own page does not vary
	_, others := SummarizeHosts(links, "own.example")
	if others[0].JobLike != 9 || !strings.Contains(others[0].Shape, "/jobanzeige/a/") {
		t.Errorf("%+v", others[0])
	}
	// different families on one host are not added up
	links = append(links, Link{URL: "https://x.example/blog/post-one-two"}, Link{URL: "https://x.example/blog/post-three-four"}, Link{URL: "https://x.example/blog/post-five-six"})
	_, others = SummarizeHosts(links, "own.example")
	if others[0].JobLike != 9 {
		t.Errorf("a second family must not be added to the first: %+v", others[0])
	}
}
