// job_match_algorithm.go implements the deterministic ("Match now",
// no AI) counterpart to the AI's own judgment call when scoring how
// well a job fits a persona — pure Go, no I/O, no external service.
// Weighted, case-insensitive whole-word/phrase keyword matching, not
// a naive substring search: a skill found in the job's own TITLE
// counts for more than one only found in its description, since a
// title mention is a stronger relevance signal. See
// plan/ai/tools/career/step-XX-deterministic-job-match.md.
package main

import (
	"regexp"
	"strings"
	"unicode"
)

// jobMatchTitleWeight/jobMatchRequirementsWeight/jobMatchDescriptionWeight
// are the per-skill weights computeJobMatch applies depending on WHERE
// a skill was found — title, a detected requirements/qualifications
// zone (splitByRequirementsZone, below), or the rest of the
// description, in descending order of how strong a relevance signal
// each location is. Plain constants, not configurable — matching this
// codebase's own "constants over premature configurability"
// convention.
//
// jobMatchSemanticWeight/jobMatchSemanticThreshold (step XX) are the
// same idea for a skill that ISN'T literally present anywhere in the
// job's text but whose meaning is close to the job description's own
// meaning — see semantic_vectors.go. Weighted below
// jobMatchDescriptionWeight deliberately: a semantic match is a
// fuzzier, lower-confidence signal than an actual literal mention, so
// it should never outweigh one. All are initial estimates, not derived
// from any real tuning data — expect to revisit once there's enough
// real match history to compare against.
//
// jobMatchSemanticThreshold was lowered from an initial 0.55 to 0.4 —
// still a guess, not a tuned number (no real match history existed to
// tune it against either time) — because 0.55 is a demanding bar for
// cosine similarity between two SHORT, averaged bag-of-words phrase
// vectors (a multi-word skill vs. one sentence): even a clearly related
// pair (e.g. "Kubernetes" vs. a sentence about container orchestration)
// can score well under that in practice, since averaging just a few
// words is a much noisier signal than averaging a large corpus of
// text. 0.4 is a deliberately more permissive starting point for the
// sentence-level comparison introduced alongside this change,
// expected to need further real-world adjustment in either direction.
//
// jobMatchWeightCap (step XX) replaced BOTH dividing by the raw
// len(skills) AND a flat, persona-size-independent ceiling as
// computeJobMatch's own score denominator — each of those two earlier
// attempts traded one failure mode for its opposite:
//
//   - Dividing by len(skills) directly meant a persona with a broad,
//     diverse skill set could never score highly against any single
//     job, no matter how precisely their relevant skills matched,
//     purely because their OTHER, irrelevant skills silently counted
//     against the denominator (the AI scored a real case 94, this
//     algorithm scored it 15).
//   - A flat ceiling (an earlier version of this constant, 3.5,
//     independent of the persona's own skill count) overcorrected:
//     ordinary tech job postings routinely contain 5+ literal
//     mentions of common skill terms ("Git," "Agile," "API,"
//     "Testing"), so almost ANY halfway-relevant persona/job pair blew
//     past a small fixed number and clamped to 100% — a real,
//     observed regression ("100% everywhere"), not just miscalibration.
//
// The denominator here is min(len(skills), jobMatchWeightCap) instead
// — scales with the persona's own skill count (so a persona with only
// 3 tightly-relevant skills can still reach 100% off those 3 alone,
// unlike a flat ceiling) but never exceeds jobMatchWeightCap (so a
// persona with 20 skills isn't required to match anywhere near all of
// them, unlike dividing by the raw count). 5 is, again, a reasoned
// estimate, not a value tuned against real match data — this is the
// third iteration of this constant, and further adjustment in either
// direction should be expected once more real cases are checked
// against it.
const (
	jobMatchTitleWeight        = 1.0
	jobMatchRequirementsWeight = 0.85
	jobMatchDescriptionWeight  = 0.6
	jobMatchSemanticWeight     = 0.35
	jobMatchSemanticThreshold  = 0.4
	jobMatchWeightCap          = 5.0
)

// skillMatch is one matched skill plus HOW it was matched — persisted
// verbatim via job_match_skills.match_kind (db.go) and surfaced to the
// frontend so a user can actually tell a semantic match from a literal
// one, instead of the two being indistinguishable in the UI. See
// plan/ai/tools/career/step-XX-semantic-match-observability.md.
type skillMatch struct {
	Skill string `json:"skill"`
	// Kind is "literal" (an exact keyword/phrase hit) or "semantic" (no
	// literal hit, but the skill's own meaning was close enough to some
	// sentence of the job description per the active model).
	Kind string `json:"kind"`
}

const (
	matchKindLiteral  = "literal"
	matchKindSemantic = "semantic"
)

