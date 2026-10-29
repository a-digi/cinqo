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
