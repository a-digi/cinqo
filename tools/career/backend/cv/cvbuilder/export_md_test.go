package cvbuilder

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"career-tool-backend/cv/cvbuilder/templates"
)

// The Markdown export of a CV (plan 251, step 5).

func richCv() templates.CvData {
	return templates.CvData{
		FullName: "Ada Lovelace", Headline: "Senior Engineer", Location: "Berlin, Germany",
		ExternalLinks: []templates.ExternalLink{{Platform: "LinkedIn", URL: "https://linkedin.com/in/ada"}, {Platform: "Portfolio", URL: "https://example.com/my page"}},
		Summary:       "Builds things.\nLoves maths.",
		Skills:        []string{"Go", "snake_case", "C++"},
		Experience: []templates.ExperienceEntry{
			{Company: "Acme", Title: "Lead", StartDate: "2020", Description: "- Led a team\n- Cut costs by 30 %\nTechnologies: Go, SQL"},
			{Title: "Dev", StartDate: "2018", EndDate: "2019"},
		},
		Education: []templates.EducationEntry{{Institution: "TU Berlin", Degree: "MSc", FieldOfStudy: "CS", StartDate: "2010", EndDate: "2012", Description: "Thesis on compilers"}},
		Courses:   []templates.CourseEntry{{Title: "Kubernetes", Provider: "CNCF", CompletedDate: "2023-05"}},
		Languages: []templates.LanguageEntry{{Language: "German", Level: "Native"}, {Language: "English", Level: "C1"}},
		Hobbies:   []templates.HobbyEntry{{Name: "Chess"}, {Name: "Climbing"}},
	}
}

const richMarkdown = `# Ada Lovelace

Senior Engineer

Berlin, Germany · [LinkedIn](https://linkedin.com/in/ada) · [Portfolio](<https://example.com/my page>)

## Summary

Builds things.

Loves maths.

## Experience

### Lead – Acme

*2020 – Present*

- Led a team
- Cut costs by 30 %

Technologies: Go, SQL

### Dev

*2018 – 2019*

## Education

### MSc, CS – TU Berlin

*2010 – 2012*

Thesis on compilers

## Courses

### Kubernetes – CNCF

*2023-05*

## Skills

Go, snake_case, C++

## Languages

German (Native), English (C1)

## Hobbies

Chess, Climbing
`

func TestTheMarkdownOfACvIsExactlyWhatItsDataSays(t *testing.T) {
	if got := RenderMarkdown(richCv()); got != richMarkdown {
		t.Errorf("the document differs.\n--- got ---\n%s\n--- want ---\n%s", got, richMarkdown)
	}
}

func TestASectionWithoutEntriesIsLeftOutAndANameAloneIsADocument(t *testing.T) {
	if got := RenderMarkdown(templates.CvData{FullName: "Ada"}); got != "# Ada\n" {
		t.Errorf("a name alone: %q", got)
	}
	if got := RenderMarkdown(templates.CvData{}); got != "# CV\n" {
		t.Errorf("no name at all: %q", got)
	}
	if got := RenderMarkdown(templates.CvData{FullName: "Ada #"}); got != "# Ada \\#\n" {
		t.Errorf("a name ending in a hash: %q", got)
	}
	d := richCv()
	d.Education, d.Courses, d.Languages, d.Hobbies, d.Skills, d.Summary = nil, []templates.CourseEntry{}, nil, []templates.HobbyEntry{{Name: "  "}}, []string{"", "  "}, "  \n "
	got := RenderMarkdown(d)
	for _, h := range []string{"## Education", "## Courses", "## Languages", "## Hobbies", "## Skills", "## Summary"} {
		if strings.Contains(got, h) {
			t.Errorf("%s must not appear without entries:\n%s", h, got)
		}
	}
	if !strings.Contains(got, "## Experience") {
		t.Errorf("the experience stays:\n%s", got)
	}
	if strings.Contains(got, "\n\n\n") || !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("one blank line between blocks, one line break at the end: %q", got)
	}
}

func TestEverySectionHasItsPlaceInTheDocument(t *testing.T) {
	got := RenderMarkdown(richCv())
	last := -1
	for _, h := range []string{"# Ada Lovelace", "## Summary", "## Experience", "## Education", "## Courses", "## Skills", "## Languages", "## Hobbies"} {
		i := strings.Index(got, h)
		if i < 0 || i <= last {
			t.Fatalf("%q is missing or out of order in:\n%s", h, got)
		}
		last = i
	}
}

