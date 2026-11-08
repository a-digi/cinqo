// runningCrawlsRules.ts (plan 214) — the words of the "a crawler is running" strip on the Jobs page, from the active crawl runs the Crawl Monitor
// also reads. Pure (no React, no network), so it can be script-checked.
import type { ActiveCrawlRunSummary } from '../../Crawler/crawlNow'
import { phaseLabel } from '../../Crawler/crawlPhase'

export function headline(count: number): string {
  if (count <= 0) return ''
  return count === 1 ? 'A crawler is running' : `${String(count)} crawlers are running`
}

// What a run does, in the words a person uses.
export function whatItDoes(kind: ActiveCrawlRunSummary['kind']): string {
  return kind === 'job_detail' ? 'Reading job details' : 'Reading the job list'
}

// The run's start as a Date. The backend writes RFC 3339 ("2026-11-08T19:50:55Z") and SQLite's own form ("2026-11-08 19:50:55", UTC, no zone);
// null when it cannot be read.
export function parseStartedAt(text: string | undefined): Date | null {
  if (!text) return null
  const t = text.trim()
  const d = new Date(/(?:[zZ]|[+-]\d\d:?\d\d)$/.test(t) ? t : `${t.replace(' ', 'T')}Z`)
  return Number.isNaN(d.getTime()) ? null : d
}

// "0:07", "1:05", "12:30"; empty when the start cannot be read; never negative (a clock a little behind the server's).
export function elapsed(startedAt: string | undefined, now: Date): string {
  const started = parseStartedAt(startedAt)
  if (!started) return ''
  const seconds = Math.max(0, Math.floor((now.getTime() - started.getTime()) / 1000))
  return `${String(Math.floor(seconds / 60))}:${String(seconds % 60).padStart(2, '0')}`
}

export interface RunLine {
  key: string
  // "Indeed · Software Engineer Hamburg": the platform and the link (its title, else its address).
  where: string
  what: string
  // The phase in words, or '' when the run has none yet.
  phase: string
  // True for a crawl the app started by itself (auto discovery).
  automatic: boolean
  elapsed: string
}

export function runLine(run: ActiveCrawlRunSummary, now: Date): RunLine {
  const link = (run.portalLinkTitle ?? '').trim() !== '' ? (run.portalLinkTitle ?? '').trim() : run.portalLinkUrl
  return {
    key: run.crawlRunId,
    where: `${run.portalName} · ${link}`,
    what: whatItDoes(run.kind),
    phase: phaseLabel(run.phase) ?? '',
    automatic: run.triggeredBy === 'auto_discovery',
    elapsed: elapsed(run.startedAt, now),
  }
}

// The runs as one string that changes when a run starts or ends (not when only its phase moves).
export function runsKey(runs: readonly ActiveCrawlRunSummary[]): string {
  return runs
    .map((r) => r.crawlRunId)
    .sort()
    .join(',')
}

// Whether a run that was there before is gone now (it finished, failed or was stopped): the page then reloads its jobs.
export function someRunEnded(before: readonly ActiveCrawlRunSummary[], after: readonly ActiveCrawlRunSummary[]): boolean {
  const now = new Set(after.map((r) => r.crawlRunId))
  return before.some((r) => !now.has(r.crawlRunId))
}
