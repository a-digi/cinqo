// Pure rules of the CV import's "Insert" for skills and experience (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md,
// step 5): what a CV states belongs to the PROFILE, so it goes into the profile's library and the chosen persona then uses it. These
// rules find an item the library already has (so nothing is added twice), tell the new entry from the others, and merge what a persona
// already uses with what was just added. No React, so they can be script-checked.
import type { ExperienceUse, ProfileExperience, ProfileSkill } from '../../api'

const norm = (s: string) => s.trim().toLowerCase()

// The library skill with this text (trimmed, any case), or undefined.
export function findSkill(library: readonly Pick<ProfileSkill, 'id' | 'skill'>[], text: string) {
  return library.find((s) => norm(s.skill) === norm(text))
}

interface EntryLike {
  company: string
  title: string
  startDate?: string
  endDate?: string
}

function sameEntry(a: EntryLike, b: EntryLike): boolean {
  return (
    norm(a.company) === norm(b.company) &&
    norm(a.title) === norm(b.title) &&
    (a.startDate ?? '').trim() === (b.startDate ?? '').trim() &&
    (a.endDate ?? '').trim() === (b.endDate ?? '').trim()
  )
}

// The library experience that is this same entry (company and title any case, same dates), or undefined.
export function findEntry<T extends EntryLike & { id: string }>(library: readonly T[], entry: EntryLike): T | undefined {
  return library.find((e) => sameEntry(e, entry))
}

// The id of the entry that was just added: the one in `after` that was not in `before` and is this entry; undefined when there is none.
export function addedEntryId(
  before: readonly Pick<ProfileExperience, 'id'>[],
  after: readonly ProfileExperience[],
  entry: EntryLike,
): string | undefined {
  const known = new Set(before.map((e) => e.id))
  return after.find((e) => !known.has(e.id) && sameEntry(e, entry))?.id
}

// What the persona uses after the insert: what it used, then the added ids it did not use yet (in order, without repeats).
export function mergeIds(used: readonly string[], added: readonly string[]): string[] {
  const out = [...used]
  for (const id of added) if (!out.includes(id)) out.push(id)
  return out
}

// The same for experience: the uses it had (with their wording), then the added entries it did not use yet, without wording.
export function mergeUses(used: readonly ExperienceUse[], added: readonly string[]): ExperienceUse[] {
  const out = [...used]
  for (const id of added) if (!out.some((u) => u.id === id)) out.push({ id })
  return out
}
