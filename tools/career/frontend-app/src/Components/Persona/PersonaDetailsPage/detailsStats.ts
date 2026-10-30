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

const FIELDS: { label: string; filled: (d: DetailsLike) => boolean }[] = [
  { label: 'Headline', filled: (d) => d.headline.trim() !== '' },
  { label: 'Location', filled: (d) => d.location.trim() !== '' },
  { label: 'Minimum salary', filled: (d) => Number(d.minSalary) > 0 },
  { label: 'Desired titles', filled: (d) => d.desiredTitles.trim() !== '' },
  { label: 'Desired locations', filled: (d) => d.desiredLocations.trim() !== '' },
  { label: 'Summary', filled: (d) => d.summary.trim() !== '' },
]

export function detailsCompleteness(details: DetailsLike | null): Completeness {
  const total = FIELDS.length
  if (!details) return { filled: 0, total, missing: FIELDS.map((f) => f.label), percent: 0 }
  const missing = FIELDS.filter((f) => !f.filled(details)).map((f) => f.label)
  const filled = total - missing.length
  return { filled, total, missing, percent: Math.round((filled / total) * 100) }
}
