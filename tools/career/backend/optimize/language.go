package optimize

import (
	"regexp"
	"strings"
)

// The language of a text, told apart by how many of its words are common function words of a language (plan 258, step 6). The suggestions of the AI must be in the language of the
// text they replace: a German text gets a German suggestion, never an English one. There is no library and no network: a text is "placed" in a language only when it is long enough
// and one language's words clearly dominate; anything else (a list of technologies, a name, a short title, a language that is not in the list) is NOT placed, and then nothing is refused.
// Only a CLEAR change of language is refused.
//
// The lists below are the same as the page's own (frontend-app/src/Optimize/language.ts): the TypeScript tests read this file and fail when the two drift apart, and both test suites
// place the sentences of testdata/language_fixtures.json.

// The words of each language: lower case, only words that are rare in the other languages of the list.
var languageWords = map[string]string{
	"de": "der die das und ist nicht mit für von zu den dem des ein eine einer einen eines auf im am bei auch wir ich sie es sich wurde wurden haben hat werden als nach über unter durch aus oder aber wie nur noch schon sehr zum zur sowie wodurch dass wird sind war waren seit bis",
	"en": "the and to that for with on by at from this were are be been has have had it its or which their our we they who not also more you your into over using built led developed than these those through while across",
	"fr": "les des du une et est que qui pour dans par sur avec aux cette ces sont été être nous vous ils elles pas mais comme sans sous chez entre depuis très aussi leur leurs ses au ont avons avez où pendant plusieurs ce qu",
	"it": "di il gli del della dello dei delle nel nella nei è che per con sono stato stata essere hanno non più come anche ma tra fra dal dalla alla alle questo questa suo sua loro uno una ho ha ed",
	"es": "el los las del y es que por para con una se su sus al como más pero sin sobre entre también fue han son está este esta muy hemos desde hasta ser he ha lo en un unos unas",
	"nl": "het een van dat op voor met zijn werd werden ook als bij door naar aan uit om niet maar of nog wel heeft hebben deze zich over tot onder tussen meer ik ben geweest waardoor",
	"sq": "dhe për një është që nga në të janë kam ka si edhe por ose nuk ky kjo këtë këto gjithashtu me së tek ndaj mes pas para gjatë qenë ekipin klientët",
}

// LanguageNames is how a language is called in the sentence the AI is told.
var languageNames = map[string]string{"de": "German", "en": "English", "fr": "French", "it": "Italian", "es": "Spanish", "nl": "Dutch", "sq": "Albanian"}

// The thresholds of "clearly this language": at least minWords words, at least minHits of them from the language's list, at least minShare of all words, and at least
// dominance times as many hits as the next language.
const (
	minWords  = 8
	minHits   = 3
	minShare  = 0.15
	dominance = 1.5
)

var wordSets = buildWordSets()

func buildWordSets() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for lang, words := range languageWords {
		set := map[string]bool{}
		for _, w := range strings.Fields(words) {
			set[w] = true
		}
		out[lang] = set
	}
	return out
}

var letters = regexp.MustCompile(`\p{L}+`)

// detectLanguage places a text in one of the languages above, or says it cannot (ok false).
func detectLanguage(text string) (lang string, ok bool) {
	words := letters.FindAllString(strings.ToLower(text), -1)
	if len(words) < minWords {
		return "", false
	}
	hits := map[string]int{}
	for _, w := range words {
		for l, set := range wordSets {
			if set[w] {
				hits[l]++
			}
		}
	}
	best, bestHits, second := "", 0, 0
	for l, n := range hits {
		switch {
		case n > bestHits:
			second = bestHits
			best, bestHits = l, n
		case n > second:
			second = n
		}
	}
	if bestHits < minHits || float64(bestHits) < minShare*float64(len(words)) || float64(bestHits) < dominance*float64(second) {
		return "", false
	}
	return best, true
}

// checkLanguage refuses a suggestion that is clearly in another language than the text it replaces, and a reason that is clearly in another language than the text (plan 258, step 6). It
// refuses only when both sides are placed (see detectLanguage): a text or a suggestion that cannot be placed is never refused.
func checkLanguage(before, after, reason string) error {
	want, ok := detectLanguage(before)
	if !ok {
		return nil
	}
	if got, ok := detectLanguage(after); ok && got != want {
		return invalid("the text is %s but your suggestion is %s: write the improved text in %s, the language of the text, and never translate it", languageNames[want], languageNames[got], languageNames[want])
	}
	if got, ok := detectLanguage(reason); ok && got != want {
		return invalid("the text is %s but your reason is %s: write the reason in %s too, the language of the text", languageNames[want], languageNames[got], languageNames[want])
	}
	return nil
}
