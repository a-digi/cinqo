// The words and the counts of the Crawl Monitor's "Job detail recrawl" part (plan 194), kept apart from the page so they can be checked on their
// own. Pure: no React, no network.
import type { RecrawlActive, RecrawlActiveBatch, RecrawlActiveWorker } from '../../api'
import { plural } from '../../Components/CompaniesPage/companyStats'
import { progressText, stoppingText, type BatchStatus } from '../../Components/JobsPage/recrawlRules'
import { phaseLabel } from '../crawlPhase'
import type { ActiveCrawlRunSummary } from '../crawlNow'

export const NO_PHASE_TEXT = "Waiting for the platform's turn"

// What a worker's page load is doing, in words. The Browser tool says nothing while it waits for the platform's pacing, so no phase means
// that (or that the page load has only just begun).
const WORKER_PHASES: Partial<Record<string, string>> = {
  queued_for_tab: 'Waiting for a free browser tab',
  navigating: 'Opening the job page',
  checking_cloudflare: 'Checking for a Cloudflare challenge',
  awaiting_human_challenge: 'Waiting for you to solve a Cloudflare challenge',
  extracting: 'Reading the job page',
  completed: 'Saving the details',
  failed: 'The page load failed',
}

export function workerPhaseText(w: Pick<RecrawlActiveWorker, 'phase'>): string {
  if (!w.phase) return NO_PHASE_TEXT
  return WORKER_PHASES[w.phase] ?? phaseLabel(w.phase) ?? w.phase
}

export function needsYou(w: Pick<RecrawlActiveWorker, 'phase'>): boolean {
  return w.phase === 'awaiting_human_challenge'
}

// "12 s", "1 min 05 s", "1 h 02 min": how long a job has been crawling. An unreadable start reads as "—".
export function elapsedText(startedAt: string, now: number): string {
  const start = Date.parse(startedAt)
  if (Number.isNaN(start)) return '—'
  const seconds = Math.max(0, Math.floor((now - start) / 1000))
  if (seconds < 60) return `${String(seconds)} s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${String(minutes)} min ${String(seconds % 60).padStart(2, '0')} s`
  return `${String(Math.floor(minutes / 60))} h ${String(minutes % 60).padStart(2, '0')} min`
}

// "Crawler 1" for the main crawler, "Crawler 2" to "Crawler 5" for the helpers; nothing for an old row that does not say.
export function crawlerText(slot: number): string {
  return slot >= 0 ? `Crawler ${String(slot + 1)}` : ''
}

// The batch line is the Jobs page's own wording (progress and stopping), built from the monitor's batch.
function asStatus(b: RecrawlActiveBatch): BatchStatus {
  return {
    total: b.total,
    queued: b.queued,
    running: b.running,
    done: b.done,
    cancelled: b.cancelled,
    counts: { refreshed: 0, unchanged: 0, filled: 0, kept: 0, failed: 0, skipped: 0 },
    jobs: [],
  }
}

export function batchLine(b: RecrawlActiveBatch): string {
  return b.stopping ? stoppingText(asStatus(b)) : progressText(asStatus(b))
}

// How much of a batch is over (done or cancelled), 0 to 100, for the bar.
export function batchPercent(b: RecrawlActiveBatch): number {
  return b.total > 0 ? Math.round(((b.done + b.cancelled) / b.total) * 100) : 0
}

export interface MonitorCounts {
  total: number
  listing: number
  detail: number
  recrawl: number
  automatic: number
  waitingForYou: number
}

// The figures of the header pill and the side rail: the crawl runs and the recrawl workers together.
export function monitorCounts(runs: ActiveCrawlRunSummary[], active: RecrawlActive): MonitorCounts {
  const listing = runs.filter((r) => r.kind === 'listing').length
  return {
    total: runs.length + active.workers.length,
    listing,
    detail: runs.length - listing,
    recrawl: active.workers.length,
    automatic: runs.filter((r) => r.triggeredBy === 'auto_discovery').length,
    waitingForYou: runs.filter((r) => r.phase === 'awaiting_human_challenge').length + active.workers.filter(needsYou).length,
  }
}

// Whether there is anything to show at all (a batch that only waits for its next job still counts).
export function isIdle(runs: ActiveCrawlRunSummary[], active: RecrawlActive): boolean {
  return runs.length === 0 && active.workers.length === 0 && active.batches.length === 0
}

export function pillText(c: MonitorCounts, active: RecrawlActive): string {
  if (c.total === 0) return active.batches.length > 0 ? 'Waiting' : 'Idle'
  return `${String(c.total)} running`
}

export const WAITING_BETWEEN_JOBS = 'No job is being crawled at this moment: the next one starts as soon as a crawler is free.'

export function workersHeading(n: number): string {
  return plural(n, 'job being crawled now', 'jobs being crawled now')
}
