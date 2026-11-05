package jobmatcher

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTheComponentWeightsAddUpToOneHundred(t *testing.T) {
	if sum := WeightSkills + WeightTitle + WeightExperience + WeightLocation + WeightLevel; sum != 100 {
		t.Fatalf("weights add up to %d", sum)
	}
}

func TestCombiningDropsWhatCannotBeJudgedAndPullsDownThinEvidence(t *testing.T) {
	// skills 1.0 (40), title 1.0 (25), location 0 (10) evaluable; experience and level are not:
	// base = 100 * 65 / 75 = 86.67, coverage 0.75, factor 0.5 + 0.5 * 0.75 = 0.875, score 75.83 -> 76
	job := Job{Title: "Backend Engineer", Location: "Berlin", Requirements: []Requirement{{Term: "Go", Tier: 3}}}
	p := Persona{Name: "x", DesiredTitles: "Backend Engineer", Location: "Hamburg", Skills: []string{"Go"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	if r.Score != 76 {
		t.Errorf("score = %d, want 76 (%s)", r.Score, r.Describe())
	}
	if r.Coverage != 0.75 || r.LowConfidence {
		t.Errorf("coverage %v low=%v", r.Coverage, r.LowConfidence)
	}
	for _, key := range []string{"experience", "level"} {
		c := comp(t, r, key)
		if c.Evaluable || c.Points != 0 || !strings.HasPrefix(c.Note, "not available: ") {
			t.Errorf("%s = %+v", key, c)
		}
	}
	if comp(t, r, "location").Points != 0 || !comp(t, r, "location").Evaluable {
		t.Errorf("a judged mismatch is 0 points, not dropped: %+v", comp(t, r, "location"))
	}
}

func TestAFullyEvidencedPerfectMatchScoresOneHundred(t *testing.T) {
	job := Job{Title: "Senior Backend Engineer", Location: "Hamburg", Description: "3 Jahre Erfahrung.",
		Requirements: []Requirement{{Term: "Go", Tier: 3}, {Term: "Docker", Tier: 3}}}
	p := Persona{Name: "x", DesiredTitles: "Senior Backend Engineer", Location: "Hamburg", Skills: []string{"Go", "Docker"},
		Experience: []Experience{{Title: "Backend Engineer", StartDate: "2015-01", EndDate: "2024-01"}}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	if r.Score != 100 || r.Coverage != 1 || r.LowConfidence {
		t.Errorf("score %d coverage %v: %+v", r.Score, r.Coverage, r.Components)
	}
}

func TestLowConfidenceIsMarkedUnderHalfTheEvidence(t *testing.T) {
	// only skills (40) and title (25) can be judged... plus nothing else: coverage 0.65. Drop the title: skills alone is 0.40
	job := Job{Title: "(m/w/d)", Requirements: []Requirement{{Term: "Go", Tier: 3}}}
	r := mustScore(t, job, []Persona{{Name: "x", Skills: []string{"Go"}}}, NewCorpus(nil))
	if r.Coverage != 0.4 || !r.LowConfidence {
		t.Errorf("coverage %v low=%v", r.Coverage, r.LowConfidence)
	}
	// base 100, factor 0.5 + 0.5*0.4 = 0.70
	if r.Score != 70 {
		t.Errorf("score = %d, want 70", r.Score)
	}
}

func TestNoEvidenceForSkillsOrTitleFailsWithReasonsNotANumber(t *testing.T) {
	// the persona has a place and a level but no skills and no titles; the job has no description
	p := Persona{Name: "Empty", Location: "Hamburg", Experience: []Experience{{StartDate: "2018-01", EndDate: "2022-01"}}}
	_, err := Score(Job{Title: "Engineer", Location: "Hamburg"}, []Persona{p}, NewCorpus(nil), now)
	var ins *InsufficientDataError
	if !errors.As(err, &ins) {
		t.Fatalf("err = %v", err)
	}
	joined := strings.Join(ins.Reasons, " | ")
	for _, want := range []string{"Empty: this persona has no skills", "Empty: this persona has no desired title"} {
		if !strings.Contains(joined, want) {
			t.Errorf("reasons %q lack %q", joined, want)
		}
	}
	if !strings.Contains(ins.Error(), "not enough data") {
		t.Errorf("error text = %q", ins.Error())
	}
}

func TestNoPersonasFails(t *testing.T) {
	if _, err := Score(goJob(), nil, corpusOf(), now); !errors.Is(err, ErrNoPersonas) {
		t.Errorf("err = %v", err)
	}
}

func TestAnInsufficientPersonaIsSkippedWhenAnotherCanBeScored(t *testing.T) {
	r := mustScore(t, goJob(), []Persona{{ID: "empty", Name: "Empty"}, goDev()}, corpusOf())
	if r.PersonaID != "p-go" {
		t.Errorf("persona = %s", r.PersonaID)
	}
}

func TestTheBestPersonaWins(t *testing.T) {
	r := mustScore(t, goJob(), []Persona{erzieher(), goDev()}, corpusOf())
	if r.PersonaID != "p-go" || r.PersonaName != "Backend developer" {
		t.Errorf("persona = %s %s", r.PersonaID, r.PersonaName)
	}
	r = mustScore(t, kitaJob(), []Persona{goDev(), erzieher()}, corpusOf())
	if r.PersonaID != "p-kita" {
		t.Errorf("persona = %s", r.PersonaID)
	}
}

func TestATieGoesToTheEarlierPersona(t *testing.T) {
	a, b := goDev(), goDev()
	a.ID, b.ID = "a", "b"
	if r := mustScore(t, goJob(), []Persona{a, b}, corpusOf()); r.PersonaID != "a" {
		t.Errorf("persona = %s", r.PersonaID)
	}
	if r := mustScore(t, goJob(), []Persona{b, a}, corpusOf()); r.PersonaID != "b" {
		t.Errorf("persona = %s", r.PersonaID)
	}
}

func TestFixtureScoresSitInTheRightBands(t *testing.T) {
	c := corpusOf()
	cases := []struct {
		name   string
		job    Job
		p      Persona
		lo, hi int
	}{
		{"go developer on a Go job", goJob(), goDev(), 70, 92},
		{"Erzieherin on a Kita job (German)", kitaJob(), erzieher(), 60, 90},
		{"Erzieherin on a Go job", goJob(), erzieher(), 0, 25},
		{"Go developer on a Kita job", kitaJob(), goDev(), 0, 25},
	}
	for _, x := range cases {
		inRange(t, x.name, mustScore(t, x.job, []Persona{x.p}, c).Score, x.lo, x.hi)
	}
	good := mustScore(t, goJob(), []Persona{goDev()}, c).Score
	bad := mustScore(t, goJob(), []Persona{erzieher()}, c).Score
	if good-bad < 50 {
		t.Errorf("a fitting persona must score far above a mismatched one: %d vs %d", good, bad)
	}
}

func TestMissingMustHavesLowerTheScoreByTheirWeightWithNoCap(t *testing.T) {
	base := Job{Title: "Backend Engineer", Location: "Hamburg", Requirements: []Requirement{
		{Term: "Go", Tier: 3}, {Term: "Docker", Tier: 3}, {Term: "Kubernetes", Tier: 3},
	}}
	full := mustScore(t, base, []Persona{goDev()}, NewCorpus(nil))
	one := base
	one.Requirements = append(append([]Requirement(nil), base.Requirements...), Requirement{Term: "Terraform", Tier: 3})
	oneMissing := mustScore(t, one, []Persona{goDev()}, NewCorpus(nil))
	if oneMissing.Score >= full.Score {
		t.Errorf("a missing must-have lowers the score: %d vs %d", oneMissing.Score, full.Score)
	}
	// 3 of 4 must-haves covered: skills 0.75 -> 30 of 40 points, and no cap beyond that
	if got := comp(t, oneMissing, "skills").Points; got != 30 {
		t.Errorf("skills points = %v, want 30", got)
	}
	if len(oneMissing.Missing) != 1 || oneMissing.Missing[0] != (Missing{"Terraform", 3}) {
		t.Errorf("missing = %v", oneMissing.Missing)
	}
	if oneMissing.Score < 75 {
		t.Errorf("no cap: one missing must-have out of four must not collapse the score, got %d", oneMissing.Score)
	}
}

func TestOutputIsSortedAndStable(t *testing.T) {
	job := Job{Title: "Engineer", Requirements: []Requirement{
		{Term: "zeta", Tier: 2}, {Term: "Alpha", Tier: 3}, {Term: "beta", Tier: 2}, {Term: "Gamma", Tier: 3},
	}}
	p := Persona{Name: "x", DesiredTitles: "Engineer", Skills: []string{"Omega skill"}}
	first := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	want := []Missing{{"Alpha", 3}, {"Gamma", 3}, {"beta", 2}, {"zeta", 2}}
	if !reflect.DeepEqual(first.Missing, want) {
		t.Errorf("missing = %v, want %v", first.Missing, want)
	}
	a, _ := json.Marshal(first)
	for i := 0; i < 25; i++ {
		b, _ := json.Marshal(mustScore(t, job, []Persona{p}, NewCorpus(nil)))
		if string(a) != string(b) {
			t.Fatalf("run %d differs:\n%s\n%s", i, a, b)
		}
	}
	got := mustScore(t, goJob(), []Persona{goDev()}, corpusOf()).MatchedSkills
	for i := 1; i < len(got); i++ {
		if strings.ToLower(got[i-1]) > strings.ToLower(got[i]) {
			t.Errorf("matched skills not sorted: %v", got)
		}
	}
}

func TestResultJSONNeverHasNullLists(t *testing.T) {
	r := mustScore(t, kitaJob(), []Persona{erzieher()}, corpusOf())
	b, _ := json.Marshal(r)
	for _, k := range []string{`"matchedSkills":null`, `"missing":null`, `"gaps":null`, `"components":null`} {
		if strings.Contains(string(b), k) {
			t.Errorf("%s in %s", k, b)
		}
	}
	if !strings.Contains(string(b), `"engineVersion":"manual-1"`) {
		t.Errorf("no engine version in %s", b)
	}
}

func TestInputCapsAreEnforced(t *testing.T) {
	// a skill mentioned only after the description cap is not found
	desc := strings.Repeat("blah ", MaxDescriptionRunes/5+10) + " Kubernetes"
	job := Job{Title: "Engineer", Description: desc}
	p := Persona{Name: "x", DesiredTitles: "Engineer", Skills: []string{"Kubernetes", "Blah"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	eqStrings(t, "matched", r.MatchedSkills, "Blah")

	// skills past the cap are ignored
	skills := make([]string, MaxSkills+10)
	for i := range skills {
		skills[i] = "skill" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26))
	}
	skills[MaxSkills+5] = "Kubernetes"
	r = mustScore(t, Job{Title: "Engineer", Description: "Kubernetes"}, []Persona{{Name: "x", DesiredTitles: "Engineer", Skills: skills}}, NewCorpus(nil))
	if len(r.MatchedSkills) != 0 {
		t.Errorf("a skill past the cap was used: %v", r.MatchedSkills)
	}
}

func TestGapsListWhatTheJobStressesAndThePersonaNeverMentions(t *testing.T) {
	job := Job{Title: "Platform Engineer", Description: "Terraform und Terraform-Module, Ansible Playbooks, Ansible Rollen. Go ist ein Plus."}
	p := Persona{Name: "x", DesiredTitles: "Platform Engineer", Skills: []string{"Ansible", "Go"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	if r.Mode != ModeText {
		t.Fatal("text mode expected")
	}
	if !contains(r.Gaps, "Terraform") {
		t.Errorf("gaps = %v", r.Gaps)
	}
	if contains(r.Gaps, "Ansible") {
		t.Errorf("a word the persona has is not a gap: %v", r.Gaps)
	}
	if len(r.Gaps) > MaxGaps {
		t.Errorf("gaps = %v", r.Gaps)
	}
	// requirements mode lists missing requirements, not gaps
	job.Requirements = []Requirement{{Term: "Terraform", Tier: 3}}
	r = mustScore(t, job, []Persona{p}, NewCorpus(nil))
	if len(r.Gaps) != 0 || len(r.Missing) != 1 {
		t.Errorf("gaps %v missing %v", r.Gaps, r.Missing)
	}
}

func TestRareWordsQualifyAsGapsInABigCorpus(t *testing.T) {
	var texts []string
	for i := 0; i < 60; i++ {
		texts = append(texts, "Wir suchen Verstärkung im Team")
	}
	texts = append(texts, "Wir suchen Verstärkung mit Kubernetes")
	c := NewCorpus(texts)
	job := Job{Title: "Engineer", Description: "Wir suchen Verstärkung mit Kubernetes Erfahrung."}
	r := mustScore(t, job, []Persona{{Name: "x", DesiredTitles: "Engineer", Skills: []string{"Go"}}}, c)
	if !contains(r.Gaps, "Kubernetes") {
		t.Errorf("a rare word is a gap even when it appears once: %v", r.Gaps)
	}
}

func TestSuggestOffersDistinctiveWordsInTheirOwnSpelling(t *testing.T) {
	job := Job{Title: "Platform Engineer", Description: "Kubernetes Cluster betreiben. Kubernetes und Terraform, Terraform Module. Die Bewerbung bitte an uns."}
	got := Suggest(job, NewCorpus(nil))
	if !contains(got, "Kubernetes") || !contains(got, "Terraform") || !contains(got, "Engineer") {
		t.Errorf("suggestions = %v", got)
	}
	for _, w := range got {
		if stopWords[strings.ToLower(w)] || len(w) < 3 {
			t.Errorf("a filler word was suggested: %q", w)
		}
	}
	// a word an existing requirement uses is not offered again
	job.Requirements = []Requirement{{Term: "Kubernetes", Tier: 3}}
	got = Suggest(job, NewCorpus(nil))
	if contains(got, "Kubernetes") || !contains(got, "Terraform") {
		t.Errorf("suggestions = %v", got)
	}
	if got := Suggest(Job{Title: "A"}, NewCorpus(nil)); got == nil || len(got) != 0 {
		t.Errorf("no words gives an empty, non-nil list: %#v", got)
	}
	many := strings.Repeat("Alpha Beta Gamma Delta Epsilon Zeta Theta Iota Kappa Lambda Omicron Sigma Upsilon Omega ", 3)
	if got := Suggest(Job{Title: "T", Description: many}, NewCorpus(nil)); len(got) != MaxSuggestions {
		t.Errorf("suggestions are capped at %d, got %d", MaxSuggestions, len(got))
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestAnUnrelatedJobInTheSameCityAtTheSameLevelStaysNearZero(t *testing.T) {
	// the live case that motivated the gate: nothing about the job resembles the persona's field, but the city and the
	// seniority happen to fit, which used to be worth about 30 points
	p := Persona{
		Name: "Software", Headline: "Software Engineer", DesiredTitles: "Software Engineer", Location: "Hamburg",
		Skills:     []string{"Python", "Git", "Docker"},
		Experience: []Experience{{Title: "Software Engineer", StartDate: "2016-01", EndDate: ""}},
	}
	job := Job{Title: "Assistenz der Leitung Jugendhilfe (m/w/d)", Location: "Hamburg"}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	if r.Score > 12 || r.RelevanceScale >= 0.5 {
		t.Errorf("score %d, relevance scale %v: %+v", r.Score, r.RelevanceScale, r.Components)
	}
	if comp(t, r, "location").Points != WeightLocation || comp(t, r, "level").Points != WeightLevel {
		t.Errorf("the place and the level did fit, only the gate keeps them from counting: %+v", r.Components)
	}
}

func TestTheRelevanceGateScalesInProportionAndLeavesRealMatchesAlone(t *testing.T) {
	p := Persona{Name: "x", DesiredTitles: "Backend Developer", Location: "Hamburg"}
	// title similarity 0.4 -> 10 points (under the floor of 12): scale 10/12
	job := Job{Title: "Backend Engineer Retail", Location: "Hamburg"}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	points := comp(t, r, "title").Points
	want := round2(points / RelevanceFloor)
	if points >= RelevanceFloor || r.RelevanceScale != want {
		t.Errorf("title points %v, scale %v, want %v", points, r.RelevanceScale, want)
	}
	// a full title match earns the floor many times over: no scaling at all
	full := mustScore(t, Job{Title: "Backend Developer", Location: "Hamburg"}, []Persona{p}, NewCorpus(nil))
	if full.RelevanceScale != 1 {
		t.Errorf("a fitting job is never scaled: %v", full.RelevanceScale)
	}
	// skills count towards relevance too: a job whose title says nothing but whose text names the persona's skills
	sk := Persona{Name: "x", DesiredTitles: "Zahnarzt", Skills: []string{"Go", "Docker", "Kubernetes", "PostgreSQL"}}
	job2 := Job{Title: "Plattformbetrieb", Description: "Go, Docker, Kubernetes und PostgreSQL im Alltag."}
	if r := mustScore(t, job2, []Persona{sk}, NewCorpus(nil)); r.RelevanceScale != 1 || r.Score < 25 {
		t.Errorf("skills alone make a job relevant: scale %v score %d", r.RelevanceScale, r.Score)
	}
}

func TestRelevanceScaleIsInTheJSON(t *testing.T) {
	r := mustScore(t, goJob(), []Persona{goDev()}, corpusOf())
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"relevanceScale":1`) {
		t.Errorf("json = %s", b)
	}
}
