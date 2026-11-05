package jobmatcher

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// preparedJob is a job read once: its words, its cleaned requirements.
type preparedJob struct {
	job         Job
	titleTokens []string // the title as written, for skills
	descTokens  []string
	titleWords  []string // the title cleaned of gender and contract markers, for title, experience and level
	titleAlts   [][]string
	workMode    WorkMode // how the job is worked, read from its text
	reqs        []reqTokens
}

func prepareJob(j Job) preparedJob {
	if utf8.RuneCountInString(j.Description) > MaxDescriptionRunes {
		j.Description = string([]rune(j.Description)[:MaxDescriptionRunes])
	}
	return preparedJob{
		job:         j,
		titleTokens: tokenize(j.Title),
		descTokens:  tokenize(j.Description),
		titleWords:  titleWords(j.Title),
		titleAlts:   titleAlternatives(j.Title),
		workMode:    jobWorkMode(j),
		reqs:        cleanRequirements(j.Requirements),
	}
}

func (pj preparedJob) requirementTerms() []string {
	out := make([]string, len(pj.reqs))
	for i, r := range pj.reqs {
		out[i] = r.Term
	}
	return out
}

func capPersona(p Persona) Persona {
	if len(p.Skills) > MaxSkills {
		p.Skills = p.Skills[:MaxSkills]
	}
	if len(p.Experience) > MaxExperience {
		p.Experience = p.Experience[:MaxExperience]
	}
	return p
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// pointsEarned is the points a component earned (0 when it could not be judged).
func (p part) pointsEarned() float64 {
	if !p.evaluable {
		return 0
	}
	return float64(p.weight) * p.value
}

type part struct {
	key, label string
	weight     int
	value      float64
	evaluable  bool
	note       string
}

// scorePersona scores a job against one persona. When neither Skills nor Title fit can be judged it returns the
// reasons instead (every one fixable by a person) and no result.
func scorePersona(pj preparedJob, p Persona, c Corpus, now time.Time) (Result, []string) {
	ev := buildEvidence(p)
	var sk skillsOutcome
	if len(pj.reqs) > 0 {
		sk = requirementsSkills(pj.reqs, ev)
	} else {
		sk = textSkills(pj, ev, c)
	}
	tv, tOK, tNote := titleFit(pj.job, p)
	if !sk.evaluable && !tOK {
		who := p.Name
		if strings.TrimSpace(who) == "" {
			who = "persona"
		}
		return Result{}, []string{who + ": " + sk.note, who + ": " + tNote}
	}
	ev2, eOK, eNote, months := experienceFit(pj, p, sk, now)
	lv, lOK, lNote := locationFit(pj, p)
	vv, vOK, vNote := levelFit(pj, p, months)
	parts := []part{
		{"skills", "Skills", WeightSkills, sk.value, sk.evaluable, sk.note},
		{"title", "Title fit", WeightTitle, tv, tOK, tNote},
		{"experience", "Experience", WeightExperience, ev2, eOK, eNote},
		{"location", "Location", WeightLocation, lv, lOK, lNote},
		{"level", "Level", WeightLevel, vv, vOK, vNote},
	}
	var earned, evaluableWeight float64
	comps := make([]Component, 0, len(parts))
	for _, x := range parts {
		c := Component{Key: x.key, Label: x.label, Weight: x.weight, Evaluable: x.evaluable, Note: x.note}
		if x.evaluable {
			c.Points = round2(float64(x.weight) * x.value)
			earned += float64(x.weight) * x.value
			evaluableWeight += float64(x.weight)
		} else {
			c.Note = "not available: " + x.note
		}
		comps = append(comps, c)
	}
	base := 100 * earned / evaluableWeight
	cov := evaluableWeight / 100
	// the points Skills and Title fit earned decide whether the job is in the persona's field at all
	relevance := math.Min(1, (parts[0].pointsEarned()+parts[1].pointsEarned())/RelevanceFloor)
	dealbreaker := 1.0
	if rulesOut(pj, p) {
		dealbreaker = DealbreakerScale
	}
	score := int(math.Round(base * (ConfidenceBase + ConfidenceSpan*cov) * relevance * dealbreaker))
	score = max(0, min(100, score))

	r := Result{
		Score:            score,
		PersonaID:        p.ID,
		PersonaName:      p.Name,
		MatchedSkills:    nonNil(sk.matched),
		Mode:             sk.mode,
		Coverage:         round2(cov),
		LowConfidence:    cov < LowConfidenceBy,
		RelevanceScale:   round2(relevance),
		DealbreakerScale: dealbreaker,
		Components:       comps,
		Missing:          sk.missing,
		Gaps:             []string{},
		EngineVersion:    EngineVersion,
	}
	if r.Missing == nil {
		r.Missing = []Missing{}
	}
	if sk.mode == ModeText {
		r.Gaps = nonNil(gapWords(pj, ev, c))
	}
	return r, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// scoreEach scores a prepared job against every persona, in the order given: the results of the personas that could be
// judged, and the reasons of those that could not.
func scoreEach(pj preparedJob, personas []Persona, corpus Corpus, now time.Time) ([]Result, []string) {
	var results []Result
	var reasons []string
	for _, p := range personas {
		r, why := scorePersona(pj, capPersona(p), corpus, now)
		if why != nil {
			reasons = append(reasons, why...)
			continue
		}
		results = append(results, r)
	}
	return results, reasons
}

// betterThan is the best-persona rule: the higher score, ties to the persona with more matched skills; a full tie keeps
// the earlier one (the caller keeps the current best unless the challenger is strictly better).
func betterThan(challenger, best Result) bool {
	return challenger.Score > best.Score || (challenger.Score == best.Score && len(challenger.MatchedSkills) > len(best.MatchedSkills))
}

// Score rates a job against every persona (oldest first) and returns the best result: the highest score, ties to
// the persona with more matched skills, then to the earlier one. It fails with ErrNoPersonas for none, and with an
// InsufficientDataError when no persona has skills or a title to judge.
func Score(job Job, personas []Persona, corpus Corpus, now time.Time) (Result, error) {
	if len(personas) == 0 {
		return Result{}, ErrNoPersonas
	}
	results, reasons := scoreEach(prepareJob(job), personas, corpus, now)
	if len(results) == 0 {
		return Result{}, &InsufficientDataError{Reasons: dedupe(reasons)}
	}
	best := results[0]
	for _, r := range results[1:] {
		if betterThan(r, best) {
			best = r
		}
	}
	return best, nil
}

// ScoreEach rates a job against every persona on its own and returns each result, in the order the personas were given
// (step 141). A persona that cannot be judged (neither skills nor a title to compare) has no entry, so its absence means
// "not enough data", never a score of 0. No persona, or none that can be judged, gives an empty list and no error. The
// best persona of these results is the one Score returns.
func ScoreEach(job Job, personas []Persona, corpus Corpus, now time.Time) []Result {
	results, _ := scoreEach(prepareJob(job), personas, corpus, now)
	return results
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Describe is a one-line summary of a result for logs and tests.
func (r Result) Describe() string {
	return fmt.Sprintf("%d (%s, coverage %.2f, %d skills)", r.Score, r.Mode, r.Coverage, len(r.MatchedSkills))
}
