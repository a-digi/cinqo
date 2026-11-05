package jobmatcher

import (
	"errors"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

var wordPool = []string{
	"Go", "Java", "JavaScript", "C++", "C#", ".NET", "Node.js", "CI/CD", "Docker", "Kubernetes", "SQL", "MS Office",
	"Erzieher", "Erzieherin", "Heilerzieher", "Pädagoge", "Sozialpädagogik", "Gruppenleitung", "Kita", "Pflege",
	"Senior", "Junior", "Werkstudent", "Teamleitung", "Backend", "Engineer", "Entwickler", "Developer", "Manager",
	"Hamburg", "Berlin", "München", "Remote", "Hybrid", "Home Office", "20095", "m/w/d", "(w/m/d)", "Erzieher/in",
	"Berufserfahrung", "3 Jahre", "5+ Jahre Erfahrung", "und", "oder", "der", "the", "with", "Ärztin", "naïve", "日本語", "",
}

func randWords(r *rand.Rand, n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, wordPool[r.Intn(len(wordPool))])
	}
	return strings.Join(parts, []string{" ", ", ", " / ", ". "}[r.Intn(4)])
}

func randPersona(r *rand.Rand, i int) Persona {
	p := Persona{ID: string(rune('a' + i)), Name: randWords(r, 1)}
	if r.Intn(3) > 0 {
		p.Headline = randWords(r, r.Intn(4))
	}
	p.Summary = randWords(r, r.Intn(10))
	p.Location = randWords(r, r.Intn(2))
	p.DesiredTitles = randWords(r, r.Intn(5))
	p.DesiredLocations = randWords(r, r.Intn(3))
	for k := r.Intn(8); k > 0; k-- {
		p.Skills = append(p.Skills, randWords(r, 1+r.Intn(2)))
	}
	dates := []string{"", "2015", "2018-04", "04/2019", "2020-11", "garbage", "heute", "present"}
	for k := r.Intn(4); k > 0; k-- {
		p.Experience = append(p.Experience, Experience{
			Title: randWords(r, r.Intn(3)), Description: randWords(r, r.Intn(8)),
			StartDate: dates[r.Intn(len(dates))], EndDate: dates[r.Intn(len(dates))],
		})
	}
	return p
}

func randJob(r *rand.Rand) Job {
	j := Job{Title: randWords(r, r.Intn(5)), Company: randWords(r, 1), Location: randWords(r, r.Intn(3)), Description: randWords(r, r.Intn(40))}
	for k := r.Intn(6); k > 0; k-- {
		j.Requirements = append(j.Requirements, Requirement{Term: randWords(r, 1+r.Intn(6)), Tier: r.Intn(5)})
	}
	return j
}

// Whatever goes in, a score is 0-100, an answer needs real evidence, and every list is sorted and non-nil.
func TestPropertiesHoldForRandomInput(t *testing.T) {
	r := rand.New(rand.NewSource(129))
	var texts []string
	for i := 0; i < 25; i++ {
		texts = append(texts, randWords(r, 30))
	}
	corpus := NewCorpus(texts)
	scored, failed := 0, 0
	for i := 0; i < 600; i++ {
		job := randJob(r)
		var personas []Persona
		for k := r.Intn(4); k > 0; k-- {
			personas = append(personas, randPersona(r, k))
		}
		res, err := Score(job, personas, corpus, now)
		if err != nil {
			var ins *InsufficientDataError
			if !errors.Is(err, ErrNoPersonas) && !errors.As(err, &ins) {
				t.Fatalf("iteration %d: unexpected error %v", i, err)
			}
			if errors.As(err, &ins) && len(ins.Reasons) == 0 {
				t.Fatalf("iteration %d: insufficient data without a reason", i)
			}
			failed++
			continue
		}
		scored++
		if res.Score < 0 || res.Score > 100 {
			t.Fatalf("iteration %d: score %d", i, res.Score)
		}
		if len(res.Components) != 5 || res.EngineVersion != EngineVersion || res.Describe() == "" {
			t.Fatalf("iteration %d: %+v", i, res)
		}
		var weight int
		evaluable := map[string]bool{}
		for _, c := range res.Components {
			evaluable[c.Key] = c.Evaluable
			if c.Evaluable {
				weight += c.Weight
			}
			if c.Points < 0 || c.Points > float64(c.Weight) || (!c.Evaluable && c.Points != 0) {
				t.Fatalf("iteration %d: component %+v", i, c)
			}
		}
		if !evaluable["skills"] && !evaluable["title"] {
			t.Fatalf("iteration %d: a score without skills or title evidence: %+v", i, res.Components)
		}
		if res.Coverage <= 0 || res.Coverage > 1 || res.LowConfidence != (res.Coverage < LowConfidenceBy) || int(res.Coverage*100+0.5) != weight {
			t.Fatalf("iteration %d: coverage %v with evaluable weight %d", i, res.Coverage, weight)
		}
		if res.MatchedSkills == nil || res.Missing == nil || res.Gaps == nil {
			t.Fatalf("iteration %d: nil list in %+v", i, res)
		}
		if !sort.SliceIsSorted(res.MatchedSkills, func(a, b int) bool { return lessFold(res.MatchedSkills[a], res.MatchedSkills[b]) }) {
			t.Fatalf("iteration %d: matched skills unsorted %v", i, res.MatchedSkills)
		}
		if !sort.SliceIsSorted(res.Missing, func(a, b int) bool {
			if res.Missing[a].Tier != res.Missing[b].Tier {
				return res.Missing[a].Tier > res.Missing[b].Tier
			}
			return strings.ToLower(res.Missing[a].Term) < strings.ToLower(res.Missing[b].Term)
		}) {
			t.Fatalf("iteration %d: missing unsorted %v", i, res.Missing)
		}
		if res.Mode == ModeRequirements && len(res.Gaps) != 0 || res.Mode == ModeText && len(res.Missing) != 0 {
			t.Fatalf("iteration %d: %s mode lists the other kind: %+v", i, res.Mode, res)
		}
		again, err := Score(job, personas, corpus, now)
		if err != nil || again.Score != res.Score || again.PersonaID != res.PersonaID {
			t.Fatalf("iteration %d: not deterministic", i)
		}
	}
	if scored < 100 || failed < 10 {
		t.Errorf("the generator should exercise both outcomes, scored %d failed %d", scored, failed)
	}
}

// Adding a requirement the persona covers never lowers a score that has only requirements to go on.
func TestACoveredRequirementNeverLowersTheSkillsComponent(t *testing.T) {
	job := Job{Title: "Engineer", Requirements: []Requirement{{Term: "Terraform", Tier: 3}}}
	p := Persona{Name: "x", DesiredTitles: "Engineer", Skills: []string{"Go", "Docker"}}
	before := comp(t, mustScore(t, job, []Persona{p}, NewCorpus(nil)), "skills").Points
	job.Requirements = append(job.Requirements, Requirement{Term: "Go", Tier: 3})
	after := comp(t, mustScore(t, job, []Persona{p}, NewCorpus(nil)), "skills").Points
	if after < before {
		t.Errorf("%v -> %v", before, after)
	}
}
