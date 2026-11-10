// Pure rules of the Languages page and form (plan/ai/tools/career/step-231-hobbies-courses-languages.md): the levels and how they read, what
// an entry can do, the early check of what the server checks again, and the count. No React, so they can be script-checked.
import type { Language } from '../../api'
import { charCount } from '../../Shared/charCount'

// The levels the server accepts, best first, with the words the form and the list show. '' is "not given".
export const LEVELS: readonly { value: string; label: string }[] = [
  { value: '', label: 'Not given' },
  { value: 'native', label: 'Native' },
  { value: 'c2', label: 'C2 · Proficient' },
  { value: 'c1', label: 'C1 · Advanced' },
  { value: 'b2', label: 'B2 · Upper intermediate' },
  { value: 'b1', label: 'B1 · Intermediate' },
  { value: 'a2', label: 'A2 · Elementary' },
  { value: 'a1', label: 'A1 · Beginner' },
]

// The words for a level, or '' when none is given (or an unknown one).
export function levelLabel(level: string): string {
  if (level === '') return ''
  return LEVELS.find((l) => l.value === level)?.label ?? ''
}

// "Spoken and written", "Spoken", "Written", or '' when neither (the server does not allow that).
export function abilityText(l: Pick<Language, 'canSpeak' | 'canWrite'>): string {
  if (l.canSpeak && l.canWrite) return 'Spoken and written'
  if (l.canSpeak) return 'Spoken'
  return l.canWrite ? 'Written' : ''
}

export const MAX_NAME_LENGTH = 200

// What stops a form from being sent: '' when it can be (the same checks the server makes).
export function formProblem(f: { language: string; canSpeak: boolean; canWrite: boolean }): string {
  const name = f.language.trim()
  if (name === '') return 'The language is required'
  if (charCount(name) > MAX_NAME_LENGTH) return `The language is too long (at most ${String(MAX_NAME_LENGTH)} characters)`
  if (!f.canSpeak && !f.canWrite) return 'Tick spoken, written or both'
  return ''
}

// "3 languages", "1 language".
export function languagesCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'language' : 'languages'}`
}
