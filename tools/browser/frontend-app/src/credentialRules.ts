// Pure rules for the Login page's vault card. No React, so they can be script-checked.

// Second-level labels that, before a two-letter country code, belong to the suffix
// ("shop.example.co.uk" is a .co.uk site, not a .uk one).
const SECOND_LEVEL = new Set(['co', 'com', 'org', 'net', 'ac', 'gov', 'edu'])

// The site type of a domain, as shown in the vault's chips: ".com", ".de", ".co.uk";
// "(local)" for a bare host name such as "localhost", "(ip)" for an address.
export function domainSuffix(domain: string): string {
  const labels = domain
    .trim()
    .toLowerCase()
    .replace(/\.$/, '')
    .split('.')
    .filter((l) => l !== '')
  if (labels.length <= 1) return '(local)'
  if (labels.every((l) => /^\d+$/.test(l))) return '(ip)'
  const last = labels[labels.length - 1]
  const before = labels[labels.length - 2]
  if (labels.length >= 3 && last.length === 2 && SECOND_LEVEL.has(before)) return `.${before}.${last}`
  return `.${last}`
}

export interface SuffixCount {
  suffix: string
  count: number
}

// How many saved logins there are per site type, most first, then by name.
export function suffixCounts(credentials: { domain: string }[]): SuffixCount[] {
  const counts = new Map<string, number>()
  for (const c of credentials) {
    const suffix = domainSuffix(c.domain)
    counts.set(suffix, (counts.get(suffix) ?? 0) + 1)
  }
  return [...counts.entries()]
    .map(([suffix, count]) => ({ suffix, count }))
    .sort((a, b) => b.count - a.count || a.suffix.localeCompare(b.suffix))
}

export type CredentialPlan =
  { kind: 'empty' } | { kind: 'invalid' } | { kind: 'add'; domain: string } | { kind: 'replace'; domain: string; currentUsername: string }

// What saving this domain will do, given the saved logins: add a new one, replace the one
// that is there (domains compare without regard to case), or nothing yet (blank) / nothing
// sensible (spaces or a path inside what should be a host name). The domain is the page's
// own normalisation of what was typed (a pasted URL already reduced to its host).
export function credentialPlan(domain: string, credentials: { domain: string; username: string }[]): CredentialPlan {
  const d = domain.trim()
  if (d === '') return { kind: 'empty' }
  if (/[\s/]/.test(d)) return { kind: 'invalid' }
  const existing = credentials.find((c) => c.domain.toLowerCase() === d.toLowerCase())
  return existing ? { kind: 'replace', domain: d, currentUsername: existing.username } : { kind: 'add', domain: d }
}
