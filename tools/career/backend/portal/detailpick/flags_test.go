package detailpick

import (
	"strings"
	"testing"
)

func longText(n int) string { return strings.Repeat("A real sentence of the posting. ", n/30+1)[:n] }

func sample(id string, v map[string]string) Sample {
	return Sample{JobID: id, SavedTitle: "Backend Developer", Values: v}
}

func good(id string) Sample {
	return sample(id, map[string]string{"title": "Backend Developer " + id, "company": "Acme " + id, "location": "Berlin " + id, "postedAt": "2026-10-28", "description": longText(900) + " " + id})
}

func codes(fl []Flag) map[string]Flag {
	m := map[string]Flag{}
	for _, f := range fl {
		m[f.Code+"|"+string(f.Field)] = f
	}
	return m
}

func TestNoFlagsForGoodSamples(t *testing.T) {
	fl := Flags([]Sample{good("1"), good("2"), good("3")}, Fields)
	if len(fl) != 0 {
		t.Fatalf("flags: %+v", fl)
	}
}

func TestEmptyDescriptionBlocksOnlyWhenEmptyEverywhere(t *testing.T) {
	empty := func(id string) Sample { s := good(id); s.Values["description"] = "  "; return s }
	fl := Flags([]Sample{empty("1"), empty("2")}, nil)
	if len(fl) == 0 || fl[0].Code != "description_empty" || fl[0].Severity != Blocking || len(fl[0].JobIDs) != 2 {
		t.Fatalf("flags: %+v", fl)
	}
	fl = Flags([]Sample{empty("1"), good("2")}, nil)
	if fl[0].Code != "description_empty" || fl[0].Severity != Warning || len(fl[0].JobIDs) != 1 || fl[0].JobIDs[0] != "1" {
		t.Fatalf("flags: %+v", fl)
	}
	for _, f := range Flags([]Sample{empty("1"), good("2")}, nil) {
		if f.Severity == Blocking {
			t.Errorf("only an everywhere-empty description blocks: %+v", f)
		}
	}
	if len(Flags(nil, nil)) != 0 {
		t.Error("no samples, no flags")
	}
}

func TestFieldEmptyOnlyForChosenFields(t *testing.T) {
	s := good("1")
	s.Values["company"] = ""
	s.Values["location"] = ""
	fl := codes(Flags([]Sample{s}, []Field{FieldCompany}))
	if _, ok := fl["field_empty|company"]; !ok {
		t.Errorf("company was chosen and is empty: %v", fl)
	}
	if _, ok := fl["field_empty|location"]; ok {
		t.Errorf("location was not chosen: %v", fl)
	}
}

func TestValueShapeFlags(t *testing.T) {
	s := good("1")
	s.Values["company"] = strings.Repeat("c", MaxCompanyRunes+1)
	s.Values["location"] = strings.Repeat("l", MaxLocationRunes+1)
	s.Values["postedAt"] = "yesterday-ish"
	fl := codes(Flags([]Sample{s}, Fields))
	for _, k := range []string{"too_long|company", "too_long|location"} {
		if f, ok := fl[k]; !ok || f.Severity != Warning {
			t.Errorf("missing %s in %v", k, fl)
		}
	}
	// plan 199: a value that is not a date is only information (the save leaves it empty), never a gate
	if f, ok := fl["date_not_a_date|postedAt"]; !ok || f.Severity != Info {
		t.Errorf("date_not_a_date must be info: %v", fl)
	}
	s.Values["postedAt"] = "2026-10-28T09:00:00+00:00" // a machine date is fine
	if _, ok := codes(Flags([]Sample{s}, Fields))["date_not_a_date|postedAt"]; ok {
		t.Error("an ISO timestamp is a date")
	}
}

