package detailpick

import (
	"encoding/json"
	"math"
	"testing"
)

func cand(sel, value, tag string) Candidate {
	return Candidate{Selector: sel, Unique: true, Matches: 1, Value: value, Chars: len([]rune(value)), Tag: tag, InMain: true, Source: "dom"}
}

func firstSel(r []Ranked) string {
	if len(r) == 0 {
		return ""
	}
	return r[0].Selector
}

// The python.org shape: the first h1 is the board's heading, the second one is the job.
func pythonPage() Page {
	return Page{
		Title:      "Founding Principal Engineer, Ferto Health GmbH | Python.org",
		Structured: Structured{Meta: Meta{OgTitle: "Job: Founding Principal Engineer at Ferto Health GmbH"}},
		Candidates: Candidates{
			Title: []Candidate{
				cand("h1.call-to-action", "18 jobs on the Python Job Board", "h1"),
				cand("h1.job-title", "Founding Principal Engineer New Ferto Health GmbH", "h1"),
			},
			Description: []Candidate{
				{Selector: "section.main-content", Unique: true, Chars: 2992, Tag: "section", InMain: true, ShareOfPage: 0.5, Headings: 5, ListItems: 8, Paragraphs: 6},
				{Selector: "#content", Unique: true, Chars: 3533, Tag: "div", InMain: true, ByAttr: true, ShareOfPage: 0.6, Headings: 6, ListItems: 8, Paragraphs: 6, Furniture: []Furniture{{Selector: ".right-sidebar", Kind: "aside", Chars: 540}}},
			},
		},
	}
}

func TestRankTitlePrefersTheHeadingThatAgreesWithThePageTitle(t *testing.T) {
	r := Rank(pythonPage(), FieldTitle)
	if firstSel(r) != "h1.job-title" {
		t.Fatalf("ranked %v", r)
	}
	if r[1].Score >= r[0].Score || len(r[0].Why) == 0 {
		t.Errorf("scores/reasons: %+v", r)
	}
}

func TestRankTitleUsesJSONLDAndPenalisesTheSiteName(t *testing.T) {
	p := Page{
		Structured: Structured{JSONLD: []JobPosting{{Title: "Account Executive - Commercial"}}, Meta: Meta{OgSiteName: "Spotify"}},
		Candidates: Candidates{Title: []Candidate{
			cand("div.logo", "Spotify", "div"),
			cand("div.posting-title", "Account Executive - Commercial", "div"),
			cand("h2", "Account Executive - Commercial", "h2"),
		}},
	}
	r := Rank(p, FieldTitle)
	if firstSel(r) == "div.logo" || r[len(r)-1].Selector != "div.logo" {
		t.Fatalf("the site name must be last: %+v", r)
	}
}

func TestRankCompany(t *testing.T) {
	p := Page{
		Structured: Structured{JSONLD: []JobPosting{{Title: "Software Engineer Enterprise Search", Company: "adesso SE"}}},
		Candidates: Candidates{Company: []Candidate{
			cand("li.other", "Unternehmensporträt", "li"),
			cand("li.company-name > span", "adesso SE", "span"),
			cand("div.blob", "Software Engineer Enterprise Search adesso SE Berlin New", "div"),
		}},
	}
	r := Rank(p, FieldCompany)
	if firstSel(r) != "li.company-name > span" {
		t.Fatalf("ranked %+v", r)
	}
	if r[len(r)-1].Selector != "div.blob" {
		t.Errorf("a value that holds the job's title is last: %+v", r)
	}
}

func TestRankLocationAndDate(t *testing.T) {
	p := Page{
		Structured: Structured{JSONLD: []JobPosting{{Location: "Berlin, DE", DatePosted: "2026-10-28T09:00:00+00:00"}}},
		Candidates: Candidates{
			Location: []Candidate{cand("ul.menu > li", "Jobs by Locations", "li"), cand("span.loc", "Berlin, Germany", "span")},
			PostedAt: []Candidate{
				cand("span.tag", "Feste Anstellung", "span"),
				{Selector: "time.similar", Unique: true, Value: "2026-09-01", Attribute: "datetime", Chars: 10, Tag: "time", PlacedIn: "aside"},
				{Selector: "time.job", Unique: true, Value: "2026-10-28", Attribute: "datetime", Chars: 10, Tag: "time", InMain: true},
			},
		},
	}
	if firstSel(Rank(p, FieldLocation)) != "span.loc" {
		t.Errorf("location: %+v", Rank(p, FieldLocation))
	}
	r := Rank(p, FieldPostedAt)
	if firstSel(r) != "time.job" || r[len(r)-1].Selector != "span.tag" {
		t.Errorf("date: %+v", r)
	}
}

