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
