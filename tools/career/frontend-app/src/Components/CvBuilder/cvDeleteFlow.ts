// Pure rules of deleting a CV document (plan/ai/tools/career/cv-delete-from-list.md, step 2), shared by every page that deletes one and
// kept apart from the components so they can be script-checked: the question the first dialog asks, and what became of one delete call.
import type { CvDocumentJob } from '../../api'

// The first confirmation. It says what the delete really does: the PDF goes too (the server deletes the file first and keeps the CV
// when it cannot), and a CV attached to several personas is removed from all of them.
export function deleteQuestion(title: string): string {
  return `Delete "${title}"? Its PDF file is deleted too, and it is removed from every target role it is attached to. This cannot be undone.`
}

export type DeleteAttempt = { kind: 'deleted' } | { kind: 'in_use'; jobs: CvDocumentJob[] } | { kind: 'error'; message: string }

// What became of one delete call. `inUseJobsOf` recognises the server's "jobs use this CV" answer (it gives the jobs) and says null for
// every other failure: only the first call of a delete may end in the second dialog, so the call after the confirmation passes a
// recogniser that never matches and shows what went wrong as an error.
export async function attemptDelete(
  remove: () => Promise<void>,
  inUseJobsOf: (err: unknown) => CvDocumentJob[] | null,
): Promise<DeleteAttempt> {
  try {
    await remove()
    return { kind: 'deleted' }
  } catch (err) {
    const jobs = inUseJobsOf(err)
    if (jobs) return { kind: 'in_use', jobs }
    return { kind: 'error', message: err instanceof Error ? err.message : String(err) }
  }
}