// computeJobMatch scores how well a persona's own skills match a
// job's own title+description. Each skill is checked for a
// case-insensitive, whole-word/phrase match against title, a detected
// requirements/qualifications zone, and the rest of the description,
// in that priority order (a skill present in more than one only ever
// counts once, at the highest-weighted location it was found);
// matchedSkills is every skill found anywhere, in the SAME order as
// skills itself (never reordered), so a caller can rely on it being a
// stable subsequence. score is round(100 * min(1, sum(weights) /
// min(len(skills), jobMatchWeightCap))) — see that constant's own doc
// comment for why the denominator is capped rather than either the
// persona's raw skill count or a flat number. A persona with no skills
// at all scores 0 with no matched skills.
//
// vectors (step XX) is the currently active semantic model's own word
// vectors (semantic_vectors.go's acquireActiveVectors), or nil if no
// model is active/loaded — nil disables the semantic step entirely,
// reproducing this function's own pre-existing literal-only behavior
// exactly, so a deployment that never downloads a model is unaffected.
//
// When non-nil, a skill that doesn't literally match anywhere falls
// back to a semantic check — but NOT by averaging the whole
// description into one vector (an earlier, weaker version of this
// function did exactly that, and diluted any one specific concept into
// the text's overall generic "gist" badly enough that the semantic
// step rarely fired in practice). Instead the requirements zone (or,
// if none was detected, the whole description) is split into sentences
// (splitIntoSentences, semantic_vectors.go) and the skill is compared
// against EACH one, keeping the best (maximum) similarity — a skill
// mentioned or implied in one specific sentence is compared against
// that sentence alone, not smeared across the whole posting, and
// restricted to the requirements zone when one exists so "About us"/
// benefits prose can't contribute a spurious semantic hit either.
func computeJobMatch(jobTitle, jobDescription string, skills []string, vectors map[string][]float32) (score int, matchedSkills []skillMatch) {
	matchedSkills = []skillMatch{}
	if len(skills) == 0 {
		return 0, matchedSkills
	}

	requirementsText, hasRequirementsZone := splitByRequirementsZone(jobDescription)

	var sentenceVectors [][]float32
	if vectors != nil {
		semanticSource := jobDescription
		if hasRequirementsZone {
			semanticSource = requirementsText
		}
		for _, sentence := range splitIntoSentences(semanticSource) {
			if v, ok := phraseVector(sentence, vectors); ok {
				sentenceVectors = append(sentenceVectors, v)
			}
		}
	}

	var totalWeight float64
	for _, skill := range skills {
		re := skillMatchPattern(skill)
		if re == nil {
			continue
		}
		switch {
		case re.MatchString(jobTitle):
			totalWeight += jobMatchTitleWeight
			matchedSkills = append(matchedSkills, skillMatch{Skill: skill, Kind: matchKindLiteral})
		case hasRequirementsZone && re.MatchString(requirementsText):
			totalWeight += jobMatchRequirementsWeight
			matchedSkills = append(matchedSkills, skillMatch{Skill: skill, Kind: matchKindLiteral})
		case re.MatchString(jobDescription):
			totalWeight += jobMatchDescriptionWeight
			matchedSkills = append(matchedSkills, skillMatch{Skill: skill, Kind: matchKindLiteral})
		case len(sentenceVectors) > 0:
			skillVector, ok := phraseVector(skill, vectors)
			if !ok {
				continue
			}
			var best float64
			for _, sv := range sentenceVectors {
				if sim := cosineSimilarity(skillVector, sv); sim > best {
					best = sim
				}
			}
			if best >= jobMatchSemanticThreshold {
				totalWeight += jobMatchSemanticWeight
				matchedSkills = append(matchedSkills, skillMatch{Skill: skill, Kind: matchKindSemantic})
			}
		}
	}

	denominator := float64(len(skills))
	if denominator > jobMatchWeightCap {
		denominator = jobMatchWeightCap
	}
	if totalWeight > denominator {
		totalWeight = denominator
	}
	raw := 100*totalWeight/denominator + 0.5 // round to nearest integer
	score = int(raw)
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score, matchedSkills
}

// skillMatchPattern compiles a case-insensitive, whole-word/phrase
// regex for one skill string — the skill's own text is escaped via
// regexp.QuoteMeta first, so a skill containing regex metacharacters
// (e.g. "C++", "C#", "ASP.NET") is matched as its own literal text,
// never misinterpreted as a pattern.
//
// A plain `\b...\b` wrapper would be wrong here: Go's RE2 engine has
// no lookaround, and \b only ever asserts a transition between a word
// character (letter/digit/underscore) and a non-word one — a skill
// that ITSELF starts or ends on a non-word character (e.g. "C++",
// ending in '+', already non-word) can never have a real \w-to-\W
// transition there, so an unconditional trailing \b would simply
// never match. \b is added on each side only when that side's own
// edge character is itself a word character — the other side's own
// literal (non-word) character already prevents it from matching
// as a mere substring of a longer token, without needing an assertion
// RE2 can't express anyway.
func skillMatchPattern(skill string) *regexp.Regexp {
	trimmed := strings.TrimSpace(skill)
	if trimmed == "" {
		return nil
	}
	runes := []rune(trimmed)
	pattern := regexp.QuoteMeta(trimmed)
	if isWordRune(runes[0]) {
		pattern = `\b` + pattern
	}
	if isWordRune(runes[len(runes)-1]) {
		pattern += `\b`
	}
	re, err := regexp.Compile(`(?i)` + pattern)
	if err != nil {
		// Unreachable in practice — QuoteMeta's own output is always a
		// valid literal pattern — but never panic over a malformed
		// skill string a persona happens to have typed in.
		return nil
	}
	return re
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
