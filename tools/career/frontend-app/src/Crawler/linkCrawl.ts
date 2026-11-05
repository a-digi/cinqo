// linkCrawl.ts (plan 185) — the "link crawler" (jobs found by URL pattern, without AI): its availability for a portal link, and the
// calls the instruct screen makes. The backend decides and enforces everything (the Browser tool's selected engine must be Katana, the
// link must not need a login, the page must be on the link's own site); this file only asks and applies the answers.
import type { LinkPagination, LinkRule } from '../api'

const LINK_CRAWL_BASE = '/api/v1/tools/career/proxy/portal-links/link-crawl'

export interface LinkCrawlAvailability {
  available: boolean
  // 'available', 'login_link', 'katana_not_selected' or 'engine_unknown'
  reason: string
  message: string
}

// The feature is shown ONLY for an explicit "available: true". While the answer is loading, when it could not be read, or for any
// other value, nothing of the feature is shown.
export function showLinkCrawl(answer: LinkCrawlAvailability | null | undefined): boolean {
  return answer?.available === true
}

export async function fetchLinkCrawlAvailability(portalLinkId: string, signal?: AbortSignal): Promise<LinkCrawlAvailability> {
  const res = await fetch(`${LINK_CRAWL_BASE}/availability?portalLinkId=${encodeURIComponent(portalLinkId)}`, {
    credentials: 'include',
    cache: 'no-store',
    signal,
  })
  if (!res.ok) throw new Error(`failed to read the link crawler availability (${res.status})`)
  return (await res.json()) as LinkCrawlAvailability
}

// --- the instruct screen's data ---

// One distinct address of the page with what the page says about it (Browser /fetch-links).
export interface PageLink {
  url: string
  texts?: string[]
  titleAttr?: string
  rel?: string
}

export interface PageLinks {
  url: string
  finalUrl: string
  title: string
  links: PageLink[]
  truncated: boolean
  droppedPrivate: number
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
export function fetchLinkCandidates(portalLinkId: string, url?: string, signal?: AbortSignal): Promise<PageLinks> {
  return call<PageLinks>('candidates', 'POST', { portalLinkId, ...(url ? { url } : {}) }, 'load the links of the page', signal)
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
  crawlMethod: 'instructions' | 'links'
  rules: LinkRule | null
}

// Store the rule (the method stays what it is) or, with rules === null, remove it.
export function saveLinkRules(
  portalLinkId: string,
  crawlMethod: 'instructions' | 'links',
  rules: LinkRule | null,
): Promise<LinkCrawlSettings> {
  return call<LinkCrawlSettings>('rules', 'PUT', { portalLinkId, crawlMethod, rules }, 'save the rule')
}
