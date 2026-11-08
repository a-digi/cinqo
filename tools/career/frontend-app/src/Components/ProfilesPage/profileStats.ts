// Pure numbers for the Profiles page's overview, worked out from the profiles already
// loaded. No React, so they can be script-checked.
export interface ProfileLike {
  externalLinks: unknown[]
}

export interface ProfileSummary {
  profiles: number
  // Profiles that have at least one / no external link.
  withLinks: number
  withoutLinks: number
  // Links over all profiles.
  links: number
}

export function summarizeProfiles(profiles: ProfileLike[]): ProfileSummary {
  const s: ProfileSummary = { profiles: profiles.length, withLinks: 0, withoutLinks: 0, links: 0 }
  for (const p of profiles) {
    if (p.externalLinks.length > 0) s.withLinks++
    else s.withoutLinks++
    s.links += p.externalLinks.length
  }
  return s
}