func TestTitleDiffersIsOnlyInformation(t *testing.T) {
	s := good("1")
	s.Values["title"] = "Imprint and privacy"
	fl := Flags([]Sample{s}, Fields)
	f, ok := codes(fl)["title_differs|title"]
	if !ok || f.Severity != Info {
		t.Fatalf("flags: %+v", fl)
	}
	s.SavedTitle = ""
	if _, ok := codes(Flags([]Sample{s}, Fields))["title_differs|title"]; ok {
		t.Error("nothing to compare with")
	}
}

func TestATitleOverSeveralLinesIsFlagged(t *testing.T) {
	s := good("1")
	s.Values["title"] = "Founding Principal Engineer New\nFerto Health GmbH Berlin"
	fl := codes(Flags([]Sample{s, good("2")}, Fields))
	f, ok := fl["title_several_lines|title"]
	if !ok || f.Severity != Warning || len(f.JobIDs) != 1 || f.JobIDs[0] != "1" {
		t.Fatalf("flags: %+v", fl)
	}
	if _, ok := codes(Flags([]Sample{s}, []Field{FieldCompany}))["title_several_lines|title"]; ok {
		t.Error("the title was not chosen, so it is not checked")
	}
	if _, ok := codes(Flags([]Sample{good("3")}, Fields))["title_several_lines|title"]; ok {
		t.Error("a one-line title is fine")
	}
	t3 := good("4")
	t3.Values["title"] = "Backend Developer\n"
	if _, ok := codes(Flags([]Sample{t3}, Fields))["title_several_lines|title"]; ok {
		t.Error("a trailing line break is only whitespace")
	}
}

func TestIdenticalOnEverySample(t *testing.T) {
	a, b := good("1"), good("2")
	for _, k := range []string{"title", "description", "company", "location"} {
		a.Values[k], b.Values[k] = a.Values["title"]+" same", a.Values["title"]+" same"
	}
	a.Values["description"] = longText(500)
	b.Values["description"] = a.Values["description"]
	fl := codes(Flags([]Sample{a, b}, Fields))
	if f := fl["identical_on_all|title"]; f.Severity != Warning || len(f.JobIDs) != 2 {
		t.Errorf("title: %+v", f)
	}
	if f := fl["identical_on_all|description"]; f.Severity != Warning {
		t.Errorf("description: %+v", f)
	}
	if f := fl["identical_on_all|company"]; f.Severity != Info {
		t.Errorf("company: %+v", f)
	}
	if _, ok := codes(Flags([]Sample{a}, Fields))["identical_on_all|title"]; ok {
		t.Error("one sample cannot be identical to another")
	}
}

