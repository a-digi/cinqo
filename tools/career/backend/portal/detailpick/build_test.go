package detailpick

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/portal"
)

func okRule() *portal.DescriptionRule {
	return &portal.DescriptionRule{Format: "structured", Parts: []portal.DescriptionPart{{Include: "#job-description", Heading: "auto", Exclude: []string{".share"}, CutAfter: []string{"Similar jobs"}}}}
}

func TestBuildYAMLFromABoxIsCanonicalAndValid(t *testing.T) {
	got, err := BuildYAML(Choices{
		DescriptionSelector: "#job-description",
		Fields: []FieldChoice{
			{Label: FieldPostedAt, Selector: "time.posted", Attribute: "datetime"},
			{Label: FieldCompany, Selector: ".company-name"},
			{Label: FieldTitle, Selector: "h1", Attribute: "text"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `fields:
    - label: description
      selector: '#job-description'
    - label: title
      selector: h1
    - label: company
      selector: .company-name
    - label: postedAt
      selector: time.posted
      attribute: datetime
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	req, err := portal.BuildJobDetailCrawlRequestFromYAML(got)
	if err != nil || len(req.Fields) != 4 || req.DescriptionRule != nil {
		t.Fatalf("the stored text must be valid for the save route: %v %+v", err, req)
	}
}

func TestBuildYAMLFromARule(t *testing.T) {
	got, err := BuildYAML(Choices{Description: okRule(), Fields: []FieldChoice{{Label: FieldCompany, Selector: ".c"}}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := portal.BuildJobDetailCrawlRequestFromYAML(got)
	if err != nil || req.DescriptionRule == nil || len(req.Fields) != 1 || req.Fields[0].Label != "company" {
		t.Fatalf("%v %+v\n%s", err, req, got)
	}
	if req.DescriptionRule.Parts[0].Include != "#job-description" || req.DescriptionRule.Parts[0].CutAfter[0] != "Similar jobs" {
		t.Errorf("the rule changed on the way: %+v", req.DescriptionRule)
	}
	// the rule alone is enough
	if _, err := BuildYAML(Choices{Description: okRule()}); err != nil {
		t.Errorf("a rule without other fields: %v", err)
	}
}

func TestBuildYAMLRefusals(t *testing.T) {
	long := strings.Repeat("a", maxSelectorRunes+1)
	for name, c := range map[string]Choices{
		"no description":          {Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1"}}},
		"both descriptions":       {DescriptionSelector: "#d", Description: okRule()},
		"unknown label":           {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: "salary", Selector: ".s"}}},
		"description as a field":  {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldDescription, Selector: ".s"}}},
		"duplicate field":         {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1"}, {Label: FieldTitle, Selector: "h2"}}},
		"empty selector":          {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "  "}}},
		"long selector":           {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: long}}},
		"long description box":    {DescriptionSelector: long},
		"newline in a selector":   {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1\n  - label: evil\n    selector: x"}}},
		"control character":       {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1\x00"}}},
		"bad attribute":           {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1", Attribute: "on click"}}},
		"attribute with a colon":  {DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1", Attribute: "a: b"}}},
		"a bad rule":              {Description: &portal.DescriptionRule{}},
		"a rule with a bad limit": {Description: &portal.DescriptionRule{Limit: 5, Parts: []portal.DescriptionPart{{Include: "#a"}}}},
	} {
		_, err := BuildYAML(c)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !IsInvalid(err) || !errors.Is(err, portal.ErrInvalidCrawlInstructions) {
			t.Errorf("%s: the refusal must be an invalid-instructions error: %v", name, err)
		}
	}
	if _, err := BuildYAML(Choices{DescriptionSelector: "#d"}); err != nil {
		t.Errorf("a box alone is valid: %v", err)
	}
}

// Text that would break a naive YAML writer must come back unchanged.
func TestBuildYAMLKeepsAwkwardSelectorsIntact(t *testing.T) {
	awkward := []string{
		"a: b", "- item", "&anchor", "*alias", "!tag x", "# not a comment", "{a: b}", "[a, b]", `it's`, `say "hi"`, `back\slash`, "%percent",
		"@at", "`tick`", "key: value #c", "|", ">", "?", "'", `"`, "null", "true", "123", "~", "a, b", "div[data-x=\"y: z\"]", "ünï̈code > p", "tab\there",
		"- label: evil", "x'\"y", ": leading", "trailing :", "---", "...",
	}
	for _, sel := range awkward {
		c := Choices{DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: sel}, {Label: FieldCompany, Selector: sel + " span"}}}
		text, err := BuildYAML(c)
		if err != nil {
			t.Errorf("%q: %v", sel, err)
			continue
		}
		back, lossy, err := ParseYAML(text)
		if err != nil || len(lossy) != 0 {
			t.Errorf("%q: parse %v %v\n%s", sel, err, lossy, text)
			continue
		}
		if back.DescriptionSelector != "#d" || len(back.Fields) != 2 || back.Fields[0].Selector != sel || back.Fields[1].Selector != sel+" span" {
			t.Errorf("%q came back as %+v\n%s", sel, back, text)
		}
		var req portal.CrawlRequest
		req, err = portal.BuildJobDetailCrawlRequestFromYAML(text)
		if err != nil || len(req.Fields) != 3 {
			t.Errorf("%q: the save route's reader: %v", sel, err)
		}
	}
}

func TestParseYAMLReadsWhatTheAIWroteAndSaysWhatItCannotShow(t *testing.T) {
	ai := `fields:
  - label: description
    selector: "[data-at='job-ad-content']"
  - label: company
    selector: .company
    attribute: text
  - label: tags
    selector: .tag
    multiple: true
  - label: salary
    selector: .salary
pagination: {}
`
	c, lossy, err := ParseYAML(ai)
	if err != nil {
		t.Fatal(err)
	}
	if c.DescriptionSelector != "[data-at='job-ad-content']" || len(c.Fields) != 1 || c.Fields[0].Label != FieldCompany {
		t.Fatalf("choices: %+v", c)
	}
	joined := strings.Join(lossy, "|")
	for _, want := range []string{"tags", "salary"} {
		if !strings.Contains(joined, want) {
			t.Errorf("lossy misses %q: %v", want, lossy)
		}
	}
	if strings.Contains(joined, "pagination") {
		t.Errorf("an empty pagination is nothing to lose: %v", lossy)
	}
	_, lossy, _ = ParseYAML("container: li\nmapping: {a: b}\npagination: {nextSelector: a, maxPages: 3}\nfields:\n  - label: description\n    selector: .d\n    attribute: href\n")
	joined = strings.Join(lossy, "|")
	for _, want := range []string{"container", "mapping", "pagination", "attribute on the description"} {
		if !strings.Contains(joined, want) {
			t.Errorf("lossy misses %q: %v", want, lossy)
		}
	}
	if _, _, err := ParseYAML("fields: ["); !IsInvalid(err) {
		t.Errorf("not yaml: %v", err)
	}
	c, lossy, err = ParseYAML("descriptionRule:\n  parts:\n    - include: '#a'\nfields:\n  - label: description\n    selector: .d\n")
	if err != nil || c.Description == nil || c.DescriptionSelector != "" || len(lossy) != 1 {
		t.Errorf("a rule next to a description field: %+v %v %v", c, lossy, err)
	}
}

// randString is hostile text for YAML (quotes, colons, anchors, unicode); about one string in ten also carries a control character, which
// BuildYAML must refuse, and one in twenty is empty.
func randString(r *rand.Rand, max int) string {
	pool := []string{"a", "b", "div", ".x", "#y", " ", ":", "-", "'", `"`, "\\", "#", "&", "*", "!", "{", "}", "[", "]", ",", "|", ">", "ü", "é", "日本", "😀", "%", "@", "`", "?", "~"}
	control := []string{"\n", "\x00", "\r"}
	if r.Intn(20) == 0 {
		return ""
	}
	n := 1 + r.Intn(max)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(pool[r.Intn(len(pool))])
	}
	if r.Intn(10) == 0 {
		b.WriteString(control[r.Intn(len(control))])
	}
	return b.String()
}

