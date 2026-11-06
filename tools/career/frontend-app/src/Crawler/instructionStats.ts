// instructionStats.ts (step 86) — display helpers for a link's
// "Generate with AI" attempt counters (api.ts's InstructionKindStats),
// kept free of React so the rules (when the alert shows, how counts
// read) live in one place. See
// plan/ai/tools/career/step-86-instruction-attempt-tracking.md.
import type { InstructionAttemptOutcome, InstructionKindStats } from '../api'
import { hasListingFor, listingNeeds, type EngineReading, type ListingNeeds } from './crawlEngine'

// savedDespiteError (step 95): a turn can end in an error (the budget ran out on
// the very round that saved, say) although the agent already saved its
// instructions. The backend's verdict on the attempt is the truth: when it says
// 'succeeded', the run is a success and the turn's error must not be shown.
export function savedDespiteError(outcome: InstructionAttemptOutcome | undefined): boolean {
  return outcome === 'succeeded'
}

export function attemptsLabel(count: number): string {
  return `${count} attempt${count === 1 ? '' : 's'}`
}

// instructionAlertAttempts returns how many attempts were made when
// the alert pill should show, else 0: attempts were made, the link
// STILL has no instructions of this kind, and nothing is generating
// right now (a run in flight may yet succeed, so alerting then would
// be premature — `generating` covers this tab's own in-flight run,
// stats.running covers one the server knows about).
export function instructionAlertAttempts(stats: InstructionKindStats | undefined, hasInstructions: boolean, generating: boolean): number {
  if (!stats || stats.attempts <= 0 || hasInstructions || generating || stats.running) return 0
  return stats.attempts
}

// LIKELY_UNCRAWLABLE_THRESHOLD (step 87) is how many failed runs in a
// row — the AI finished each without saving instructions — it takes
// before the link is flagged as probably not crawlable. See
// plan/ai/tools/career/step-87-likely-uncrawlable-pill.md.
export const LIKELY_UNCRAWLABLE_THRESHOLD = 3

// isLikelyUncrawlable (step 87): the listing kind only (a job-detail
// failure doesn't mean the page can't be crawled), at least the
// threshold of consecutive gave-up failures, still no instructions,
// and nothing generating right now.
export function isLikelyUncrawlable(stats: InstructionKindStats | undefined, hasInstructions: boolean, generating: boolean): boolean {
  if (!stats || hasInstructions || generating || stats.running) return false
  return stats.consecutiveFailures >= LIKELY_UNCRAWLABLE_THRESHOLD
}

// isNotCrawlable (step 159): the "Not crawlable" pill next to "Not instructed" — the link has no listing
// instructions, so nothing can be crawled from it yet. Hidden while a run may still produce instructions
// (generating / stats.running) and when the stronger "Probably can't be crawled" pill already says it.
export function isNotCrawlable(stats: InstructionKindStats | undefined, hasInstructions: boolean, generating: boolean): boolean {
  if (hasInstructions || generating || stats?.running) return false
  return !isLikelyUncrawlable(stats, hasInstructions, generating)
}

// The state of a link's instructions as the pill shows it (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 2): a link is
// "instructed" only when BOTH instruction sets are stored, the listing ones (how to read the job list) and the job-detail ones (how to read a
// job's own page). With only the listing ones it can be crawled but its jobs cannot be read in full ("job details missing"); with none it is "not
// instructed"; while either set is being written by the AI it is "instructing" (it has not reached a state yet). While the AI waits for the
// person's answer to its question ("does this page list jobs?") nothing is being written, so the stored instructions decide, and the
// "Question for you" pill says the rest.
export type InstructionState = 'instructed' | 'job_details_missing' | 'not_instructed' | 'instructing'

export interface InstructionStateInput {
  crawlInstructions?: string | null
  // Plan 190: what the job list needs depends on the engine (a rule under Katana), so the rule and the login setting count too.
  linkRules?: unknown
  requiresLogin?: boolean
  jobDetailCrawlInstructions?: string | null
  // Set while the generation agent's question is waiting for an answer.
  jobsQuestion?: unknown
  instructionStats?: { listing?: Pick<InstructionKindStats, 'running'>; jobDetail?: Pick<InstructionKindStats, 'running'> }
}

const stored = (text: string | null | undefined) => (text ?? '').trim() !== ''

