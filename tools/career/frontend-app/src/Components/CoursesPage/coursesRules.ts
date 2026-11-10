// Pure rules of the Courses page and form (plan/ai/tools/career/step-231-hobbies-courses-languages.md): the early check of what the server
// checks again (the title, the completed date, the certificate link, the lengths), when a link may be shown as a link, and the count. No
// React, so they can be script-checked.

import { charCount } from '../../Shared/charCount'

const SHAPE = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$/

// Whether a date is empty (not given) or YYYY, YYYY-MM or YYYY-MM-DD with a year of 1900 to 2100, a month of 1 to 12 and a day of 1 to 31.
export function isValidDate(text: string): boolean {
  const t = text.trim()
  if (t === '') return true
  const m = SHAPE.exec(t)
  if (!m) return false
  const year = Number(m[1])
  const month = m[2] ? Number(m[2]) : 1
  const day = m[3] ? Number(m[3]) : 1
  return year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31
}

// Whether a link is an absolute http or https address with a host and no spaces. The list shows a link as a link only when this is true,
// so a stored javascript: or data: address is never made clickable.
export function isHttpLink(link: string): boolean {
  const t = link.trim()
  if (t === '' || /\s/.test(t)) return false
  try {
    const u = new URL(t)
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.host !== ''
  } catch {
    return false
  }
}

export const MAX_SHORT_LENGTH = 200
export const MAX_LONG_LENGTH = 2000

function tooLong(text: string, max: number): boolean {
  return charCount(text) > max
}

// What stops a form from being sent: '' when it can be.
export function formProblem(f: { title: string; provider: string; completedDate: string; credentialUrl: string; description: string }): string {
  if (f.title.trim() === '') return 'The title is required'
  if (tooLong(f.title.trim(), MAX_SHORT_LENGTH)) return `The title is too long (at most ${String(MAX_SHORT_LENGTH)} characters)`
  if (tooLong(f.provider.trim(), MAX_SHORT_LENGTH)) return `The provider is too long (at most ${String(MAX_SHORT_LENGTH)} characters)`
  if (!isValidDate(f.completedDate)) return 'The completed date must be YYYY, YYYY-MM or YYYY-MM-DD'
  const link = f.credentialUrl.trim()
  if (link !== '' && !isHttpLink(link)) return 'The certificate link must be an http or https address'
  if (tooLong(link, MAX_LONG_LENGTH)) return `The certificate link is too long (at most ${String(MAX_LONG_LENGTH)} characters)`
  if (tooLong(f.description, MAX_LONG_LENGTH)) return `The description is too long (at most ${String(MAX_LONG_LENGTH)} characters)`
  return ''
}

// "Completed 2024-05", or "No date" when none is given.
export function completedText(date: string): string {
  const d = date.trim()
  return d === '' ? 'No date' : `Completed ${d}`
}

// "3 courses", "1 course".
export function coursesCountText(count: number): string {
  return `${String(count)} ${count === 1 ? 'course' : 'courses'}`
}
