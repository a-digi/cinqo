// linkCrawlRules.ts (plan 185, step 5) — the pure rules of the instruct screen: no React, no network, so they can be script-checked.
import type { LinkPagination } from '../api'
import type { HostSummary, PageLink } from './linkCrawl'

// The text a person recognises a link by: its first text, else its title attribute, else nothing.
export function linkLabel(l: PageLink): string {
  return (l.texts?.find((t) => t.trim() !== '') ?? l.titleAttr ?? '').trim()
}

function hostOf(url: string): string {
  try {
    return new URL(url).host.toLowerCase()
  } catch {
    return ''
  }
}

// The address as it is shown: without the scheme and, for the link's own site, without the host.
export function shortUrl(url: string, siteHost: string): string {
  try {
    const u = new URL(url)
    const rest = `${u.pathname}${u.search}`
    return u.host.toLowerCase() === siteHost.toLowerCase() ? rest || '/' : `${u.host}${rest === '/' ? '' : rest}`
  } catch {
    return url
  }
}

// The links the person sees: by default only the link's own site (the only one a rule can be for), narrowed by what they type
// (matched against the address and the texts, case-insensitive, every word must match).
export function filterLinks(links: PageLink[], query: string, sameSiteOnly: boolean, siteHost: string, jobHost = ''): PageLink[] {
  const words = query
    .toLowerCase()
    .split(/\s+/)
    .filter((w) => w !== '')
  return links.filter((l) => {
    if (sameSiteOnly && hostOf(l.url) !== siteHost.toLowerCase() && (jobHost === '' || hostOf(l.url) !== jobHost.toLowerCase()))
      return false
    if (words.length === 0) return true
    const hay = `${l.url} ${(l.texts ?? []).join(' ')} ${l.titleAttr ?? ''}`.toLowerCase()
    return words.every((w) => hay.includes(w))
  })
}

export type MarkKind = 'picked' | 'notJob' | 'pages'

export interface Marks {
  picked: string[] // links that are jobs
  notJob: string[] // links that look like jobs but are not
  pages: string[] // links to the next page(s) of the listing
}

export const NO_MARKS: Marks = { picked: [], notJob: [], pages: [] }

// The most links one rule can be learned from (the backend refuses more, plan 185).
export const MAX_PICKED = 200

// Marks several links at once (a selection in the page, plan 247): each gets exactly this mark, or none for kind null; a link keeps one meaning. Ticking
// stops at MAX_PICKED; left counts the links that were not ticked for that reason.
export function setMarks(marks: Marks, urls: string[], kind: MarkKind | null): { marks: Marks; left: number } {
  const gone = new Set(urls)
  const next: Marks = {
    picked: marks.picked.filter((u) => !gone.has(u)),
    notJob: marks.notJob.filter((u) => !gone.has(u)),
    pages: marks.pages.filter((u) => !gone.has(u)),
  }
  if (kind === null) return { marks: next, left: 0 }
  const room = kind === 'picked' ? Math.max(0, MAX_PICKED - next.picked.length) : Infinity
  const add = [...new Set(urls)]
  next[kind] = [...next[kind], ...add.slice(0, room)]
  return { marks: next, left: Math.max(0, add.length - room) }
}

// A link has one meaning: marking it as a job, as not a job or as a next page removes its other marks; marking it again unmarks it.
export function toggleMark(marks: Marks, url: string, kind: MarkKind): Marks {
  const next: Marks = {
    picked: marks.picked.filter((u) => u !== url),
    notJob: marks.notJob.filter((u) => u !== url),
    pages: marks.pages.filter((u) => u !== url),
  }
  if (!marks[kind].includes(url)) next[kind] = [...next[kind], url]
  return next
}

export interface SaveState {
  pattern: string
  preview: { matched: number; needsConfirm: boolean } | null
  previewError: string
  proposalNeedsConfirm: boolean
  // The next-page part: its error, and whether its proposal needs a confirm.
  paginationError: string
  paginationNeedsConfirm: boolean
  confirmed: boolean
  busy: boolean
}

// Why the rule cannot be saved yet ('' = it can). The server checks everything again; this only keeps the person from a pointless call.
export function saveBlockedReason(s: SaveState): string {
  if (s.busy) return 'Working…'
  if (s.pattern.trim() === '') return 'Pick the job links first.'
  if (s.previewError !== '') return s.previewError
  if (s.paginationError !== '') return s.paginationError
  if (s.preview === null) return 'Checking the pattern…'
  if (s.preview.matched === 0) return 'This pattern matches no link on this page.'
  if ((s.proposalNeedsConfirm || s.preview.needsConfirm || s.paginationNeedsConfirm) && !s.confirmed)
    return 'Confirm that you checked the matches and the next pages.'
  return ''
}

