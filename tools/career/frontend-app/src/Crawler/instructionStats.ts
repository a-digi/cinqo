// instructionStats.ts (step 86) — display helpers for a link's
// "Generate with AI" attempt counters (api.ts's InstructionKindStats),
// kept free of React so the rules (when the alert shows, how counts
// read) live in one place. See
// plan/ai/tools/career/step-86-instruction-attempt-tracking.md.
import type { InstructionKindStats } from '../api'

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
