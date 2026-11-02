// Pure rules of the Summary and Goals pages (plan/ai/tools/career/step-174-split-summary-and-goals.md, step 4): the profile holds one
// summary and one set of goals, a persona FOLLOWS them unless it has its own value. These rules say which values are a persona's own,
// what a form sends (only what changed, so an untouched field keeps following), and what is wrong with a form before it is sent.
// No React, so they can be script-checked.
import type { PersonaDetails, ProfileDetails, ProfileDetailsPatch, UpdatePersonaDetailsArgs } from '../../api'

export type RemotePreferenceValue = '' | 'onsite' | 'open' | 'remote_only'

// The four goals as a form holds them (the salary as text).
export interface GoalsForm {
  desiredTitles: string
  desiredLocations: string
  minSalary: string
  remotePreference: RemotePreference
}
type RemotePreference = RemotePreferenceValue

export type GoalKey = 'desiredTitles' | 'desiredLocations' | 'minSalary' | 'remotePreference'
export const GOAL_KEYS: readonly GoalKey[] = ['desiredTitles', 'desiredLocations', 'minSalary', 'remotePreference']

export const FOLLOWING_TEXT = 'Following the profile'
export const OWN_TEXT = 'Own wording'

// A remote preference the server sent as a value the form knows: anything else is "not set".
export function asRemote(value: unknown): RemotePreference {
  return value === 'onsite' || value === 'open' || value === 'remote_only' ? value : ''
}

// Whether the persona words this value itself (an answer without the list: it follows).
export function isOwn(overridden: readonly string[] | undefined, field: string): boolean {
  return overridden?.includes(field) ?? false
}

export function markText(own: boolean): string {
  return own ? OWN_TEXT : FOLLOWING_TEXT
}

export function goalsFormOf(
  d: Pick<PersonaDetails, 'desiredTitles' | 'desiredLocations' | 'minSalary' | 'remotePreference'> | null,
): GoalsForm {
  return {
    desiredTitles: d?.desiredTitles ?? '',
    desiredLocations: d?.desiredLocations ?? '',
    minSalary: d && d.minSalary > 0 ? String(d.minSalary) : '',
    remotePreference: asRemote(d?.remotePreference),
  }
}

export function goalsFormOfProfile(d: ProfileDetails | null): GoalsForm {
  return goalsFormOf(d)
}

// The salary text as a number: empty is 0 (no minimum); anything that is not a whole number of 0 or more is NaN.
export function salaryNumber(text: string): number {
  const t = text.trim()
  if (t === '') return 0
  return /^\d+$/.test(t) ? Number(t) : Number.NaN
}

// What is wrong with the goals form, in words, '' when it can be sent.
export function goalsProblem(f: GoalsForm): string {
  return Number.isNaN(salaryNumber(f.minSalary)) ? 'The minimum salary must be a whole number of 0 or more' : ''
}

// Only the goals that differ from what was saved: a persona sends its own values (an untouched goal keeps following), the profile its
// changed ones.
export function goalsPatch(form: GoalsForm, saved: GoalsForm): ProfileDetailsPatch {
  const patch: ProfileDetailsPatch = {}
  if (form.desiredTitles !== saved.desiredTitles) patch.desiredTitles = form.desiredTitles
  if (form.desiredLocations !== saved.desiredLocations) patch.desiredLocations = form.desiredLocations
  if (form.minSalary.trim() !== saved.minSalary.trim()) patch.minSalary = salaryNumber(form.minSalary)
  if (form.remotePreference !== saved.remotePreference) patch.remotePreference = form.remotePreference
  return patch
}

export function hasChanges(patch: object): boolean {
  return Object.keys(patch).length > 0
}

// The summary text of a persona page: sent (as the persona's own) only when it differs from what the page showed.
export function summaryPatch(form: string, saved: string): { summary?: string } {
  return form !== saved ? { summary: form } : {}
}

// "Use the profile's value" for a goal or the summary: what to send. A persona that already follows has nothing to send.
export function followRequest(own: boolean, field: string): UpdatePersonaDetailsArgs {
  return own ? { followProfile: [field] } : {}
}

// The profile summary form is sent when it differs from what was saved.
export function profileSummaryPatch(form: string, saved: string): ProfileDetailsPatch {
  return form !== saved ? { summary: form } : {}
}

// --- the CV import's "Insert" of the personal details (plan 174, decision 6) ---

interface ImportedDetails {
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  minSalary: number
}

// Where each imported value goes: a summary or goal the CV states fills the PROFILE's value when the profile has none; where the profile
// has one, the persona follows it when it is the same (trimmed) and keeps the imported one as its own when it differs; an imported value
// that is empty is "no opinion" and changes nothing. Headline and location always go to the persona.
export function importDetailsPlan(
  imported: ImportedDetails,
  profile: ProfileDetails | null,
): { profilePatch: ProfileDetailsPatch; personaArgs: UpdatePersonaDetailsArgs } {
  const profilePatch: ProfileDetailsPatch = {}
  const personaArgs: UpdatePersonaDetailsArgs = { headline: imported.headline, location: imported.location }
  const text = (key: 'summary' | 'desiredTitles' | 'desiredLocations') => {
    const value = imported[key].trim()
    if (value === '') return
    const have = (profile?.[key] ?? '').trim()
    if (have === '') profilePatch[key] = value
    else if (have !== value) personaArgs[key] = imported[key]
  }
  text('summary')
  text('desiredTitles')
  text('desiredLocations')
  if (imported.minSalary > 0) {
    const have = profile?.minSalary ?? 0
    if (have === 0) profilePatch.minSalary = imported.minSalary
    else if (have !== imported.minSalary) personaArgs.minSalary = imported.minSalary
  }
  return { profilePatch, personaArgs }
}