// listingGenerating / jobDetailGenerating: a run started in this tab that the server does not know about yet (the stats cover the rest).
export function instructionState(
  link: InstructionStateInput,
  listingGenerating = false,
  jobDetailGenerating = false,
  engine: EngineReading = null,
): InstructionState {
  const working =
    listingGenerating ||
    jobDetailGenerating ||
    Boolean(link.instructionStats?.listing?.running) ||
    Boolean(link.instructionStats?.jobDetail?.running)
  if (working && !link.jobsQuestion) return 'instructing'
  // The job list is judged by the engine in use (plan 190): its rule under Katana, its AI instructions under Rod; without an engine reading
  // either one counts. Job details count as before, under every engine.
  if (
    !hasListingFor(
      { crawlInstructions: link.crawlInstructions, linkRules: (link.linkRules ?? null) as never, requiresLogin: link.requiresLogin },
      engine,
    )
  ) {
    return 'not_instructed'
  }
  if (!stored(link.jobDetailCrawlInstructions)) return 'job_details_missing'
  return 'instructed'
}

export interface InstructionPill {
  label: string
  className: string
  tooltip: string
}

export function instructionPill(state: InstructionState): InstructionPill {
  switch (state) {
    case 'instructed':
      return {
        label: 'Instructed',
        className: 'bg-green-100 text-green-800',
        tooltip: 'Both sets of instructions are written: how to read the job list and how to read each job.',
      }
    case 'job_details_missing':
      return {
        label: 'Job details missing',
        className: 'bg-amber-100 text-amber-800',
        tooltip: 'The job list can be crawled, but there are no instructions for the job details yet, so the jobs cannot be read in full.',
      }
    case 'not_instructed':
      return {
        label: 'Not instructed',
        className: 'bg-red-100 text-red-800',
        tooltip: 'There are no crawl instructions for this link yet.',
      }
    case 'instructing':
      return {
        label: 'Instructing…',
        className: 'animate-pulse bg-blue-100 text-blue-800',
        tooltip: 'The AI is writing instructions for this link.',
      }
  }
}

// notCrawlableTooltip says what to do about the pill.
export const NOT_CRAWLABLE_TOOLTIP =
  'This link has no crawl instructions yet, so it cannot be crawled. Generate them with the AI (the Instructions section of this link) or add the jobs manually.'

// Under Katana the job list is crawled with a rule, not with AI instructions (plan 190).
export const NOT_CRAWLABLE_TOOLTIP_RULE =
  'This link has no rules for the Katana engine yet, so it cannot be crawled. Pick its job links in the Crawl instructions section, or select Rod on the Browser & Engine page to crawl it with AI instructions, or add the jobs manually.'

export function notCrawlableTooltip(needs: ListingNeeds): string {
  return needs === 'rule' ? NOT_CRAWLABLE_TOOLTIP_RULE : NOT_CRAWLABLE_TOOLTIP
}

// Whether the AI's attempts to write the job-list instructions matter for this link: not under Katana (the rule is made by hand there), and
// never for a link that is crawled with a rule.
export function listingIsWrittenByAi(engine: EngineReading, requiresLogin: boolean): boolean {
  return listingNeeds(engine, requiresLogin) !== 'rule'
}

// uncrawlableTooltip explains the pill, with the latest failure if known.
export function uncrawlableTooltip(stats: InstructionKindStats | undefined): string {
  const latest = instructionStatsTooltip(stats)
  return (
    `The AI could not write crawl instructions for this page ${stats?.consecutiveFailures ?? 0} times in a row, ` +
    `so it probably can't be crawled automatically. Its jobs may have to be added manually ` +
    `(for example, ask the AI chat to save them).` +
    (latest ? ` ${latest}` : '')
  )
}

// formatInstructionStats is the one-line summary, or null when there
// is nothing to report yet (no finished attempt).
export function formatInstructionStats(stats: InstructionKindStats | undefined): string | null {
  if (!stats || stats.attempts <= 0) return null
  return `${attemptsLabel(stats.attempts)} · ${stats.succeeded} succeeded · ${stats.failed} failed`
}

// instructionStatsTooltip names the latest failure, if any.
export function instructionStatsTooltip(stats: InstructionKindStats | undefined): string | undefined {
  if (!stats?.lastError) return undefined
  const when = stats.lastFailureAt ? ` (${new Date(stats.lastFailureAt).toLocaleString()})` : ''
  return `Last failure${when}: ${stats.lastError}`
}
