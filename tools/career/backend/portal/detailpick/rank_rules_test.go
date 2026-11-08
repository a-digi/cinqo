package detailpick

import (
	"strings"
	"testing"
)

func why(r Ranked) string { return strings.Join(r.Why, "|") }

func byValue(list []Ranked, value string) Ranked {
	for _, r := range list {
		if r.Value == value {
			return r
		}
	}
	return Ranked{}
}

// Each rule is pinned on its own: two candidates that differ in nothing else, and the one with the property must score higher (or lower)
// and say why.

func TestRuleTitleAgreesWithThePageTitle(t *testing.T) {
	p := Page{Structured: Structured{Meta: Meta{OgTitle: "Backend Engineer - Acme"}}, Candidates: Candidates{Title: []Candidate{cand("a", "Cat pictures", "h1"), cand("b", "Backend Engineer", "h1")}}}
	r := Rank(p, FieldTitle)
	a, b := byValue(r, "Cat pictures"), byValue(r, "Backend Engineer")
	if b.Score-a.Score < 5 || !strings.Contains(why(b), "agrees with the page's own title") {
		t.Errorf("agreement: %+v vs %+v", b, a)
	}
}

func TestRuleTitleSharesMostWords(t *testing.T) {
	p := Page{Title: "Backend Engineer remote role", Candidates: Candidates{Title: []Candidate{cand("a", "Senior Backend Engineer Berlin remote", "h1"), cand("b", "Cat pictures", "h1")}}}
	r := Rank(p, FieldTitle)
	a, b := byValue(r, "Senior Backend Engineer Berlin remote"), byValue(r, "Cat pictures")
	if !strings.Contains(why(a), "shares most words") || strings.Contains(why(b), "shares most words") {
		t.Errorf("overlap bonus: %q / %q", why(a), why(b))
	}
	if !strings.Contains(why(b), "does not look like the page's own title") || strings.Contains(why(a), "does not look like") {
		t.Errorf("overlap penalty: %q / %q", why(a), why(b))
	}
	if a.Score-b.Score < 4.9 {
		t.Errorf("scores %v %v", a.Score, b.Score)
	}
}

func TestRuleTitleSiteNameAndLength(t *testing.T) {
	p := Page{Structured: Structured{Meta: Meta{OgSiteName: "Jobs R Us"}}, Candidates: Candidates{Title: []Candidate{cand("a", "Jobs R Us", "h1"), cand("b", "Plumber", "h1"), cand("c", strings.Repeat("long title ", 20), "h1"), cand("d", "ab", "h1")}}}
	r := Rank(p, FieldTitle)
	if !strings.Contains(why(byValue(r, "Jobs R Us")), "name of the site") || strings.Contains(why(byValue(r, "Plumber")), "name of the site") {
		t.Errorf("site name: %v", r)
	}
	if !strings.Contains(why(byValue(r, strings.Repeat("long title ", 20))), "very long") || !strings.Contains(why(byValue(r, "ab")), "very short") {
		t.Errorf("length: %v", r)
	}
}

func TestRuleTitleHeadingLevels(t *testing.T) {
	p := Page{Candidates: Candidates{Title: []Candidate{cand("a", "Same text", "div"), cand("b", "Same text", "h2"), cand("c", "Same text", "h1")}}}
	r := Rank(p, FieldTitle)
	// an h1 beats a second-level heading, which scores like an element named like a title
	if r[0].Selector != "c" || !strings.Contains(why(r[0]), "first-level heading") || r[1].Score != r[2].Score || r[0].Score-r[1].Score < 1.9 {
		t.Errorf("h1 first: %+v", r)
	}
}

