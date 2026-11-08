package toolmeta

import (
	"encoding/json"
	"strings"
	"testing"
)

func good() Info {
	return Info{
		Authors:   []Author{{Name: "Ada Lovelace", Email: "ada@example.com", URL: "https://ada.example"}, {Name: "Grace Hopper"}},
		Company:   &Company{Name: "Acme GmbH", URL: "https://acme.example"},
		License:   "MIT",
		Links:     &Links{Homepage: "https://acme.example/x", Documentation: "https://acme.example/docs", Source: "https://github.com/acme/x", Issues: "https://github.com/acme/x/issues", Support: "https://acme.example/support", Security: "mailto:security@acme.example"},
		Copyright: "© 2026 Acme GmbH",
		Category:  "browser",
		Keywords:  []string{"crawl", "chrome", "Web scraping"},
	}
}

func has(ps []Problem, path, contains string) bool {
	for _, p := range ps {
		if p.Path == path && strings.Contains(p.Message, contains) {
			return true
		}
	}
	return false
}

func TestAGoodInfoHasNoProblems(t *testing.T) {
	for _, required := range []bool{true, false} {
		if ps := good().Validate("", required); len(ps) != 0 {
			t.Fatalf("required=%v: %v", required, ps)
		}
	}
	// only the two required things, and nothing optional
	min := Info{Authors: []Author{{Name: "A"}}, License: "Proprietary"}
	if ps := min.Validate("tools[0].versions[0]", true); len(ps) != 0 {
		t.Fatal(ps)
	}
}

func TestRequiredMeansAuthorsAndLicense(t *testing.T) {
	ps := Info{}.Validate("v", true)
	if !has(ps, "v.authors", "required") || !has(ps, "v.license", "required") || len(ps) != 2 {
		t.Fatalf("%v", ps)
	}
	// not required: an empty Info is fine, a present but broken one is still reported
	if ps := (Info{}).Validate("v", false); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	if ps := (Info{License: "not a licence!"}).Validate("v", false); !has(ps, "v.license", "SPDX") {
		t.Fatalf("%v", ps)
	}
}

func TestPathsAreRootedWhereTheCallerSays(t *testing.T) {
	bad := Info{Authors: []Author{{Name: "ok"}, {Name: ""}}}
	if ps := bad.Validate("tools[2].versions[1]", true); !has(ps, "tools[2].versions[1].authors[1].name", "required") {
		t.Fatalf("%v", ps)
	}
	if ps := bad.Validate("", true); !has(ps, "authors[1].name", "required") {
		t.Fatalf("a manifest has no prefix: %v", ps)
	}
}

