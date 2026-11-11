package cvbuilder

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/courses"
	"career-tool-backend/cv/cvbuilder/templates"
	"career-tool-backend/education"
	"career-tool-backend/hobbies"
	"career-tool-backend/languages"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// The four sections of a CV that come from the profile (plan 251, step 2): education, courses, languages and hobbies.

func fullSections() templates.CvData {
	return templates.CvData{
		FullName:  "Ada Lovelace",
		Education: []templates.EducationEntry{{Institution: "TU Berlin", Degree: "MSc", FieldOfStudy: "Computer Science", StartDate: "2010", EndDate: "2012", Description: "Thesis on compilers"}},
		Courses:   []templates.CourseEntry{{Title: "Kubernetes", Provider: "CNCF", CompletedDate: "2023-05"}},
		Languages: []templates.LanguageEntry{{Language: "German", Level: "Native"}, {Language: "English", Level: "C1"}},
		Hobbies:   []templates.HobbyEntry{{Name: "Chess"}, {Name: "Climbing"}},
	}
}

// --- the data shape ---

func TestACvWithoutTheNewSectionsHasNoSectionKeysInItsJson(t *testing.T) {
	raw, err := json.Marshal(templates.CvData{FullName: "Ada", Skills: []string{"Go"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"education"`, `"courses"`, `"languages"`, `"hobbies"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("%s must stay out of the JSON of a CV that has none: %s", key, raw)
		}
	}
}

func TestACvStoredBeforeTheSectionsExistedStillReadsAndWritesTheSame(t *testing.T) {
	old := `{"fullName":"Ada","headline":"","summary":"","location":"","externalLinks":[],"skills":["Go"],"experience":[]}`
	var data templates.CvData
	if err := json.Unmarshal([]byte(old), &data); err != nil {
		t.Fatal(err)
	}
	if data.Education != nil || data.Courses != nil || data.Languages != nil || data.Hobbies != nil {
		t.Errorf("an old CV has no sections: %+v", data)
	}
	back, _ := json.Marshal(data)
	if string(back) != strings.ReplaceAll(old, `"externalLinks":[]`, `"externalLinks":null`) && string(back) != old {
		t.Errorf("an old CV must write back as it was read: %s", back)
	}
}

func TestTheSectionsRoundTripThroughJsonWithTheirFieldNames(t *testing.T) {
	raw, err := json.Marshal(fullSections())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"education":[{"institution":"TU Berlin","degree":"MSc","fieldOfStudy":"Computer Science","startDate":"2010","endDate":"2012","description":"Thesis on compilers"}]`,
		`"courses":[{"title":"Kubernetes","provider":"CNCF","completedDate":"2023-05"}]`, `"languages":[{"language":"German","level":"Native"},{"language":"English","level":"C1"}]`,
		`"hobbies":[{"name":"Chess"},{"name":"Climbing"}]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	var back templates.CvData
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Errorf("round trip changed the data:\n%s\n%s", raw, again)
	}
}

func TestTheAiDataArgsCarryTheSectionsIntoTheCvData(t *testing.T) {
	var args CvDocumentDataArgs
	in := `{"fullName":"Ada","education":[{"institution":"MIT","degree":"BSc"}],"courses":[{"title":"Go","provider":"Gophers","completedDate":"2024"}],` +
		`"languages":[{"language":"French","level":"B2"}],"hobbies":[{"name":"Sailing"}]}`
	if err := json.Unmarshal([]byte(in), &args); err != nil {
		t.Fatal(err)
	}
	data := args.ToCvData()
	if len(data.Education) != 1 || data.Education[0].Institution != "MIT" || data.Education[0].Degree != "BSc" ||
		len(data.Courses) != 1 || data.Courses[0].Provider != "Gophers" ||
		len(data.Languages) != 1 || data.Languages[0].Language != "French" || data.Languages[0].Level != "B2" ||
		len(data.Hobbies) != 1 || data.Hobbies[0].Name != "Sailing" {
		t.Errorf("a section was lost on the way: %+v", data)
	}
}

// --- validation ---

