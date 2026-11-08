// The rules and the words of deleting the selected jobs in one go (step 149), kept apart from the page so they can be checked on
// their own: what a selection holds (crawled, added by hand, on a board), the question asked before deleting, the button's label and
// the notice afterwards. Pure (no React, no network). See plan/ai/tools/career/step-149-batch-delete-jobs.md.
import { plural } from '../CompaniesPage/companyStats'
import type { Selection, SelectedJob } from './recrawlRules'

export interface DeleteBreakdown {
  total: number
  // Crawled jobs are remembered when deleted, so the crawler does not add them again (a job whose source is unknown counts as crawled,
  // like the single delete's question does).
  crawled: number
  // Jobs added by hand are gone for good.
  manual: number
  // Jobs that are on at least one profile's job board: their cards go with them.
  onBoard: number
}

export function deleteBreakdown(jobs: readonly SelectedJob[]): DeleteBreakdown {
  const manual = jobs.filter((j) => j.source === 'manual').length
  return {
    total: jobs.length,
    crawled: jobs.length - manual,
    manual,
    onBoard: jobs.filter((j) => (j.boardCount ?? 0) > 0).length,
  }
}

// The label of the selection bar's delete button and of the confirmation's button.
export function deleteButtonLabel(count: number): string {
  return `Delete ${plural(count, 'job', 'jobs')}`
}

export interface DeleteQuestion {
  title: string
  message: string
  confirmLabel: string
}

// The question asked before deleting the selected jobs: what it means for the crawled ones (not added again, can be allowed again),
// the ones added by hand (gone for good) and the ones on a job board (their cards go too). Null for an empty selection. Always asked,
// whatever the selection holds.
export function deleteQuestion(jobs: readonly SelectedJob[]): DeleteQuestion | null {
  const b = deleteBreakdown(jobs)
  if (b.total === 0) return null
  const parts: string[] = []
  if (b.crawled > 0) {
    parts.push(
      b.crawled === 1
        ? '1 crawled job will not be added again by the crawler (you can allow it again under Settings → Job cleanup).'
        : `${String(b.crawled)} crawled jobs will not be added again by the crawler (you can allow them again under Settings → Job cleanup).`,
    )
  }
  if (b.manual > 0) {
    parts.push(
      b.manual === 1
        ? '1 job you added by hand will be gone for good.'
        : `${String(b.manual)} jobs you added by hand will be gone for good.`,
    )
  }
  if (b.onBoard > 0) {
    parts.push(
      b.onBoard === 1
        ? '1 of them is on a job board: its card goes too.'
        : `${String(b.onBoard)} of them are on a job board: their cards go too.`,
    )
  }
  return { title: `${deleteButtonLabel(b.total)}?`, message: parts.join(' '), confirmLabel: deleteButtonLabel(b.total) }
}

// What the server answered to a delete.
export interface DeleteAnswer {
  deleted: number
  notFound: number
}

// The notice after a delete: "12 jobs deleted." and, when some ids were already gone, "12 jobs deleted; 2 were not found (already gone)."
export function deleteNotice(a: DeleteAnswer): string {
  const gone = a.notFound > 0 ? `${plural(a.notFound, 'job', 'jobs')} ${a.notFound === 1 ? 'was' : 'were'} not found (already gone)` : ''
  if (a.deleted === 0) return gone === '' ? 'No job was deleted.' : `No job was deleted: ${gone}.`
  const base = `${plural(a.deleted, 'job', 'jobs')} deleted`
  return gone === '' ? `${base}.` : `${base}; ${gone}.`
}

// The message when the delete failed (partway when some jobs went before it stopped): the server's words, then what was done.
export function deleteFailureText(message: string, deleted: number): string {
  const reason = message.replace(/[.\s]+$/, '')
  if (deleted > 0) {
    return `Could not delete all the jobs: ${reason}. ${plural(deleted, 'job', 'jobs')} ${deleted === 1 ? 'was' : 'were'} deleted before it stopped.`
  }
  return `Could not delete the jobs: ${reason}.`
}

// The selection after a delete: on success the requested jobs leave it (the others stay); after a failure it is left as it is, because
// the answer says how many went, not which, and the reloaded list shows what is left.
export function selectionAfterDelete(selection: Selection, requested: readonly string[], ok: boolean): Selection {
  if (!ok) return selection
  const next = new Set(selection)
  for (const id of requested) next.delete(id)
  return next
}
