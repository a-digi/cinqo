package detailpick

import (
	"encoding/json"
	"strings"
	"testing"
)

// What Stepstone gave the Browser (plan 188's live finding): a cookie banner heading and a list of OTHER jobs, no heading of the job's own;
// the job's title only in the <title>.
func reducedPage() Page {
	return Page{
		Title: "SAP APPLICATION DEVELOPER – HCM (W/M/D) - Job bei der Firma Rundfunk Berlin-Brandenburg (RBB ) in Potsdam, Köln",
		Candidates: Candidates{Title: []Candidate{
			{Selector: "h1.ccm-col.title", Unique: true, Value: "Wir verwenden Cookies", Tag: "h1", PlacedIn: "consent"},
			{Selector: "h2.recom a", Value: "IT-Anwendungsmanager (m/w/d)", Tag: "a"},
			{Selector: "h2.recom a", Value: "Senior IT-Spezialist / SAP HCM Berater (m/w/d)", Tag: "a"},
		}},
		Elements: []Element{{Selector: "p.x", Tag: "p", Text: "Diese Jobs waren bei anderen Jobsuchenden beliebt"}},
	}
}

const reducedJob = "SAP APPLICATION DEVELOPER – HCM (W/M/D)"

func TestAReducedPageIsReportedAsHavingNoHeadingOfTheJobsOwn(t *testing.T) {
	ns := PageNotices(reducedPage(), reducedJob)
	if len(ns) != 1 || ns[0].Code != NoticeJobHeadingMissing || !strings.Contains(ns[0].Message, reducedJob) || !strings.Contains(ns[0].Message, "<title>") {
		t.Fatalf("%+v", ns)
	}
}

func TestNoNoticeWhenThePageShowsTheJobsTitle(t *testing.T) {
	headed := func() Page {
		p := reducedPage()
		p.Candidates.Title = append(p.Candidates.Title, Candidate{Selector: "h1.job", Value: "Alpha Developer (m/w/d)", Tag: "h1"})
		return p
	}
	cases := map[string]func() Page{
		"a heading that has a suffix the saved title lacks": headed,
		"a text element": func() Page {
			p := reducedPage()
			p.Elements = append(p.Elements, Element{Tag: "div", Text: "Alpha Developer"})
			return p
		},
		"JobPosting JSON-LD": func() Page {
			p := reducedPage()
			p.Structured.JSONLD = []JobPosting{{Type: "JobPosting", Title: "Alpha Developer"}}
			return p
		},
		"og:title": func() Page { p := reducedPage(); p.Structured.Meta.OgTitle = "Alpha Developer - Acme"; return p },
	}
	for name, mk := range cases {
		if ns := PageNotices(mk(), "Alpha Developer"); len(ns) != 0 {
			t.Errorf("%s: %+v", name, ns)
		}
	}
	// nearly all the words of a longer title are enough (the saved one lacks a word)
	p := Page{Candidates: Candidates{Title: []Candidate{{Value: "Senior Backend Software Engineer Payments Platform", Tag: "h1"}}}}
	if ns := PageNotices(p, "Senior Backend Software Engineer Payments"); len(ns) != 0 {
		t.Errorf("%+v", ns)
	}
	// the same words in another order, with one more on the page: neither text contains the other, only the word overlap says it is the same job
	p = Page{Candidates: Candidates{Title: []Candidate{{Value: "Software Engineer Backend Senior Payments Platform", Tag: "h1"}}}}
	if ns := PageNotices(p, "Senior Backend Software Engineer Payments"); len(ns) != 0 {
		t.Errorf("reordered words: %+v", ns)
	}
}

func TestAPageThatDoesNotMentionTheJobAtAllIsReportedAsNotTheJob(t *testing.T) {
	p := Page{Title: "Sign in - Acme", Candidates: Candidates{Title: []Candidate{{Value: "Sign in", Tag: "h1"}}}}
	ns := PageNotices(p, "Alpha Developer")
	if len(ns) != 1 || ns[0].Code != NoticePageNotTheJob {
		t.Fatalf("%+v", ns)
	}
	// near-misses are still different jobs
	p = Page{Candidates: Candidates{Title: []Candidate{{Value: "Beta Engineer (m/w/d)", Tag: "h1"}}}}
	if ns := PageNotices(p, "Alpha Developer"); len(ns) != 1 || ns[0].Code != NoticePageNotTheJob {
		t.Errorf("%+v", ns)
	}
}

