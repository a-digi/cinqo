// Which lines the Job cell of the Jobs table has under the title (plan/ai/tools/career/step-235-jobs-table-company-and-platform-under-the-title.md):
// the company, a note under it for a job linked to a company record, and the platform. A line that has nothing to say is not drawn at all (a job
// found by the link crawler has only a title and an address), so there is never an empty gap. Pure, so it can be script-checked.
import type { CompanyCell } from './companyCell'

export interface JobCellLines {
  // The company line, with companyText as its text.
  company: boolean
  // The small "linked to … · unlink" note under the company: only for a job linked to a company record.
  companyNote: boolean
  // The platform pill.
  platform: boolean
}

// The text of the company line: the name, or "…" for a linked job whose name has not come yet (the companies are still loading).
export function companyText(cell: Pick<CompanyCell, 'text' | 'linkTo'>): string {
  const text = cell.text.trim()
  if (text !== '') return cell.text
  return cell.linkTo !== null ? '…' : ''
}

// platform is what platformLabel gives: the platform's name, "Manual", or "—" for none.
export function jobCellLines(cell: Pick<CompanyCell, 'text' | 'linkTo'>, platform: string): JobCellLines {
  return {
    company: companyText(cell) !== '',
    companyNote: cell.linkTo !== null,
    platform: platform !== '—',
  }
}
