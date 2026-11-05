package jobmatcher

import (
	"sort"
	"strings"
	"unicode"
)

type scoredWord struct {
	word  string
	score float64
}

// distinctiveWords are the words that set the job apart, best first (ties alphabetical): frequent in the job (the
// title counts double), rare in your saved jobs. Only words that read like a term are candidates: capitalised in
// the middle of a sentence (German nouns, product and skill names), in the title, or mixing letters and digits; a
// skill is often named once, so one mention is enough. Lower-case words (verb forms, adverbs, English fillers),
// words that only start sentences, boilerplate (fillerWords) and the words of the job's own place are left out.
func distinctiveWords(pj preparedJob, c Corpus) []scoredWord {
	tf := map[string]float64{}
	inTitle := map[string]bool{}
	for _, w := range pj.titleTokens {
		tf[w] += 2
		inTitle[w] = true
	}
	for _, w := range pj.descTokens {
		tf[w]++
	}
	capital := capitalisedMidSentence(pj.job.Title, pj.job.Description)
	place := map[string]bool{}
	for _, w := range cityWords(pj.job.Location) {
		place[w] = true
	}
	var out []scoredWord
	for w, n := range tf {
		if (len(w) < 3 && !hasLetterAndDigit(w)) || stopWords[w] || fillerWords[w] || place[w] || isNumber(w) {
			continue
		}
		if !capital[w] && !inTitle[w] && !hasLetterAndDigit(w) {
			continue
		}
		out = append(out, scoredWord{w, n * c.idf(w)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].word < out[j].word
	})
	return out
}

func hasLetterAndDigit(s string) bool {
	letter, digit := false, false
	for _, r := range s {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	return letter && digit
}

// capitalisedMidSentence marks the words that read like a term: written with a capital letter somewhere other than the
// start of a sentence or line (in German a noun, in any language a name), or always capitalised and repeated (a name
// that only ever opens sentences). Keys are folded single words, as tokenize gives them.
func capitalisedMidSentence(texts ...string) map[string]bool {
	mid := map[string]bool{}
	capital := map[string]int{}
	lower := map[string]int{}
	for _, t := range texts {
		sentenceStart := true
		runes := []rune(t)
		for i := 0; i < len(runes); {
			r := runes[i]
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				j := i
				for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j])) {
					j++
				}
				if toks := tokenize(string(runes[i:j])); len(toks) == 1 {
					w := toks[0]
					if unicode.IsUpper(r) {
						capital[w]++
						if !sentenceStart {
							mid[w] = true
						}
					} else {
						lower[w]++
					}
				}
				sentenceStart = false
				i = j
				continue
			}
			if r == '.' || r == '!' || r == '?' || r == ':' || r == '\n' || r == '•' {
				sentenceStart = true
			}
			i++
		}
	}
	m := map[string]bool{}
	for w := range mid {
		m[w] = true
	}
	for w, n := range capital {
		if n >= 2 && lower[w] == 0 {
			m[w] = true
		}
	}
	return m
}

func isNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

// surfaces map each folded word of a text to the way it was first written there, so a suggestion reads
// "Kubernetes" and not "kubernetes".
func surfaces(texts ...string) map[string]string {
	m := map[string]string{}
	for _, t := range texts {
		for _, w := range strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if toks := tokenize(w); len(toks) == 1 {
				if _, ok := m[toks[0]]; !ok {
					m[toks[0]] = w
				}
			}
		}
	}
	return m
}

// gapWords are the job's distinctive words that nothing the persona offers mentions (text mode).
func gapWords(pj preparedJob, ev evidence, c Corpus) []string {
	have := append(append(append([]string(nil), ev.skillTokens...), ev.expTokens...), ev.otherTokens...)
	surf := surfaces(pj.job.Title, pj.job.Description)
	var out []string
	for _, d := range distinctiveWords(pj, c) {
		if bestSim(d.word, have) >= 1 {
			continue
		}
		out = append(out, surfaceOf(surf, d.word))
		if len(out) == MaxGaps {
			break
		}
	}
	return out
}

func surfaceOf(surf map[string]string, word string) string {
	if s, ok := surf[word]; ok {
		return s
	}
	return word
}

// Suggest offers up to MaxSuggestions of the job's most distinctive words as candidate requirements, none of them
// a word an existing requirement already uses. It is a word count against your saved jobs, not an AI call.
func Suggest(job Job, c Corpus) []string {
	pj := prepareJob(job)
	used := map[string]bool{}
	for _, r := range pj.reqs {
		for _, t := range r.tokens {
			used[t] = true
		}
	}
	surf := surfaces(job.Title, job.Description)
	out := []string{}
	for _, d := range distinctiveWords(pj, c) {
		if used[d.word] {
			continue
		}
		out = append(out, surfaceOf(surf, d.word))
		if len(out) == MaxSuggestions {
			break
		}
	}
	return out
}