func TestNothingIsJudgedWithoutAUsableJobTitleAndTheListIsNeverNull(t *testing.T) {
	for _, jt := range []string{"", "   ", "Dev", "—", "(m/w/d)"} {
		ns := PageNotices(reducedPage(), jt)
		if len(ns) != 0 {
			t.Errorf("%q: %+v", jt, ns)
		}
		if b, _ := json.Marshal(ns); string(b) != "[]" {
			t.Errorf("%q: %s, want []", jt, b)
		}
	}
}

func TestACutPageSaysSoAndALongTitleIsShortened(t *testing.T) {
	p := reducedPage()
	p.Truncated = true
	long := reducedJob + strings.Repeat(" very long", 30)
	p.Title = long
	ns := PageNotices(p, long)
	if len(ns) != 1 || !strings.Contains(ns[0].Message, "only its first elements were listed.") || strings.Contains(ns[0].Message, "very long very long very long very long very long very long very long very long very long very long") {
		t.Fatalf("%+v", ns)
	}
	if !strings.Contains(ns[0].Message, "…") {
		t.Errorf("the title is cut with an ellipsis: %s", ns[0].Message)
	}
}

func TestThePageTitleIsOfferedForTheTitleAndRanksFirstOnAReducedPage(t *testing.T) {
	p := reducedPage()
	ranked := Rank(p, FieldTitle)
	if len(ranked) != 4 || ranked[0].Selector != "title" || ranked[0].Tag != "title" || ranked[0].Value != p.Title {
		t.Fatalf("%+v", ranked)
	}
	if ranked[0].Reason != "the page's <title>" || !strings.Contains(strings.Join(ranked[0].Why, ","), "<title>") {
		t.Errorf("reason/why: %+v", ranked[0])
	}
	// the headings of other jobs and the cookie box rank below it
	for _, r := range ranked[1:] {
		if r.Score >= ranked[0].Score {
			t.Errorf("%q (%v) must rank below the page title (%v)", r.Value, r.Score, ranked[0].Score)
		}
	}
}

func TestAHeadingThatAgreesWithThePageRanksAboveItsTitleAndNothingIsOfferedTwice(t *testing.T) {
	p := Page{Title: "Alpha Developer at Acme", Candidates: Candidates{Title: []Candidate{
		{Selector: "h1.job", Unique: true, Value: "Alpha Developer", Tag: "h1", InMain: true},
	}}}
	ranked := Rank(p, FieldTitle)
	if len(ranked) != 2 || ranked[0].Selector != "h1.job" || ranked[1].Selector != "title" {
		t.Fatalf("%+v", ranked)
	}
	// a candidate that already carries the page title text: no second copy
	p.Candidates.Title = append(p.Candidates.Title, Candidate{Selector: "h2.same", Value: "alpha developer at ACME!", Tag: "h2"})
	for _, r := range Rank(p, FieldTitle) {
		if r.Selector == "title" {
			t.Errorf("the page title must not be offered twice: %+v", r)
		}
	}
	// no <title>: nothing is added; other fields are never touched
	q := Page{Candidates: Candidates{Title: []Candidate{{Selector: "h1", Value: "X title", Tag: "h1"}}, Company: []Candidate{{Selector: ".c", Value: "Acme"}}}}
	if len(Rank(q, FieldTitle)) != 1 || len(Rank(Page{Title: "T", Candidates: q.Candidates}, FieldCompany)) != 1 {
		t.Error("an empty <title> adds nothing and company candidates are untouched")
	}
	// a very long <title> is cut for the list
	long := Page{Title: strings.Repeat("ä", 500)}
	if r := Rank(long, FieldTitle); len(r) != 1 || runeLen(r[0].Value) != 200 {
		t.Errorf("%+v", r)
	}
}

func FuzzPageNotices(f *testing.F) {
	f.Add(`{"title":"Alpha Developer","candidates":{"title":[{"value":"Alpha Developer","tag":"h1"}]}}`, "Alpha Developer")
	f.Add(`{"title":"x","truncated":true,"elements":[{"text":"\u0000"}]}`, "Ünïcode Title (m/w/d)")
	f.Fuzz(func(t *testing.T, raw, jobTitle string) {
		var p Page
		if json.Unmarshal([]byte(raw), &p) != nil {
			return
		}
		ns := PageNotices(p, jobTitle)
		if ns == nil || len(ns) > 1 {
			t.Fatalf("%v", ns)
		}
		for _, r := range Rank(p, FieldTitle) {
			if r.Selector == "title" && runeLen(r.Value) > 200 {
				t.Fatal("a long page title must be cut")
			}
		}
	})
}
