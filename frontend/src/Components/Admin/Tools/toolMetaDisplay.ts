import type { Publisher, ToolAuthor, ToolLinks, ToolMetadata } from '../../../api/toolMeta'

// Pure display rules for who made a tool or a repository (plan/ai/tools/repository/tool-metadata.md), kept apart
// from React so they can be script-checked. Nothing here says a name is verified: it is the publisher's claim.

export const NOT_STATED = 'Developer not stated'
export const NOT_STATED_HINT = 'The publisher did not say who made this.'
export const CLAIM_HINT = 'Stated by the publisher. Cinqo cannot verify it.'

// "Ada", "Ada and Grace", "Ada, Grace +1".
export function authorsLabel(authors: Pick<ToolAuthor, 'name'>[] | undefined): string {
  const names = (authors ?? []).map((a) => a.name.trim()).filter((n) => n !== '')
  if (names.length === 0) return ''
  if (names.length === 1) return names[0]
  if (names.length === 2) return `${names[0]} and ${names[1]}`
  return `${names[0]}, ${names[1]} +${names.length - 2}`
}

// "by Ada +1 · Acme GmbH", or null when nobody is named (the caller then shows NOT_STATED).
export function byLine(meta: Pick<ToolMetadata, 'authors' | 'company'> | null | undefined): string | null {
  const who = authorsLabel(meta?.authors)
  if (who === '') return null
  const company = meta?.company?.name.trim() ?? ''
  return company === '' ? `by ${who}` : `by ${who} · ${company}`
}

// The maintainer line of a repository: "Maintained by Cinqo team · Acme GmbH", or null.
export function publisherLine(publisher: Pick<Publisher, 'authors' | 'company'> | null | undefined): string | null {
  const line = byLine(publisher)
  return line === null ? null : `Maintained ${line}`
}

export interface LinkEntry {
  key: keyof ToolLinks
  label: string
  url: string
  // What to show next to the label so a look-alike address is visible: the host, or the mail address.
  host: string
  mail: boolean
}

const LINK_LABELS: [keyof ToolLinks, string][] = [
  ['homepage', 'Homepage'],
  ['documentation', 'Documentation'],
  ['source', 'Source'],
  ['issues', 'Issues'],
  ['support', 'Support'],
  ['security', 'Security'],
]

// Only an https address (and mailto: for the security contact) is ever made into a link: the server checks this
// too, and this is the second check before anything reaches an `href`.
export function safeLink(url: string, allowMail: boolean): { host: string; mail: boolean } | null {
  if (allowMail && /^mailto:[^\s@?]+@[^\s@?]+$/i.test(url)) return { host: url.slice('mailto:'.length), mail: true }
  try {
    const u = new URL(url)
    if (u.protocol !== 'https:' || u.hostname === '' || u.username !== '' || u.password !== '' || /\s/.test(url)) return null
    return { host: u.hostname, mail: false }
  } catch {
    return null
  }
}

// The links to draw, in a fixed order, skipping any that are empty or not safe.
export function linkEntries(links: ToolLinks | undefined): LinkEntry[] {
  const out: LinkEntry[] = []
  for (const [key, label] of LINK_LABELS) {
    const url = links?.[key]
    if (!url) continue
    const safe = safeLink(url, key === 'security')
    if (safe) out.push({ key, label, url, host: safe.host, mail: safe.mail })
  }
  return out
}

// An author's own address: a mailto link for the email, an https link for the page, each only if safe.
export function authorLinks(author: ToolAuthor): { email: string | null; url: { href: string; host: string } | null } {
  const email = author.email && /^[^\s@<>"]+@[^\s@<>"]+\.[^\s@<>"]+$/.test(author.email) ? author.email : null
  const safe = author.url ? safeLink(author.url, false) : null
  return { email, url: author.url && safe ? { href: author.url, host: safe.host } : null }
}

// What the search box matches on an installed tool: its name and slug (as before) and who made it.
export function searchText(name: string, slug: string, meta: ToolMetadata | null | undefined): string {
  const parts = [name, slug]
  for (const a of meta?.authors ?? []) parts.push(a.name)
  if (meta?.company) parts.push(meta.company.name)
  if (meta?.license) parts.push(meta.license)
  if (meta?.category) parts.push(meta.category)
  parts.push(...(meta?.keywords ?? []))
  return parts.join('\n').toLowerCase()
}

export function matchesQuery(haystack: string, query: string): boolean {
  const q = query.trim().toLowerCase()
  return q === '' || haystack.includes(q)
}

export function categoryLabel(category: string | undefined): string {
  if (!category) return ''
  return category.charAt(0).toUpperCase() + category.slice(1)
}

// "31 Oct 2026" from a date or an RFC 3339 timestamp, or '' when it cannot be read.
export function generatedLabel(value: string | undefined): string {
  if (!value) return ''
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(value)
  if (!m) return ''
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  const month = months[Number(m[2]) - 1]
  return month ? `${Number(m[3])} ${month} ${m[1]}` : ''
}
