// Pure rules of the "All CVs" list on the CV Documents page (plan/ai/tools/career/cv-delete-from-list.md, step 3). No React, so they
// can be script-checked.
import type { CvDocument } from '../../api'
import { createdLabel } from './cvCardRules'
import { personaNames } from './cvPersonaRules'

export interface AllCvRow {
  id: string
  title: string
  // "Backend, Data Lead": every persona the CV is attached to, the one it was built from first; '' when the list gave none.
  personas: string
  // "30 Oct 2026", or '' when the timestamp cannot be read.
  created: string
}

// One row per CV, newest first (the order the server gives, kept stable for CVs made at the same moment), none dropped.
export function allCvRows(docs: Pick<CvDocument, 'id' | 'title' | 'personas' | 'personaName' | 'createdAt'>[]): AllCvRow[] {
  return docs
    .map((doc, index) => ({ doc, index }))
    .sort((a, b) => (a.doc.createdAt < b.doc.createdAt ? 1 : a.doc.createdAt > b.doc.createdAt ? -1 : a.index - b.index))
    .map(({ doc }) => ({ id: doc.id, title: doc.title, personas: personaNames(doc), created: createdLabel(doc.createdAt) }))
}

// The line under a row's title: "Backend, Data Lead · 30 Oct 2026", with whatever part is missing left out.
export function rowFacts(row: Pick<AllCvRow, 'personas' | 'created'>): string {
  return [row.personas, row.created].filter((part) => part !== '').join(' · ')
}

export const NO_CVS_TEXT = 'No CVs yet. Open a persona to create one.'
