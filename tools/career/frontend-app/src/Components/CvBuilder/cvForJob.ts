// The company is omitted by the backend when empty, so it may be missing at runtime.
interface JobLike {
  title: string
  company?: string
}

// Pure rules for building a CV from a job ("Generate manually", step 98): the
// CV Builder is opened with ?jobId=, shows which job it is for, starts the title
// from the job, attaches the new CV to the job on Generate and returns to the
// Jobs page. See plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.

export const JOBS_PATH = '/tools/career/jobs'

// The job a CV is being built for, from the page's query string (null if absent).
export function jobIdFromSearch(search: string): string | null {
  const id = new URLSearchParams(search).get('jobId')
  return id !== null && id.trim() !== '' ? id.trim() : null
}

// "Backend Engineer – Acme", or just the title when the job has no company.
export function defaultCvTitle(job: JobLike): string {
  const title = job.title.trim()
  const company = (job.company ?? '').trim()
  return company !== '' ? `${title} – ${company}` : title
}

export function bannerText(job: JobLike): string {
  const company = (job.company ?? '').trim()
  return `Creating a CV for ${job.title.trim()}${company !== '' ? ` at ${company}` : ''}. When it is created it becomes this job’s CV.`
}

export function backLabel(forJob: boolean): string {
  return forJob ? '← Back to Jobs' : '← Back to CV Documents'
}

// Shown when the CV was created but linking it to the job failed.
export function attachFailedMessage(reason: string): string {
  const why = reason.trim() !== '' ? ` (${reason.trim()})` : ''
  return `The CV was created, but it could not be attached to the job${why}. Click Generate to try attaching it again, or use “Select existing” on the job.`
}
