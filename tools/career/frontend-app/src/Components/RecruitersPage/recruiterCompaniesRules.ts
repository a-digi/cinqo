// Pure rules for the companies of a recruiter (plan/ai/tools/career/step-169-recruiters-many-companies.md, step 3) and for the
// freelancer choice (plan/ai/tools/career/step-171-freelancer-recruiters.md, step 3): a recruiter works for zero or more companies,
// or is a freelancer (attached to none, on purpose). The new-recruiter form needs a company or Freelancer; the list shows the chips
// (or "Freelancer", or "No company") and sends only what changed. No React, so they can be script-checked.

export interface CompanyRef {
  id: string
  name: string
}

export const NO_COMPANY_LABEL = 'No company'
export const FREELANCER_LABEL = 'Freelancer'

// The companies in the order they are given, without blanks and repeats.
export function cleanIds(ids: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of ids) {
    const id = raw.trim()
    if (id === '' || seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  return out
}

// The start value of the new-recruiter form: the company the list was filtered by (?companyId=), else none.
export function initialCompanyIds(search: string): string[] {
  return cleanIds([new URLSearchParams(search).get('companyId') ?? ''])
}

// ...and whether it starts as a freelancer: when the list was filtered by the freelancers (?freelancer=true).
export function initialFreelancer(search: string): boolean {
  return new URLSearchParams(search).get('freelancer') === 'true'
}

// The form's own rule: a new recruiter is a freelancer or is tied to at least one company (the data model allows neither, the form
// does not).
export function kindError(freelancer: boolean, ids: string[]): string {
  return freelancer || cleanIds(ids).length > 0 ? '' : 'Pick at least one company, or choose Freelancer'
}

// Two selections are the same set when they hold the same companies, in any order.
export function sameCompanySet(a: string[], b: string[]): boolean {
  const x = cleanIds(a)
  const y = new Set(cleanIds(b))
  return x.length === y.size && x.every((id) => y.has(id))
}

// What an edit sends: the new set when it differs from the saved one, otherwise nothing (undefined leaves the companies alone).
export function companiesToSend(saved: string[], edited: string[]): string[] | undefined {
  return sameCompanySet(saved, edited) ? undefined : cleanIds(edited)
}

// Where the form returns to after creating: the freelancers' list for a freelancer, the filtered list when exactly one company was
// chosen, else the whole list. The result is a list state for buildRecruitersSearch.
export function listStateAfterCreate(freelancer: boolean, ids: string[]): { companyId: string; freelancer: boolean } {
  if (freelancer) return { companyId: '', freelancer: true }
  const clean = cleanIds(ids)
  return { companyId: clean.length === 1 ? clean[0] : '', freelancer: false }
}

// What an edit sends (plan step-171): only what changed. Becoming a freelancer sends the flag alone (the server removes the companies);
// a freelancer who stays one sends nothing; a freelancer who becomes a company recruiter sends false together with the companies in
// the same request; a company recruiter sends the new set when it differs.
export interface KindState {
  freelancer: boolean
  companyIds: string[]
}

export function kindToSend(saved: KindState, edited: KindState): { freelancer?: boolean; companyIds?: string[] } {
  if (edited.freelancer) return saved.freelancer ? {} : { freelancer: true }
  if (saved.freelancer) return { freelancer: false, companyIds: cleanIds(edited.companyIds) }
  const companyIds = companiesToSend(saved.companyIds, edited.companyIds)
  return companyIds === undefined ? {} : { companyIds }
}

// What a recruiter's row shows: the chips, a "Freelancer" marker, or one grey "No company" marker.
export function rowKind(r: { freelancer: boolean; companies: CompanyRef[] }): 'freelancer' | 'companies' | 'none' {
  if (r.freelancer) return 'freelancer'
  return r.companies.length > 0 ? 'companies' : 'none'
}

export function companyChips(companies: CompanyRef[]): { id: string; label: string }[] {
  return companies.map((c) => ({ id: c.id, label: c.name }))
}
