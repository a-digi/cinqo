// The pure rules of a profile's auto-add settings page (step 140): reading the percentage box, the statuses a rule may
// use and what is wrong with a rule before it is sent. Kept apart from the page so they can be checked on their own; the
// server checks the same things again.
import type { JobStatusInfo, ProfileMatchRule } from '../../api'

export const MIN_PERCENT = 1
export const MAX_PERCENT = 100
export const MAX_RULES = 10

// A percentage typed by hand: a whole number of at most three digits from 1 to 100, otherwise null. Spaces around it are
// ignored; a sign, a decimal, an exponent and text are not numbers here.
export function percentFromText(raw: string): number | null {
  const text = raw.trim()
  if (!/^[0-9]{1,3}$/.test(text)) return null
  const n = Number(text)
  return n >= MIN_PERCENT && n <= MAX_PERCENT ? n : null
}

// The statuses a rule of this profile may use: the four fixed ones and the profile's own, never another profile's. The
// order of the list is kept (the fixed ones come first).
export function statusChoices(statuses: JobStatusInfo[], profileId: string): JobStatusInfo[] {
  return statuses.filter((s) => s.profileId === undefined || s.profileId === '' || s.profileId === profileId)
}

// What is wrong with a rule at this percentage, or '' when nothing is. ignoreId is the rule being changed (it may keep
// its own percentage and does not count against the limit).
export function ruleProblem(rules: ProfileMatchRule[], percent: number, ignoreId?: string): string {
  if (rules.some((r) => r.minPercent === percent && r.id !== ignoreId)) return `This profile already has a rule for ${String(percent)}%.`
  if (ignoreId === undefined && rules.length >= MAX_RULES) return `A profile can have at most ${String(MAX_RULES)} rules.`
  return ''
}

// The status a rule shows when the list of statuses has no entry for it (it never should: a rule goes with its status).
export function statusLabel(statuses: JobStatusInfo[], statusId: string, fallback: string): string {
  return statuses.find((s) => s.id === statusId)?.name ?? fallback
}
