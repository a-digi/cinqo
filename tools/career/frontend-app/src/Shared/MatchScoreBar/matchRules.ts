import type { ManualMatchResult, MatchComponent } from '../../api'

// Pure display rules for a manual match result, kept apart from the components so they can be checked on their own.

const TIER_LABELS: Record<number, string> = { 3: 'Must-have', 2: 'Important', 1: 'Nice to have' }

export function tierLabel(tier: number): string {
  return TIER_LABELS[tier] ?? `Tier ${String(tier)}`
}

// The confidence line: how much of the evidence the score rests on.
export function confidenceText(coverage: number, lowConfidence: boolean): string {
  const text = `Based on ${String(Math.round(coverage * 100))} % of the available evidence`
  return lowConfidence ? `${text}: low confidence` : text
}

// "28.8 / 40" for a judged component, "not available" otherwise.
export function pointsText(c: MatchComponent): string {
  if (!c.evaluable) return 'not available'
  const points = Number.isInteger(c.points) ? String(c.points) : c.points.toFixed(1)
  return `${points} / ${String(c.weight)}`
}

// The share of a component's weight it earned, 0 to 1 (0 when it was not judged).
export function pointsShare(c: MatchComponent): number {
  return c.evaluable && c.weight > 0 ? Math.max(0, Math.min(1, c.points / c.weight)) : 0
}

// Why a score was scaled down, or '' when it was not (a scale of 1, or none stored).
export function relevanceText(scale: number | undefined): string {
  if (scale === undefined || scale >= 1) return ''
  return `The job's title and skills hardly resemble this persona's field, so the score is scaled down to ${String(Math.round(scale * 100))} %.`
}

export function modeText(result: Pick<ManualMatchResult, 'mode'>): string {
  return result.mode === 'requirements' ? "Skills scored from the job's requirements" : 'Skills scored from the job text'
}

// One line for a refused run: what to fix.
export function refusalText(message: string, reasons: string[]): string {
  return reasons.length > 0 ? `Job match was not possible. ${reasons.join(' ')}` : `Job match was not possible: ${message}`
}

// The colours of a score chip, in the bands the score bar uses: 91-100 dark green, 81-90 light green, 60-80 orange,
// 40-59 violet, under 40 red.
export function scoreChipClasses(score: number): string {
  if (score >= 91) return 'bg-green-700 text-white'
  if (score >= 81) return 'bg-green-100 text-green-800'
  if (score >= 60) return 'bg-orange-100 text-orange-800'
  if (score >= 40) return 'bg-violet-100 text-violet-800'
  return 'bg-red-100 text-red-700'
}

// The match shown for a job: its engine ('ai' for a result made by an AI, which has no breakdown).
export function engineLabel(kind: 'ai' | 'manual' | undefined): string {
  return kind === 'manual' ? 'Manual' : kind === 'ai' ? 'AI' : ''
}
