package jobmatcher

import (
	"strings"
	"testing"
	"time"
)

func TestParseMonth(t *testing.T) {
	cases := []struct {
		in   string
		end  bool
		want int
		ok   bool
	}{
		{"2021-03", false, monthIndex(2021, 3), true},
		{"2021-03-15", false, monthIndex(2021, 3), true},
		{"03/2021", false, monthIndex(2021, 3), true},
		{"3.2021", false, monthIndex(2021, 3), true},
		{"2021", false, monthIndex(2021, 1), true},
		{"2021", true, monthIndex(2021, 12), true},
		{"since 2019", false, monthIndex(2019, 1), true},
		{"2021-13", false, monthIndex(2021, 1), true}, // an impossible month falls back to the year
		{"spring", false, 0, false},
		{"", false, 0, false},
	}
	for _, c := range cases {
		got, ok := parseMonth(c.in, c.end)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseMonth(%q, end=%v) = %d,%v want %d,%v", c.in, c.end, got, ok, c.want, c.ok)
		}
	}
}

func TestEntrySpanReadsPresentAndRejectsBadDates(t *testing.T) {
	cur, ok := entrySpan(Experience{StartDate: "2025-11", EndDate: ""}, now)
	if !ok || cur.to != monthIndex(2026, 10) {
		t.Errorf("an open end runs to now: %+v %v", cur, ok)
	}
	if sp, ok := entrySpan(Experience{StartDate: "2025-11", EndDate: "heute"}, now); !ok || sp.to != monthIndex(2026, 10) {
		t.Errorf("heute = present: %+v %v", sp, ok)
	}
	if _, ok := entrySpan(Experience{StartDate: "2025-11", EndDate: "whenever"}, now); ok {
		t.Error("an unreadable end is rejected")
	}
	if _, ok := entrySpan(Experience{StartDate: "", EndDate: "2020"}, now); ok {
		t.Error("no start is rejected")
	}
	if _, ok := entrySpan(Experience{StartDate: "2022-05", EndDate: "2021-01"}, now); ok {
		t.Error("an end before the start is rejected")
	}
}

func TestUnionMonthsCountsOverlapOnce(t *testing.T) {
	a := span{monthIndex(2020, 1), monthIndex(2020, 12)}
	b := span{monthIndex(2020, 7), monthIndex(2021, 6)}
	c := span{monthIndex(2022, 1), monthIndex(2022, 3)}
	if got := unionMonths([]span{a, b, c}); got != 18+3 {
		t.Errorf("union = %d, want 21", got)
	}
	if got := unionMonths([]span{a, {monthIndex(2021, 1), monthIndex(2021, 2)}}); got != 14 {
		t.Errorf("touching spans merge: %d, want 14", got)
	}
	if unionMonths(nil) != 0 {
		t.Error("none is 0")
	}
}

func TestRequiredYears(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"Mindestens 5 Jahre Berufserfahrung", 5},
		{"3+ Jahre Erfahrung mit Go", 3},
		{"5 years of experience in Go, 2 years of experience with Docker", 2},
		{"Wir sind seit 30 Jahren am Markt", DefaultRequiredYears}, // no word about experience nearby
		{"Gründung vor 7 Jahren. Wir bieten Weiterbildung.", DefaultRequiredYears},
		{"20 years of experience", DefaultRequiredYears}, // not plausible
		{"", DefaultRequiredYears},
	}
	for _, c := range cases {
		if got := requiredYears(c.text); got != c.want {
			t.Errorf("requiredYears(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestExperienceCountsOnlySimilarRolesAndScalesToTheYearsAsked(t *testing.T) {
	// the persona's Go roles run 92 months; the job asks for 3 years (36 months): full marks
	r := mustScore(t, goJob(), []Persona{goDev()}, corpusOf())
	if got := comp(t, r, "experience"); got.Points != WeightExperience || !got.Evaluable {
		t.Errorf("experience = %+v", got)
	}
	// a job asking for 10 years: 92 / 120 of the points
	job := goJob()
	job.Description = strings.Replace(job.Description, "Mindestens 3 Jahre", "Mindestens 10 Jahre", 1)
	r = mustScore(t, job, []Persona{goDev()}, corpusOf())
	want := float64(WeightExperience) * 92.0 / 120.0
	if got := comp(t, r, "experience").Points; got < want-0.05 || got > want+0.05 {
		t.Errorf("experience points = %v, want about %.2f", got, want)
	}
}

func TestExperienceRelevanceAlsoComesFromCoveredRequirementsOrHitSkills(t *testing.T) {
	// the title of the past job is unrelated, but its text mentions two of the job's skills
	p := Persona{Name: "x", DesiredTitles: "Developer", Skills: []string{"Go", "Docker"}, Experience: []Experience{
		{Title: "Consultant", Description: "Daily work with Go and Docker", StartDate: "2020-01", EndDate: "2023-01"},
	}}
	job := Job{Title: "Platform Developer", Description: "We use Go and Docker."}
	if got := comp(t, mustScore(t, job, []Persona{p}, NewCorpus(nil)), "experience").Points; got != WeightExperience {
		t.Errorf("two hit skills in the entry make it relevant: %v", got)
	}
	// with one skill only it is not
	job.Description = "We use Go."
	if got := comp(t, mustScore(t, job, []Persona{p}, NewCorpus(nil)), "experience").Points; got != 0 {
		t.Errorf("one hit skill is not enough: %v", got)
	}
}

func TestExperienceIsNotEvaluableWithoutUsableEntries(t *testing.T) {
	for name, exp := range map[string][]Experience{
		"none":      nil,
		"no dates":  {{Title: "Backend Developer"}},
		"bad dates": {{Title: "Backend Developer", StartDate: "later", EndDate: "never"}},
	} {
		p := goDev()
		p.Experience = exp
		c := comp(t, mustScore(t, goJob(), []Persona{p}, corpusOf()), "experience")
		if c.Evaluable || c.Points != 0 || c.Note == "" {
			t.Errorf("%s: %+v", name, c)
		}
	}
	_ = time.Now
}

func TestAPastTitleCountsAsSimilarWorkOnlyWhenItReallyResembles(t *testing.T) {
	entry := func(title string) Persona {
		return Persona{Name: "x", DesiredTitles: "Whatever", Experience: []Experience{{Title: title, StartDate: "2020-01", EndDate: "2023-01"}}}
	}
	cases := []struct {
		job, past string
		relevant  bool
	}{
		{"Senior Software Engineer", "Software Engineer", true},
		{"Backend Engineer", "Backend Developer", true}, // half the words
		{"Software Engineer", "Engineer", true},
		{"Project Manager", "Manager", true},
		{"Assistenz der Leitung Jugendhilfe (m/w/d)", "Computer Lab Assistant", false}, // one generic stem in common
		{"Erzieher / Heilerzieher (m/w/d)", "Computer Lab Assistant", false},
		{"Platform Developer", "Consultant", false},
		{"Erzieher / Heilerzieher (m/w/d)", "Erzieherin", true},
	}
	for _, c := range cases {
		p := entry(c.past)
		r := mustScore(t, Job{Title: c.job, Description: "Aufgaben im Team."}, []Persona{p}, NewCorpus(nil))
		got := comp(t, r, "experience").Points > 0
		if got != c.relevant {
			t.Errorf("%q as past work for %q: relevant = %v, want %v", c.past, c.job, got, c.relevant)
		}
	}
}