func TestRuleDate(t *testing.T) {
	mk := func(sel, val, attr string) Candidate {
		return Candidate{Selector: sel, Unique: true, Value: val, Chars: len(val), Tag: "span", Attribute: attr, InMain: true}
	}
	p := Page{Structured: Structured{JSONLD: []JobPosting{{DatePosted: "2026-10-28T09:00:00+00:00"}}}, Candidates: Candidates{PostedAt: []Candidate{
		mk("a", "Feste Anstellung", ""), mk("b", "2026-09-01", ""), mk("c", "2026-09-01", "datetime"), mk("d", "2026-10-28", ""),
	}}}
	r := Rank(p, FieldPostedAt)
	if !strings.Contains(why(byValue(r, "Feste Anstellung")), "does not read like a date") || strings.Contains(why(byValue(r, "2026-09-01")), "does not read like") {
		t.Errorf("not a date: %v", r)
	}
	var plain, machine Ranked
	for _, x := range r {
		if x.Selector == "b" {
			plain = x
		}
		if x.Selector == "c" {
			machine = x
		}
	}
	if machine.Score-plain.Score < 2.9 || !strings.Contains(why(machine), "machine date") {
		t.Errorf("datetime attribute: %+v vs %+v", machine, plain)
	}
	d := byValue(r, "2026-10-28")
	if !strings.Contains(why(d), "agrees with the date the page states") || strings.Contains(why(plain), "agrees with the date") {
		t.Errorf("json-ld date (a timestamp on the page's side): %+v", d)
	}
	if long := Rank(Page{Candidates: Candidates{PostedAt: []Candidate{mk("x", "2026-10-28 "+strings.Repeat("z", 70), "")}}}, FieldPostedAt); !strings.Contains(why(long[0]), "long for a date") {
		t.Errorf("long: %+v", long)
	}
}

func TestRuleLocationLength(t *testing.T) {
	p := Page{Candidates: Candidates{Location: []Candidate{cand("a", strings.Repeat("Berlin ", 20), "span"), cand("b", "B", "span"), cand("c", "Berlin", "span")}}}
	r := Rank(p, FieldLocation)
	if !strings.Contains(why(byValue(r, strings.Repeat("Berlin ", 20))), "long for a location") || !strings.Contains(why(byValue(r, "B")), "too short") || r[0].Selector != "c" {
		t.Errorf("%+v", r)
	}
}

