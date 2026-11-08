// Pure rules for the profile view page (step 117). No React, so they can be script-checked.

interface CvLike {
  id: string
  createdAt: string
  personas: { id: string }[]
}

// The CVs that belong to the profile: built from one of its personas or attached to one, newest
// first (the same CV is listed once however many of the personas it is attached to).
export function cvsOfProfile<T extends CvLike>(docs: T[], personaIds: readonly string[]): T[] {
  const mine = new Set(personaIds)
  return docs
    .filter((d) => d.personas.some((p) => mine.has(p.id)))
    .sort((a, b) => (a.createdAt < b.createdAt ? 1 : a.createdAt > b.createdAt ? -1 : 0))
}

// The route's list of CVs holds at most this many (the server's cap); at that count some may be missing.
export const CV_LIST_CAP = 200

export function cvListMayBeCut(totalCvs: number): boolean {
  return totalCvs >= CV_LIST_CAP
}

// "Created 28.10.2026 · Updated 30.10.2026", or just the first when it was never updated. Dates that
// cannot be read are left out.
export function lifetimeText(createdAt: string, updatedAt: string | undefined, format: (iso: string) => string): string {
  const parts: string[] = []
  const created = format(createdAt)
  if (created !== '') parts.push(`Created ${created}`)
  const updated = updatedAt === undefined ? '' : format(updatedAt)
  if (updated !== '' && updated !== created) parts.push(`Updated ${updated}`)
  return parts.join(' · ')
}

// "3 skills · 2 roles", with the singular for one.
export function countsText(skills: number, roles: number): string {
  return `${skills} ${skills === 1 ? 'skill' : 'skills'} · ${roles} ${roles === 1 ? 'role' : 'roles'}`
}
