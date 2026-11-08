// The Recruiters list in the address (step 136, plan step-171): the pure rules that turn an address into the list's state and a state
// back into its one canonical address.
//
//   /tools/career/recruiters?companyId=<id>      the recruiters of one company
//   /tools/career/recruiters?freelancer=true     the freelancers
//
// No parameter means every recruiter. Reading is forgiving (the first value counts, it is cleaned like every text in an address,
// unknown parameters are ignored, `freelancer` wins over `companyId`: a freelancer works for no company); writing is canonical (one
// parameter, only when it is set).
import { cleanText } from '../../Shared/AddressState/addressText'

export interface RecruitersListState {
  companyId: string
  freelancer: boolean
}

export const DEFAULT_RECRUITERS_STATE: RecruitersListState = { companyId: '', freelancer: false }

export function parseRecruitersUrl(search: string): RecruitersListState {
  const params = new URLSearchParams(search)
  if (cleanText(params.get('freelancer')) === 'true') return { companyId: '', freelancer: true }
  return { companyId: cleanText(params.get('companyId')), freelancer: false }
}

// "" for every recruiter, "?freelancer=true", or "?companyId=<id>" (a space written as %20, anything else encoded).
export function buildRecruitersSearch(state: RecruitersListState): string {
  if (state.freelancer) return '?freelancer=true'
  const id = cleanText(state.companyId)
  if (id === '') return ''
  return `?${new URLSearchParams({ companyId: id }).toString().replace(/\+/g, '%20')}`
}

// True when the address is already exactly what the page would write for the state it means.
export function isCanonicalRecruitersSearch(search: string): boolean {
  const withMark = search !== '' && !search.startsWith('?') ? `?${search}` : search
  return buildRecruitersSearch(parseRecruitersUrl(search)) === withMark
}
