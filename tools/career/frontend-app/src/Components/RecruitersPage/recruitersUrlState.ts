// The Recruiters list in the address (step 136): the pure rules that turn an address into the list's state and a state back
// into its one canonical address.
//
//   /tools/career/recruiters?companyId=<id>
//
// No parameter means every company's recruiters. Reading is forgiving (the first value of `companyId` counts, it is cleaned
// like every text in an address, unknown parameters are ignored); writing is canonical (the parameter only when it is set).
import { cleanText } from '../../Shared/AddressState/addressText'

export interface RecruitersListState {
  companyId: string
}

export const DEFAULT_RECRUITERS_STATE: RecruitersListState = { companyId: '' }

export function parseRecruitersUrl(search: string): RecruitersListState {
  return { companyId: cleanText(new URLSearchParams(search).get('companyId')) }
}

// "" for every company, otherwise "?companyId=<id>" (a space written as %20, anything else encoded).
export function buildRecruitersSearch(state: RecruitersListState): string {
  const id = cleanText(state.companyId)
  if (id === '') return ''
  return `?${new URLSearchParams({ companyId: id }).toString().replace(/\+/g, '%20')}`
}

// True when the address is already exactly what the page would write for the state it means.
export function isCanonicalRecruitersSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildRecruitersSearch(parseRecruitersUrl(search)) === withMark
}