func TestDescriptionFlags(t *testing.T) {
	short := good("1")
	short.Values["description"] = "Too short."
	furn := good("2")
	furn.Values["description"] = longText(600) + "\nSimilar jobs\nOther job A"
	cookie := good("3")
	cookie.Values["description"] = longText(600) + "\nWe use cookies to improve your experience"
	rep := good("4")
	rep.Values["description"] = longText(300) + strings.Repeat("\nJoin our team today!", 3)
	cut := good("5")
	cut.Description = &DescriptionReport{Truncated: true, Chars: 3000, FullLength: 8000, Limit: 3000, Parts: []PartReport{{Overlap: true}, {NotFound: true}}, Errors: []string{"bad"}}
	fl := codes(Flags([]Sample{short, furn, cookie, rep, cut}, Fields))
	for _, k := range []string{
		"description_short|description", "description_furniture_words|description", "description_repeated_line|description",
		"description_cut_at_limit|description", "description_parts_overlap|description", "description_part_not_found|description", "description_selector_errors|description",
	} {
		if _, ok := fl[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if f := fl["description_furniture_words|description"]; len(f.JobIDs) != 2 {
		t.Errorf("both the similar-jobs line and the cookie line: %+v", f)
	}
	if f := fl["description_cut_at_limit|description"]; !strings.Contains(f.Message, "3000 of 8000") {
		t.Errorf("message: %q", f.Message)
	}
}

func TestAMentionInTheMiddleOfALineIsNotFurniture(t *testing.T) {
	s := good("1")
	s.Values["description"] = longText(600) + "\nWe believe in similar jobs for similar people and apply now is not our motto."
	if _, ok := codes(Flags([]Sample{s}, Fields))["description_furniture_words|description"]; ok {
		t.Error("only a line that IS furniture counts")
	}
}

func TestFlagsKeepAStableOrder(t *testing.T) {
	a, b := good("1"), good("2")
	a.Values["company"], b.Values["company"] = "", ""
	x := Flags([]Sample{a, b}, Fields)
	y := Flags([]Sample{a, b}, Fields)
	if len(x) != len(y) {
		t.Fatal("unstable")
	}
	for i := range x {
		if x[i].Code != y[i].Code || x[i].Field != y[i].Field {
			t.Fatal("unstable order")
		}
	}
}

func FuzzFlags(f *testing.F) {
	f.Add("title", "company", "location", "2026-10-28", "description text")
	f.Add("", "\x00", strings.Repeat("x", 500), "date", "Similar jobs\nSimilar jobs\nSimilar jobs")
	f.Fuzz(func(t *testing.T, title, company, location, date, desc string) {
		s := Sample{JobID: "j", SavedTitle: title, Values: map[string]string{"title": title, "company": company, "location": location, "postedAt": date, "description": desc}}
		fl := Flags([]Sample{s, s}, Fields)
		for _, f := range fl {
			if f.Code == "" || f.Message == "" || len(f.JobIDs) == 0 {
				t.Fatalf("incomplete flag %+v", f)
			}
		}
	})
}

// Found on python.org's job pages (plan 188's on-screen flow): the AI-written selector gave "Backend Developer New UNICEF" as the company. The
// save ignores such a value, so the check must say so before the person saves.
func TestACompanyOrPlaceThatHoldsTheJobsOwnTitleIsFlagged(t *testing.T) {
	s := good("1")
	s.Values["company"] = "backend developer NEW Acme"
	s.Values["location"] = "Remote — Backend Developer"
	got := codes(Flags([]Sample{s, good("2")}, Fields))
	for _, f := range []Field{FieldCompany, FieldLocation} {
		fl, ok := got["has_the_title|"+string(f)]
		if !ok || fl.Severity != Warning || len(fl.JobIDs) != 1 || fl.JobIDs[0] != "1" {
			t.Errorf("%s: %+v", f, got)
		}
	}
	// a short title cannot be judged; a company that merely shares words is fine; the title field itself is never flagged by this
	short := good("3")
	short.SavedTitle = "Dev"
	short.Values["company"] = "Dev Acme"
	other := good("4")
	other.Values["company"] = "Backend Acme Developer"
	for _, s := range []Sample{short, other} {
		if _, ok := codes(Flags([]Sample{s}, Fields))["has_the_title|company"]; ok {
			t.Errorf("must not flag %q for the title %q", s.Values["company"], s.SavedTitle)
		}
	}
	if _, ok := codes(Flags([]Sample{good("5")}, Fields))["has_the_title|title"]; ok {
		t.Error("the title field is never flagged for holding the title")
	}
}

// plan 199: only the description is required. A chosen title, company, location or date that is empty on some or on every sample is
// information, never a warning or a block.
func TestAnOptionalFieldThatIsEmptyIsOnlyInformation(t *testing.T) {
	s := good("1")
	for _, f := range []string{"title", "company", "location", "postedAt"} {
		s.Values[f] = ""
	}
	fl := Flags([]Sample{s}, Fields)
	got := map[string]Severity{}
	for _, f := range fl {
		got[f.Code+"|"+string(f.Field)] = f.Severity
	}
	for _, f := range []Field{FieldTitle, FieldCompany, FieldLocation, FieldPostedAt} {
		if sev, ok := got["field_empty|"+string(f)]; !ok || sev != Info {
			t.Errorf("%s empty: want info, got %v (%v)", f, sev, got)
		}
	}
	for _, f := range fl {
		if f.Severity == Blocking || f.Severity == Warning {
			t.Errorf("an empty optional field must not warn or block: %+v", f)
		}
	}
}
