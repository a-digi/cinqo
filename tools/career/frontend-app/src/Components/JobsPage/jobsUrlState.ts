// The Jobs list in the address (step 135): the pure rules that turn an address into the list's state and a state back
// into its one canonical address, kept apart from the page so they can be checked on their own.
//
//   /tools/career/jobs?p=2&q=golang&location=Hamburg&companyId=A&companyId=B&portalId=P&sort=match
//
// Reading is forgiving (a bad value falls back to the default, unknown parameters are ignored); writing is canonical
// (fixed order, defaults left out, the same state always gives the same address). So write(read(x)) is the canonical
// form of x, and read(write(s)) is s.
import type { JobsSort } from '../../api'

export interface JobsListState {
  page: number
  query: string
  location: string
  companyIds: string[]
  portalId: string
  sort: JobsSort
}

export const DEFAULT_JOBS_STATE: JobsListState = { page: 1, query: '', location: '', companyIds: [], portalId: '', sort: 'newest' }

// The longest search text and location kept (in characters), and the most companies in one address.
export const MAX_TEXT_LENGTH = 200
export const MAX_COMPANIES = 50
// Digits accepted for a page number; more than this is not a page, it is the default.
const MAX_PAGE_DIGITS = 7

// Control characters become a space, the ends are trimmed, and the text is cut at MAX_TEXT_LENGTH characters.
function cleanText(raw: string | null): string {
  if (raw === null) return ''
  // eslint-disable-next-line no-control-regex
  const spaced = raw.replace(/[\u0000-\u001F\u007F]+/g, ' ').trim()
  return Array.from(spaced).slice(0, MAX_TEXT_LENGTH).join('').trim()
}

function cleanPage(raw: string | null | number): number {
  const text = typeof raw === 'number' ? (Number.isInteger(raw) ? String(raw) : '') : (raw ?? '').trim()
  if (!new RegExp(`^[0-9]{1,${String(MAX_PAGE_DIGITS)}}$`).test(text)) return 1
  return Math.max(1, Number(text))
}

// Trimmed, no empty values, no repeats (the first one wins), at most MAX_COMPANIES.
function cleanIds(raw: string[]): string[] {
  const out: string[] = []
  for (const value of raw) {
    const id = cleanText(value)
    if (id !== '' && !out.includes(id)) out.push(id)
    if (out.length === MAX_COMPANIES) break
  }
  return out
}

// Turns an address (the part after the path, with or without its leading "?") into the list's state.
export function parseJobsUrl(search: string): JobsListState {
  const params = new URLSearchParams(search)
  return {
    page: cleanPage(params.get('p')),
    query: cleanText(params.get('q')),
    location: cleanText(params.get('location')),
    companyIds: cleanIds(params.getAll('companyId')),
    portalId: cleanText(params.get('portalId')),
    sort: params.get('sort') === 'match' ? 'match' : 'newest',
  }
}

// Brings a state to the form an address can carry: the page a whole number of at least 1, texts cleaned, companies
// cleaned. buildJobsSearch writes this form.
export function normalizeJobsState(s: JobsListState): JobsListState {
  return {
    page: cleanPage(s.page),
    query: cleanText(s.query),
    location: cleanText(s.location),
    companyIds: cleanIds(s.companyIds),
    portalId: cleanText(s.portalId),
    sort: s.sort === 'match' ? 'match' : 'newest',
  }
}

// The canonical address of a state: "" for the default list, otherwise "?" and the parameters in the fixed order p, q,
// location, companyId (repeated), portalId, sort, each only when it is not its default. A space is written as %20.
export function buildJobsSearch(state: JobsListState): string {
  const s = normalizeJobsState(state)
  const params = new URLSearchParams()
  if (s.page > 1) params.set('p', String(s.page))
  if (s.query !== '') params.set('q', s.query)
  if (s.location !== '') params.set('location', s.location)
  for (const id of s.companyIds) params.append('companyId', id)
  if (s.portalId !== '') params.set('portalId', s.portalId)
  if (s.sort === 'match') params.set('sort', 'match')
  const text = params.toString().replace(/\+/g, '%20')
  return text === '' ? '' : `?${text}`
}

// The page to show once the total is known: never before the first, never past the last (a list of nothing has one page).
export function clampPage(page: number, total: number, pageSize: number): number {
  const pages = Math.max(1, Math.ceil(Math.max(0, total) / Math.max(1, pageSize)))
  return Math.min(Math.max(1, cleanPage(page)), pages)
}

// True when the address is already exactly what the page would write for the state it means (so nothing needs rewriting).
export function isCanonicalJobsSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildJobsSearch(parseJobsUrl(search)) === withMark
}

// The same list: two states that write the same address.
export function sameJobsState(a: JobsListState, b: JobsListState): boolean {
  return buildJobsSearch(a) === buildJobsSearch(b)
}

// Whether a state carries a filter of the filter panel (location, companies, platform); the search and the order are not
// in that panel. A filter that arrives pre-set pre-opens the panel.
export function hasJobsFilters(s: JobsListState): boolean {
  return s.location !== '' || s.companyIds.length > 0 || s.portalId !== ''
}
