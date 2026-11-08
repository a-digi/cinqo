// Pure rules of the Education page and form (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md, step 7): how an
// entry reads, the period, the early check of the dates the server checks again, and the counts. No React, so they can be
// script-checked.
import type { Education } from '../../api'

// "B.Sc. in Physics", "Physics", "B.Sc.", or '' when there is neither a degree nor a field.
export function degreeLine(e: Pick<Education, 'degree' | 'fieldOfStudy'>): string {
  const degree = e.degree.trim()
  const field = e.fieldOfStudy.trim()
  if (degree !== '' && field !== '') return `${degree} in ${field}`
  return degree !== '' ? degree : field
}

// "2010-09 – 2014", "2023 – present", "– 2014" (no start), "—" when there are no dates at all.
export function periodText(start: string, end: string): string {
  const s = start.trim()
  const e = end.trim()
  if (s === '' && e === '') return '—'
  if (e === '') return `${s} – present`
  return `${s} – ${e}`.trim()
}

// An entry without an end date is still going on.
export function isOngoing(e: Pick<Education, 'endDate'>): boolean {
  return e.endDate.trim() === ''
}

const SHAPE = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$/

// A sortable number for a valid date, null when the text is not one (empty is "not set", also null: see datesProblem).
export function dateKey(text: string): number | null {
  const m = SHAPE.exec(text.trim())
  if (!m) return null
  const year = Number(m[1])
  const month = m[2] ? Number(m[2]) : 1
  const day = m[3] ? Number(m[3]) : 1
  if (year < 1900 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31) return null
  return year * 10000 + month * 100 + day
}

// The first thing wrong with the two dates, in words, or '' when they are fine: the same checks the server makes, so the form says it
// before sending.
export function datesProblem(start: string, end: string): string {
  const startSet = start.trim() !== ''
  const endSet = end.trim() !== ''
  const s = dateKey(start)
  const e = dateKey(end)
  if (startSet && s === null) return 'The start date must be YYYY, YYYY-MM or YYYY-MM-DD'
  if (endSet && e === null) return 'The end date must be YYYY, YYYY-MM or YYYY-MM-DD'
  if (s !== null && e !== null && e < s) return 'The end date cannot be before the start date'
  return ''
}

// What stops a form from being sent: '' when it can be.
export function formProblem(f: { institution: string; startDate: string; endDate: string }): string {
  if (f.institution.trim() === '') return 'The institution is required'
  return datesProblem(f.startDate, f.endDate)
}

// "3 entries", "1 entry".
export function educationCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'entry' : 'entries'}`
}
