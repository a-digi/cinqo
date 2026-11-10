// linkCrawl.ts (plan 185) — the "link crawler" (jobs found by URL pattern, without AI): the calls the rules screen makes. The backend decides
// and enforces everything (the Browser tool's selected engine must be Katana, the link must not need a login, the page must be on the link's
// own site); this file only asks and applies the answers.
import type { LinkPagination, LinkRule } from '../api'

const LINK_CRAWL_BASE = '/api/v1/tools/career/proxy/portal-links/link-crawl'

// --- the instruct screen's data ---

// One distinct address of the page with what the page says about it (Browser /fetch-links).
export interface PageLink {
  url: string
  texts?: string[]
  titleAttr?: string
  rel?: string
  // plan 247: the numbers of this link's <a> elements in the page copy (data-cinqo-a); present only when a copy was asked for.
  anchors?: number[]
}

// plan 247: the page copy the Browser tool kept for the picker to show (see PageView).
export interface PageSnapshot {
  id: string
  bytes: number
  createdAt: string
  truncated?: boolean
}

// How many of the page's links are on one host and how many of them look like postings (the biggest group of links with the same address
// shape whose last part varies, at least three): what the screen needs to say which links it hides (plan 203).
export interface HostSummary {
  host: string
  links: number
  jobLike: number
  shape?: string
  sample?: string
  // true for the link's confirmed job site
  allowed?: boolean
}

export interface PageLinks {
  url: string
  finalUrl: string
  title: string
  links: PageLink[]
  truncated: boolean
  droppedPrivate: number
  // The link's own site and the other hosts the links point to, the one with the most posting-like links first. Absent from an older Career
  // backend, so the screen treats both as optional.
  site?: HostSummary
  otherHosts?: HostSummary[]
  jobHost?: string
  // plan 247: the copy of the page, when one was asked for and made; snapshotError says why not (the list is good either way).
  snapshot?: PageSnapshot
  snapshotError?: string
}

export interface Proposal {
  pattern: string
  host: string
  exclude?: string[]
  matches: string[]
  matchCount: number
  unmatched?: string[]
  excludedStillMatched?: string[]
  needsConfirm: boolean
  notes?: string[]
  rule: LinkRule
}

export interface RuleItem {
  title: string
  url: string
}

export interface RulePreview {
  items: RuleItem[]
  matched: number
  skippedNoTitle: number
  skippedSamples?: string[]
  otherHost: number
  needsConfirm: boolean
  notes?: string[]
}

// What one loaded page of a test run gave.
export interface TestPage {
  url: string
  finalUrl: string
  title: string
  links: number
  matched: number
  newJobs: number
  items: number
}

// A test run: the pages it loaded (at most two), the jobs found over them and why it stopped.
export interface RuleTest {
  url: string
  pages: TestPage[]
  items: RuleItem[]
  matched: number
  skippedNoTitle: number
  skippedSamples?: string[]
  otherHost: number
  stopReason: string
  stopDetail?: string
  notes?: string[]
}

// How the pages follow each other, as worked out from the picked next-page link(s).
export interface PaginationProposal {
  pagination: LinkPagination
  nextUrls: string[]
  needsConfirm: boolean
  notes?: string[]
}

// An error the backend explained: its message, its code ('katana_not_selected', 'cloudflare_challenge_unresolved', ...) and, for a
// site that stopped the page, the Browser's advice.
export class LinkCrawlError extends Error {
  readonly status: number
  readonly code: string
  readonly advice: string
  constructor(message: string, status: number, code = '', advice = '') {
    super(message)
    this.name = 'LinkCrawlError'
    this.status = status
    this.code = code
    this.advice = advice
  }
}

async function readError(res: Response, action: string): Promise<LinkCrawlError> {
  const text = (await res.text()).trim()
  try {
    const j = JSON.parse(text) as { code?: string; message?: string; advice?: string }
    if (j.message) return new LinkCrawlError(j.message, res.status, j.code ?? '', j.advice ?? '')
  } catch {
    // plain text
  }
  return new LinkCrawlError(text || `failed to ${action} (${res.status})`, res.status)
}

