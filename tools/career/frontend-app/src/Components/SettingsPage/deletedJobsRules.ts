// Deleted jobs (step 133): the pure rules of the "Deleted jobs" section on Settings → Job cleanup and of the delete
// confirmation on the Jobs page, kept apart from the components so they can be checked on their own.
import type { DeletedJob, DeletedJobsList } from '../../api'

export function reasonLabel(reason: string): string {
  if (reason === 'deleted') return 'Deleted by you'
  if (reason === 'low_match') return 'Removed by the cleanup: matched no profile well'
  return reason
}

// The score the job had when it was deleted; '' when it never had one.
export function scoreText(score: number | null): string {
  return score === null ? '' : `last match ${String(score)} %`
}

// "2026-06-30 12:00:00" (UTC, what the server stores) as the viewer's local date and time; the text itself when it
// cannot be read.
export function deletedAtText(text: string): string {
  const date = new Date(`${text.replace(' ', 'T')}Z`)
  return Number.isNaN(date.getTime()) ? text : date.toLocaleString()
}

function plural(n: number, one: string, many: string): string {
  return `${String(n)} ${n === 1 ? one : many}`
}

// The line above the list.
export function listHeadline(l: DeletedJobsList): string {
  if (!l.available) return 'The list of deleted jobs is not available: nothing is being remembered right now.'
  if (l.total === 0) return 'No deleted jobs are remembered: the crawler may add any job it finds.'
  const base = `${plural(l.total, 'job', 'jobs')} the crawler will not add again.`
  return l.items.length < l.total ? `${base} Showing the newest ${String(l.items.length)}.` : base
}

// What a row says about a job: company and portal where known, joined with a middle dot.
export function rowDetail(j: DeletedJob): string {
  return [j.company, j.location, j.portalName].filter((p) => p !== '').join(' · ')
}

// Deleting a job you added by hand is plain; deleting a crawled job is remembered, so the person is asked first and told
// what it means. Returns null when no question is needed.
export function deleteQuestion(
  source: 'crawled' | 'manual' | undefined,
  title: string,
): { title: string; message: string; confirmLabel: string } | null {
  if (source === 'manual') return null
  return {
    title: 'Delete this job?',
    message: `“${title}” will be deleted, and the crawler will not add it again. You can allow it again under Settings → Job cleanup.`,
    confirmLabel: 'Delete',
  }
}
