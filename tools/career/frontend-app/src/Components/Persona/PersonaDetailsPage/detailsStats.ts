// How complete a persona's details are: six fields, each counted once it has content.
// Pure (no React), so it can be script-checked.
export interface DetailsLike {
  headline: string
  summary: string
  location: string
  desiredTitles: string
  desiredLocations: string
  // The form holds the salary as text, the saved details as a number.
  minSalary: string | number
}

export interface Completeness {
  filled: number
  total: number
  // Labels of the fields still empty, in the order the form shows them.
  missing: string[]
  // Whole percent; 0 when there are no details at all.
  percent: number
}

export type DetailsKey = 'headline' | 'location' | 'summary' | 'desiredTitles' | 'desiredLocations' | 'minSalary'

export interface DetailsStep {
  key: DetailsKey
  label: string
  filled: boolean
}

// The six fields in the order the form shows them, each counted once it has content.
// The stepper and the figures both come from this list, so they cannot disagree.
const FIELDS: { key: DetailsKey; label: string; filled: (d: DetailsLike) => boolean }[] = [
  { key: 'headline', label: 'Headline', filled: (d) => d.headline.trim() !== '' },
  { key: 'location', label: 'Location', filled: (d) => d.location.trim() !== '' },
  { key: 'summary', label: 'Summary', filled: (d) => d.summary.trim() !== '' },
  { key: 'desiredTitles', label: 'Desired titles', filled: (d) => d.desiredTitles.trim() !== '' },
  { key: 'desiredLocations', label: 'Desired locations', filled: (d) => d.desiredLocations.trim() !== '' },
  { key: 'minSalary', label: 'Minimum salary', filled: (d) => Number(d.minSalary) > 0 },
]

// The steps for a persona's details, none filled when there are no details at all.
export function detailsSteps(details: DetailsLike | null): DetailsStep[] {
  return FIELDS.map((f) => ({ key: f.key, label: f.label, filled: details !== null && f.filled(details) }))
}

export function detailsCompleteness(details: DetailsLike | null): Completeness {
  const steps = detailsSteps(details)
  const total = steps.length
  const missing = steps.filter((s) => !s.filled).map((s) => s.label)
  const filled = total - missing.length
  return { filled, total, missing, percent: Math.round((filled / total) * 100) }
}

// The Summary page holds only the positioning (the goals have their own page, plan 174): these three fields.
export const POSITIONING_KEYS: readonly DetailsKey[] = ['headline', 'location', 'summary']

export type PositioningLike = Pick<DetailsLike, 'headline' | 'location' | 'summary'>

// The goals are not part of this page: they count as empty here and are filtered out.
const withoutGoals = (d: PositioningLike | null): DetailsLike | null =>
  d === null ? null : { ...d, desiredTitles: '', desiredLocations: '', minSalary: 0 }

export function positioningSteps(details: PositioningLike | null): DetailsStep[] {
  return detailsSteps(withoutGoals(details)).filter((s) => POSITIONING_KEYS.includes(s.key))
}

export function positioningCompleteness(details: PositioningLike | null): Completeness {
  const steps = positioningSteps(details)
  const missing = steps.filter((s) => !s.filled).map((s) => s.label)
  const filled = steps.length - missing.length
  return { filled, total: steps.length, missing, percent: Math.round((filled / steps.length) * 100) }
}
