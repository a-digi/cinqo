// Pure rules for the shared persona list (no React, so they can be script-checked).

interface PersonaLike {
  id: string
  profileId: string
  name: string
}

interface ProfileLike {
  id: string
  firstName: string
  lastName: string
}

export interface PersonaGroup<P extends PersonaLike> {
  // The profile's id, or '' for personas whose profile is not in the list.
  profileId: string
  label: string
  personas: P[]
}

// A profile's name as the other pages show it ("Ada Lovelace"), or "Profile 1a2b3c4d" when it has none.
function labelOf(p: ProfileLike): string {
  const name = `${p.firstName} ${p.lastName}`.trim()
  return name || `Profile ${p.id.slice(0, 8)}`
}

// The personas grouped by their profile: the groups by the profile's name (ignoring case), the personas inside
// a group by name; a profile with no personas gets no group; personas whose profile is unknown come last, under
// "Other personas".
export function groupPersonasByProfile<P extends PersonaLike>(personas: P[], profiles: ProfileLike[]): PersonaGroup<P>[] {
  const byProfile = new Map<string, P[]>()
  for (const p of personas) {
    const list = byProfile.get(p.profileId)
    if (list) list.push(p)
    else byProfile.set(p.profileId, [p])
  }
  const byName = (a: P, b: P) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  const known = profiles
    .filter((pr) => byProfile.has(pr.id))
    .map((pr) => ({ profileId: pr.id, label: labelOf(pr), personas: [...(byProfile.get(pr.id) ?? [])].sort(byName) }))
    .sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }))
  const knownIds = new Set(profiles.map((pr) => pr.id))
  const orphans = personas.filter((p) => !knownIds.has(p.profileId)).sort(byName)
  return orphans.length > 0 ? [...known, { profileId: '', label: 'Other personas', personas: orphans }] : known
}

// "a, b, c +2 more": the first `max` items and how many are left out.
export function previewItems(items: string[], max: number): { shown: string[]; more: number } {
  return { shown: items.slice(0, max), more: Math.max(0, items.length - max) }
}
