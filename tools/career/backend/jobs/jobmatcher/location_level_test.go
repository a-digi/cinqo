package jobmatcher

import "testing"

func TestCityWords(t *testing.T) {
	cases := map[string][]string{
		"20095 Hamburg-Altona": {"hamburg", "altona"},
		"Hamburg, Deutschland": {"hamburg"},
		"München und Umgebung": {"muenchen"},
		"New York, USA":        {"new", "york"},
		"":                     nil,
		"1234":                 nil,
	}
	for in, want := range cases {
		eqStrings(t, in, cityWords(in), want...)
	}
}

func locOf(job, persona string, jobTitle, desc, personaRemote string) (float64, bool) {
	pj := prepareJob(Job{Title: jobTitle, Location: job, Description: desc})
	v, ok, _ := locationFit(pj, Persona{Location: persona, DesiredLocations: personaRemote})
	return v, ok
}

func TestLocationFit(t *testing.T) {
	if v, ok := locOf("20095 Hamburg-Altona", "Hamburg", "x", "", ""); !ok || v != 1 {
		t.Errorf("same city = %v %v", v, ok)
	}
	if v, ok := locOf("Berlin", "Hamburg", "x", "", ""); !ok || v != 0 {
		t.Errorf("another city = %v %v", v, ok)
	}
	if v, ok := locOf("Berlin", "Hamburg", "x", "Hybrid möglich", ""); !ok || v != LocationRemote {
		t.Errorf("remote job, other city = %v %v", v, ok)
	}
	if v, ok := locOf("Berlin", "Hamburg", "x", "", "Remote"); !ok || v != 0 {
		t.Errorf("a remote persona does not make a office job remote = %v %v", v, ok)
	}
	if v, ok := locOf("Berlin", "Hamburg", "Developer (Remote)", "", "Hamburg, Remote"); !ok || v != 1 {
		t.Errorf("remote persona, remote job = %v %v", v, ok)
	}
	if v, ok := locOf("", "Hamburg", "Developer", "Home Office möglich", ""); !ok || v != LocationRemote {
		t.Errorf("no place but remote = %v %v", v, ok)
	}
	if _, ok := locOf("", "Hamburg", "Developer", "Office in the city", ""); ok {
		t.Error("a job with no place and no remote mark cannot be judged")
	}
	if _, ok := locOf("Hamburg", "", "x", "", ""); ok {
		t.Error("a persona with no place cannot be judged")
	}
}

func TestLevelOfWords(t *testing.T) {
	cases := map[string]int{
		"Junior Developer":              0,
		"Werkstudent Marketing":         0,
		"Werkstudentin (m/w/d)":         0,
		"Praktikum im Bereich Pflege":   0,
		"Senior Backend Engineer":       2,
		"Teamleitung Ambulante Hilfen":  2,
		"Lead Architect":                2,
		"Backend Engineer":              -1,
		"Senior oder Junior Entwickler": -1,
		"Seniorenbetreuer":              2, // prefix of a 5+ letter marker: a known limit, kept visible here
	}
	for in, want := range cases {
		if got := levelOfWords(titleWords(in)); got != want {
			t.Errorf("level(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLevelFit(t *testing.T) {
	senior := prepareJob(Job{Title: "Senior Engineer"})
	junior := prepareJob(Job{Title: "Junior Engineer"})
	cases := []struct {
		name   string
		pj     preparedJob
		p      Persona
		months int
		want   float64
		ok     bool
	}{
		{"senior job, senior months", senior, Persona{}, 100, 1, true},
		{"senior job, mid months", senior, Persona{}, 48, 0.5, true},
		{"senior job, junior months", senior, Persona{}, 6, 0, true},
		{"junior job, senior months", junior, Persona{}, 100, 0, true},
		{"desired title overrides months", junior, Persona{DesiredTitles: "Junior Developer"}, 100, 1, true},
		{"no level on the job", prepareJob(Job{Title: "Engineer"}), Persona{}, 100, 0, false},
		{"no experience, no word", senior, Persona{}, 0, 0, false},
		{"no experience but a word", senior, Persona{DesiredTitles: "Senior Developer"}, 0, 1, true},
	}
	for _, c := range cases {
		v, ok, _ := levelFit(c.pj, c.p, c.months)
		if ok != c.ok || (ok && v != c.want) {
			t.Errorf("%s: %v %v, want %v %v", c.name, v, ok, c.want, c.ok)
		}
	}
}
