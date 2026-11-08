package jobmatcher

import "testing"

func TestTitleMarkersAreRemoved(t *testing.T) {
	cases := map[string][]string{
		"Staatlich anerkannter Erzieher (m/w/d)":     {"staatlich", "anerkannter", "erzieher"},
		"Erzieher/in":                                {"erzieher"},
		"Heilerzieher:in":                            {"heilerzieher"},
		"Sozialpädagog*innen":                        {"sozialpaedagog"},
		"Software Engineer (w/m/d) Vollzeit":         {"software", "engineer"},
		"Java Developer m/w/x unbefristet":           {"java", "developer"},
		"Pflegekraft (m/w/d) in Voll- oder Teilzeit": {"pflegekraft"},
		"Developer (all genders)":                    {"developer"},
		"Entwickler/-innen":                          {"entwickler"},
	}
	for in, want := range cases {
		eqStrings(t, in, titleWords(in), want...)
	}
}

func TestTitleAlternativesSplitTheRolesANameCovers(t *testing.T) {
	alts := titleAlternatives("Heilerzieher, Sozialpädagogen / Erzieher (d/m/w) in Voll- oder Teilzeit.")
	if len(alts) != 3 {
		t.Fatalf("alternatives = %v", alts)
	}
	alts = titleAlternatives("Erzieher oder Heilerzieher")
	if len(alts) != 2 {
		t.Fatalf("alternatives = %v", alts)
	}
}

func TestTitleFitUsesTheBestAlternativeAndCandidate(t *testing.T) {
	v, ok, note := titleFit(kitaJob(), erzieher())
	if !ok || v < 0.9 {
		t.Fatalf("exact desired title: %v %v %q", v, ok, note)
	}
	// the headline counts a little less than a desired title
	p := Persona{Headline: "Backend Engineer"}
	v, ok, _ = titleFit(Job{Title: "Backend Engineer"}, p)
	if !ok || v != TitleWeightHeadline {
		t.Errorf("headline exact = %v, want %v", v, TitleWeightHeadline)
	}
	// a past job title counts least
	p = Persona{Experience: []Experience{{Title: "Backend Engineer"}}}
	v, _, _ = titleFit(Job{Title: "Backend Engineer"}, p)
	if v != TitleWeightExperience {
		t.Errorf("past title exact = %v, want %v", v, TitleWeightExperience)
	}
	v, _, _ = titleFit(Job{Title: "Backend Engineer"}, Persona{DesiredTitles: "Backend Engineer"})
	if v != 1 {
		t.Errorf("desired exact = %v", v)
	}
}

func TestTitleFitCompoundsAndFormsMatchPartly(t *testing.T) {
	v, _, _ := titleFit(Job{Title: "Heilerzieher (m/w/d)"}, Persona{DesiredTitles: "Erzieherin"})
	if v <= 0.3 || v >= 1 {
		t.Errorf("a compound is a partial match, got %v", v)
	}
	v, _, _ = titleFit(Job{Title: "Software Engineer"}, Persona{DesiredTitles: "Erzieher"})
	if v != 0 {
		t.Errorf("unrelated titles = %v", v)
	}
}

func TestTitleFitNeedsBothSides(t *testing.T) {
	if _, ok, note := titleFit(Job{Title: "(m/w/d)"}, Persona{DesiredTitles: "x y"}); ok || note == "" {
		t.Error("a title of markers only cannot be judged")
	}
	if _, ok, note := titleFit(Job{Title: "Engineer"}, Persona{}); ok || note == "" {
		t.Error("a persona without titles cannot be judged")
	}
}

func TestAPersonaTitleContainedInTheJobsTitleIsANearPerfectFit(t *testing.T) {
	cases := []struct {
		job, candidate string
		want           float64
	}{
		{"Senior Software Engineer", "Software Engineer", ContainedTitleSim},
		{"Software Engineer Java Agentic Coding", "Software Engineer", ContainedTitleSim},
		{"Lead Software Engineer / Developer", "Software Engineer", ContainedTitleSim},
		{"Software Engineer", "Software Engineer", 1},
	}
	for _, c := range cases {
		alts := titleAlternatives(c.job)
		best := 0.0
		for _, a := range alts {
			if v := titleSim(a, titleWords(c.candidate)); v > best {
				best = v
			}
		}
		if best != c.want {
			t.Errorf("titleSim(%q, %q) = %v, want %v", c.job, c.candidate, best, c.want)
		}
	}
	// a single word is not "contained": "Engineer" fits "Software Engineer" only by the Dice coefficient
	job, one := titleWords("Software Engineer"), titleWords("Engineer")
	if got := titleSim(job, one); got >= ContainedTitleSim || got != dice(job, one) {
		t.Errorf("a one-word title gets no containment bonus: %v", got)
	}
	// words that only match as parts of long compounds (Heilerzieher contains Erzieher) are no full containment
	compound := titleSim(titleWords("Heilerzieher Teamleitung"), titleWords("Erzieher Leitung"))
	if compound != dice(titleWords("Heilerzieher Teamleitung"), titleWords("Erzieher Leitung")) || compound >= ContainedTitleSim {
		t.Errorf("a compound match is no containment: %v", compound)
	}
	// words missing from the job's title: no bonus
	if got := titleSim(titleWords("Senior Backend Engineer"), titleWords("Software Engineer")); got >= ContainedTitleSim {
		t.Errorf("only half of the persona's title is in the job's: %v", got)
	}
	v, _, _ := titleFit(Job{Title: "Senior Software Engineer (all genders)"}, Persona{DesiredTitles: "Software Engineer"})
	if v != ContainedTitleSim {
		t.Errorf("titleFit = %v, want %v", v, ContainedTitleSim)
	}
}
