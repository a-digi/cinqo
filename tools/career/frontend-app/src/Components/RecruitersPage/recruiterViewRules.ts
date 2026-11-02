// Pure rules of the recruiter view page (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 6): the date of a mapping, the line
// under a job, the pill's text and the list after one job was removed. No React, so they can be script-checked.
import type { RecruiterJob } from '../../api'

// "2 Nov 2026"-style date of a stored UTC time ("2026-11-02 10:00:00", the way SQLite writes it); '' when it cannot be read.
export function formatDay(value: string): string {
  const trimmed = value.trim()
  if (trimmed === '') return ''
  // A stored time has a space and no zone: read it as UTC (a plain `new Date` of it is not portable across browsers).
  const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(trimmed) ? `${trimmed.replace(' ', 'T')}Z` : trimmed
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString()
}

// The line under a job's title: company and location, whichever there are.
export function jobMeta(job: Pick<RecruiterJob, 'company' | 'location'>): string {
  return [job.company ?? '', job.location ?? '']
    .map((part) => part.trim())
    .filter((part) => part !== '')
    .join(' · ')
}

// "No mapped jobs", "1 mapped job", "5 mapped jobs": the header pill.
export function mappedJobsPill(count: number): string {
  if (count === 0) return 'No mapped jobs'
  return count === 1 ? '1 mapped job' : `${count} mapped jobs`
}

// The jobs without one (a new array; the others keep their order).
export function withoutJob(list: readonly RecruiterJob[], jobId: string): RecruiterJob[] {
  return list.filter((j) => j.id !== jobId)
}