func TestRankDescription(t *testing.T) {
	r := Rank(pythonPage(), FieldDescription)
	if firstSel(r) != "#content" {
		t.Fatalf("the box its id names beats the section: %+v", r)
	}
	// greenhouse: the application form is long but mostly furniture
	g := Page{Candidates: Candidates{Description: []Candidate{
		{Selector: "div.form", Unique: true, Chars: 3686, Tag: "div", ShareOfPage: 0.2, Furniture: []Furniture{{Kind: "form", Chars: 3000}}},
		{Selector: "div.job__description", Unique: true, Chars: 8574, Tag: "div", ByAttr: true, InMain: true, ShareOfPage: 0.5, Headings: 12, ListItems: 24, Paragraphs: 9},
	}}}
	if firstSel(Rank(g, FieldDescription)) != "div.job__description" {
		t.Errorf("greenhouse: %+v", Rank(g, FieldDescription))
	}
	// a consent dialog is last however large
	c := Page{Candidates: Candidates{Description: []Candidate{
		{Selector: "div.dialog", Unique: true, Chars: 70000, Tag: "div", PlacedIn: "consent", ByAttr: true},
		{Selector: "main", Unique: true, Chars: 403, Tag: "main", InMain: true, ShareOfPage: 0.6},
	}}}
	if r := Rank(c, FieldDescription); r[len(r)-1].Selector != "div.dialog" {
		t.Errorf("consent: %+v", r)
	}
	// JSON-LD length: a part of the text (the Lever case) is ranked below a box that has all of it
	l := Page{Structured: Structured{JSONLD: []JobPosting{{DescriptionChars: 4500}}}, Candidates: Candidates{Description: []Candidate{
		{Selector: "div.part", Unique: true, Chars: 1500, Tag: "div", ByAttr: true, ShareOfPage: 0.2},
		{Selector: "div.all", Unique: true, Chars: 4400, Tag: "div", ByAttr: true, ShareOfPage: 0.4},
	}}}
	if firstSel(Rank(l, FieldDescription)) != "div.all" {
		t.Errorf("json-ld length: %+v", Rank(l, FieldDescription))
	}
}

func TestRankCommonRulesAndStability(t *testing.T) {
	p := Page{Candidates: Candidates{Company: []Candidate{
		{Selector: "a", Unique: true, Value: "", Tag: "div"},
		{Selector: "b", Unique: true, Value: "Acme", Tag: "div", PlacedIn: "footer"},
		{Selector: "c", Unique: false, Value: "Acme", Tag: "div", InMain: true},
		{Selector: "d", Unique: false, Value: "Acme", Tag: "div", InMain: true},
	}}}
	r := Rank(p, FieldCompany)
	if r[len(r)-1].Selector != "a" {
		t.Errorf("an empty value is last: %+v", r)
	}
	if r[0].Selector != "c" || r[1].Selector != "d" {
		t.Errorf("ties keep the page order, and the main part beats the footer: %+v", r)
	}
	if Rank(Page{}, FieldTitle) == nil || len(Rank(Page{}, FieldTitle)) != 0 {
		t.Error("no candidates: an empty (not nil) list")
	}
	if len(Rank(p, Field("nonsense"))) != 0 {
		t.Error("an unknown field has no candidates")
	}
	all := RankAll(p)
	if len(all) != len(Fields) {
		t.Errorf("RankAll: %d fields", len(all))
	}
}

func TestScoresAreAlwaysFinite(t *testing.T) {
	weird := Candidate{Selector: "x", Chars: -5, ShareOfPage: math.NaN(), Headings: -1, Matches: -3, Value: "\x00‮"}
	p := Page{Candidates: Candidates{Title: []Candidate{weird}, Company: []Candidate{weird}, Location: []Candidate{weird}, PostedAt: []Candidate{weird}, Description: []Candidate{weird}}}
	for f, list := range RankAll(p) {
		for _, r := range list {
			if math.IsNaN(r.Score) || math.IsInf(r.Score, 0) {
				t.Errorf("%s: score %v", f, r.Score)
			}
		}
	}
}

