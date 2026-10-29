// manualJob.ts (step 97) — the rules for adding a job by hand and for how a
// manual job is shown. Pure, no React, so they can be script-checked. The
// limits mirror the backend's (jobs/manual_job.go), which enforces them too;
// the form only saves a round trip. See
// plan/ai/tools/career/step-97-add-jobs-manually.md.
import type { Job } from '../../api'

export const MANUAL_JOB_LIMITS = {
  title: 300,
  company: 200,
  location: 200,
  postedAt: 100,
  description: 100_000,
  sourceUrl: 2000,
} as const

export interface NewJobForm {
  title: string
  company: string
  location: string
  postedAt: string
  sourceUrl: string
  description: string
}

export const EMPTY_NEW_JOB: NewJobForm = { title: '', company: '', location: '', postedAt: '', sourceUrl: '', description: '' }

export type NewJobErrors = Partial<Record<keyof NewJobForm, string>>

// isHttpUrl is the same rule the backend applies: an absolute http or https URL.
export function isHttpUrl(value: string): boolean {
  if (/\s/.test(value)) return false
  try {
    const u = new URL(value)
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.hostname !== ''
  } catch {
    return false
  }
}

// validateNewJob returns the first problem per field; empty when the form can
// be sent. Only the title is required.
export function validateNewJob(form: NewJobForm): NewJobErrors {
  const errors: NewJobErrors = {}
  const trimmed = {
    title: form.title.trim(),
    company: form.company.trim(),
    location: form.location.trim(),
    postedAt: form.postedAt.trim(),
    sourceUrl: form.sourceUrl.trim(),
    description: form.description.trim(),
  }
  if (trimmed.title === '') errors.title = 'A title is required.'
  for (const key of Object.keys(MANUAL_JOB_LIMITS) as (keyof typeof MANUAL_JOB_LIMITS)[]) {
    // Characters, not UTF-16 units, as the server counts.
    if (!errors[key] && Array.from(trimmed[key]).length > MANUAL_JOB_LIMITS[key]) {
      errors[key] = `Must be at most ${MANUAL_JOB_LIMITS[key].toLocaleString()} characters.`
    }
  }
  if (!errors.sourceUrl && trimmed.sourceUrl !== '' && !isHttpUrl(trimmed.sourceUrl)) {
    errors.sourceUrl = 'Must be a full web address starting with http:// or https://.'
  }
  return errors
}

export function hasErrors(errors: NewJobErrors): boolean {
  return Object.keys(errors).length > 0
}

export function isManualJob(job: Pick<Job, 'source'>): boolean {
  return job.source === 'manual'
}

// What the Platform column shows: the portal it was found through, else
// "Manual" for a job added by hand, else a dash.
export function platformLabel(job: Pick<Job, 'source' | 'portalName'>): string {
  if (job.portalName) return job.portalName
  return isManualJob(job) ? 'Manual' : '—'
}

// A manual job added without a posting URL has nothing to open.
export function canOpenOriginal(job: Pick<Job, 'sourceUrl'>): boolean {
  return job.sourceUrl.trim() !== ''
}

// A crawled job with no description and no detail crawl yet cannot show its
// details; a manual job always can (its page says "No description available").
export type DetailsState = 'available' | 'not-crawled' | 'failed'

export function detailsState(job: Pick<Job, 'source' | 'description' | 'detailCrawlStatus'>): DetailsState {
  if (isManualJob(job)) return 'available'
  const hasDescription = !!job.description && job.description.trim() !== ''
  if (hasDescription) return 'available'
  if (job.detailCrawlStatus === 'failed') return 'failed'
  return job.detailCrawlStatus ? 'available' : 'not-crawled'
}
