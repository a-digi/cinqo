// language.ts — the language of a text, told apart by how many of its words are common function words of a language (plan 258, step 6). The suggestions of the AI must be in the language of
// the text they replace: a German text gets a German suggestion, never an English one. No library and no network: a text is "placed" in a language only when it is long enough and one
// language's words clearly dominate; anything else (a list of technologies, a name, a short title, a language that is not in the list) is NOT placed, and then nothing is refused.
//
// The word lists are the server's own (backend/optimize/language.go): the tests read that file and fail when the two drift apart, and both test suites place the sentences of
// backend/optimize/testdata/language_fixtures.json.

export type LanguageCode = 'de' | 'en' | 'fr' | 'it' | 'es' | 'nl' | 'sq'

export const LANGUAGE_WORDS: Record<LanguageCode, string> = {
  de: 'der die das und ist nicht mit für von zu den dem des ein eine einer einen eines auf im am bei auch wir ich sie es sich wurde wurden haben hat werden als nach über unter durch aus oder aber wie nur noch schon sehr zum zur sowie wodurch dass wird sind war waren seit bis',
  en: 'the and to that for with on by at from this were are be been has have had it its or which their our we they who not also more you your into over using built led developed than these those through while across',
  fr: 'les des du une et est que qui pour dans par sur avec aux cette ces sont été être nous vous ils elles pas mais comme sans sous chez entre depuis très aussi leur leurs ses au ont avons avez où pendant plusieurs ce qu',
  it: 'di il gli del della dello dei delle nel nella nei è che per con sono stato stata essere hanno non più come anche ma tra fra dal dalla alla alle questo questa suo sua loro uno una ho ha ed',
  es: 'el los las del y es que por para con una se su sus al como más pero sin sobre entre también fue han son está este esta muy hemos desde hasta ser he ha lo en un unos unas',
  nl: 'het een van dat op voor met zijn werd werden ook als bij door naar aan uit om niet maar of nog wel heeft hebben deze zich over tot onder tussen meer ik ben geweest waardoor',
  sq: 'dhe për një është që nga në të janë kam ka si edhe por ose nuk ky kjo këtë këto gjithashtu me së tek ndaj mes pas para gjatë qenë ekipin klientët',
}

export const LANGUAGE_NAMES: Record<LanguageCode, string> = {
  de: 'German',
  en: 'English',
  fr: 'French',
  it: 'Italian',
  es: 'Spanish',
  nl: 'Dutch',
  sq: 'Albanian',
}

// What the one-sentence reason of a suggestion says when the page makes it (the local driver), in the language of the text.
export const REASONS: Record<LanguageCode, string> = {
  de: 'Klarere Formulierung.',
  en: 'Clearer wording.',
  fr: 'Formulation plus claire.',
  it: 'Formulazione più chiara.',
  es: 'Redacción más clara.',
  nl: 'Duidelijkere formulering.',
  sq: 'Formulim më i qartë.',
}

// The thresholds of "clearly this language": at least MIN_WORDS words, at least MIN_HITS of them from the language's list, at least MIN_SHARE of all words, and at least DOMINANCE
// times as many hits as the next language.
export const MIN_WORDS = 8
export const MIN_HITS = 3
export const MIN_SHARE = 0.15
export const DOMINANCE = 1.5

const CODES = Object.keys(LANGUAGE_WORDS) as LanguageCode[]
const SETS = new Map<LanguageCode, Set<string>>(CODES.map((c) => [c, new Set(LANGUAGE_WORDS[c].split(/\s+/).filter((w) => w !== ''))]))

// Places a text in one of the languages above, or says it cannot (null).
export function detectLanguage(text: string): LanguageCode | null {
  const words = text.toLowerCase().match(/\p{L}+/gu) ?? []
  if (words.length < MIN_WORDS) return null
  const hits = new Map<LanguageCode, number>()
  for (const w of words) {
    for (const c of CODES) {
      if (SETS.get(c)?.has(w)) hits.set(c, (hits.get(c) ?? 0) + 1)
    }
  }
  let best: LanguageCode | null = null
  let bestHits = 0
  let second = 0
  for (const [c, n] of hits) {
    if (n > bestHits) {
      second = bestHits
      best = c
      bestHits = n
    } else if (n > second) {
      second = n
    }
  }
  if (best === null || bestHits < MIN_HITS || bestHits < MIN_SHARE * words.length || bestHits < DOMINANCE * second) return null
  return best
}

// The reason sentence in the language of the text (English when the text cannot be placed).
export function reasonFor(text: string): string {
  const lang = detectLanguage(text)
  return lang ? REASONS[lang] : REASONS.en
}
