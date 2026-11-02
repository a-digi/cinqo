// Pure rules for choosing a recruiter to map to a job (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 3): the label of a
// recruiter, the list the modal shows (the job's own company first, then by company and name), the search over it, which rows can be
// picked and what to say when there is nothing to show. No React, so they can be script-checked.
import type { Recruiter } from '../../api'

export interface RecruiterChoice {
  id: string
  label: string
  email: string
  // The names of every company the recruiter works for, "Acme, Beta" (empty when they have none).
  companyName: string
  // The recruiter works for the company of the job being mapped (any one of their companies counts).
  sameCompany: boolean
  // Already mapped to this job: shown, but not pickable.
  mapped: boolean
}

// "First Last", else the e-mail address, else a plain placeholder.
export function recruiterLabel(r: { firstName: string; lastName: string; email: string }): string {
  const name = `${r.firstName.trim()} ${r.lastName.trim()}`.trim()
  if (name !== '') return name
  if (r.email.trim() !== '') return r.email.trim()
  return '(unnamed recruiter)'
}

// "Acme, Beta": the names of a recruiter's companies in the order they were linked, without blanks ('' when there are none).
export function companyNamesText(companies: readonly { name: string }[]): string {
  return companies
    .map((c) => c.name.trim())
    .filter((name) => name !== '')
    .join(', ')
}

const byText = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' })

// Every recruiter as a choice: the ones who work for the job's company (any of their companies) first, then the others by company
// names, last name, first name (and id, so the order never depends on how the list arrived). A recruiter with no company has an empty
// company text.
export function buildChoices(
  recruiters: readonly Recruiter[],
  mappedIds: readonly string[],
  jobCompanyId: string | undefined,
): RecruiterChoice[] {
  const mapped = new Set(mappedIds)
  return recruiters
    .map((r) => ({
      sort: r,
      choice: {
        id: r.id,
        label: recruiterLabel(r),
        email: r.email,
        companyName: companyNamesText(r.companies),
        sameCompany: jobCompanyId !== undefined && jobCompanyId !== '' && r.companyIds.includes(jobCompanyId),
        mapped: mapped.has(r.id),
      } satisfies RecruiterChoice,
    }))
    .sort((a, b) => {
      if (a.choice.sameCompany !== b.choice.sameCompany) return a.choice.sameCompany ? -1 : 1
      return (
        byText(a.choice.companyName, b.choice.companyName) ||
        byText(a.sort.lastName, b.sort.lastName) ||
        byText(a.sort.firstName, b.sort.firstName) ||
        byText(a.choice.id, b.choice.id)
      )
    })
    .map((x) => x.choice)
}

// The search: every word of the query must appear in the name, the e-mail or the company name (case does not matter).
export function filterChoices(choices: readonly RecruiterChoice[], query: string): RecruiterChoice[] {
  const words = query
    .toLowerCase()
    .split(/\s+/)
    .filter((w) => w !== '')
  if (words.length === 0) return [...choices]
  return choices.filter((c) => {
    const haystack = `${c.label} ${c.email} ${c.companyName}`.toLowerCase()
    return words.every((w) => haystack.includes(w))
  })
}

// The choice with this id when it can be picked (it exists in the list and is not mapped yet), else null.
export function pickable(choices: readonly RecruiterChoice[], id: string | null): RecruiterChoice | null {
  if (id === null) return null
  const choice = choices.find((c) => c.id === id)
  return choice && !choice.mapped ? choice : null
}

export type ModalState = 'no-recruiters' | 'all-mapped' | 'no-match' | 'list'

// What the modal shows: no recruiter exists at all, every recruiter is already mapped, the search matches nothing, or the list.
export function modalState(choices: readonly RecruiterChoice[], visible: readonly RecruiterChoice[], query: string): ModalState {
  if (choices.length === 0) return 'no-recruiters'
  if (choices.every((c) => c.mapped)) return 'all-mapped'
  if (visible.length === 0 && query.trim() !== '') return 'no-match'
  return 'list'
}