func TestAuthors(t *testing.T) {
	cases := map[string]struct {
		a    []Author
		path string
		msg  string
	}{
		"empty name":        {[]Author{{Name: ""}}, "authors[0].name", "required"},
		"blank name":        {[]Author{{Name: "   "}}, "authors[0].name", "required"},
		"long name":         {[]Author{{Name: strings.Repeat("a", 101)}}, "authors[0].name", "longer than 100"},
		"control in name":   {[]Author{{Name: "Ada\x00"}}, "authors[0].name", "control"},
		"newline in name":   {[]Author{{Name: "Ada\nLovelace"}}, "authors[0].name", "control"},
		"bidi override":     {[]Author{{Name: "Ada ‮Lovelace"}}, "authors[0].name", "text-direction"},
		"line separator":    {[]Author{{Name: "Ada "}}, "authors[0].name", "control or text"},
		"email without @":   {[]Author{{Name: "A", Email: "ada.example.com"}}, "authors[0].email", "name@example.com"},
		"email with space":  {[]Author{{Name: "A", Email: "a b@example.com"}}, "authors[0].email", "name@example.com"},
		"email angle":       {[]Author{{Name: "A", Email: "Ada <a@example.com>"}}, "authors[0].email", "name@example.com"},
		"email too long":    {[]Author{{Name: "A", Email: strings.Repeat("a", 250) + "@x.io"}}, "authors[0].email", "254"},
		"url http":          {[]Author{{Name: "A", URL: "http://ada.example"}}, "authors[0].url", "https"},
		"url javascript":    {[]Author{{Name: "A", URL: "javascript:alert(1)"}}, "authors[0].url", "https"},
		"url data":          {[]Author{{Name: "A", URL: "data:text/html,x"}}, "authors[0].url", "https"},
		"url credentials":   {[]Author{{Name: "A", URL: "https://u:p@ada.example"}}, "authors[0].url", "credentials"},
		"url no host":       {[]Author{{Name: "A", URL: "https:///path"}}, "authors[0].url", "host"},
		"url with space":    {[]Author{{Name: "A", URL: "https://ada.example/a b"}}, "authors[0].url", "spaces"},
		"url mailto author": {[]Author{{Name: "A", URL: "mailto:a@example.com"}}, "authors[0].url", "https"},
		"url too long":      {[]Author{{Name: "A", URL: "https://a.example/" + strings.Repeat("x", 2000)}}, "authors[0].url", "2000"},
	}
	for name, c := range cases {
		if ps := ValidateAuthors("", c.a, true); !has(ps, c.path, c.msg) {
			t.Errorf("%s: %v", name, ps)
		}
	}
	many := make([]Author, MaxAuthors+1)
	for i := range many {
		many[i] = Author{Name: "A"}
	}
	if ps := ValidateAuthors("", many, true); !has(ps, "authors", "at most 20") {
		t.Fatalf("%v", ps)
	}
	if ps := ValidateAuthors("", many[:MaxAuthors], true); len(ps) != 0 {
		t.Fatalf("twenty authors are allowed: %v", ps)
	}
	// two people may share a name; unicode names are fine
	if ps := ValidateAuthors("", []Author{{Name: "Wang Wei"}, {Name: "Wang Wei"}, {Name: "Zoë Ångström 李雷"}}, true); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
	// every problem at once
	ps := ValidateAuthors("", []Author{{Name: ""}, {Name: "B", Email: "bad"}, {Name: "C", URL: "http://x"}}, true)
	if len(ps) != 3 {
		t.Fatalf("all problems must be reported: %v", ps)
	}
}

func TestCompany(t *testing.T) {
	if ps := ValidateCompany("", nil); len(ps) != 0 {
		t.Fatal(ps)
	}
	if ps := ValidateCompany("", &Company{Name: ""}); !has(ps, "company.name", "required") {
		t.Fatalf("%v", ps)
	}
	if ps := ValidateCompany("", &Company{Name: strings.Repeat("a", 101)}); !has(ps, "company.name", "100") {
		t.Fatalf("%v", ps)
	}
	if ps := ValidateCompany("", &Company{Name: "A", URL: "ftp://a.example"}); !has(ps, "company.url", "https") {
		t.Fatalf("%v", ps)
	}
	if ps := ValidateCompany("", &Company{Name: "A"}); len(ps) != 0 {
		t.Fatalf("a company needs only a name: %v", ps)
	}
}

func TestLinks(t *testing.T) {
	for _, k := range []string{"homepage", "documentation", "source", "issues", "support"} {
		l := &Links{}
		switch k {
		case "homepage":
			l.Homepage = "http://x.example"
		case "documentation":
			l.Documentation = "javascript:1"
		case "source":
			l.Source = "mailto:a@example.com"
		case "issues":
			l.Issues = "https://"
		case "support":
			l.Support = "/relative"
		}
		if ps := ValidateLinks("", l); !has(ps, "links."+k, "") || len(ps) != 1 {
			t.Errorf("%s: %v", k, ps)
		}
	}
	// security may be mailto: with a plain address and nothing else
	for _, ok := range []string{"mailto:security@acme.example", "https://acme.example/security"} {
		if ps := ValidateLinks("", &Links{Security: ok}); len(ps) != 0 {
			t.Errorf("%q: %v", ok, ps)
		}
	}
	for _, bad := range []string{"mailto:", "mailto:not-an-address", "mailto:a@example.com?subject=x", "mailto:a@example.com,b@example.com extra", "http://acme.example", "tel:123"} {
		if ps := ValidateLinks("", &Links{Security: bad}); !has(ps, "links.security", "") {
			t.Errorf("%q accepted", bad)
		}
	}
	if ps := ValidateLinks("", nil); len(ps) != 0 {
		t.Fatal(ps)
	}
}

