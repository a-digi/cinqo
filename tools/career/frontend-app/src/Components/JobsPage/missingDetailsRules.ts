// The words of the "Crawl missing details" button on the Jobs page: its label and its tooltip, from what the server says about the jobs
// that have no crawled details. Pure (no React, no network), so it can be checked on its own.
import { plural } from '../CompaniesPage/companyStats'
import type { MissingDetails } from '../../api'

export function missingLabel(m: MissingDetails | null, busy: boolean): string {
  if (busy) return 'Crawling details…'
  if (m === null || m.jobIds.length === 0) return 'Crawl missing details'
  return `Crawl missing details (${String(m.jobIds.length)})`
}

// Why some jobs are left out, as a sentence; empty when none is.
export function leftOutText(m: MissingDetails): string {
  const parts: string[] = []
  if (m.noInstructions > 0) parts.push(`${plural(m.noInstructions, 'job', 'jobs')} whose platform has no job detail instructions yet`)
  if (m.needsLogin > 0)
    parts.push(`${plural(m.needsLogin, 'job', 'jobs')} of a platform that needs a login (crawl those on the platform's link)`)
  if (m.noPortal > 0) parts.push(`${plural(m.noPortal, 'job', 'jobs')} without a platform or a posting address`)
  if (m.alreadyBeingCrawled > 0) parts.push(`${plural(m.alreadyBeingCrawled, 'job', 'jobs')} already waiting to be crawled`)
  return parts.length === 0 ? '' : `Left out: ${parts.join('; ')}.`
}

export function missingTitle(m: MissingDetails | null, busy: boolean): string {
  if (busy) return 'A crawl of job details is already running; its progress is shown below.'
  if (m === null)
    return 'Crawl the detail page of every job whose details were never crawled. It starts at once, whatever other crawl is running.'
  const left = leftOutText(m)
  if (m.jobIds.length === 0) return left === '' ? 'Every job already has its details crawled.' : `No job can be crawled now. ${left}`
  const base = `Crawl the detail page of ${plural(m.jobIds.length, 'job', 'jobs')} whose details were never crawled, or whose last try failed. It starts at once, whatever other crawl is running.`
  return left === '' ? base : `${base} ${left}`
}
