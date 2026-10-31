package jobmatcher

import (
	"math"
	"strings"
	"testing"
)

func TestTextModeFindsWholeWordSkillsAndSaturates(t *testing.T) {
	r := mustScore(t, goJob(), []Persona{goDev()}, corpusOf())
	if r.Mode != ModeText {
		t.Fatalf("mode = %s", r.Mode)
	}
	eqStrings(t, "matched", r.MatchedSkills, "CI/CD", "Docker", "Go", "Kubernetes", "PostgreSQL", "REST")
	sk := comp(t, r, "skills")
	// six hits of weight 1 (a corpus under 20 jobs), Go is also in the title (+0.25): 1 - exp(-6.25/4), of 40 points
	want := 40 * (1 - math.Exp(-(6.0+SkillTitleBonus)/SkillSaturation))
	if math.Abs(sk.Points-want) > 0.05 {
		t.Errorf("skills points = %v, want about %.2f", sk.Points, want)
	}
	if !sk.Evaluable || !strings.Contains(sk.Note, "job text") {
		t.Errorf("note = %q", sk.Note)
	}
}

func TestShortSkillsNeverMatchInsideOtherWords(t *testing.T) {
	job := Job{Title: "Chef de Partie", Description: "Wir kochen bei Google mit JavaScript und Rust. Gute Chefs gesucht, Good food."}
	p := Persona{Name: "x", DesiredTitles: "Koch", Skills: []string{"Go", "Java", "C", "R", "C++", "Rust"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	eqStrings(t, "matched", r.MatchedSkills, "Rust")
}

func TestSymbolsInSkillNamesMatchTheirJobSpelling(t *testing.T) {
	job := Job{Title: "Entwickler", Description: "Kenntnisse in C++ und C#, .NET sowie Node.js; Linux und CI/CD."}
	p := Persona{Name: "x", DesiredTitles: "Entwickler", Skills: []string{"C++", "C#", ".NET", "Node.js", "CI/CD", "C", "Linux"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	eqStrings(t, "matched", r.MatchedSkills, ".NET", "C#", "C++", "CI/CD", "Linux", "Node.js")
}

func TestAliasesMatchAcrossSpellings(t *testing.T) {
	job := Job{Title: "Frontend", Description: "JavaScript, Kubernetes und PostgreSQL gesucht."}
	p := Persona{Name: "x", DesiredTitles: "Frontend", Skills: []string{"JS", "k8s", "Postgres", "Golang"}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	eqStrings(t, "matched", r.MatchedSkills, "JS", "k8s", "Postgres")
}

func TestRareSkillsWeighMoreWhenTheCorpusIsBigEnough(t *testing.T) {
	var texts []string
	for i := 0; i < 30; i++ {
		texts = append(texts, "Wir nutzen MS Office und Excel täglich")
	}
	texts = append(texts, "Kubernetes Cluster Betrieb")
	c := NewCorpus(texts)
	office := c.skillWeight(tokenize("MS Office"))
	kube := c.skillWeight(tokenize("Kubernetes"))
	if !(kube > office) || office < SkillWeightMin || kube > SkillWeightMin+1 {
		t.Errorf("office %.2f, kubernetes %.2f", office, kube)
	}
	if w := NewCorpus(texts[:5]).skillWeight(tokenize("Kubernetes")); w != 1 {
		t.Errorf("a small corpus gives weight 1, got %v", w)
	}
	// the title bonus never passes the cap
	job := Job{Title: "Kubernetes Engineer", Description: "Kubernetes"}
	r := mustScore(t, job, []Persona{{Name: "x", DesiredTitles: "Engineer", Skills: []string{"Kubernetes"}}}, c)
	if got := comp(t, r, "skills").Points; got > WeightSkills*(1-math.Exp(-SkillWeightMax/SkillSaturation))+0.01 {
		t.Errorf("one skill cannot weigh more than the cap: %v points", got)
	}
}

func TestTextModeNeedsSkillsAndADescription(t *testing.T) {
	_, err := Score(Job{Title: "Engineer", Description: ""}, []Persona{{Name: "x", Skills: []string{"Go"}}}, NewCorpus(nil), now)
	var ins *InsufficientDataError
	if err == nil || !asInsufficient(err, &ins) {
		t.Fatalf("a job with no description and a persona with no title cannot be scored: %v", err)
	}
	if !strings.Contains(strings.Join(ins.Reasons, " "), "no description") {
		t.Errorf("reasons = %v", ins.Reasons)
	}
}

func TestRequirementsModeUsesTierWeightsAndEvidenceStrength(t *testing.T) {
	job := Job{
		Title: "Backend Engineer", Location: "Hamburg",
		Requirements: []Requirement{
			{Term: "Go", Tier: 3},        // a skill: 1.0
			{Term: "Docker", Tier: 2},    // a skill: 1.0
			{Term: "Terraform", Tier: 1}, // nothing: 0
		},
	}
	r := mustScore(t, job, []Persona{goDev()}, NewCorpus(nil))
	if r.Mode != ModeRequirements {
		t.Fatalf("mode = %s", r.Mode)
	}
	// the library's own example: (3 + 2) / (3 + 2 + 1) = 5/6 of 40 points
	if got := comp(t, r, "skills").Points; math.Abs(got-40*5.0/6.0) > 0.01 {
		t.Errorf("skills points = %v, want %.2f", got, 40*5.0/6.0)
	}
	eqStrings(t, "matched", r.MatchedSkills, "Docker", "Go")
	if len(r.Missing) != 0 {
		t.Errorf("a missing nice-to-have is not listed: %v", r.Missing)
	}
}

func TestRequirementsAreCoveredByExperienceAndHeadlineAtLowerStrength(t *testing.T) {
	p := Persona{
		Name: "x", Headline: "Scrum Master", Skills: []string{"Go"},
		Experience: []Experience{{Title: "Dev", Description: "Led Kubernetes migrations", StartDate: "2020-01", EndDate: "2022-01"}},
	}
	job := Job{Title: "Dev", Requirements: []Requirement{{Term: "Go", Tier: 2}, {Term: "Kubernetes", Tier: 2}, {Term: "Scrum", Tier: 2}, {Term: "Rust", Tier: 2}}}
	r := mustScore(t, job, []Persona{p}, NewCorpus(nil))
	// (1.0 + 0.7 + 0.5 + 0) * 2 / (4 * 2)
	if got := comp(t, r, "skills").Points; math.Abs(got-40*2.2/4) > 0.01 {
		t.Errorf("skills points = %v, want %.2f", got, 40*2.2/4)
	}
	eqStrings(t, "matched", r.MatchedSkills, "Go")
	if len(r.Missing) != 1 || r.Missing[0].Term != "Rust" {
		t.Errorf("missing = %v", r.Missing)
	}
}

func TestLongRequirementsAreMatchedByTheirWords(t *testing.T) {
	job := Job{Title: "Erzieher", Requirements: []Requirement{
		{Term: "Abgeschlossene Ausbildung als Erzieher", Tier: 3},
		{Term: "Erfahrung in der Arbeit mit Jugendlichen im Bereich Suchtprävention", Tier: 2},
	}}
	r := mustScore(t, job, []Persona{erzieher()}, NewCorpus(nil))
	// the first: "abgeschlossene" is not in the persona, ausbildung neither: 2 of 4 words (erzieher, als is a stop word)
	// is below the threshold for skills, but the headline / experience cover "erzieher" only: the check is by words
	if len(r.Missing) != 2 {
		t.Fatalf("both are missing for this persona, got %v", r.Missing)
	}
	if r.Missing[0].Tier != 3 || r.Missing[1].Tier != 2 {
		t.Errorf("missing is sorted by tier, most important first: %v", r.Missing)
	}

	p := erzieher()
	p.Headline = "Staatlich anerkannte Erzieherin mit abgeschlossener Ausbildung"
	p.Summary = "Abgeschlossene Ausbildung als Erzieherin, Erfahrung in der Arbeit mit Jugendlichen"
	r = mustScore(t, job, []Persona{p}, NewCorpus(nil))
	covered := 0
	for _, m := range r.Missing {
		_ = m
		covered++
	}
	if covered != 1 {
		t.Errorf("the summary now covers the first requirement; the second (Suchtprävention) is still missing: %v", r.Missing)
	}
}

func TestRequirementsModeDoesNotNeedADescription(t *testing.T) {
	job := Job{Title: "Backend Engineer", Requirements: []Requirement{{Term: "Go", Tier: 3}}}
	r := mustScore(t, job, []Persona{goDev()}, NewCorpus(nil))
	if !comp(t, r, "skills").Evaluable || r.Mode != ModeRequirements {
		t.Errorf("a job with requirements but no description is still scored: %+v", r)
	}
}

func TestBadRequirementsAreIgnored(t *testing.T) {
	job := Job{Title: "Backend Engineer", Description: goJobDesc, Requirements: []Requirement{
		{Term: "  ", Tier: 3}, {Term: "Go", Tier: 0}, {Term: "Go", Tier: 4}, {Term: "!!!", Tier: 2},
	}}
	r := mustScore(t, job, []Persona{goDev()}, NewCorpus(nil))
	if r.Mode != ModeText {
		t.Errorf("no usable requirement means text mode, got %s", r.Mode)
	}
	if got := cleanRequirements([]Requirement{{"Go", 3}, {"go", 2}, {"GO", 1}}); len(got) != 1 {
		t.Errorf("duplicates ignoring case collapse: %v", got)
	}
	many := make([]Requirement, 50)
	for i := range many {
		many[i] = Requirement{Term: "skill" + strings.Repeat("x", i), Tier: 1}
	}
	if got := cleanRequirements(many); len(got) != MaxRequirements {
		t.Errorf("capped at %d, got %d", MaxRequirements, len(got))
	}
}

func asInsufficient(err error, target **InsufficientDataError) bool {
	e, ok := err.(*InsufficientDataError)
	if ok {
		*target = e
	}
	return ok
}