func TestLicense(t *testing.T) {
	for _, ok := range []string{"MIT", "Apache-2.0", "MIT OR Apache-2.0", "GPL-3.0-or-later WITH Classpath-exception-2.0", "BSD-3-Clause", "Proprietary", "LicenseRef-Acme-1", "AGPL-3.0+"} {
		if ps := (Info{Authors: []Author{{Name: "A"}}, License: ok}).Validate("", true); len(ps) != 0 {
			t.Errorf("%q: %v", ok, ps)
		}
	}
	for _, bad := range []string{"not a licence!", "MIT OR", "OR MIT", "MIT; rm -rf /", "<script>", "MIT  OR  Apache-2.0", strings.Repeat("A", 101), "proprietary sort of"} {
		if ps := (Info{Authors: []Author{{Name: "A"}}, License: bad}).Validate("", true); !has(ps, "license", "") {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCopyrightCategoryKeywords(t *testing.T) {
	base := Info{Authors: []Author{{Name: "A"}}, License: "MIT"}
	with := func(mut func(*Info)) []Problem { i := base; mut(&i); return i.Validate("", true) }
	if ps := with(func(i *Info) { i.Copyright = strings.Repeat("c", 201) }); !has(ps, "copyright", "200") {
		t.Fatalf("%v", ps)
	}
	if ps := with(func(i *Info) { i.Copyright = "© x\x07" }); !has(ps, "copyright", "control") {
		t.Fatalf("%v", ps)
	}
	if ps := with(func(i *Info) { i.Category = "games" }); !has(ps, "category", "must be one of") {
		t.Fatalf("%v", ps)
	}
	for _, c := range Categories {
		if ps := with(func(i *Info) { i.Category = c }); len(ps) != 0 {
			t.Errorf("category %q: %v", c, ps)
		}
	}
	if ps := with(func(i *Info) {
		i.Keywords = make([]string, 11)
		for k := range i.Keywords {
			i.Keywords[k] = string(rune('a' + k))
		}
	}); !has(ps, "keywords", "at most 10") {
		t.Fatalf("%v", ps)
	}
	for _, bad := range []string{"", " lead", "-lead", strings.Repeat("k", 31), "semi;colon", "<b>", "emoji 😀"} {
		if ps := with(func(i *Info) { i.Keywords = []string{bad} }); !has(ps, "keywords[0]", "") {
			t.Errorf("keyword %q accepted", bad)
		}
	}
	if ps := with(func(i *Info) { i.Keywords = []string{"Crawl", "crawl"} }); !has(ps, "keywords[1]", "duplicate") {
		t.Fatalf("%v", ps)
	}
	if ps := with(func(i *Info) { i.Keywords = []string{"web scraping", "pdf-tools", "日本語", "v2"} }); len(ps) != 0 {
		t.Fatalf("%v", ps)
	}
}

// Info is embedded anonymously by the structs that carry it, so its fields sit at the top level of the JSON.
func TestInfoFlattensWhenEmbedded(t *testing.T) {
	type version struct {
		Info
		Name string `json:"name"`
	}
	raw := `{"name":"Browser","authors":[{"name":"Ada"}],"license":"MIT","company":{"name":"Acme"},"links":{"homepage":"https://a.example"},"keywords":["x"],"unknown":1}`
	var v version
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "Browser" || len(v.Authors) != 1 || v.Authors[0].Name != "Ada" || v.License != "MIT" || v.Company.Name != "Acme" || v.Links.Homepage != "https://a.example" || v.Keywords[0] != "x" {
		t.Fatalf("%+v", v)
	}
	out, _ := json.Marshal(version{Name: "n", Info: Info{Authors: []Author{{Name: "A"}}, License: "MIT"}})
	if string(out) != `{"authors":[{"name":"A"}],"license":"MIT","name":"n"}` && string(out) != `{"name":"n","authors":[{"name":"A"}],"license":"MIT"}` {
		t.Fatalf("an empty optional field must not be written: %s", out)
	}
}

func TestProblemString(t *testing.T) {
	if (Problem{Path: "a.b", Message: "m"}).String() != "a.b: m" || (Problem{Message: "m"}).String() != "m" {
		t.Fatal("String")
	}
}

func TestIsZero(t *testing.T) {
	if !(Info{}).IsZero() || (Info{License: "MIT"}).IsZero() || (Info{Authors: []Author{{Name: "A"}}}).IsZero() || (Info{Keywords: []string{"x"}}).IsZero() {
		t.Fatal("IsZero")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	raw, err := Encode(good())
	if err != nil {
		t.Fatal(err)
	}
	back := Decode(raw)
	if back == nil || back.Authors[0].Name != "Ada Lovelace" || back.Company.Name != "Acme GmbH" || back.License != "MIT" || back.Links.Security != "mailto:security@acme.example" || back.Keywords[2] != "Web scraping" {
		t.Fatalf("%+v", back)
	}
}

// A tool installed before the column existed, or a row that cannot be read, must never break a listing.
func TestDecodeNeverFails(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "{}", `{"authors":[]}`, "not json", `{"authors":"x"}`, `[1,2]`} {
		if got := Decode(raw); got != nil {
			t.Errorf("%q decoded to %+v", raw, got)
		}
	}
}

func TestMismatch(t *testing.T) {
	ada := []Author{{Name: "Ada Lovelace"}}
	cases := []struct {
		name            string
		claimed, actual Info
		wantContains    string
	}{
		{"agree", Info{Authors: ada, License: "MIT"}, Info{Authors: ada, License: "MIT", Copyright: "more"}, ""},
		{"repository states nothing (older format)", Info{}, Info{Authors: ada, License: "MIT"}, ""},
		{"repository states only a licence", Info{License: "MIT"}, Info{Authors: []Author{{Name: "Anyone"}}, License: "MIT"}, ""},
		{"authors differ", Info{Authors: ada}, Info{Authors: []Author{{Name: "Grace Hopper"}}}, "Ada Lovelace but the package says Grace Hopper"},
		{"different count", Info{Authors: ada}, Info{Authors: []Author{{Name: "Ada Lovelace"}, {Name: "Grace Hopper"}}}, "authors"},
		{"package says nobody", Info{Authors: ada}, Info{}, "nobody"},
		{"same names, other order", Info{Authors: []Author{{Name: "A"}, {Name: "B"}}}, Info{Authors: []Author{{Name: "B"}, {Name: "A"}}}, "authors"},
		{"licence differs", Info{License: "MIT"}, Info{License: "GPL-3.0"}, `"MIT" but the package says "GPL-3.0"`},
		{"email and url are not compared", Info{Authors: []Author{{Name: "A", Email: "a@x.io"}}}, Info{Authors: []Author{{Name: "A", Email: "other@x.io"}}}, ""},
	}
	for _, c := range cases {
		got := Mismatch(c.claimed, c.actual)
		if (c.wantContains == "") != (got == "") || !strings.Contains(got, c.wantContains) {
			t.Errorf("%s: %q (want it to contain %q)", c.name, got, c.wantContains)
		}
	}
}

func TestPublisherJSON(t *testing.T) {
	out, _ := json.Marshal(Publisher{Authors: []Author{}, GeneratedAt: "2026-10-31"})
	if string(out) != `{"authors":[],"generatedAt":"2026-10-31"}` {
		t.Fatalf("authors must always be a list, and an empty company or links left out: %s", out)
	}
}