// Random choices, hostile text included: BuildYAML either refuses or returns text the save route accepts and that reads back as it was.
func TestBuildYAMLRandomChoicesAlwaysRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(186))
	built, refused := 0, 0
	for i := 0; i < 4000; i++ {
		c := Choices{}
		switch r.Intn(3) {
		case 0:
			c.DescriptionSelector = randString(r, 40)
		case 1:
			c.Description = &portal.DescriptionRule{Format: []string{"", "plain", "structured", "plain", "structured", "x"}[r.Intn(6)], Limit: []int{0, 1000, 60000, 0, 60000, 999, 300000}[r.Intn(7)]}
			for p := 0; p < 1+r.Intn(3); p++ {
				part := portal.DescriptionPart{Include: randString(r, 30), Heading: []string{"", "none", "auto", "selector", "zzz"}[r.Intn(5)]}
				if part.Heading == "selector" {
					part.HeadingSelector = randString(r, 10)
				}
				for e := 0; e < r.Intn(3); e++ {
					part.Exclude = append(part.Exclude, randString(r, 20))
				}
				for e := 0; e < r.Intn(3); e++ {
					part.CutAfter = append(part.CutAfter, randString(r, 20))
				}
				c.Description.Parts = append(c.Description.Parts, part)
			}
		}
		for _, f := range []Field{FieldTitle, FieldCompany, FieldLocation, FieldPostedAt, "bogus"} {
			if r.Intn(2) == 0 && (f != "bogus" || r.Intn(20) == 0) {
				c.Fields = append(c.Fields, FieldChoice{Label: f, Selector: randString(r, 40), Attribute: []string{"", "text", "datetime", "href", "href", "datetime", "text", "", "bad attr", "x:y"}[r.Intn(10)]})
			}
		}
		c.ReplaceTitle = r.Intn(5) == 0 // refused unless a title field was drawn too
		text, err := BuildYAML(c)
		if err != nil {
			if !IsInvalid(err) {
				t.Fatalf("a refusal that is not an invalid-instructions error: %v", err)
			}
			refused++
			continue
		}
		built++
		if _, err := portal.BuildJobDetailCrawlRequestFromYAML(text); err != nil {
			t.Fatalf("built text refused by the save route: %v\n%s", err, text)
		}
		back, lossy, err := ParseYAML(text)
		if err != nil || len(lossy) != 0 {
			t.Fatalf("parse: %v %v\n%s", err, lossy, text)
		}
		if again, err := BuildYAML(back); err != nil || again != text {
			t.Fatalf("not stable: %v\nfirst:\n%s\nagain:\n%s", err, text, again)
		}
		if !sameChoices(c, back) {
			t.Fatalf("round trip changed the choices:\n%+v\n%+v\n%s", c, back, text)
		}
	}
	if built < 300 || refused < 300 {
		t.Errorf("the generator is not exercising both sides: built %d refused %d", built, refused)
	}
}