// How a failure is told: the Browser's advice is added when the site stopped the page.
export function describeFailure(message: string, advice: string): string {
  return advice !== '' ? `${message} ${advice}` : message
}

export const MAX_PAGES_LIMIT = 50
export const DEFAULT_MAX_PAGES = 5

// The most pages a crawl loads, as the person typed it: a whole number from 1 to 50; anything else is not accepted.
export function parseMaxPages(text: string): number | null {
  const t = text.trim()
  if (!/^\d{1,3}$/.test(t)) return null
  const n = Number(t)
  return n >= 1 && n <= MAX_PAGES_LIMIT ? n : null
}

// How the pages follow each other, in words.
export function describePagination(p: LinkPagination | undefined): string {
  if (!p || p.kind === 'none') return 'Only the first page.'
  const max = p.maxPages && p.maxPages > 0 ? p.maxPages : DEFAULT_MAX_PAGES
  if (p.kind === 'template') {
    const step = p.step && p.step > 0 ? p.step : 1
    return `Page 2, 3, … are ${p.template ?? ''} with {n} = ${p.second ?? '?'}, ${(p.second ?? 0) + step}, … (at most ${max} pages).`
  }
  return `Follows the link of each page that matches ${p.pattern ?? ''} (at most ${max} pages).`
}

// Why a run of pages ended, in words.
export function stopReasonText(reason: string, detail?: string): string {
  switch (reason) {
    case 'max_pages':
      return 'stopped at the page limit'
    case 'no_next_page':
      return 'there is no next page'
    case 'no_new_jobs':
      return 'the last page had no new jobs'
    case 'next_page_already_visited':
      return 'the next page was already loaded'
    case 'fetch_failed':
      return `a page could not be loaded${detail ? `: ${detail}` : ''}`
    case 'left_the_site':
      return `a page ended up on another site${detail ? ` (${detail})` : ''}`
    case 'cancelled':
      return 'stopped'
    default:
      return reason
  }
}

// What the screen tells the person about the links of OTHER sites (plan 203). A rule can only be for the link's own site, so links on another
// host are hidden by default and cannot be picked. When the page's postings are on another site the person must be told, not left with a list
// that has no jobs in it. null when there is nothing to say.
export interface HiddenSitesNotice {
  // 'warn': the postings are on another site; 'info': other sites are hidden but they do not hold the postings.
  tone: 'warn' | 'info'
  text: string
}

export const MIN_JOB_LIKE = 3

export function hiddenSitesNotice(
  site: HostSummary | undefined,
  others: HostSummary[] | undefined,
  siteHost: string,
  sameSiteOnly: boolean,
): HiddenSitesNotice | null {
  others = others?.filter((h) => !h.allowed) // the confirmed job site is part of the site now
  if (!others || others.length === 0) return null
  const top = others[0]
  const ownJobLike = site?.jobLike ?? 0
  if (top.jobLike >= MIN_JOB_LIKE && top.jobLike > ownJobLike) {
    const sample = top.shape ? ` (addresses like ${top.shape})` : ''
    return {
      tone: 'warn',
      text:
        `The job links of this page look to be on another site: ${top.host}, ${top.jobLike} similar links${sample}. ` +
        `A rule can only be for this link's own site (${siteHost}), so those links cannot be picked here. ` +
        `If ${top.host} shows the same list, you can confirm it as this link's job site.`,
    }
  }
  if (!sameSiteOnly) return null
  const hidden = others.reduce((n, h) => n + h.links, 0)
  const names = others
    .slice(0, 2)
    .map((h) => h.host)
    .join(', ')
  const more = others.length > 2 ? ` and ${others.length - 2} more` : ''
  return {
    tone: 'info',
    text: `${hidden} link${hidden === 1 ? '' : 's'} on other sites ${hidden === 1 ? 'is' : 'are'} hidden (${names}${more}). A rule can only be for this link's own site.`,
  }
}

// The question asked before a job site is confirmed (plan 203). A rule will then read job links on that site, so the person must know what
// they are trusting.
export function jobHostConfirmText(jobHost: string, siteHost: string): string {
  return (
    `Use ${jobHost} for this link's job links? Crawls of this link will then also take job links from ${jobHost} (and read their job pages), ` +
    `not only from ${siteHost}. The list pages stay on ${siteHost}. Only confirm a site you know belongs to the same job board.`
  )
}
