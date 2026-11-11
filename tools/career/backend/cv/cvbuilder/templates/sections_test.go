package templates

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// The four sections every design shows (plan 251, step 4): education and courses after the experience, languages and hobbies after the skills. Every design of
// ListTemplates() is checked with a CV that has all of them, with one that has none, and with each section alone, so a design added later has to show them too.

func everything() CvData {
	return CvData{
		FullName: "Test Person", Headline: "HEADLINE-1", Summary: "SUMMARY-1", Location: "LOCATION-1",
		Skills:     []string{"SKILL-1", "SKILL-2"},
		Experience: []ExperienceEntry{{Company: "EXP-COMPANY-1", Title: "EXP-TITLE-1", StartDate: "2015", EndDate: "2019", Description: "EXP-DESC-1"}},
		Education: []EducationEntry{
			{Institution: "EDU-INST-1", Degree: "EDU-DEG-1", FieldOfStudy: "EDU-FIELD-1", StartDate: "2001", EndDate: "2003", Description: "EDU-DESC-1"},
			{Institution: "EDU-INST-2", Degree: "EDU-DEG-2", StartDate: "2010"},
		},
		Courses:   []CourseEntry{{Title: "CRS-TITLE-1", Provider: "CRS-PROV-1", CompletedDate: "2005-05"}, {Title: "CRS-TITLE-2"}},
		Languages: []LanguageEntry{{Language: "LANG-1", Level: "LVL-1"}, {Language: "LANG-2"}},
		Hobbies:   []HobbyEntry{{Name: "Rock & Roll"}, {Name: "HOB-2"}},
	}
}

func render(t *testing.T, id string, data CvData) string {
	t.Helper()
	x, err := Render(id, data, "")
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return x
}

func headings(x string) map[string]int {
	return map[string]int{"Education": strings.Count(x, ">Education<"), "Courses": strings.Count(x, ">Courses<"), "Languages": strings.Count(x, ">Languages<"), "Hobbies": strings.Count(x, ">Hobbies<")}
}

