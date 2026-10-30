// Pure numbers for the Companies page's overview, worked out from the companies already
// loaded. No React, so they can be script-checked.
export interface CompanyLike {
  jobCount: number
  recruiterCount: number
}

export interface CompanySummary {
  companies: number
  // Companies that have at least one linked job / at least one recruiter.
  withJobs: number
  withRecruiters: number
  // Sums over all companies.
  jobs: number
  recruiters: number
}

export function summarizeCompanies(companies: CompanyLike[]): CompanySummary {
  const s: CompanySummary = { companies: companies.length, withJobs: 0, withRecruiters: 0, jobs: 0, recruiters: 0 }
  for (const c of companies) {
    if (c.jobCount > 0) s.withJobs++
    if (c.recruiterCount > 0) s.withRecruiters++
    s.jobs += c.jobCount
    s.recruiters += c.recruiterCount
  }
  return s
}

// "3 companies", "1 company".
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}

// A whole percent of part in total, or null when there is no total (so the ring shows a dash).
export function percentOrNull(part: number, total: number): number | null {
  return total <= 0 ? null : Math.round((part / total) * 100)
}
