// Pure numbers and display rules for the Recruiters page. No React, so they can be
// script-checked.
export interface RecruiterLike {
  companyId: string
  firstName: string
  lastName: string
  email: string
}

export interface RecruiterSummary {
  recruiters: number
  withEmail: number
  withoutEmail: number
  // How many different companies the listed recruiters belong to.
  companies: number
}

export function summarizeRecruiters(recruiters: RecruiterLike[]): RecruiterSummary {
  const companies = new Set<string>()
  let withEmail = 0
  for (const r of recruiters) {
    companies.add(r.companyId)
    if (r.email.trim() !== '') withEmail++
  }
  return { recruiters: recruiters.length, withEmail, withoutEmail: recruiters.length - withEmail, companies: companies.size }
}

// "First Last", or "(unnamed)" when a recruiter has neither name.
export function recruiterName(r: Pick<RecruiterLike, 'firstName' | 'lastName'>): string {
  const name = `${r.firstName} ${r.lastName}`.trim()
  return name === '' ? '(unnamed)' : name
}
