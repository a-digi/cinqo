package jobmatcher

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// skillEntry is one persona skill with its tokens.
type skillEntry struct {
	name   string
	phrase []string
}

// evidence is everything a persona offers as proof, in the three strengths of requirements mode.
type evidence struct {
	skills      []skillEntry // distinct skills
	skillTokens []string
	expTokens   []string
	otherTokens []string // headline, summary, desired titles
	any         bool
}

func buildEvidence(p Persona) evidence {
	var ev evidence
	seen := map[string]bool{}
	for _, s := range p.Skills {
		ph := tokenize(s)
		key := strings.Join(ph, " ")
		if len(ph) == 0 || seen[key] {
			continue
		}
		seen[key] = true
		ev.skills = append(ev.skills, skillEntry{name: strings.TrimSpace(s), phrase: ph})
		ev.skillTokens = append(ev.skillTokens, ph...)
	}
	for _, e := range p.Experience {
		ev.expTokens = append(ev.expTokens, tokenize(e.Title+" "+e.Description)...)
	}
	ev.otherTokens = tokenize(p.Headline + " " + p.Summary + " " + p.DesiredTitles)
	ev.any = len(ev.skills) > 0 || len(ev.expTokens) > 0 || len(ev.otherTokens) > 0
	return ev
}

// reqTokens is a cleaned requirement with its tokens.
type reqTokens struct {
	Requirement
	tokens []string
}

// skillsOutcome is what the Skills component hands back.
type skillsOutcome struct {
	mode      string
	value     float64
	evaluable bool
	note      string
	matched   []string
	missing   []Missing
	// hits are the token phrases of the skills the job's text mentions (text mode), used to judge experience.
	hits [][]string
	// reqs are the cleaned requirements (requirements mode), used to judge experience.
	reqs []reqTokens
}

func cleanRequirements(in []Requirement) []reqTokens {
	var out []reqTokens
	seen := map[string]bool{}
	for _, r := range in {
		term := strings.TrimSpace(r.Term)
		toks := tokenize(term)
		key := strings.Join(toks, " ")
		if term == "" || len(toks) == 0 || r.Tier < 1 || r.Tier > 3 || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, reqTokens{Requirement: Requirement{Term: term, Tier: r.Tier}, tokens: toks})
		if len(out) == MaxRequirements {
			break
		}
	}
	return out
}

// coveredBy reports whether a token stream covers a requirement: a short requirement as a whole phrase, a long
// one when at least CoverThreshold of its significant words are found (word forms matched).
func coveredBy(req []string, tokens []string) bool {
	if len(req) < LongRequirement {
		return containsPhrase(tokens, req)
	}
	return coverage(req, tokens) >= CoverThreshold
}

func coverage(req, tokens []string) float64 {
	sig := significant(req)
	if len(sig) == 0 {
		sig = req
	}
	if len(tokens) == 0 {
		return 0
	}
	sum := 0.0
	for _, w := range sig {
		sum += bestSim(w, tokens)
	}
	return sum / float64(len(sig))
}

// requirementStrength is how well the persona covers one requirement (EvidenceSkill, EvidenceExperience,
// EvidenceOther or 0) and the skills that covered it.
func requirementStrength(r reqTokens, ev evidence) (float64, []string) {
	var used []string
	for _, s := range ev.skills {
		if containsPhrase(r.tokens, s.phrase) || containsPhrase(s.phrase, r.tokens) {
			used = append(used, s.name)
		}
	}
	if len(r.tokens) < LongRequirement {
		if len(used) > 0 {
			return EvidenceSkill, used
		}
	} else if coverage(r.tokens, ev.skillTokens) >= CoverThreshold {
		return EvidenceSkill, used
	}
	if coveredBy(r.tokens, ev.expTokens) {
		return EvidenceExperience, nil
	}
	if coveredBy(r.tokens, ev.otherTokens) {
		return EvidenceOther, nil
	}
	return 0, nil
}

// requirementsSkills is mode 1: the share of the demanded weight the persona covers.
func requirementsSkills(reqs []reqTokens, ev evidence) skillsOutcome {
	out := skillsOutcome{mode: ModeRequirements, reqs: reqs}
	if !ev.any {
		out.note = "this target role has no skills, experience, headline or summary to check the requirements against"
		return out
	}
	var earned, total float64
	var matched []string
	covered := 0
	for _, r := range reqs {
		w := float64(r.Tier)
		total += w
		strength, used := requirementStrength(r, ev)
		earned += w * strength
		matched = append(matched, used...)
		if strength > 0 {
			covered++
		} else if r.Tier >= 2 {
			out.missing = append(out.missing, Missing{Term: r.Term, Tier: r.Tier})
		}
	}
	sort.SliceStable(out.missing, func(i, j int) bool {
		if out.missing[i].Tier != out.missing[j].Tier {
			return out.missing[i].Tier > out.missing[j].Tier
		}
		return strings.ToLower(out.missing[i].Term) < strings.ToLower(out.missing[j].Term)
	})
	out.evaluable = true
	out.value = earned / total
	out.matched = uniqueSorted(matched, lessFold)
	out.note = fmt.Sprintf("scored from the job's requirements: %d of %d covered", covered, len(reqs))
	return out
}

// textSkills is mode 2: the persona's skills found in the job's title and description, rarer ones counting more,
// on a saturating curve.
func textSkills(pj preparedJob, ev evidence, c Corpus) skillsOutcome {
	out := skillsOutcome{mode: ModeText}
	switch {
	case len(ev.skills) == 0:
		out.note = "this target role has no skills"
		return out
	case len(pj.descTokens) == 0:
		out.note = "this job has no description and no requirements yet"
		return out
	}
	var sum float64
	var matched []string
	for _, s := range ev.skills {
		inTitle := containsPhrase(pj.titleTokens, s.phrase)
		if !inTitle && !containsPhrase(pj.descTokens, s.phrase) {
			continue
		}
		w := c.skillWeight(s.phrase)
		if inTitle {
			w = math.Min(SkillWeightMax, w+SkillTitleBonus)
		}
		sum += w
		matched = append(matched, s.name)
		out.hits = append(out.hits, s.phrase)
	}
	out.evaluable = true
	out.value = 1 - math.Exp(-sum/SkillSaturation)
	out.matched = uniqueSorted(matched, lessFold)
	out.note = fmt.Sprintf("scored from the job text: %d of %d skills found", len(out.matched), len(ev.skills))
	return out
}

func lessFold(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }

func sortStrings(s []string, less func(a, b string) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}
