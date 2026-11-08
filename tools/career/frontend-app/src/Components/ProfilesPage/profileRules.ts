// Pure rules for the Profiles page's platform shelf: how many links each platform has.
// No React, so they can be script-checked.
export interface LinkedProfile {
  id: string
  externalLinks: { platform: string }[]
}

export interface PlatformCount {
  // The platform as it was first written ("LinkedIn"), whatever the case of later ones.
  platform: string
  // Links to that platform, over all profiles.
  count: number
}

// Platforms are the same when they differ only in case or surrounding spaces. Most links
// first, then by name; a blank platform is ignored.
export function platformCounts(profiles: LinkedProfile[]): PlatformCount[] {
  const byKey = new Map<string, PlatformCount>()
  for (const p of profiles) {
    for (const link of p.externalLinks) {
      const name = link.platform.trim()
      if (name === '') continue
      const key = name.toLowerCase()
      const found = byKey.get(key)
      if (found) found.count++
      else byKey.set(key, { platform: name, count: 1 })
    }
  }
  return [...byKey.values()].sort((a, b) => b.count - a.count || a.platform.localeCompare(b.platform))
}

// The first profile that has no link at all, or null when every profile has one.
export function firstWithoutLinks(profiles: LinkedProfile[]): string | null {
  return profiles.find((p) => p.externalLinks.length === 0)?.id ?? null
}

export interface CompletenessItemLike {
  label: string
  weight: number
  done: boolean
}

// The words printed next to the percentage, so the band never rests on colour alone.
export function bandLabel(band: string): string {
  switch (band) {
    case 'complete':
      return 'Complete'
    case 'strong':
      return 'Strong'
    case 'getting-there':
      return 'Getting there'
    default:
      return 'Just started'
  }
}

// The missing items, heaviest first (ties keep their order), as the hint list.
export function missingItems<T extends CompletenessItemLike>(items: T[]): T[] {
  return items.filter((i) => !i.done).sort((a, b) => b.weight - a.weight)
}

// What to do next: the heaviest missing item, or null when nothing is missing.
export function nextStep(items: CompletenessItemLike[]): string | null {
  return missingItems(items)[0]?.label ?? null
}

// The average score over all profiles, rounded, or null when there are none.
export function averageScore(profiles: { completeness: { score: number } }[]): number | null {
  if (profiles.length === 0) return null
  return Math.round(profiles.reduce((sum, p) => sum + p.completeness.score, 0) / profiles.length)
}
