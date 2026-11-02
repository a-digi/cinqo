// Pure rules of the profile's library pages (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md, step 3): which
// profile a page shows, the address that says it, what "used by" and a delete's warning say, and how the library maps onto the shapes
// the career timeline reads. No React, so they can be script-checked.
import type { Persona, Profile, ProfileExperience } from '../../api'

// The profile a library page shows: the one the address names when it exists, else the one remembered in this browser when it exists,
// else the first profile; '' when there is none.
export function chooseProfileId(profiles: readonly Pick<Profile, 'id'>[], fromAddress: string, remembered: string | null): string {
  const has = (id: string) => profiles.some((p) => p.id === id)
  if (fromAddress !== '' && has(fromAddress)) return fromAddress
  if (remembered !== null && remembered !== '' && has(remembered)) return remembered
  return profiles[0]?.id ?? ''
}

// ?profileId=<id> in the address, '' when there is none (the first value counts, surrounding blanks do not).
export function parseProfileParam(search: string): string {
  return (new URLSearchParams(search).get('profileId') ?? '').trim()
}

// The address a page writes for a profile: '' for none, else "?profileId=<id>".
export function buildProfileSearch(profileId: string): string {
  const id = profileId.trim()
  return id === '' ? '' : `?${new URLSearchParams({ profileId: id }).toString()}`
}

// "Not used", "Used by 1 persona", "Used by 3 personas".
export function usedByText(count: number): string {
  if (count <= 0) return 'Not used by any persona'
  return count === 1 ? 'Used by 1 persona' : `Used by ${String(count)} personas`
}

// What asking to remove a library item says when personas use it (null when none does: nothing to warn about, it is removed at once).
export function removeWarning(count: number): string | null {
  if (count <= 0) return null
  return count === 1
    ? 'One persona uses this. Removing it takes it off that persona too.'
    : `${String(count)} personas use this. Removing it takes it off all of them.`
}

// The personas of one profile, in the order they are given.
export function personasOfProfile(personas: readonly Persona[], profileId: string): Persona[] {
  return personas.filter((p) => p.profileId === profileId)
}

// "3 skills in the library", "1 skill in the library".
export function skillCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'skill' : 'skills'} in the library`
}

export function experienceCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'entry' : 'entries'} in the library`
}

// The timeline reads entries with every date as text; a library entry may leave the optional ones out.
export function timelineEntries(list: readonly ProfileExperience[]) {
  return list.map((e) => ({
    id: e.id,
    company: e.company,
    title: e.title,
    startDate: e.startDate ?? '',
    endDate: e.endDate ?? '',
    description: e.description ?? '',
  }))
}

// Whether a new skill's text can be sent: not empty after trimming.
export function skillTextOk(text: string): boolean {
  return text.trim() !== ''
}

// A date as it is shown: the text, or the fallback ("—", "present") when there is none.
export function dateText(value: string | undefined, fallback: string): string {
  const text = (value ?? '').trim()
  return text === '' ? fallback : text
}
