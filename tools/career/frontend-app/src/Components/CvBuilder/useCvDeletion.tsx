import { useState, type ReactNode } from 'react'
import { deleteCvDocument, CvDocumentInUseError, type CvDocument, type CvDocumentJob } from '../../api'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { Modal } from '../../Shared/Modal/Modal'
import { attemptDelete, deleteQuestion } from './cvDeleteFlow'
import { inUseFooter, inUseHeading, inUseLines } from './cvDeleteDisplay'

// The whole "delete a CV document" flow, the same wherever it is offered (plan/ai/tools/career/cv-delete-from-list.md, step 2): the first
// confirmation, the second one when jobs use the CV (it lists every one of them and unlinks them), the call, and what to do with the
// result. The page gives it what it needs to react: `onDeleted` once the CV and its file are gone, `onError` with the text to show (an
// empty text clears the earlier error, called before every attempt). The page renders `dialogs` once and calls `requestDelete(doc)`.
export function useCvDeletion({ onDeleted, onError }: { onDeleted: (doc: CvDocument) => void; onError: (message: string) => void }): {
  requestDelete: (doc: CvDocument) => void
  dialogs: ReactNode
} {
  const [pending, setPending] = useState<CvDocument | null>(null)
  // Set when the server says jobs use the CV being deleted (step 98): the second confirmation lists every one of them.
  const [inUse, setInUse] = useState<{ doc: CvDocument; jobs: CvDocumentJob[] } | null>(null)

  const inUseJobsOf = (err: unknown): CvDocumentJob[] | null => (err instanceof CvDocumentInUseError ? err.jobs : null)

  function confirmDelete() {
    if (!pending) return
    const doc = pending
    setPending(null)
    onError('')
    void attemptDelete(() => deleteCvDocument(doc.id), inUseJobsOf).then((result) => {
      if (result.kind === 'deleted') onDeleted(doc)
      else if (result.kind === 'in_use') setInUse({ doc, jobs: result.jobs })
      else onError(result.message)
    })
  }

  // The second confirmation: the jobs lose their link, then the CV is deleted.
  function confirmDeleteAndUnlink() {
    if (!inUse) return
    const { doc } = inUse
    setInUse(null)
    onError('')
    void attemptDelete(
      () => deleteCvDocument(doc.id, { unlinkJobs: true }),
      () => null,
    ).then((result) => {
      if (result.kind === 'deleted') onDeleted(doc)
      else if (result.kind === 'error') onError(result.message)
    })
  }

  const dialogs = (
    <>
      <ConfirmationModal
        open={pending !== null}
        title="Delete CV document"
        message={pending ? deleteQuestion(pending.title) : ''}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={confirmDelete}
        onCancel={() => {
          setPending(null)
        }}
      />

      <Modal
        open={inUse !== null}
        title="Delete CV document"
        onClose={() => {
          setInUse(null)
        }}
      >
        {inUse && (
          <>
            <p className="mb-2 text-sm font-medium text-gray-900">{inUseHeading(inUse.jobs.length)}</p>
            <ul className="mb-3 max-h-64 list-disc overflow-y-auto pl-5 pr-1 text-sm text-gray-700">
              {inUseLines(inUse.jobs).map((line, i) => (
                <li key={inUse.jobs[i].id}>{line}</li>
              ))}
            </ul>
            <p className="mb-5 text-sm text-gray-600">{inUseFooter(inUse.jobs.length)}</p>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => {
                  setInUse(null)
                }}
                className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={confirmDeleteAndUnlink}
                className="rounded-md bg-red-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-red-700"
              >
                Delete and unlink
              </button>
            </div>
          </>
        )}
      </Modal>
    </>
  )

  return { requestDelete: setPending, dialogs }
}
