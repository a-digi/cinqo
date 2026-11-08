// Pure rules for the Skills page's A-Z index. No React, so they can be script-checked.

// The letters the index shows, in order; skills that do not start with one of them
// (a digit, a symbol, another script) are grouped under '#'.
export const LETTERS = [
  'A',
  'B',
  'C',
  'D',
  'E',
  'F',
  'G',
  'H',
  'I',
  'J',
  'K',
  'L',
  'M',
  'N',
  'O',
  'P',
  'Q',
  'R',
  'S',
  'T',
  'U',
  'V',
  'W',
  'X',
  'Y',
  'Z',
  '#',
]

// The index letter of a skill: its first character, upper-cased and without accents
// ("élan" is E), or '#' when that is not one of A-Z.
export function letterOf(skill: string): string {
  const first = Array.from(skill.trim())[0]
  if (!first) return '#'
  const base = first.normalize('NFD').replace(/[̀-ͯ]/g, '').toUpperCase()
  return /^[A-Z]$/.test(base) ? base : '#'
}

export interface LetterCount {
  letter: string
  count: number
}

// How many skills start with each letter, for every entry of LETTERS (zero included).
export function letterIndex(skills: string[]): LetterCount[] {
  const counts = new Map<string, number>()
  for (const s of skills) counts.set(letterOf(s), (counts.get(letterOf(s)) ?? 0) + 1)
  return LETTERS.map((letter) => ({ letter, count: counts.get(letter) ?? 0 }))
}

// The skills that belong to a letter, in their order.
export function filterByLetter(skills: string[], letter: string): string[] {
  return skills.filter((s) => letterOf(s) === letter)
}
