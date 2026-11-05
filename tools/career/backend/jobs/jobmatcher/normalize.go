package jobmatcher

import (
	"regexp"
	"strings"
	"unicode"
)

var foldReplacer = strings.NewReplacer(
	// invisible characters some sites put inside words ("En­gineer" with a soft hyphen) are removed, so the word stays whole
	"\u00ad", "", "\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "",
	"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	"à", "a", "á", "a", "â", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ç", "c", "ñ", "n",
)

// fold lower-cases a text and folds German umlauts and ß to ae oe ue ss (and other accents to their letter), so
// "Pädagoge" and "Paedagoge" are the same word on both sides.
func fold(s string) string { return foldReplacer.Replace(strings.ToLower(s)) }

// canonRules turn names with symbols into plain words before the text is split, so they survive as one token:
// c++ becomes cpp, c# becomes csharp, .net becomes dotnet, node.js becomes nodejs, ci/cd becomes cicd.
var canonRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(^|[^a-z0-9])c\+\+`), "${1}cpp"},
	{regexp.MustCompile(`(^|[^a-z0-9])c#`), "${1}csharp"},
	{regexp.MustCompile(`(^|[^a-z0-9])f#`), "${1}fsharp"},
	{regexp.MustCompile(`(^|[^a-z0-9])asp\.net\b`), "${1}aspnet"},
	{regexp.MustCompile(`(^|[^a-z0-9])\.net\b`), "${1}dotnet"},
	{regexp.MustCompile(`\bnode\.js\b`), "nodejs"},
	{regexp.MustCompile(`\bvue\.js\b`), "vue"},
	{regexp.MustCompile(`\breact\.js\b`), "react"},
	{regexp.MustCompile(`\bnext\.js\b`), "nextjs"},
	{regexp.MustCompile(`\bci\s*/\s*cd\b`), "cicd"},
}

func canon(s string) string {
	for _, r := range canonRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// tokenize splits a text into folded words with the aliases applied. Letters and digits make a word; everything
// else separates. "C/C++" gives c and cpp, "Node.js" gives nodejs.
func tokenize(s string) []string {
	s = canon(fold(s))
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return applyAliases(words)
}

// tokenizeFolded splits an already folded and canonical text into words (no aliases applied).
func tokenizeFolded(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// applyAliases rewrites known spellings (js, k8s, ms office ...) to one form, the longest match first.
func applyAliases(words []string) []string {
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); {
		matched := false
		for n := maxAliasWords; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			if to, ok := aliases[strings.Join(words[i:i+n], " ")]; ok {
				out = append(out, strings.Fields(to)...)
				i += n
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, words[i])
			i++
		}
	}
	return out
}

// significant drops stop words and one-letter words.
func significant(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) >= 2 && !stopWords[t] {
			out = append(out, t)
		}
	}
	return out
}

// containsPhrase reports whether phrase occurs in tokens as consecutive whole words.
func containsPhrase(tokens, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(tokens) {
		return false
	}
	for i := 0; i+len(phrase) <= len(tokens); i++ {
		ok := true
		for j, p := range phrase {
			if tokens[i+j] != p {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// wordSim is how alike two folded words are: 1 when equal or one is a form of the other (a prefix of at least 5
// characters, at most 3 characters shorter: Erzieher / Erzieherin, developer / developers); StemMatch when they
// share a stem and differ in their endings (Erzieherin / Erziehern, Entwickler / Entwicklung); PartialWordMatch
// when a long German compound ends with the other word (Heilerzieher ends with Erzieher, also Erzieherin).
func wordSim(a, b string) float64 {
	if a == b {
		return 1
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(a) >= 5 && strings.HasPrefix(b, a) && len(b)-len(a) <= 3 {
		return 1
	}
	if len(a) >= 5 {
		if cp := commonPrefix(a, b); cp >= 5 && cp < len(a) && len(a)-cp <= 3 && len(b)-cp <= 3 {
			return StemMatch
		}
	}
	if endsWithCompound(b, a) {
		return PartialWordMatch
	}
	return 0
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// germanEndings are what a feminine or plural form adds, stripped when looking for a compound's last part.
var germanEndings = []string{"innen", "in", "en", "n", "s"}

func endingVariants(w string) []string {
	out := []string{w}
	for _, e := range germanEndings {
		if strings.HasSuffix(w, e) && len(w)-len(e) >= 6 {
			out = append(out, strings.TrimSuffix(w, e))
		}
	}
	return out
}

// endsWithCompound reports whether the long word ends with the short one (at least 6 characters), endings aside:
// heilerzieher ends with erzieherin's erzieher.
func endsWithCompound(long, short string) bool {
	for _, sv := range endingVariants(short) {
		if len(sv) < 6 {
			continue
		}
		for _, lv := range endingVariants(long) {
			if lv != sv && strings.HasSuffix(lv, sv) {
				return true
			}
		}
	}
	return false
}

// dice is the Dice coefficient of two word lists, matched greedily with wordSim: 2 * matched / (|a| + |b|).
func dice(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	used := make([]bool, len(b))
	sum := 0.0
	for _, x := range a {
		best, at := 0.0, -1
		for j, y := range b {
			if used[j] {
				continue
			}
			if s := wordSim(x, y); s > best {
				best, at = s, j
			}
		}
		if at >= 0 {
			used[at] = true
			sum += best
		}
	}
	return 2 * sum / float64(len(a)+len(b))
}

// bestSim is the best wordSim of a word against a list of words.
func bestSim(word string, tokens []string) float64 {
	best := 0.0
	for _, t := range tokens {
		if s := wordSim(word, t); s > best {
			best = s
			if best == 1 {
				break
			}
		}
	}
	return best
}

func uniqueSorted(in []string, less func(a, b string) bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToLower(s)
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	sortStrings(out, less)
	return out
}
