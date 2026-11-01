// The rules and the words of "recrawl jobs for a fresh description" (step 148), kept apart from the pages so they can be checked
// on their own: which jobs can be recrawled and why not, how a selection is counted and worded, what a batch's progress and result
// say, and the wording of each job's outcome. Pure (no React, no network). See plan/ai/tools/career/step-148-recrawl-jobs.md.
import { plural } from '../CompaniesPage/companyStats'

// What the rules read of a job.
export interface RecrawlJob {
  source?: 'crawled' | 'manual'
  sourceUrl: string
  portalLinkId?: string
  canRecrawl?: boolean
}

export function isManual(job: Pick<RecrawlJob, 'source'>): boolean {
  return job.source === 'manual'
}

export interface RecrawlCheck {
  ok: boolean
  // Why not, for a disabled action's tooltip; empty when ok.
  reason: string
}

// Whether a job can be recrawled, and when it cannot, the reason to show.
export function recrawlCheck(job: RecrawlJob): RecrawlCheck {
  if (job.canRecrawl === true) return { ok: true, reason: '' }
  if (job.sourceUrl === '') return { ok: false, reason: 'It has no posting address to crawl.' }
  if (job.portalLinkId === undefined || job.portalLinkId === '') {
    return {
      ok: false,
      reason: isManual(job) ? 'Added by hand: it has no platform to crawl it from.' : 'It has no platform to crawl it from.',
    }
  }
  return { ok: false, reason: 'Its platform has no job detail crawl instructions yet.' }
}

// The tooltip of the Recrawl action: what it does, or why it is not available; a job added by hand says its own text is kept.
export function recrawlTitle(job: RecrawlJob): string {
  const check = recrawlCheck(job)
  if (!check.ok) return check.reason
  if (isManual(job)) return 'Recrawl this job. It was added by hand: its own text is kept, an empty description is filled in.'
  return 'Crawl this job’s page again for a fresh description'
}

// ---- the selection ----

// The ids of the selected jobs.
export type Selection = ReadonlySet<string>

