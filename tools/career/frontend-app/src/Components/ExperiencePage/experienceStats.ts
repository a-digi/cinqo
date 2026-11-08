// Pure numbers for the Experience page's overview. Dates are free text in the data, so
// they are read leniently: YYYY, YYYY-MM or YYYY-MM-DD count, anything else is ignored.
// No React, so it can be script-checked.
export interface ExperienceLike {
  startDate: string
  endDate: string
}

export interface ExperienceSummary {
  entries: number
  // No end date means the role is current.
  current: number
  past: number
  // The earliest start and the latest end that could be read, as written; null when none.
  earliestStart: string | null
  latestEnd: string | null
}

// A sortable number for a date, or null when it is not one of the accepted shapes.
export function dateKey(text: string): number | null {
  const m = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$/.exec(text.trim())
  if (!m) return null
  const year = Number(m[1])
  const month = m[2] ? Number(m[2]) : 1
  const day = m[3] ? Number(m[3]) : 1
  if (year < 1900 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31) return null
  return year * 10000 + month * 100 + day
}

export function summarizeExperience(entries: ExperienceLike[]): ExperienceSummary {
  const s: ExperienceSummary = { entries: entries.length, current: 0, past: 0, earliestStart: null, latestEnd: null }
  let earliest: number | null = null
  let latest: number | null = null
  for (const e of entries) {
    if (e.endDate.trim() === '') s.current++
    else s.past++
    const start = dateKey(e.startDate)
    if (start !== null && (earliest === null || start < earliest)) {
      earliest = start
      s.earliestStart = e.startDate.trim()
    }
    const end = dateKey(e.endDate)
    if (end !== null && (latest === null || end > latest)) {
      latest = end
      s.latestEnd = e.endDate.trim()
    }
  }
  return s
}