func TestRuleCommonPlacementAndUniqueness(t *testing.T) {
	mk := func(sel, placed string, main, unique bool) Candidate {
		return Candidate{Selector: sel, Unique: unique, Value: "Acme", Chars: 4, Tag: "div", InMain: main, PlacedIn: placed}
	}
	p := Page{Candidates: Candidates{Company: []Candidate{mk("a", "consent", false, true), mk("b", "nav", false, true), mk("c", "", false, true), mk("d", "", true, true), mk("e", "", true, false)}}}
	r := Rank(p, FieldCompany)
	order := ""
	for _, x := range r {
		order += x.Selector
	}
	want := []string{"d", "c", "e", "b", "a"}
	for i, w := range want {
		if r[i].Selector != w {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
	if !strings.Contains(why(r[0]), "its selector matches only this element") || strings.Contains(why(r[2]), "matches only this element") {
		t.Errorf("uniqueness reasons: %q / %q", why(r[0]), why(r[2]))
	}
	if !strings.Contains(why(r[4]), "consent") || !strings.Contains(why(r[3]), "nav") || !strings.Contains(why(r[2]), "also matches other elements") {
		t.Errorf("reasons: %q %q %q", why(r[4]), why(r[3]), why(r[2]))
	}
}

func TestRuleDescription(t *testing.T) {
	base := func(sel string) Candidate {
		return Candidate{Selector: sel, Unique: true, Chars: 3000, Tag: "div", InMain: true, ShareOfPage: 0.4}
	}
	score := func(c Candidate, p Page) Ranked {
		p.Candidates.Description = []Candidate{c}
		return Rank(p, FieldDescription)[0]
	}
	plain := score(base("p"), Page{})
	byAttr := base("a")
	byAttr.ByAttr = true
	if s := score(byAttr, Page{}); s.Score-plain.Score < 1.4 || !strings.Contains(why(s), "id or class names it") {
		t.Errorf("byAttr: %+v vs %+v", s, plain)
	}
	main := base("m")
	main.Tag = "main"
	if s := score(main, Page{}); plain.Score-s.Score < 0.9 || !strings.Contains(why(s), "whole-page container") {
		t.Errorf("container: %+v", s)
	}
	short := base("s")
	short.Chars = 200
	if s := score(short, Page{}); !strings.Contains(why(s), "very short") {
		t.Errorf("short: %+v", s)
	}
	furn := base("f")
	furn.Furniture = []Furniture{{Kind: "share", Chars: 2000}}
	if s := score(furn, Page{}); !strings.Contains(why(s), "mostly page furniture") {
		t.Errorf("furniture: %+v", s)
	}
	furn.Furniture = []Furniture{{Kind: "share", Chars: 1200}}
	if s := score(furn, Page{}); !strings.Contains(why(s), "a lot of page furniture") {
		t.Errorf("some furniture: %+v", s)
	}
	ld := Page{Structured: Structured{JSONLD: []JobPosting{{DescriptionChars: 3200}}}}
	if s := score(base("l"), ld); !strings.Contains(why(s), "about as long as the description the page states") {
		t.Errorf("json-ld length: %+v", s)
	}
	part := base("q")
	part.Chars = 1000
	if s := score(part, Page{Structured: Structured{JSONLD: []JobPosting{{DescriptionChars: 3200}}}}); !strings.Contains(why(s), "maybe only a part") {
		t.Errorf("part: %+v", s)
	}
	whole := base("w")
	whole.ShareOfPage = 0.95
	if s := score(whole, Page{}); !strings.Contains(why(s), "almost the whole page") {
		t.Errorf("whole page: %+v", s)
	}
	tiny := base("t")
	tiny.ShareOfPage = 0.02
	if s := score(tiny, Page{}); !strings.Contains(why(s), "a small part of the page") {
		t.Errorf("tiny: %+v", s)
	}
	for _, placed := range []string{"consent", "nav"} {
		c := base("c")
		c.PlacedIn = placed
		if s := score(c, Page{}); !strings.Contains(why(s), "inside") {
			t.Errorf("%s: %+v", placed, s)
		}
	}
}

func TestParseYAMLSaysWhenAKnownFieldCollectsEveryMatch(t *testing.T) {
	_, lossy, err := ParseYAML("fields:\n  - label: description\n    selector: .d\n  - label: company\n    selector: .c\n    multiple: true\n")
	if err != nil || len(lossy) != 1 || !strings.Contains(lossy[0], "company") {
		t.Fatalf("%v %v", lossy, err)
	}
}

func TestAFurnitureLineMayStartWithTheTextAndGoOn(t *testing.T) {
	for _, line := range []string{"Apply now to join the team", "Apply now: today", "Similar jobs near you"} {
		s := good("1")
		s.Values["description"] = longText(600) + "\n" + line
		if _, ok := codes(Flags([]Sample{s}, Fields))["description_furniture_words|description"]; !ok {
			t.Errorf("%q was not flagged", line)
		}
	}
}

func TestDiffBeyondTheExactLimitComparesSetsOfLines(t *testing.T) {
	var a, b []string
	for i := 0; i < maxDiffLines; i++ {
		a = append(a, "line "+strings.Repeat("x", i%7)+string(rune('a'+i%26))+strings.Repeat("y", i/26))
	}
	for i := len(a) - 1; i >= 0; i-- {
		b = append(b, a[i])
	}
	d := Diff(strings.Join(a, "\n"), strings.Join(b, "\n"))
	if d.Kept != maxDiffLines || !d.Unchanged {
		t.Errorf("past the exact limit the order is not compared (a reorder of the same lines is no change): %+v", d)
	}
	small := Diff("a\nb\nc", "c\nb\na")
	if small.Unchanged {
		t.Error("within the limit a reorder is a change")
	}
}

func TestRuleCompanyLength(t *testing.T) {
	p := Page{Candidates: Candidates{Company: []Candidate{
		cand("a", strings.Repeat("c", 130), "div"), cand("b", strings.Repeat("c", 90), "div"), cand("c", "C", "div"), cand("d", "Acme", "div"),
	}}}
	r := Rank(p, FieldCompany)
	if !strings.Contains(why(byValue(r, strings.Repeat("c", 130))), "too long") || !strings.Contains(why(byValue(r, strings.Repeat("c", 90))), "long for a company") ||
		!strings.Contains(why(byValue(r, "C")), "too short") || r[0].Value != "Acme" {
		t.Errorf("%+v", r)
	}
	if byValue(r, strings.Repeat("c", 130)).Score >= byValue(r, strings.Repeat("c", 90)).Score {
		t.Error("longer is worse")
	}
}
