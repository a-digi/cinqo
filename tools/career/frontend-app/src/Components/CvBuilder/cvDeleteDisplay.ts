// Pure rules for the second dialog shown when a CV that jobs use is deleted
// (step 98): it names EVERY job that will lose its link, never a shortened list,
// because several jobs can share one CV. See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.

interface JobLine {
  title: string
  company?: string
}

// "This CV is used for 3 jobs"
export function inUseHeading(count: number): string {
  return `This CV is used for ${count} ${count === 1 ? 'job' : 'jobs'}`
}

// "Backend Engineer, Acme", or just the title when the job has no company.
export function jobLine(job: JobLine): string {
  const company = (job.company ?? '').trim()
  return company !== '' ? `${job.title.trim()}, ${company}` : job.title.trim()
}

// One line per job, in the order given, none dropped.
export function inUseLines(jobs: JobLine[]): string[] {
  return jobs.map(jobLine)
}

export function inUseFooter(count: number): string {
  return count === 1 ? 'Deleting it removes it from this job.' : 'Deleting it removes it from all of them.'
}
