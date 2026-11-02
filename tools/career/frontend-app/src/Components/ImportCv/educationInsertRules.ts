// Pure rules of the CV import's education (plan/ai/tools/career/step-175-cv-import-education.md, step 3): how a proposed entry reads in
// the review, how an entry the profile already has is found (so nothing is added twice), and what is sent to the profile's education
// route. No React, so they can be script-checked.
import type { Education, EducationInput } from '../../api'
import { degreeLine, periodText } from '../EducationPage/educationRules'

interface EntryLike {
  institution: string
  degree: string
  fieldOfStudy: string
  startDate: string
  endDate: string
  description: string
}

// What a row says when the profile has the entry already (it is not added again).
export const ALREADY_THERE = 'already in the profile'

// "B.Sc. in Physics · MIT · 2010-09 – 2014"; the degree part and the period are left out when empty, a missing institution is said.
export function educationLabel(e: Pick<EntryLike, 'institution' | 'degree' | 'fieldOfStudy' | 'startDate' | 'endDate'>): string {
  const period = periodText(e.startDate, e.endDate)
  return [degreeLine(e), e.institution.trim() === '' ? '(no institution)' : e.institution.trim(), period === '—' ? '' : period]
    .filter((part) => part !== '')
    .join(' · ')
}

const norm = (s: string) => s.trim().toLowerCase()

// The entry the profile already has that is this one (institution, degree, field of study and start date, any case, trimmed), or undefined.
export function findEducation(existing: readonly Education[], e: EntryLike): Education | undefined {
  return existing.find(
    (x) =>
      norm(x.institution) === norm(e.institution) &&
      norm(x.degree) === norm(e.degree) &&
      norm(x.fieldOfStudy) === norm(e.fieldOfStudy) &&
      x.startDate.trim() === e.startDate.trim(),
  )
}

// What is sent to the profile's education route for a proposed entry (the server trims and checks it again).
export function educationInputOf(e: EntryLike): EducationInput {
  return {
    institution: e.institution.trim(),
    degree: e.degree.trim(),
    fieldOfStudy: e.fieldOfStudy.trim(),
    startDate: e.startDate.trim(),
    endDate: e.endDate.trim(),
    description: e.description,
  }
}
