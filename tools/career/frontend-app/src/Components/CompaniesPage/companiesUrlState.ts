// The Companies list in the address (step 137): the pure rules that turn an address into the list's state and a state back
// into its one canonical address.
//
//   /tools/career/companies?p=2&q=adesso&limit=50
//
// Reading is forgiving (a bad value falls back to the default, unknown parameters are ignored); writing is canonical (fixed
// order, defaults left out, the same state always gives the same address). So write(read(x)) is the canonical form of x,
// and read(write(s)) is s.
import { cleanText } from '../../Shared/AddressState/addressText'
import { cleanLimit as cleanLimitOr, cleanPage, limitFromText, MAX_PAGE_LIMIT, MIN_PAGE_LIMIT } from '../../Shared/AddressState/paging'

export { limitFromText }

// How many companies a page shows when the address says nothing, and the range the person may set by hand.
export const DEFAULT_COMPANIES_LIMIT = 25
export const MIN_COMPANIES_LIMIT = MIN_PAGE_LIMIT
export const MAX_COMPANIES_LIMIT = MAX_PAGE_LIMIT

export interface CompaniesListState {
  page: number
  query: string
  limit: number
}

export const DEFAULT_COMPANIES_STATE: CompaniesListState = { page: 1, query: '', limit: DEFAULT_COMPANIES_LIMIT }

// A limit read from the address or a state: itself when it is valid, otherwise the default.
function cleanLimit(raw: string | null | number): number {
  return cleanLimitOr(raw, DEFAULT_COMPANIES_LIMIT)
}

// Turns an address (with or without its leading "?") into the list's state.
export function parseCompaniesUrl(search: string): CompaniesListState {
  const params = new URLSearchParams(search)
  return { page: cleanPage(params.get('p')), query: cleanText(params.get('q')), limit: cleanLimit(params.get('limit')) }
}

// Brings a state to the form an address can carry. buildCompaniesSearch writes this form.
export function normalizeCompaniesState(s: CompaniesListState): CompaniesListState {
  return { page: cleanPage(s.page), query: cleanText(s.query), limit: cleanLimit(s.limit) }
}

// "" for the default list, otherwise "?" and the parameters in the fixed order p, q, limit, each only when it is not its
// default. A space is written as %20.
export function buildCompaniesSearch(state: CompaniesListState): string {
  const s = normalizeCompaniesState(state)
  const params = new URLSearchParams()
  if (s.page > 1) params.set('p', String(s.page))
  if (s.query !== '') params.set('q', s.query)
  if (s.limit !== DEFAULT_COMPANIES_LIMIT) params.set('limit', String(s.limit))
  const text = params.toString().replace(/\+/g, '%20')
  return text === '' ? '' : `?${text}`
}

// True when the address is already exactly what the page would write for the state it means.
export function isCanonicalCompaniesSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildCompaniesSearch(parseCompaniesUrl(search)) === withMark
}

// How many companies to skip to reach a page.
export function offsetForPage(page: number, limit: number): number {
  return (cleanPage(page) - 1) * cleanLimit(limit)
}
