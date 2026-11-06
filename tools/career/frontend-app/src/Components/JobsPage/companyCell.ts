// What the Company column of the Jobs list shows for one job. A job always has a company *name* (what the crawl or
// the person typed), but only a job linked to a company *record* (companyId) has a page to open, so only that one
// gets a link. An unlinked job is never matched to a company by its name: the company page lists linked jobs, so
// it would not contain this one. See plan/ai/tools/career/step-99-company-links-on-the-jobs-list.md.

export interface CompanyCellJob {
  // May be absent: the backend leaves out the company of a job that has none.
  company?: string
  companyId?: string
}

export interface CompanyCell {
  // The name to show.
  text: string
  // The company page to open, or null for an unlinked job (plain text).
  linkTo: string | null
  // The second line "linked to …": the company record's name, only when it says something the text does not.
  // "…" while the companies have not loaded yet. null for an unlinked job or when the name is the same.
  linkedName: string | null
}

const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase()

// viewPath is the company page's path (VIEW_COMPANY_PATH); the id goes into its query string, encoded.
export function companyCell(job: CompanyCellJob, companyNames: Partial<Record<string, string>>, viewPath: string): CompanyCell {
  // The backend leaves the company out of a job that has none (a job found by the link crawler has only a title and an address), so it may be
  // absent although the type says a string; a job linked to a company record then shows the record's name.
  const company = job.company ?? ''
  if (!job.companyId) return { text: company, linkTo: null, linkedName: null }
  const record = companyNames[job.companyId]
  const text = company.trim() !== '' ? company : (record ?? '')
  return {
    text,
    linkTo: `${viewPath}?id=${encodeURIComponent(job.companyId)}`,
    linkedName: record === undefined ? '…' : same(record, text) ? null : record,
  }
}
