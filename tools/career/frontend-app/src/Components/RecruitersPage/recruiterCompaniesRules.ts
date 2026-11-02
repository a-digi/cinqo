// Pure rules for the companies of a recruiter (plan/ai/tools/career/step-169-recruiters-many-companies.md, step 3): a recruiter works
// for zero or more companies. The new-recruiter form needs at least one; the list shows the chips (or "No company") and sends a
// changed set only. No React, so they can be script-checked.

export interface CompanyRef {
  id: string
  name: string
}

export const NO_COMPANY_LABEL = 'No company'

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

// The form's own rule: a new recruiter is tied to at least one company (the data model allows none, the form does not).
export function companiesError(ids: string[]): string {
  return cleanIds(ids).length === 0 ? 'Pick at least one company' : ''
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

// Where the form returns to after creating: the filtered list when exactly one company was chosen, else the whole list.
export function listFilterAfterCreate(ids: string[]): string {
  const clean = cleanIds(ids)
  return clean.length === 1 ? clean[0] : ''
}

// What a recruiter's row shows: the chips, or one grey "No company" marker.
export function companyChips(companies: CompanyRef[]): { id: string; label: string }[] {
  return companies.map((c) => ({ id: c.id, label: c.name }))
}