async function call<T>(path: string, method: 'POST' | 'PUT' | 'GET', body: unknown, action: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${LINK_CRAWL_BASE}/${path}`, {
    method,
    credentials: 'include',
    cache: 'no-store',
    signal,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) throw await readError(res, action)
  return (await res.json()) as T
}

// Load the links of the link's page (or of another page of its site).
// With snapshot the Browser tool also keeps a safe copy of the page for the picker to show; it is deleted when the picker closes.
export function fetchLinkCandidates(
  portalLinkId: string,
  url?: string,
  signal?: AbortSignal,
  options?: { snapshot?: boolean },
): Promise<PageLinks> {
  return call<PageLinks>(
    'candidates',
    'POST',
    { portalLinkId, ...(url ? { url } : {}), ...(options?.snapshot ? { snapshot: true } : {}) },
    'load the links of the page',
    signal,
  )
}

// plan 247: the page copy as html (for a sandboxed iframe), and its removal (also fine when it is already gone).
export async function fetchSnapshotHtml(id: string, signal?: AbortSignal): Promise<string> {
  const res = await fetch(`${LINK_CRAWL_BASE}/snapshot?id=${encodeURIComponent(id)}`, { credentials: 'include', cache: 'no-store', signal })
  if (!res.ok) throw await readError(res, 'show the page')
  return res.text()
}

// plan 247: how many temporary page files the Browser tool holds, and a way to delete all of them.
export interface SnapshotStats {
  count: number
  bytes: number
}

export async function fetchSnapshotStats(): Promise<SnapshotStats> {
  const res = await fetch(`${LINK_CRAWL_BASE}/snapshots`, { credentials: 'include', cache: 'no-store' })
  if (!res.ok) throw await readError(res, 'count the temporary page files')
  return (await res.json()) as SnapshotStats
}

export async function deleteAllSnapshots(): Promise<number> {
  const res = await fetch(`${LINK_CRAWL_BASE}/snapshots`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok) throw await readError(res, 'delete the temporary page files')
  return ((await res.json()) as { deleted: number }).deleted
}

export async function deleteSnapshot(id: string): Promise<void> {
  const res = await fetch(`${LINK_CRAWL_BASE}/snapshot?id=${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'include' })
  if (!res.ok && res.status !== 404) throw await readError(res, 'delete the page copy')
}

// The picked links -> the proposed partial URL (nothing is loaded or saved).
export function learnLinkPattern(
  portalLinkId: string,
  picked: string[],
  notJob: string[],
  candidates: PageLink[],
  signal?: AbortSignal,
): Promise<Proposal> {
  return call<Proposal>('learn', 'POST', { portalLinkId, picked, notJob, candidates }, 'work out the pattern', signal)
}

// The picked next-page link(s) -> how the pages follow each other (nothing is loaded or saved).
export function learnLinkPagination(
  portalLinkId: string,
  picked: string[],
  candidates: PageLink[],
  currentUrl?: string,
  signal?: AbortSignal,
): Promise<PaginationProposal> {
  return call<PaginationProposal>(
    'learn-pagination',
    'POST',
    { portalLinkId, picked, candidates, ...(currentUrl ? { currentUrl } : {}) },
    'work out the next pages',
    signal,
  )
}

// What a rule finds among the given links (nothing is loaded or saved).
export function previewLinkRule(portalLinkId: string, rules: LinkRule, candidates: PageLink[], signal?: AbortSignal): Promise<RulePreview> {
  return call<RulePreview>('preview', 'POST', { portalLinkId, rules, candidates }, 'check the rule', signal)
}

// Load the page now and run the rule on it (nothing is saved).
export function testLinkRule(portalLinkId: string, rules: LinkRule, url?: string, signal?: AbortSignal): Promise<RuleTest> {
  return call<RuleTest>('test', 'POST', { portalLinkId, rules, ...(url ? { url } : {}) }, 'test the rule', signal)
}

export interface LinkCrawlSettings {
  rules: LinkRule | null
}

// Store the rule (rules === null removes it). The Browser's engine decides how a link is crawled (plan 190), so no method is sent.
export function saveLinkRules(portalLinkId: string, rules: LinkRule | null): Promise<LinkCrawlSettings> {
  return call<LinkCrawlSettings>('rules', 'PUT', { portalLinkId, rules }, 'save the rule')
}

// plan 198: the person's answer "I cleared the Cloudflare check in the browser window" for this link's site. The Browser then reads that
// site through the window; the screen reads again afterwards.
// The card's "Keep Katana in the visible browser": the Browser opens its Katana window at the link's address for the person to clear a check.
export function openKatanaWindow(portalLinkId: string): Promise<{ ok: boolean }> {
  return call<{ ok: boolean }>('window-open', 'POST', { portalLinkId }, 'open the browser window')
}

export const WINDOW_CHECK_CODE = 'human_check_requested'

export function confirmWindowCleared(portalLinkId: string, url?: string): Promise<{ ok: boolean }> {
  return call<{ ok: boolean }>(
    'window-cleared',
    'POST',
    { portalLinkId, ...(url ? { url } : {}) },
    'tell the Browser the check was cleared',
  )
}