func TestWhatMarkdownWouldReadAsMarkupIsEscaped(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"emphasis", "a *b* c", `a \*b\* c`},
		{"underscore at a word edge", "_x_ and y_", `\_x\_ and y\_`},
		{"underscore inside a word", "snake_case_name", "snake_case_name"},
		{"brackets and a link", "[x](http://e.com)", `\[x\](http://e.com)`},
		{"angle brackets", "<b>x</b>", `\<b\>x\</b\>`},
		{"backtick", "run `ls`", "run \\`ls\\`"},
		{"backslash", `a\b`, `a\\b`},
		{"a hash is not markup inside a line", "C# and F#", "C# and F#"},
		{"a closing hash in a list line stays", "C#", "C#"},
	}
	for _, c := range cases {
		if got := escapeInline(c.in); got != c.want {
			t.Errorf("%s: escapeInline(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
	for in, want := range map[string]string{"C #": `C \#`, "F#": `F\#`, "C# and F#": `C# and F\#`, "a*": `a\*`, "# x #": `# x \#`, "": ""} {
		if got := escapeHeading(in); got != want {
			t.Errorf("escapeHeading(%q) = %q, want %q", in, got, want)
		}
	}
	lines := []struct{ in, want string }{
		{"# not a heading", `\# not a heading`}, {"## x", `\## x`}, {"- not a bullet", `\- not a bullet`}, {"+ nor this", `\+ nor this`}, {"1. nor this", `1\. nor this`},
		{"12) nor this", `12\) nor this`}, {"> not a quote", `\> not a quote`}, {"===", `\===`}, {"#hashtag is fine", "#hashtag is fine"}, {"2020 is fine", "2020 is fine"}, {"-5 degrees", "-5 degrees"},
	}
	for _, c := range lines {
		if got := escapeLine(c.in); got != c.want {
			t.Errorf("escapeLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHostileTextCannotMakeMarkdownThatTheCvDidNotWrite(t *testing.T) {
	nasty := []string{"# heading", "## heading", "- item", "* item", "+ item", "1. item", "> quote", "[link](http://evil)", "<script>alert(1)</script>", "![img](http://evil/x.png)", "`code`", "**bold**", "___", "---", "===", "\\", "|a|b|", "# x #"}
	for _, n := range nasty {
		d := templates.CvData{
			FullName: n, Headline: n, Location: n, Summary: n,
			ExternalLinks: []templates.ExternalLink{{Platform: n, URL: "https://e.com"}},
			Skills:        []string{n}, Experience: []templates.ExperienceEntry{{Title: n, Company: n, StartDate: n, EndDate: n, Description: n + "\n" + n}},
			Education: []templates.EducationEntry{{Institution: n, Degree: n, FieldOfStudy: n, StartDate: n, EndDate: n, Description: n}},
			Courses:   []templates.CourseEntry{{Title: n, Provider: n, CompletedDate: n}}, Languages: []templates.LanguageEntry{{Language: n, Level: n}}, Hobbies: []templates.HobbyEntry{{Name: n}},
		}
		got := RenderMarkdown(d)
		// the only headings are the ones the document writes itself: one "# " for the name, "## " for each of the 7 sections, "### " for each of the 3 entries (experience, education, course)
		headings := regexp.MustCompile(`(?m)^#{1,6} `).FindAllString(got, -1)
		if len(headings) != 1+7+3 {
			t.Errorf("%q: %d headings, want 11:\n%s", n, len(headings), got)
		}
		for i, line := range strings.Split(got, "\n") {
			l := strings.TrimSpace(line)
			// a line is not a quote, and a list item only where the document writes a bullet: the description lines (which were "- " lines or paragraphs)
			if strings.HasPrefix(l, ">") || strings.HasPrefix(l, "<") || strings.HasPrefix(l, "![") {
				t.Errorf("%q: line %d starts as markup: %q", n, i, line)
			}
		}
		// what is left of the document once every escaped character is taken out must hold no tag, no image and no link to the hostile address
		left := regexp.MustCompile(`\\.`).ReplaceAllString(got, "")
		if strings.Contains(left, "<") || strings.Contains(left, "![") || strings.Contains(left, "](http://evil") {
			t.Errorf("%q: markup got through:\n%s", n, got)
		}
	}
}

func TestADescriptionKeepsItsBulletsAndItsParagraphs(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bullets stay consecutive", "- a\n- b\n- c", "- a\n- b\n- c"},
		{"other bullet signs become dashes", "• a\n* b\n– c", "- a\n- b\n- c"},
		{"nested bullets keep their indent", "- a\n  - b\n    - c", "- a\n  - b\n    - c"},
		{"paragraphs are separated so the breaks survive", "line one\nline two", "line one\n\nline two"},
		{"a paragraph after bullets", "- a\nplain", "- a\n\nplain"},
		{"bullets after a paragraph", "plain\n- a\n- b", "plain\n\n- a\n- b"},
		{"blank lines and trailing spaces go", "a   \n\n\n- b  \n", "a\n\n- b"},
		{"windows line endings", "a\r\n- b\r\n", "a\n\n- b"},
		{"old Mac line endings", "a\rb", "a\n\nb"},
		{"nesting deeper than eight spaces is cut to eight", "            - deep", "        - deep"},
		{"a tab is two spaces of nesting", "\t- tabbed", "  - tabbed"},
		{"a dash with nothing after it is a paragraph", "-\nx", "\\-\n\nx"},
		{"markup in a bullet is escaped", "- *bold* [x]", `- \*bold\* \[x\]`},
		{"a paragraph that looks like a heading", "# big", `\# big`},
		{"control characters are dropped", "a\x00b\x1bc", "abc"},
		{"nothing", "  \n\t\n", ""},
	}
	for _, c := range cases {
		if got := mdBlock(c.in); got != c.want {
			t.Errorf("%s: mdBlock(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestLinksAreWrittenSoTheyWork(t *testing.T) {
	cases := []struct{ platform, url, want string }{
		{"LinkedIn", "https://linkedin.com/in/ada", "[LinkedIn](https://linkedin.com/in/ada)"},
		{"Site", "https://e.com/a b", "[Site](<https://e.com/a b>)"},
		{"Wiki", "https://e.com/a_(b)", "[Wiki](<https://e.com/a_(b)>)"},
		{"", "https://e.com", "[https://e.com](https://e.com)"},
		{"GitHub", "", "GitHub"},
		{"", "", ""},
		{"Bad", "https://e.com/<x>", "Bad: https://e.com/\\<x\\>"},
		{"A *star*", "https://e.com", `[A \*star\*](https://e.com)`},
		{" Padded ", "  https://e.com  ", "[Padded](https://e.com)"},
	}
	for _, c := range cases {
		if got := mdLink(c.platform, c.url); got != c.want {
			t.Errorf("mdLink(%q, %q) = %q, want %q", c.platform, c.url, got, c.want)
		}
	}
}

func TestAnEntryIsATitleADateLineAndItsText(t *testing.T) {
	cases := []struct{ name, title, company, dates, desc, want string }{
		{"all of it", "Lead", "Acme", "2020 – Present", "Did things", "### Lead – Acme\n\n*2020 – Present*\n\nDid things"},
		{"no company", "Lead", "", "2020", "", "### Lead\n\n*2020*"},
		{"no title", "", "Acme", "", "", "### Acme"},
		{"nothing to call it by", "", "  ", "2020", "text", ""},
		{"markup in the head", "C# *Lead*", "R&D_", "", "", "### C# \\*Lead\\* – R&D\\_"},
		{"a title that ends in a hash", "F#", "", "", "", "### F\\#"},
		{"a company that ends in a hash", "Dev", "C #", "", "", "### Dev – C \\#"},
		{"line breaks in a title", "A\nB", "", "", "", "### A B"},
	}
	for _, c := range cases {
		if got := mdEntry(c.title, c.company, c.dates, c.desc); got != c.want {
			t.Errorf("%s: mdEntry = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTheDatesOfAnExperienceFollowTheSameRuleAsEducation(t *testing.T) {
	d := templates.CvData{FullName: "A", Experience: []templates.ExperienceEntry{
		{Title: "Still", StartDate: "2020"}, {Title: "Done", StartDate: "2018", EndDate: "2019"}, {Title: "OnlyEnd", EndDate: "2015"}, {Title: "NoDates"},
	}}
	got := RenderMarkdown(d)
	for _, want := range []string{"### Still\n\n*2020 – Present*", "### Done\n\n*2018 – 2019*", "### OnlyEnd\n\n*2015*"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "### NoDates\n") || strings.Contains(got, "### NoDates\n\n*") {
		t.Errorf("an entry without dates has no date line:\n%s", got)
	}
}

func TestSkillsLanguagesAndHobbiesAreOneLineEachAndEmptyItemsAreSkipped(t *testing.T) {
	d := templates.CvData{FullName: "A", Skills: []string{"Go", " ", "C#", "a\nb"}, Languages: []templates.LanguageEntry{{Language: "German", Level: "Native"}, {Level: "C1"}, {Language: "French"}}, Hobbies: []templates.HobbyEntry{{Name: "  Chess "}, {}}}
	got := RenderMarkdown(d)
	for _, want := range []string{"## Skills\n\nGo, C#, a b\n", "## Languages\n\nGerman (Native), French\n", "## Hobbies\n\nChess\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestTheHeadlineAndTheContactLine(t *testing.T) {
	got := RenderMarkdown(templates.CvData{FullName: "A", Headline: "# Boss", Location: "Bonn"})
	if got != "# A\n\n\\# Boss\n\nBonn\n" {
		t.Errorf("a headline that looks like a heading is escaped once: %q", got)
	}
	if got := RenderMarkdown(templates.CvData{FullName: "A", ExternalLinks: []templates.ExternalLink{{Platform: "X", URL: "https://x.io"}}}); got != "# A\n\n[X](https://x.io)\n" {
		t.Errorf("links without a location: %q", got)
	}
}

// --- the route ---

func TestTheExportRouteAnswersWithTheStoredCvAsADownload(t *testing.T) {
	pid := setupCvDB(t)
	doc, err := InsertCvDocument(pid, "classic", `Ada – "Backend"/2026`, richCv(), "media-1", "")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	ExportHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-documents/export?cvDocumentId="+doc.ID+"&format=md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != richMarkdown {
		t.Errorf("the body must be the Markdown of the stored data:\n%s", rec.Body.String())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "text/markdown; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", h)
	}
	if got := h.Get("Content-Disposition"); got != `attachment; filename="Ada Backend2026.md"; filename*=UTF-8''Ada%20Backend2026.md` {
		t.Errorf("Content-Disposition = %q", got)
	}
}

func TestTheExportRouteRefusesWhatItCannotAnswer(t *testing.T) {
	pid := setupCvDB(t)
	doc, _ := InsertCvDocument(pid, "classic", "CV", richCv(), "media-1", "")
	call := func(method, query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ExportHandler(rec, httptest.NewRequest(method, "/cv-documents/export?"+query, nil))
		return rec
	}
	for _, c := range []struct {
		name, method, query string
		want                int
	}{
		{"unknown id", http.MethodGet, "cvDocumentId=nope&format=md", http.StatusNotFound},
		{"no id", http.MethodGet, "format=md", http.StatusBadRequest},
		{"an empty id", http.MethodGet, "cvDocumentId=&format=md", http.StatusBadRequest},
		{"no format", http.MethodGet, "cvDocumentId=" + doc.ID, http.StatusBadRequest},
		{"a format that is not offered", http.MethodGet, "cvDocumentId=" + doc.ID + "&format=pdf", http.StatusBadRequest},
		{"a capital letter is not md", http.MethodGet, "cvDocumentId=" + doc.ID + "&format=MD", http.StatusBadRequest},
		{"a POST", http.MethodPost, "cvDocumentId=" + doc.ID + "&format=md", http.StatusMethodNotAllowed},
		{"a DELETE", http.MethodDelete, "cvDocumentId=" + doc.ID + "&format=md", http.StatusMethodNotAllowed},
	} {
		rec := call(c.method, c.query)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
		if rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: a refusal must not be a download", c.name)
		}
	}
}

func TestTheNameOfTheDownloadedFileCannotCarryAnythingButAName(t *testing.T) {
	cases := []struct{ title, base string }{
		{"Ada - Backend", "Ada - Backend"},
		{`  spaced   out  `, "spaced out"},
		{`a/b\c:d*e?f"g<h>i|j`, "abcdefghij"},
		{"line\r\nbreak\tand tab", "line break and tab"},
		{"../../etc/passwd", "etcpasswd"},
		{"Ünal Çelik – CV", "Ünal Çelik – CV"[:0] + "Ünal Çelik  CV"},
		{"", "cv"}, {"???", "cv"}, {"\r\n", "cv"},
		{strings.Repeat("a", 300), strings.Repeat("a", 100)},
		{strings.Repeat("a", 100), strings.Repeat("a", 100)}, {strings.Repeat("a", 101), strings.Repeat("a", 100)}, {strings.Repeat("a", 120), strings.Repeat("a", 100)},
		{strings.Repeat("a", 99) + " b", strings.Repeat("a", 99)}, // a cut that ends on a space does not leave it
		{strings.Repeat("ü", 300), strings.Repeat("ü", 100)},
		{"dash-and_underscore 2026", "dash-and_underscore 2026"},
	}
	for _, c := range cases {
		want := strings.Join(strings.Fields(c.base), " ")
		if got := exportBaseName(c.title); got != want {
			t.Errorf("exportBaseName(%q) = %q, want %q", c.title, got, want)
		}
	}
	h := exportContentDisposition("Ünal Çelik CV", "md")
	if h != `attachment; filename="_nal _elik CV.md"; filename*=UTF-8''%C3%9Cnal%20%C3%87elik%20CV.md` {
		t.Errorf("a name with letters outside ASCII: %s", h)
	}
	for _, title := range []string{"x\r\nSet-Cookie: a=b", `"; filename="evil.exe`, "a\x00b"} {
		if h := exportContentDisposition(title, "md"); strings.ContainsAny(h, "\r\n\x00") || strings.Count(h, `"`) != 2 {
			t.Errorf("header injection through %q: %q", title, h)
		}
	}
}
