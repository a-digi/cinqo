package jobmatcher

import "math"

// Corpus is the saved jobs' texts, tokenized once. It tells how common a skill or a word is across your jobs, so a
// rare one counts more than "MS Office". Build it from the jobs' title and description.
type Corpus struct {
	docs   [][]string
	wordDF map[string]int
}

// NewCorpus tokenizes the given job texts.
func NewCorpus(texts []string) Corpus {
	c := Corpus{wordDF: map[string]int{}}
	for _, t := range texts {
		toks := tokenize(t)
		c.docs = append(c.docs, toks)
		seen := map[string]bool{}
		for _, w := range toks {
			if !seen[w] {
				seen[w] = true
				c.wordDF[w]++
			}
		}
	}
	return c
}

// N is the number of jobs in the corpus.
func (c Corpus) N() int { return len(c.docs) }

// phraseDF is the number of jobs that contain the phrase.
func (c Corpus) phraseDF(phrase []string) int {
	n := 0
	for _, d := range c.docs {
		if containsPhrase(d, phrase) {
			n++
		}
	}
	return n
}

// small reports whether the corpus is too small to say what is common: every weight is then neutral.
func (c Corpus) small() bool { return c.N() < CorpusMinJobs }

// skillWeight is the weight of one skill hit, from SkillWeightMin (in every job) to SkillWeightMin+1 (in none).
func (c Corpus) skillWeight(phrase []string) float64 {
	if c.small() {
		return 1
	}
	n := float64(c.N())
	rare := math.Log((n+1)/float64(c.phraseDF(phrase)+1)) / math.Log(n+1)
	return SkillWeightMin + math.Min(1, math.Max(0, rare))
}

// idf is the distinctiveness of a word: ln((N+1)/(df+1)); 1 for a small corpus.
func (c Corpus) idf(word string) float64 {
	if c.small() {
		return 1
	}
	return math.Log((float64(c.N()) + 1) / float64(c.wordDF[word]+1))
}