func sameChoices(a, b Choices) bool {
	norm := func(c Choices) Choices {
		out := Choices{DescriptionSelector: c.DescriptionSelector, Description: c.Description}
		for _, l := range labelOrder {
			for _, f := range c.Fields {
				if f.Label == l {
					if f.Attribute = strings.TrimSpace(f.Attribute); f.Attribute == "text" {
						f.Attribute = ""
					}
					out.Fields = append(out.Fields, f)
				}
			}
		}
		return out
	}
	ja, _ := json.Marshal(norm(a))
	jb, _ := json.Marshal(norm(b))
	return reflect.DeepEqual(ja, jb)
}

func FuzzBuildYAML(f *testing.F) {
	for _, s := range []string{"h1", "a: b", "x\ny", "- label: evil", "", "\x00", "div > p", "😀"} {
		f.Add(s, "datetime", s)
	}
	f.Fuzz(func(t *testing.T, sel, attr, include string) {
		c := Choices{
			Fields:      []FieldChoice{{Label: FieldTitle, Selector: sel, Attribute: attr}},
			Description: &portal.DescriptionRule{Parts: []portal.DescriptionPart{{Include: include, Exclude: []string{sel}, CutAfter: []string{include}}}},
		}
		text, err := BuildYAML(c)
		if err != nil {
			return
		}
		if _, err := portal.BuildJobDetailCrawlRequestFromYAML(text); err != nil {
			t.Fatalf("built text refused: %v\n%s", err, text)
		}
		back, lossy, err := ParseYAML(text)
		if err != nil || len(lossy) != 0 || !sameChoices(c, back) {
			t.Fatalf("round trip: %v %v\n%s", err, lossy, text)
		}
	})
}

// ---- the title option (decision D2) ----

func TestReplaceTitleIsOffByDefaultAndNeedsTheTitleField(t *testing.T) {
	c := Choices{DescriptionSelector: "#d", Fields: []FieldChoice{{Label: FieldTitle, Selector: "h1"}}}
	plain, err := BuildYAML(c)
	if err != nil || strings.Contains(plain, "replaceTitle") {
		t.Fatalf("default: %v\n%s", err, plain)
	}
	c.ReplaceTitle = true
	text, err := BuildYAML(c)
	if err != nil || !strings.Contains(text, "replaceTitle: true") {
		t.Fatalf("on: %v\n%s", err, text)
	}
	req, err := portal.BuildJobDetailCrawlRequestFromYAML(text)
	if err != nil || !req.ReplaceTitle {
		t.Fatalf("the save route's reader: %v %+v", err, req)
	}
	back, lossy, err := ParseYAML(text)
	if err != nil || len(lossy) != 0 || !back.ReplaceTitle || len(back.Fields) != 1 {
		t.Fatalf("read back: %+v %v %v", back, lossy, err)
	}
	for name, bad := range map[string]Choices{
		"no title field": {DescriptionSelector: "#d", ReplaceTitle: true},
		"other field":    {DescriptionSelector: "#d", ReplaceTitle: true, Fields: []FieldChoice{{Label: FieldCompany, Selector: ".c"}}},
	} {
		if _, err := BuildYAML(bad); !errors.Is(err, ErrReplaceTitleNeedsTitle) || !IsInvalid(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