export function toggleId(selection: Selection, id: string): Selection {
  const next = new Set(selection)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

export type PageState = 'none' | 'some' | 'all'

// The state of the header checkbox: how many of the jobs on this page are selected.
export function pageState(selection: Selection, pageIds: readonly string[]): PageState {
  if (pageIds.length === 0) return 'none'
  const n = pageIds.filter((id) => selection.has(id)).length
  if (n === 0) return 'none'
  return n === pageIds.length ? 'all' : 'some'
}

// What the header checkbox does: selects every job on the page, or when they all are selected already, clears them (the jobs of
// other pages stay as they are).
export function togglePage(selection: Selection, pageIds: readonly string[]): Selection {
  const next = new Set(selection)
  if (pageState(selection, pageIds) === 'all') {
    for (const id of pageIds) next.delete(id)
  } else {
    for (const id of pageIds) next.add(id)
  }
  return next
}

export interface SelectedJob {
  source?: 'crawled' | 'manual'
  canRecrawl: boolean
  // How many profiles' job boards the job is on (step 149: the delete question warns that those cards go too).
  boardCount?: number
}

// How a selection is described: "5 selected · 4 can be recrawled · 2 added by hand (their own text is kept)". The "can be recrawled"
// part appears only when some cannot; the "added by hand" part counts the manual jobs among those that can. Empty for no selection.
export function selectionText(jobs: readonly SelectedJob[]): string {
  if (jobs.length === 0) return ''
  const can = jobs.filter((j) => j.canRecrawl)
  const parts = [`${String(jobs.length)} selected`]
  if (can.length < jobs.length) parts.push(`${String(can.length)} can be recrawled`)
  const manual = can.filter((j) => j.source === 'manual').length
  if (manual > 0) parts.push(`${String(manual)} added by hand (${manual === 1 ? 'its' : 'their'} own text is kept)`)
  return parts.join(' · ')
}

// The label of the selection bar's button.
export function recrawlButtonLabel(selectedCount: number): string {
  return selectedCount === 1 ? 'Recrawl 1 job' : `Recrawl ${String(selectedCount)} jobs`
}

// ---- progress and result ----

export interface BatchCounts {
  refreshed: number
  unchanged: number
  filled: number
  kept: number
  failed: number
  skipped: number
}

export interface BatchJob {
  jobId: string
  title: string
  manual: boolean
  state: 'queued' | 'running' | 'done' | 'cancelled'
  outcome?: string
  reason?: string
}

export interface BatchStatus {
  total: number
  queued: number
  running: number
  done: number
  cancelled: number
  counts: BatchCounts
  jobs: BatchJob[]
}

// Whether the batch still has work: anything waiting or being crawled.
export function isRunning(s: BatchStatus): boolean {
  return s.queued + s.running > 0
}

// The panel's line while it runs: "Recrawling 12 jobs · 7 done · 5 waiting".
export function progressText(s: BatchStatus): string {
  const waiting = s.queued + s.running
  return `Recrawling ${plural(s.total, 'job', 'jobs')} · ${String(s.done)} done · ${String(waiting)} waiting`
}

// The panel's line when the batch is over: "Refreshed 8 · unchanged 2 · filled in 1 · kept 1 · failed 1 · skipped 1", only the parts
// that are not zero, then "cancelled 3" for jobs that never started. startSkipped are the jobs the request refused at once (they
// have no row in the batch) and count as skipped. "Nothing was recrawled." when there is nothing to report.
export function resultText(s: BatchStatus, startSkipped = 0): string {
  const c = s.counts
  const parts: string[] = []
  if (c.refreshed > 0) parts.push(`Refreshed ${String(c.refreshed)}`)
  if (c.unchanged > 0) parts.push(`unchanged ${String(c.unchanged)}`)
  if (c.filled > 0) parts.push(`filled in ${String(c.filled)}`)
  if (c.kept > 0) parts.push(`kept ${String(c.kept)}`)
  if (c.failed > 0) parts.push(`failed ${String(c.failed)}`)
  const skipped = c.skipped + startSkipped
  if (skipped > 0) parts.push(`skipped ${String(skipped)}`)
  if (s.cancelled > 0) parts.push(`cancelled ${String(s.cancelled)}`)
  if (parts.length === 0) return 'Nothing was recrawled.'
  const [first, ...rest] = parts
  return [first.charAt(0).toUpperCase() + first.slice(1), ...rest].join(' · ')
}

export const WAITING_FOR_SESSION = 'waiting for a signed-in session'

// The outcome of one job in the details list.
export function outcomeLabel(job: Pick<BatchJob, 'state' | 'outcome'>): string {
  switch (job.state) {
    case 'queued':
      return 'Waiting'
    case 'running':
      return 'Crawling…'
    case 'cancelled':
      return 'Cancelled'
    case 'done':
      switch (job.outcome) {
        case 'refreshed':
          return 'Refreshed'
        case 'unchanged':
          return 'Unchanged'
        case 'filled':
          return 'Filled in'
        case 'kept':
          return 'Text kept as you typed it'
        case 'failed':
          return 'Failed'
        case 'skipped':
          return 'Skipped'
        default:
          return 'Done'
      }
  }
}

// The detail line under a job in the list: the reason, or a hint for a job that waits for a session; empty when there is none.
export function detailText(job: Pick<BatchJob, 'state' | 'reason'>): string {
  const reason = job.reason ?? ''
  if (reason === '') return ''
  if (job.state === 'queued' && reason === WAITING_FOR_SESSION) return 'Waiting for a signed-in session: open the app again to continue.'
  return reason.charAt(0).toUpperCase() + reason.slice(1)
}

// The notice on a job's own page after its recrawl (a batch of one) finished.
export function pageNotice(job: Pick<BatchJob, 'state' | 'outcome' | 'reason'>): { ok: boolean; text: string } {
  const reason = detailText(job)
  switch (job.state) {
    case 'queued':
    case 'running':
      return { ok: true, text: 'Recrawling…' }
    case 'cancelled':
      return { ok: false, text: 'The recrawl was cancelled.' }
    case 'done':
      switch (job.outcome) {
        case 'refreshed':
          return { ok: true, text: 'Description refreshed.' }
        case 'unchanged':
          return { ok: true, text: 'Nothing changed: the page has the same text.' }
        case 'filled':
          return { ok: true, text: 'Description filled in from the page.' }
        case 'kept':
          return { ok: true, text: 'Kept as you typed it: this job was added by hand.' }
        case 'failed':
          return { ok: false, text: reason === '' ? 'Could not refresh the description.' : `Could not refresh: ${reason}` }
        case 'skipped':
          return { ok: false, text: reason === '' ? 'Could not recrawl this job.' : `Could not recrawl: ${reason}` }
        default:
          return { ok: true, text: 'Done.' }
      }
  }
}

export interface SkippedJob {
  jobId: string
  reason: string
}

// What a refused request says: how many jobs were skipped and why, grouped by reason, e.g.
// "2 jobs skipped: no portal to crawl it from (1), already being recrawled (1)". Empty for none.
export function skippedText(skipped: readonly SkippedJob[]): string {
  if (skipped.length === 0) return ''
  const counts = new Map<string, number>()
  for (const s of skipped) counts.set(s.reason, (counts.get(s.reason) ?? 0) + 1)
  const reasons = [...counts.entries()].map(([reason, n]) => `${reason} (${String(n)})`)
  return `${plural(skipped.length, 'job', 'jobs')} skipped: ${reasons.join(', ')}`
}

// Whether the list should reload after a poll: some job finished since the last look.
export function shouldReload(previousDone: number, s: BatchStatus): boolean {
  return s.done > previousDone
}

// ---- remembering the batch ----

const BATCH_KEY = 'career.recrawlBatch'

// The storage key of the batch to remember. The Jobs list keeps one batch under the plain key; a job's own page keeps its batch under a
// key of its own (scope = the job's id), so a batch started in the list never shows up as a notice on another job's page.
export function batchStorageKey(scope = ''): string {
  return scope === '' ? BATCH_KEY : `${BATCH_KEY}.${scope}`
}

// The batch id to follow from what the browser's storage holds: a non-empty string, or null (nothing, junk, or an empty value).
export function parseStoredBatch(raw: string | null): string | null {
  if (raw === null) return null
  const id = raw.trim()
  return id === '' || id.length > 100 || /\s/.test(id) ? null : id
}
