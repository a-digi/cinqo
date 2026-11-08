// Pure rules of the persona pages (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md, step 4): choosing which
// library skills and experiences a persona uses, and its own wording of an experience for its role. No React, so they can be
// script-checked.
import type { ExperienceUse, ProfileExperience } from '../../api'

// The ids with one more or one less, in the order they were: a repeat is not added, an absent one is not removed.
export function toggleId(ids: readonly string[], id: string, on: boolean): string[] {
  const has = ids.includes(id)
  if (on) return has ? [...ids] : [...ids, id]
  return ids.filter((x) => x !== id)
}

// The uses with one more (without wording: it follows the profile) or one less.
export function toggleUse(uses: readonly ExperienceUse[], id: string, on: boolean): ExperienceUse[] {
  const has = uses.some((u) => u.id === id)
  if (on) return has ? [...uses] : [...uses, { id }]
  return uses.filter((u) => u.id !== id)
}

// "Uses 3 of 30 skills", "Uses 0 of 1 entry" and so on.
export function usesText(used: number, total: number, singular: string, plural: string): string {
  return `Uses ${String(used)} of ${String(total)} ${total === 1 ? singular : plural}`
}

// The wording a persona gives an entry: off follows the profile; on has a title and a description of its own.
export interface Wording {
  on: boolean
  title: string
  description: string
}

// What the persona currently has for an entry: its overrides when it has any, the profile's text as the starting point otherwise.
export function wordingOf(use: ExperienceUse | undefined, library: Pick<ProfileExperience, 'title' | 'description'>): Wording {
  const on = use !== undefined && (use.titleOverride !== undefined || use.descriptionOverride !== undefined)
  return {
    on,
    title: use?.titleOverride ?? library.title,
    description: use?.descriptionOverride ?? library.description ?? '',
  }
}

// The use to send for an entry and its wording: off sends the id alone (follow the profile); on sends the title (trimmed) and the
// description as written.
export function useFromWording(id: string, w: Wording): ExperienceUse {
  if (!w.on) return { id }
  return { id, titleOverride: w.title.trim(), descriptionOverride: w.description }
}

// A wording that cannot be saved: its title is empty. '' when it can.
export function wordingProblem(w: Wording): string {
  return w.on && w.title.trim() === '' ? 'The title cannot be empty' : ''
}

// The uses with this entry's use replaced by the new wording (the entry must be used; the others are untouched, in their order).
export function withWording(uses: readonly ExperienceUse[], id: string, w: Wording): ExperienceUse[] {
  return uses.map((u) => (u.id === id ? useFromWording(id, w) : u))
}

// Whether a use has a wording of its own (a title or a description override).
export function hasWording(use: ExperienceUse | undefined): boolean {
  return use !== undefined && (use.titleOverride !== undefined || use.descriptionOverride !== undefined)
}

// The drafts without the one of this entry (a new object; the others stay).
export function withoutDraft<T>(drafts: Readonly<Record<string, T>>, id: string): Record<string, T> {
  return Object.fromEntries(Object.entries(drafts).filter(([key]) => key !== id))
}

// The note a persona's item carries where a persona is shown (plan 173): '' for an item from the profile's library, "Only on this
// persona" for one it has on its own.
export const OWN_NOTE = 'Only on this persona'

export function skillNote(source: 'profile' | 'persona' | undefined): string {
  return source === 'persona' ? OWN_NOTE : ''
}

// The notes of an experience of a persona: "Only on this persona", or, for a library entry, "Own wording" when the persona words it
// itself (a title or a description of its own).
export function experienceNotes(e: {
  source?: 'profile' | 'persona'
  titleOverridden?: boolean
  descriptionOverridden?: boolean
}): string[] {
  if (e.source === 'persona') return [OWN_NOTE]
  return e.titleOverridden === true || e.descriptionOverridden === true ? ['Own wording'] : []
}