func TestLooksLikeDate(t *testing.T) {
	for s, want := range map[string]bool{
		"2026-10-28": true, "2026-10-28T09:00:00+00:00": true, "28 October 2026": true, "23-11-2026": true, "12.11.2026": true,
		"Posted 08 September 2026": true, "3. Nëntor 2026": true, "Feste Anstellung": false, "Posted 3 days ago": false, "": false,
		"Homeoffice möglich, Vollzeit": false, "May": false, "Berlin": false,
	} {
		if got := LooksLikeDate(s); got != want {
			t.Errorf("%q: %v, want %v", s, got, want)
		}
	}
}

func TestSuggestExcludes(t *testing.T) {
	box := Candidate{Chars: 4000, Furniture: []Furniture{
		{Selector: ".share-buttons", Kind: "share", Chars: 14, Value: "Share this job"},
		{Selector: "a.apply-now", Kind: "apply", Chars: 9},
		{Selector: "div", Kind: "related", Chars: 300},
		{Selector: "aside", Kind: "aside", Chars: 540},
		{Selector: ".big-related", Kind: "related", Chars: 2200},
		{Selector: ".share-buttons", Kind: "share", Chars: 14},
		{Selector: ".x", Kind: "unknownkind", Chars: 100},
		{Selector: "", Kind: "share", Chars: 5},
	}}
	got := SuggestExcludes(box)
	var sels []string
	for _, g := range got {
		sels = append(sels, g.Selector)
	}
	want := []string{".big-related", "aside", "div", ".share-buttons", "a.apply-now"}
	if len(sels) != len(want) {
		t.Fatalf("got %v want %v", sels, want)
	}
	for i := range want {
		if sels[i] != want[i] {
			t.Fatalf("got %v want %v", sels, want)
		}
	}
	risky := map[string]bool{}
	for _, g := range got {
		risky[g.Selector] = g.Risky
	}
	if !risky[".big-related"] || !risky["div"] || risky["aside"] || risky[".share-buttons"] {
		t.Errorf("risky flags: %v", risky)
	}
	if len(SuggestExcludes(Candidate{})) != 0 {
		t.Error("no furniture, no suggestions")
	}
}

func TestSuggestCutAfter(t *testing.T) {
	got := SuggestCutAfter(Candidate{LineSignals: []string{"similar jobs", "apply now", "Ähnliche", "bewerben", "jetzt bewerben", "teilen"}})
	want := []string{"Similar jobs", "Ähnliche", "Teilen"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v", got)
		}
	}
}

func TestStructuredSuggestions(t *testing.T) {
	p := Page{Structured: Structured{
		JSONLD: []JobPosting{{Title: "T", Company: "C", Location: "L", DatePosted: "2026-10-28"}},
		Meta:   Meta{OgTitle: "T - C"},
	}}
	got := StructuredSuggestions(p)
	if len(got) != 5 || got[0].Source != "jsonld" || got[4].Source != "meta" || got[4].Field != FieldTitle {
		t.Fatalf("got %+v", got)
	}
	if len(StructuredSuggestions(Page{})) != 0 {
		t.Error("nothing stated, nothing suggested")
	}
}

func FuzzRank(f *testing.F) {
	f.Add(`{"title":"T","candidates":{"title":[{"selector":"h1","unique":true,"value":"T","tag":"h1","chars":1}]}}`)
	f.Add(`{"candidates":{"description":[{"selector":"x","chars":-1,"shareOfPage":9,"furniture":[{"chars":5}]}],"postedAt":[{"value":"2026-10-28T00:00:00Z"}]},"structured":{"jsonld":[{"datePosted":"2026-10-28T00:00:00Z","descriptionChars":0}]}}`)
	f.Fuzz(func(t *testing.T, raw string) {
		var p Page
		if json.Unmarshal([]byte(raw), &p) != nil {
			return
		}
		for field, list := range RankAll(p) {
			for _, r := range list {
				if math.IsNaN(r.Score) || math.IsInf(r.Score, 0) {
					t.Fatalf("%s: %v", field, r.Score)
				}
			}
		}
		for _, c := range p.Candidates.Description {
			SuggestExcludes(c)
			SuggestCutAfter(c)
		}
		StructuredSuggestions(p)
	})
}
