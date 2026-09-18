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

// jobMatchTitleWeight/jobMatchDescriptionWeight are the per-skill
// weights computeJobMatch applies depending on WHERE a skill was
// found. Plain constants, not configurable — matching this codebase's
// own "constants over premature configurability" convention.
const (
	jobMatchTitleWeight       = 1.0
	jobMatchDescriptionWeight = 0.6
)

// computeJobMatch scores how well a persona's own skills match a
// job's own title+description. Each skill is checked for a
// case-insensitive, whole-word/phrase match against title and
// description separately (title first — a skill present in both only
// ever counts once, at the higher weight); matchedSkills is every
// skill found in either, in the SAME order as skills itself (never
// reordered), so a caller can rely on it being a stable subsequence.
// score is round(100 * sum(weights) / len(skills)), clamped 0-100. A
// persona with no skills at all scores 0 with no matched skills
// (never divides by zero).
func computeJobMatch(jobTitle, jobDescription string, skills []string) (score int, matchedSkills []string) {
	matchedSkills = []string{}
	if len(skills) == 0 {
		return 0, matchedSkills
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
			matchedSkills = append(matchedSkills, skill)
		case re.MatchString(jobDescription):
			totalWeight += jobMatchDescriptionWeight
			matchedSkills = append(matchedSkills, skill)
		}
	}

	raw := 100*totalWeight/float64(len(skills)) + 0.5 // round to nearest integer
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