// wellFormed is what pdf_tools asks of the XHTML before it prints it: the cloned blocks must be balanced and closed.
func wellFormed(x string) error {
	d := xml.NewDecoder(strings.NewReader(x))
	d.Strict = true
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestEveryDesignShowsEverySectionInItsPlace(t *testing.T) {
	designs := ListTemplates()
	if len(designs) < 100 {
		t.Fatalf("expected the whole set of designs, got %d", len(designs))
	}
	for _, m := range designs {
		t.Run(m.ID, func(t *testing.T) {
			x := render(t, m.ID, everything())
			if err := wellFormed(x); err != nil {
				t.Fatalf("the page is not well-formed XHTML: %v", err)
			}
			for _, want := range []string{
				"EDU-INST-1", "EDU-DEG-1, EDU-FIELD-1", "2001 – 2003", "EDU-DESC-1", "EDU-INST-2", "EDU-DEG-2", "2010 – Present",
				"CRS-TITLE-1", "CRS-PROV-1", "2005-05", "CRS-TITLE-2",
				"LANG-1 (LVL-1)", "LANG-2",
				"Rock &amp; Roll", "HOB-2",
				"EXP-TITLE-1", "SKILL-1", "HEADLINE-1", "SUMMARY-1",
			} {
				if !strings.Contains(x, want) {
					t.Errorf("the page does not show %q", want)
				}
			}
			if strings.Contains(x, "Rock & Roll") {
				t.Error("a hobby with an & must be escaped")
			}
			h := headings(x)
			for name, n := range h {
				if n != 1 {
					t.Errorf("the %s heading appears %d times, want once", name, n)
				}
			}
			at := func(s string) int { return strings.Index(x, s) }
			if !(at("EXP-TITLE-1") < at(">Education<") && at(">Education<") < at(">Courses<")) {
				t.Errorf("education and courses come after the experience, education first (experience %d, education %d, courses %d)", at("EXP-TITLE-1"), at(">Education<"), at(">Courses<"))
			}
			if !(at("SKILL-1") < at(">Languages<") && at(">Languages<") < at(">Hobbies<")) {
				t.Errorf("languages and hobbies come after the skills, languages first (skills %d, languages %d, hobbies %d)", at("SKILL-1"), at(">Languages<"), at(">Hobbies<"))
			}
			// the education entry that has an institution only is shown by it, once
			only := render(t, m.ID, CvData{FullName: "A", Education: []EducationEntry{{Institution: "ONLY-INST"}}})
			if strings.Count(only, "ONLY-INST") != 1 {
				t.Errorf("an education entry with only an institution shows it %d times, want once", strings.Count(only, "ONLY-INST"))
			}
		})
	}
}

func TestNoSectionIsShownWhenItHasNoEntries(t *testing.T) {
	plain := CvData{FullName: "Test Person", Skills: []string{"SKILL-1"}, Experience: []ExperienceEntry{{Company: "C", Title: "EXP-TITLE-1", StartDate: "2020"}}}
	empty := plain
	empty.Education, empty.Courses, empty.Languages, empty.Hobbies = []EducationEntry{}, []CourseEntry{}, []LanguageEntry{}, []HobbyEntry{}
	for _, m := range ListTemplates() {
		a := render(t, m.ID, plain)
		for name, n := range headings(a) {
			if n != 0 {
				t.Errorf("%s: a CV without entries shows the %s heading", m.ID, name)
			}
		}
		if b := render(t, m.ID, empty); a != b {
			t.Errorf("%s: empty lists must render exactly like no lists", m.ID)
		}
	}
}

func TestEachSectionAppearsOnlyWhenItHasEntries(t *testing.T) {
	for _, m := range ListTemplates() {
		for _, c := range []struct {
			section string
			set     func(d *CvData)
		}{
			{"Education", func(d *CvData) { d.Education = everything().Education }},
			{"Courses", func(d *CvData) { d.Courses = everything().Courses }},
			{"Languages", func(d *CvData) { d.Languages = everything().Languages }},
			{"Hobbies", func(d *CvData) { d.Hobbies = everything().Hobbies }},
		} {
			d := CvData{FullName: "A", Skills: []string{"SKILL-1"}, Experience: []ExperienceEntry{{Title: "T", Company: "C"}}}
			c.set(&d)
			for name, n := range headings(render(t, m.ID, d)) {
				if want := map[bool]int{true: 1, false: 0}[name == c.section]; n != want {
					t.Errorf("%s: with only %s, the %s heading appears %d times, want %d", m.ID, c.section, name, n, want)
				}
			}
		}
	}
}

// The designs have three ways of building an entry (see entryShape); education and courses are built the same way as the experience of their design.
func TestTheDesignsHaveTheThreeKnownEntryShapes(t *testing.T) {
	counts := map[string]int{}
	for _, m := range ListTemplates() {
		counts[entryShape(t, m.ID)]++
	}
	if counts["head"] != 50 || counts["body"] != 34 || counts["block"] != 26 {
		t.Errorf("entry shapes = %v, want head 50, body 34, block 26", counts)
	}
}

func TestEducationIsShownAsItsDegreeAndFieldUnderTheInstitution(t *testing.T) {
	cases := []struct {
		name                  string
		e                     EducationEntry
		title, company, dates string
	}{
		{"degree and field", EducationEntry{Institution: " TU ", Degree: " MSc ", FieldOfStudy: " CS ", StartDate: "2010", EndDate: "2012"}, "MSc, CS", "TU", "2010 – 2012"},
		{"degree only", EducationEntry{Institution: "TU", Degree: "MSc"}, "MSc", "TU", ""},
		{"field only", EducationEntry{Institution: "TU", FieldOfStudy: "CS"}, "CS", "TU", ""},
		{"institution only", EducationEntry{Institution: "TU"}, "TU", "", ""},
		{"still studying", EducationEntry{Institution: "TU", Degree: "BSc", StartDate: "2020"}, "BSc", "TU", "2020 – Present"},
		{"only an end date", EducationEntry{Institution: "TU", Degree: "BSc", EndDate: "2010"}, "BSc", "TU", "2010"},
		{"spaces are not dates", EducationEntry{Institution: "TU", StartDate: "  ", EndDate: "  "}, "TU", "", ""},
		{"nothing", EducationEntry{}, "", "", ""},
	}
	for _, c := range cases {
		if got := c.e.Title(); got != c.title {
			t.Errorf("%s: Title = %q, want %q", c.name, got, c.title)
		}
		if got := c.e.Company(); got != c.company {
			t.Errorf("%s: Company = %q, want %q", c.name, got, c.company)
		}
		if got := c.e.DateRange(); got != c.dates {
			t.Errorf("%s: DateRange = %q, want %q", c.name, got, c.dates)
		}
	}
}

func TestCoursesLanguagesAndHobbiesAreShownAsOneLineEach(t *testing.T) {
	c := CourseEntry{Title: "K8s", Provider: " CNCF ", CompletedDate: " 2023-05 "}
	if c.Company() != "CNCF" || c.DateRange() != "2023-05" || c.Description() != "" {
		t.Errorf("a course: %q %q %q", c.Company(), c.DateRange(), c.Description())
	}
	for _, c := range []struct{ lang, level, want string }{
		{"German", "Native", "German (Native)"}, {"English", "", "English"}, {" French ", " C1 ", "French (C1)"}, {"", "C1", ""}, {"", "", ""}, {"Latin", "   ", "Latin"},
	} {
		if got := (LanguageEntry{Language: c.lang, Level: c.level}).Display(); got != c.want {
			t.Errorf("Display(%q, %q) = %q, want %q", c.lang, c.level, got, c.want)
		}
	}
	if (LanguageEntry{Language: "German", Level: "Native"}).Display() != "German (Native)" || (LanguageEntry{Language: " German "}).Display() != "German" {
		t.Error("a language is its name and, when it has one, its level in brackets")
	}
	if (HobbyEntry{Name: "  Chess "}).Display() != "Chess" {
		t.Error("a hobby is its trimmed name")
	}
}
