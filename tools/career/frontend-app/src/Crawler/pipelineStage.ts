// The words of the pipeline line on a platform link (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 1): the stage the
// backend derived (portal/pipeline.go) turned into a label, a hint about what happens next or what to do, and how it should look. No React,
// so they can be script-checked.
import type { PipelineLink, PipelineStageId, PipelineWaitingFor } from '../api'

// active: something is running right now (the line pulses and refreshes faster); waiting: it goes on by itself or after one click; attention:
// a person has to act or something failed; done: the chain is complete; neutral: nothing to say yet.
export type PipelineTone = 'active' | 'waiting' | 'attention' | 'done' | 'neutral'

export interface PipelineText {
  label: string
  hint: string
  tone: PipelineTone
  // The position in the chain, 1 to 4 (listing instructions, listing crawl, job-detail instructions, job-detail crawl); 0 before the first,
  // 4 and complete when all four are done. Used for the little progress marks.
  position: number
}

export const PIPELINE_STEPS = 4

const plural = (n: number, one: string, other: string) => `${String(n)} ${n === 1 ? one : other}`

export function pipelineText(link: Pick<PipelineLink, 'stage' | 'detail'>): PipelineText {
  const { jobs, jobsWithDescription, detailFailedRuns } = link.detail
  switch (link.stage) {
    case 'listing_question':
      return {
        label: 'The AI asked you a question',
        hint: 'Answer it so the AI can go on with the listing instructions.',
        tone: 'attention',
        position: 0,
      }
    case 'listing_instructions_generating':
      return {
        label: 'The AI is writing the listing instructions',
        hint: 'You can watch it in the chat window.',
        tone: 'active',
        position: 0,
      }
    case 'no_listing_instructions':
      return {
        label: 'No listing instructions yet',
        hint: 'Generate them with the AI, or add the jobs by hand.',
        tone: 'attention',
        position: 0,
      }
    case 'listing_ready':
      return { label: 'Listing instructions ready, no jobs yet', hint: 'Use Crawl now to find the jobs.', tone: 'waiting', position: 1 }
    case 'listing_crawl_waiting':
      return {
        label: 'Listing crawl waiting for the page',
        hint: 'It starts as soon as a Platforms page is open.',
        tone: 'waiting',
        position: 1,
      }
    case 'listing_crawling':
      return { label: 'Crawling the job list', hint: 'The jobs are saved as they are found.', tone: 'active', position: 1 }
    case 'detail_instructions_requested':
      return {
        label: `${plural(jobs, 'job', 'jobs')} saved, job-detail instructions requested`,
        hint: 'The AI writes them as soon as a Platforms page picks the request up.',
        tone: 'waiting',
        position: 2,
      }
    case 'detail_instructions_generating':
      return {
        label: 'The AI is writing the job-detail instructions',
        hint: 'You can watch it in the chat window.',
        tone: 'active',
        position: 2,
      }
    case 'detail_instructions_failed':
      return {
        label: `Job-detail instructions failed (${plural(detailFailedRuns, 'time', 'times')})`,
        hint: 'Generate them again, or try another model.',
        tone: 'attention',
        position: 2,
      }
    case 'detail_instructions_missing':
      return {
        label: `${plural(jobs, 'job', 'jobs')} saved, no job-detail instructions`,
        hint: 'Generate them with the AI so the jobs can be read in full.',
        tone: 'attention',
        position: 2,
      }
    case 'detail_crawl_waiting':
      return {
        label: 'Job-detail crawl waiting for the page',
        hint: 'It starts as soon as a Platforms page is open.',
        tone: 'waiting',
        position: 3,
      }
    case 'detail_crawling':
      return { label: 'Crawling the job details', hint: 'Each job’s own page is read for its description.', tone: 'active', position: 3 }
    case 'detail_crawl_pending':
      return {
        label: `${String(jobsWithDescription)} of ${plural(jobs, 'job has', 'jobs have')} a description`,
        hint: 'Crawl the job details to fill in the rest.',
        tone: 'waiting',
        position: 3,
      }
    case 'complete':
      return {
        label: `Complete: ${String(jobsWithDescription)} of ${plural(jobs, 'job has', 'jobs have')} a description`,
        hint: 'Both sets of instructions are written and every job has been read.',
        tone: 'done',
        position: PIPELINE_STEPS,
      }
  }
}

const KNOWN_STAGES: readonly PipelineStageId[] = [
  'listing_question',
  'listing_instructions_generating',
  'no_listing_instructions',
  'listing_crawling',
  'detail_instructions_generating',
  'detail_crawling',
  'listing_crawl_waiting',
  'listing_ready',
  'detail_instructions_requested',
  'detail_instructions_failed',
  'detail_instructions_missing',
  'detail_crawl_waiting',
  'detail_crawl_pending',
  'complete',
]

// Every stage the backend can send has words (a stage this code does not know, from a newer backend, gets none).
export function isKnownStage(stage: string): stage is PipelineStageId {
  return (KNOWN_STAGES as readonly string[]).includes(stage)
}

export function waitingText(waitingFor: PipelineWaitingFor): string {
  switch (waitingFor) {
    case 'user':
      return 'Waiting for you'
    case 'page':
      return 'Waiting for an open Platforms page'
    case 'ai':
      return 'The AI is working'
    case 'crawl':
      return 'A crawl is running'
    case 'nothing':
      return 'Nothing left to do'
  }
}

// "for 45 s", "for 3 min", "for 2 h" from a stored UTC time ("2026-11-02 10:00:00") and the current time in ms; '' when the time cannot be
// read or lies in the future.
export function runningFor(since: string | undefined, nowMs: number): string {
  if (since === undefined || since.trim() === '') return ''
  const trimmed = since.trim()
  const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(trimmed) ? `${trimmed.replace(' ', 'T')}Z` : trimmed
  const started = new Date(iso).getTime()
  if (Number.isNaN(started) || started > nowMs) return ''
  const seconds = Math.floor((nowMs - started) / 1000)
  if (seconds < 60) return `for ${String(seconds)} s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `for ${String(minutes)} min`
  return `for ${String(Math.floor(minutes / 60))} h`
}

// How long to wait before asking again: often while something runs, rarely while it waits for a person or a page.
export function refreshDelayMs(tone: PipelineTone | null): number {
  return tone === 'active' ? 4000 : 20000
}
