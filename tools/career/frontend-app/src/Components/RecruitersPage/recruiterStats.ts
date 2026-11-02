// Pure numbers and display rules for the Recruiters page. No React, so they can be
// script-checked.
export interface RecruiterLike {
  companyIds: string[]
  // A freelancer has no company on purpose (plan step-171): counted as a freelancer, never as "without a company".
  freelancer: boolean
  firstName: string
  lastName: string
  email: string
}

export interface RecruiterSummary {
  recruiters: number
  withEmail: number
  withoutEmail: number
  // How many different companies the listed recruiters work for, how many are freelancers, and how many have no company and are
  // not freelancers (e.g. their last company was deleted).
  companies: number
  freelancers: number
  withoutCompany: number
}

export function summarizeRecruiters(recruiters: RecruiterLike[]): RecruiterSummary {
  const companies = new Set<string>()
  let withEmail = 0
  let withoutCompany = 0
  let freelancers = 0
  for (const r of recruiters) {
    for (const id of r.companyIds) companies.add(id)
    if (r.freelancer) freelancers++
    else if (r.companyIds.length === 0) withoutCompany++
    if (r.email.trim() !== '') withEmail++
  }
  return {
    recruiters: recruiters.length,
    withEmail,
    withoutEmail: recruiters.length - withEmail,
    companies: companies.size,
    freelancers,
    withoutCompany,
  }
}

// "First Last", or "(unnamed)" when a recruiter has neither name.
export function recruiterName(r: Pick<RecruiterLike, 'firstName' | 'lastName'>): string {
  const name = `${r.firstName} ${r.lastName}`.trim()
  return name === '' ? '(unnamed)' : name
}
