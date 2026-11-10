package jobmatcher

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reGenderWords = regexp.MustCompile(`\((?:gn|all genders?|gender)\)`)
	reContract    = regexp.MustCompile(`\b(?:voll-?\s*(?:und|oder|/)\s*teilzeit|vollzeit|teilzeit|unbefristet|befristet|festanstellung|full[- ]?time|part[- ]?time|permanent)\b`)
	reAlternative = regexp.MustCompile(`\s*(?:/|,|;|\|)\s*|\s+(?:oder|or)\s+`)
	reTitleList   = regexp.MustCompile(`[,;/|\n]`)
)

// cleanTitle folds a title and removes what says nothing about the job: spelled-out gender markers ("(all
// genders)") and contract words (Vollzeit, unbefristet). The short markers, (m/w/d) and erzieher/in, need no
// removal: their letters and "in" / "innen" are dropped as stop words and one-letter words when the title is split.
func cleanTitle(s string) string {
	s = canon(fold(s))
	s = reGenderWords.ReplaceAllString(s, " ")
	s = reContract.ReplaceAllString(s, " ")
	return s
}

// titleWords are a title's significant words, aliases applied.
func titleWords(s string) []string {
	return significant(applyAliases(tokenizeFolded(cleanTitle(s))))
}

// titleAlternatives cuts a title into the roles it names ("Erzieher / Heilerzieher oder Sozialpädagoge" is three)
// and returns each one's significant words.
func titleAlternatives(title string) [][]string {
	var out [][]string
	for _, part := range reAlternative.Split(cleanTitle(title), -1) {
		if w := significant(applyAliases(tokenizeFolded(part))); len(w) > 0 {
			out = append(out, w)
		}
	}
	return out
}

type titleCandidate struct {
	label  string
	source string
	weight float64
	words  []string
}

// titleCandidates are the persona's titles with how much each counts: desired titles 1.0, the headline 0.85, the
// titles of past jobs 0.75.
func titleCandidates(p Persona) []titleCandidate {
	var out []titleCandidate
	for _, t := range reTitleList.Split(p.DesiredTitles, -1) {
		if w := titleWords(t); len(w) > 0 {
			out = append(out, titleCandidate{strings.TrimSpace(t), "desired title", TitleWeightDesired, w})
		}
	}
	if w := titleWords(p.Headline); len(w) > 0 {
		out = append(out, titleCandidate{strings.TrimSpace(p.Headline), "headline", TitleWeightHeadline, w})
	}
	for _, e := range p.Experience {
		if w := titleWords(e.Title); len(w) > 0 {
			out = append(out, titleCandidate{strings.TrimSpace(e.Title), "past job title", TitleWeightExperience, w})
		}
	}
	return out
}

// titleSim is how alike two titles' word lists are: the Dice coefficient, or ContainedTitleSim when one of at least
// two words is entirely found in the other (the job adds words to the title the persona wants).
func titleSim(job, candidate []string) float64 {
	sim := dice(job, candidate)
	if len(candidate) >= 2 {
		all := true
		for _, w := range candidate {
			if bestSim(w, job) < 1 {
				all = false
				break
			}
		}
		if all && ContainedTitleSim > sim {
			sim = ContainedTitleSim
		}
	}
	return sim
}

// titleFit is the best weighted similarity between the job's title (each role it names) and the persona's titles.
func titleFit(job Job, p Persona) (value float64, evaluable bool, note string) {
	alts := titleAlternatives(job.Title)
	cands := titleCandidates(p)
	switch {
	case len(alts) == 0:
		return 0, false, "this job's title has no usable words"
	case len(cands) == 0:
		return 0, false, "this target role has no desired title, headline or past job title"
	}
	best := 0.0
	var bestCand titleCandidate
	for _, c := range cands {
		for _, a := range alts {
			if v := c.weight * titleSim(a, c.words); v > best {
				best, bestCand = v, c
			}
		}
	}
	if best == 0 {
		return 0, true, "no title of this target role resembles the job's title"
	}
	return best, true, fmt.Sprintf("closest: %q (%s)", bestCand.label, bestCand.source)
}