func TestEverySectionHasItsEntryLimit(t *testing.T) {
	mk := func(n int, f func(i int, d *templates.CvData)) templates.CvData {
		d := templates.CvData{FullName: "Ada"}
		for i := 0; i < n; i++ {
			f(i, &d)
		}
		return d
	}
	cases := []struct {
		name, word string
		limit      int
		fill       func(i int, d *templates.CvData)
	}{
		{"education", "education", maxEducationEntries, func(i int, d *templates.CvData) {
			d.Education = append(d.Education, templates.EducationEntry{Institution: "X"})
		}},
		{"courses", "course", maxCourseEntries, func(i int, d *templates.CvData) { d.Courses = append(d.Courses, templates.CourseEntry{Title: "X"}) }},
		{"languages", "language", maxLanguageEntries, func(i int, d *templates.CvData) {
			d.Languages = append(d.Languages, templates.LanguageEntry{Language: "X"})
		}},
		{"hobbies", "hobby", maxHobbyEntries, func(i int, d *templates.CvData) { d.Hobbies = append(d.Hobbies, templates.HobbyEntry{Name: "X"}) }},
	}
	for _, c := range cases {
		if err := ValidateCvData(mk(c.limit, c.fill)); err != nil {
			t.Errorf("%s: exactly %d entries must be accepted: %v", c.name, c.limit, err)
		}
		err := ValidateCvData(mk(c.limit+1, c.fill))
		if err == nil || !strings.Contains(err.Error(), "too many "+c.word+" entries") {
			t.Errorf("%s: %d entries must be refused with a clear message, got %v", c.name, c.limit+1, err)
		}
	}
	if maxEducationEntries != 20 || maxCourseEntries != 30 || maxLanguageEntries != 20 || maxHobbyEntries != 30 {
		t.Errorf("the limits are 20 / 30 / 20 / 30")
	}
}

func TestEveryTextFieldOfEverySectionMustBePlainAndNotTooLong(t *testing.T) {
	type field struct {
		section, name string
		set           func(d *templates.CvData, v string)
	}
	fields := []field{
		{"education", "institution", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{Institution: v}} }},
		{"education", "degree", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{Degree: v}} }},
		{"education", "fieldOfStudy", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{FieldOfStudy: v}} }},
		{"education", "startDate", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{StartDate: v}} }},
		{"education", "endDate", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{EndDate: v}} }},
		{"education", "description", func(d *templates.CvData, v string) { d.Education = []templates.EducationEntry{{Description: v}} }},
		{"course", "title", func(d *templates.CvData, v string) { d.Courses = []templates.CourseEntry{{Title: v}} }},
		{"course", "provider", func(d *templates.CvData, v string) { d.Courses = []templates.CourseEntry{{Provider: v}} }},
		{"course", "completedDate", func(d *templates.CvData, v string) { d.Courses = []templates.CourseEntry{{CompletedDate: v}} }},
		{"language", "language", func(d *templates.CvData, v string) { d.Languages = []templates.LanguageEntry{{Language: v}} }},
		{"language", "level", func(d *templates.CvData, v string) { d.Languages = []templates.LanguageEntry{{Level: v}} }},
		{"hobby", "name", func(d *templates.CvData, v string) { d.Hobbies = []templates.HobbyEntry{{Name: v}} }},
	}
	for _, f := range fields {
		with := func(v string) error {
			d := templates.CvData{FullName: "Ada"}
			f.set(&d, v)
			return ValidateCvData(d)
		}
		if err := with("Plain text, 5 < 6 and tea & coffee"); err != nil {
			t.Errorf("%s.%s: plain text with a < and an & must pass: %v", f.section, f.name, err)
		}
		if err := with(strings.Repeat("a", maxFieldLength)); err != nil {
			t.Errorf("%s.%s: exactly %d characters must pass: %v", f.section, f.name, maxFieldLength, err)
		}
		if err := with(strings.Repeat("a", maxFieldLength+1)); err == nil || !strings.Contains(err.Error(), "too long") || !strings.Contains(err.Error(), f.section) {
			t.Errorf("%s.%s: one character too many must be refused as too long: %v", f.section, f.name, err)
		}
		if err := with("some <strong>bold</strong> text"); err == nil || !strings.Contains(err.Error(), "plain text") || !strings.Contains(err.Error(), f.name) {
			t.Errorf("%s.%s: a markup tag must be refused as not plain text, naming the field: %v", f.section, f.name, err)
		}
	}
}

