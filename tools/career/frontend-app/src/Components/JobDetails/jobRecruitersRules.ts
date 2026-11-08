// Pure rules of the Recruiters card on a job's page (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 5): how a mapped
// recruiter is shown, the e-mail link, and the list after one was removed. No React, so they can be script-checked.
import type { JobRecruiter } from '../../api'
import { companyText, recruiterLabel } from '../Recruiters/recruiterMapRules'

export interface RecruiterRow {
  id: string
  // The name (or the e-mail when there is no name).
  title: string
  // The companies (or "Freelancer") and, when the name is shown, the e-mail: "Acme, Beta · ada@acme.test" (a recruiter with no company
  // shows none).
  subtitle: string
}

export function recruiterRow(r: JobRecruiter): RecruiterRow {
  const title = recruiterLabel(r)
  // When the e-mail is already the title it is not repeated in the subtitle.
  const email = r.email.trim() !== '' && r.email.trim() !== title ? r.email.trim() : ''
  return { id: r.id, title, subtitle: [companyText(r), email].filter((part) => part !== '').join(' · ') }
}

// A mailto link for a recruiter's e-mail, or null when there is none.
export function mailtoHref(email: string): string | null {
  const trimmed = email.trim()
  return trimmed === '' ? null : `mailto:${trimmed}`
}

// The list without one recruiter (a new array; the others keep their order).
export function withoutRecruiter(list: readonly JobRecruiter[], id: string): JobRecruiter[] {
  return list.filter((r) => r.id !== id)
}
