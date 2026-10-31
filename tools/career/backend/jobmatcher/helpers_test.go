package jobmatcher

import (
	"strings"
	"testing"
	"time"
)

// now is fixed so months of experience never depend on the day the tests run.
var now = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func mustScore(t *testing.T, job Job, personas []Persona, c Corpus) Result {
	t.Helper()
	r, err := Score(job, personas, c, now)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	return r
}

func comp(t *testing.T, r Result, key string) Component {
	t.Helper()
	for _, c := range r.Components {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("no component %q in %+v", key, r.Components)
	return Component{}
}

func inRange(t *testing.T, name string, got, lo, hi int) {
	t.Helper()
	if got < lo || got > hi {
		t.Errorf("%s = %d, want %d..%d", name, got, lo, hi)
	}
}

func eqStrings(t *testing.T, name string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// ---- fixtures ----

func goDev() Persona {
	return Persona{
		ID: "p-go", Name: "Backend developer",
		Headline: "Backend Engineer", DesiredTitles: "Software Engineer, Backend Developer", DesiredLocations: "Hamburg, Remote",
		Location: "Hamburg",
		Skills:   []string{"Go", "Docker", "PostgreSQL", "Kubernetes", "REST", "Git", "SQL", "C++", "CI/CD", "Linux"},
		Experience: []Experience{
			{Title: "Backend Developer", Description: "Built Go services with Docker and PostgreSQL", StartDate: "2019-03", EndDate: "2022-02"},
			{Title: "Senior Software Engineer", Description: "Kubernetes platform, Go, CI/CD pipelines", StartDate: "2022-03", EndDate: ""},
		},
	}
}

func erzieher() Persona {
	return Persona{
		ID: "p-kita", Name: "Erzieherin",
		Headline: "Staatlich anerkannte Erzieherin", DesiredTitles: "Erzieher, Heilerzieher/in, Sozialpädagogische Assistenz", DesiredLocations: "Hamburg",
		Location: "Hamburg",
		Skills:   []string{"Gruppenleitung", "Elternarbeit", "Sprachförderung", "Beobachtung und Dokumentation", "MS Office"},
		Experience: []Experience{
			{Title: "Erzieherin", Description: "Gruppenleitung in einer Kita, Elternarbeit, Sprachförderung", StartDate: "2016-08", EndDate: "2021-07"},
			{Title: "Erzieherin", Description: "Krippe, Elterngespräche", StartDate: "2021-08", EndDate: ""},
		},
	}
}

const goJobDesc = "Wir suchen Verstärkung für unser Plattform-Team in Hamburg. Du entwickelst Backend-Services in Go, " +
	"betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Erfahrung mit REST-APIs und CI/CD ist ein Plus. " +
	"Mindestens 3 Jahre Berufserfahrung in der Softwareentwicklung. Hybrid möglich."

func goJob() Job {
	return Job{Title: "Backend Engineer (m/w/d) Go", Company: "Acme", Location: "Hamburg", Description: goJobDesc}
}

const kitaJobDesc = "Die Kita Sonnenschein sucht ab sofort Erzieher / Heilerzieher (m/w/d) in Voll- oder Teilzeit. " +
	"Sie begleiten Kinder in der Gruppenleitung, pflegen die Elternarbeit und fördern die Sprachentwicklung. " +
	"Wir bieten ein familiäres Team. Wünschenswert sind 2 Jahre Berufserfahrung."

func kitaJob() Job {
	return Job{Title: "Erzieher / Heilerzieher (m/w/d) für die Kita", Location: "20095 Hamburg-Altona", Description: kitaJobDesc}
}

func corpusOf(extra ...string) Corpus {
	texts := []string{goJobDesc, kitaJobDesc}
	return NewCorpus(append(texts, extra...))
}