func TestAnEmptyCvAndOneWithOnlyOldFieldsStayValid(t *testing.T) {
	if err := ValidateCvData(templates.CvData{}); err != nil {
		t.Errorf("an empty CV: %v", err)
	}
	if err := ValidateCvData(fullSections()); err != nil {
		t.Errorf("a full CV: %v", err)
	}
}

// --- assembling the defaults from the profile ---

func seedSections(t *testing.T, profileID string) {
	t.Helper()
	for _, in := range []education.Input{
		{Institution: "Old School", Degree: "BSc", FieldOfStudy: "Maths", StartDate: "2005", EndDate: "2008", Description: "First degree"},
		{Institution: "Open University", Degree: "MSc", FieldOfStudy: "Software", StartDate: "2020"}, // still studying: listed first
	} {
		if _, err := education.Add(profileID, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []courses.Input{
		{Title: "Older course", Provider: "A", CompletedDate: "2019-03", CredentialURL: "https://example.com/c", Description: "not carried over"},
		{Title: "Newer course", Provider: "B", CompletedDate: "2024-06"},
		{Title: "Undated course", Provider: "C"},
	} {
		if _, err := courses.Add(profileID, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []languages.Input{{Language: "English", Level: "b2", CanSpeak: true}, {Language: "German", Level: "native", CanSpeak: true}, {Language: "French", Level: "c1", CanWrite: true}, {Language: "Latin", CanWrite: true}} {
		if _, err := languages.Add(profileID, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []hobbies.Input{{Name: "Climbing", Description: "not carried over"}, {Name: "Chess"}} {
		if _, err := hobbies.Add(profileID, in); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheDefaultsOfAPersonaCarryTheProfilesFourSectionsInTheirOrder(t *testing.T) {
	pid := setupCvDB(t)
	seedSections(t, profileOf(t, pid))
	data, err := AssembleCvData(pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Education) != 2 || data.Education[0].Institution != "Open University" || data.Education[0].EndDate != "" || data.Education[1].Institution != "Old School" {
		t.Fatalf("education: still studying first, then the latest end: %+v", data.Education)
	}
	if e := data.Education[1]; e.Degree != "BSc" || e.FieldOfStudy != "Maths" || e.StartDate != "2005" || e.EndDate != "2008" || e.Description != "First degree" {
		t.Errorf("an education entry loses a field: %+v", e)
	}
	if len(data.Courses) != 3 || data.Courses[0].Title != "Newer course" || data.Courses[1].Title != "Older course" || data.Courses[2].Title != "Undated course" {
		t.Fatalf("courses: the latest first, the undated last: %+v", data.Courses)
	}
	if c := data.Courses[1]; c.Provider != "A" || c.CompletedDate != "2019-03" {
		t.Errorf("a course loses a field: %+v", c)
	}
	var got []string
	for _, l := range data.Languages {
		got = append(got, l.Language+"|"+l.Level)
	}
	if strings.Join(got, ",") != "German|Native,French|C1,English|B2,Latin|" {
		t.Errorf("languages: native first, then C2 down to A1, a language without a level last, levels as shown: %v", got)
	}
	if len(data.Hobbies) != 2 || data.Hobbies[0].Name != "Chess" || data.Hobbies[1].Name != "Climbing" {
		t.Errorf("hobbies by name: %+v", data.Hobbies)
	}
	// what a CV line does not show is not carried over: nothing of the profile's descriptions and addresses for courses and hobbies
	raw, _ := json.Marshal(data)
	for _, leaked := range []string{"not carried over", "example.com", "credential"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("%q leaked into the CV data: %s", leaked, raw)
		}
	}
}

func TestAProfileWithoutTheSectionsGivesAnEmptyCvWithoutThem(t *testing.T) {
	pid := setupCvDB(t)
	data, err := AssembleCvData(pid)
	if err != nil {
		t.Fatal(err)
	}
	if data.Education != nil || data.Courses != nil || data.Languages != nil || data.Hobbies != nil {
		t.Errorf("nothing to carry over: %+v", data)
	}
}

func TestTheSectionsAreTheProfilesNotThePersonasAndNeverAnotherProfiles(t *testing.T) {
	pid := setupCvDB(t)
	profileID := profileOf(t, pid)
	seedSections(t, profileID)
	other, err := persona.CreatePersona(profileID, "Second role", "")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := AssembleCvData(pid)
	b, err := AssembleCvData(other)
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal([]any{a.Education, a.Courses, a.Languages, a.Hobbies})
	jb, _ := json.Marshal([]any{b.Education, b.Courses, b.Languages, b.Hobbies})
	if string(ja) != string(jb) {
		t.Errorf("two personas of the same profile share its sections:\n%s\n%s", ja, jb)
	}
	// a second profile with its own entries does not leak into the first, and the first does not leak into the second
	p2, err := profile.CreateProfile("Grace", "Hopper")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hobbies.Add(p2, hobbies.Input{Name: "Sailing"}); err != nil {
		t.Fatal(err)
	}
	p2persona, err := persona.CreatePersona(p2, "Admiral", "")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := AssembleCvData(p2persona)
	if len(c.Hobbies) != 1 || c.Hobbies[0].Name != "Sailing" || c.Education != nil || c.Courses != nil || c.Languages != nil {
		t.Errorf("the second profile has only its own hobby: %+v", c)
	}
	again, _ := AssembleCvData(pid)
	for _, h := range again.Hobbies {
		if h.Name == "Sailing" {
			t.Errorf("another profile's hobby leaked: %+v", again.Hobbies)
		}
	}
}

func TestLanguageLevelsAreShownAsOnACv(t *testing.T) {
	for in, want := range map[string]string{"native": "Native", "c2": "C2", "c1": "C1", "b2": "B2", "b1": "B1", "a2": "A2", "a1": "A1", "": ""} {
		if got := languageLevelLabel(in); got != want {
			t.Errorf("languageLevelLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThePersonaDefaultsRouteAnswersWithTheSections(t *testing.T) {
	pid := setupCvDB(t)
	seedSections(t, profileOf(t, pid))
	rec := httptest.NewRecorder()
	PersonaDefaultsHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-documents/persona-defaults?personaId="+pid, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data templates.CvData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Education) != 2 || len(data.Courses) != 3 || len(data.Languages) != 4 || len(data.Hobbies) != 2 {
		t.Errorf("the route must carry all four sections: %s", rec.Body.String())
	}
}

// --- saving and reading a CV with the sections ---

func TestACvWithTheSectionsIsStoredAndReadBackWhole(t *testing.T) {
	pid := setupCvDB(t)
	doc, err := InsertCvDocument(pid, "classic", "With sections", fullSections(), "media-1", "")
	if err != nil {
		t.Fatal(err)
	}
	back, err := GetCvDocument(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(fullSections())
	b, _ := json.Marshal(back.Data)
	if string(a) != string(b) {
		t.Errorf("a stored CV must read back with all four sections:\n%s\n%s", a, b)
	}
}

func TestAnAiCvWithMarkupInASectionIsRefusedAndNothingIsStored(t *testing.T) {
	pid := setupCvDB(t)
	core := newFakeTitleCore(t, http.StatusOK)
	data := fullSections()
	data.Hobbies = []templates.HobbyEntry{{Name: "<strong>Chess</strong>"}}
	if _, err := SaveCvForPersona(pid, "classic", "T", data, "media-1"); err == nil || !strings.Contains(err.Error(), "plain text") {
		t.Fatalf("markup in a hobby must be refused: %v", err)
	}
	if cvRows(t) != 0 || len(core.paths) != 0 {
		t.Errorf("a refused save stores nothing and never touches Media: rows %d, media calls %v", cvRows(t), core.paths)
	}
	if _, err := SaveCvForPersona(pid, "classic", "T", fullSections(), "media-1"); err != nil {
		t.Fatalf("the same CV without markup must save: %v", err)
	}
	if cvRows(t) != 1 {
		t.Errorf("cv rows = %d", cvRows(t))
	}
}
