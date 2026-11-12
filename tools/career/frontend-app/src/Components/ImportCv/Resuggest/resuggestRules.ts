// resuggestRules.ts — the pure rules of "Re-suggest" on the CV import review (plan 268): which section or rows a chat is about, what it is called, which buttons are shown, and
// what the person's wish is. No React, so they can be script-checked.

// The sections of the review, named as the server names them.
export type ResuggestSection = 'profile' | 'persona' | 'personalDetails' | 'skills' | 'experience' | 'education'

export const RESUGGEST_SECTIONS: ResuggestSection[] = ['profile', 'persona', 'personalDetails', 'skills', 'experience', 'education']

// The headings of the review's cards.
export const SECTION_TITLES: Record<ResuggestSection, string> = {
  profile: 'Profile',
  persona: 'Target roles',
  personalDetails: 'Personal Details',
  skills: 'Skills',
  experience: 'Experience',
  education: 'Education',
}

// A list section can be re-suggested row by row; the single ones are one row each.
export function isListSection(section: ResuggestSection): boolean {
  return section === 'skills' || section === 'experience' || section === 'education'
}

// What a chat is about: a whole section, or some rows of a list (`label` names the row for the person: "Acme · Senior Engineer").
export interface ResuggestScope {
  section: ResuggestSection
  itemIds: string[]
  label: string
}

export function sectionScope(section: ResuggestSection, label = ''): ResuggestScope {
  return { section, itemIds: [], label }
}

const NOUNS: Record<'skills' | 'experience' | 'education', [string, string]> = {
  skills: ['skill', 'skills'],
  experience: ['job', 'jobs'],
  education: ['entry', 'entries'],
}

// What the whole-section button and the dialog's chip call a list: "All 4 jobs". Empty for a single section and for a list with fewer than two rows (one row is the row's own button).
export function wholeLabel(section: ResuggestSection, count: number): string {
  if (!isListSection(section) || count < 2) return ''
  const [, many] = NOUNS[section as 'skills' | 'experience' | 'education']
  return `All ${String(count)} ${many}`
}

// The text of a section's button: "Re-suggest all 4" for a list of several rows, else "Re-suggest".
export function sectionButtonText(section: ResuggestSection, count: number): string {
  return wholeLabel(section, count) === '' ? 'Re-suggest' : `Re-suggest all ${String(count)}`
}

export function rowScope(section: ResuggestSection, id: string, label: string): ResuggestScope {
  return { section, itemIds: [id], label }
}

export function scopeTitle(scope: ResuggestScope): string {
  return `Re-suggest · ${SECTION_TITLES[scope.section]}`
}

// The chip under the title: "The whole section" or the row's own name.
export function scopeChip(scope: ResuggestScope): string {
  if (scope.itemIds.length === 0) return scope.label !== '' ? scope.label : 'The whole section'
  return scope.label !== '' ? scope.label : scope.itemIds.length === 1 ? 'One row' : `${String(scope.itemIds.length)} rows`
}

export function conversationTitle(scope: ResuggestScope, originalFilename: string): string {
  return `Re-suggest ${SECTION_TITLES[scope.section]}: ${originalFilename}`
}

// The wish of the person; an empty one asks the AI to look again and improve.
export const DEFAULT_WISH = 'Look at this again against the CV and improve it: write it differently, with several concrete points.'
export const MAX_WISH_LENGTH = 1000

export function wishOf(raw: string): string {
  const text = raw.trim()
  return text === '' ? DEFAULT_WISH : text.slice(0, MAX_WISH_LENGTH)
}

// What the person has on the review page that decides which buttons are shown.
export interface ResuggestPageState {
  // A profile was chosen for the import (plan 261): the proposed profile is not used, so there is nothing to re-suggest in the Profile card.
  targetChosen: boolean
  // Something was already saved from this review: the suggestions are then closed to the AI.
  savedAnything: boolean
  // A turn of a chat is running.
  busy: boolean
}

// The button on a section card.
export function showSectionButton(section: ResuggestSection, page: ResuggestPageState): boolean {
  if (page.savedAnything || page.busy) return false
  return !(section === 'profile' && page.targetChosen)
}

// The button on a row of a list: not for a row that was inserted, one without an id yet (an older run that the server has not given ids), nor when the section button is not shown.
export function showRowButton(section: ResuggestSection, row: { id?: string; done: boolean }, page: ResuggestPageState): boolean {
  if (!isListSection(section) || row.done || !row.id) return false
  return showSectionButton(section, page)
}

export const BUSY_HINT = 'The AI is changing the suggestions…'

// What a failed turn says; the message of the error itself when there is one.
export function turnFailure(err: unknown): string {
  const text = err instanceof Error ? err.message.trim() : String(err).trim()
  return text !== '' ? text : 'The AI could not finish. Try again.'
}

// Said under the AI line when the chosen model is a small local one (it is asked small questions and cannot add or remove jobs or education).
export const LOCAL_MODEL_NOTE =
  'A small local model looks at the suggestions one by one and changes the ones that are there; it can add or remove skills, but not jobs or education entries. It can take a while.'

// The transcript of the dialog: what the person wrote, what the AI answered, and what went wrong.
export interface ResuggestLine {
  role: 'user' | 'ai' | 'error'
  text: string
}
