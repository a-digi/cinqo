import { useEffect, useState } from 'react'
import { fetchAllCvDocuments, fetchCvTemplates, linkJobCv, mediaDownloadUrl, type CvDocument, type CvTemplate, type Job } from '../../api'
import { Modal } from '../../Shared/Modal/Modal'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { documentSubtitle, filterDocuments, replaceConfirmation, selectableDocuments } from './cvActions'

interface SelectCvModalProps {
  // The job to attach a CV to; the modal is open while this is set.
  job: Job | null
  onClose: () => void
  // Called after the CV was attached, so the page can reload its rows.
  onLinked: () => void
}

// "Select existing" (step 98): pick one of the CVs already made and attach it to
// the job as its current CV. Lists every persona's CVs newest first, filterable by
// title or persona; only a CV with a rendered PDF can be chosen. Replacing a CV
// the job already has asks first. See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
export function SelectCvModal({ job, onClose, onLinked }: SelectCvModalProps) {
  const [documents, setDocuments] = useState<CvDocument[]>([])
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirmingReplace, setConfirmingReplace] = useState(false)

  const jobId = job?.id ?? null
  useEffect(() => {
    if (jobId === null) return
    setQuery('')
    setSelectedId(null)
    setError('')
    setConfirmingReplace(false)
    setLoading(true)
    Promise.all([fetchAllCvDocuments(), fetchCvTemplates().catch(() => [] as CvTemplate[])])
      .then(([docs, tpls]) => {
        setDocuments(docs)
        setTemplates(tpls)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }, [jobId])

  const selectable = selectableDocuments(documents)
  const visible = filterDocuments(selectable, query)
  const replace = job ? replaceConfirmation(job, 'existing') : null

  function attach() {
    if (!job || !selectedId) return
    setSaving(true)
    setError('')
    linkJobCv(job.id, selectedId, 'existing')
      .then(() => {
        onLinked()
        onClose()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  function handleUse() {
    if (replace) setConfirmingReplace(true)
    else attach()
  }

  return (
    <>
      <Modal open={job !== null} title="Select an existing CV" onClose={onClose}>
        {job && (
          <p className="mb-3 text-xs text-gray-500">
            For <span className="font-medium text-gray-700">{job.title}</span>
            {job.company ? ` at ${job.company}` : ''}
          </p>
        )}
        {loading && <p className="text-sm text-gray-500">Loading…</p>}
        {error && <p className="mb-2 text-sm text-red-700">{error}</p>}
        {!loading && selectable.length === 0 && !error && (
          <p className="text-sm text-gray-400">No CVs yet. Use “Generate manually” to create one.</p>
        )}
        {!loading && selectable.length > 0 && (
          <>
            <input
              type="text"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
              }}
              placeholder="Filter by title or persona…"
              aria-label="Filter CVs"
              className="mb-2 w-full rounded-md border border-gray-200 px-3 py-1.5 text-sm focus:border-gray-400 focus:outline-none"
            />
            {visible.length === 0 && <p className="py-2 text-sm text-gray-400">No CV matches your filter.</p>}
            <ul className="flex max-h-72 flex-col gap-1.5 overflow-y-auto pr-1">
              {visible.map((doc) => (
                <li key={doc.id}>
                  <label
                    className={`flex cursor-pointer items-start gap-2.5 rounded-md border px-3 py-2 text-sm ${
                      selectedId === doc.id ? 'border-gray-900 bg-gray-50' : 'border-gray-200 hover:bg-gray-50'
                    }`}
                  >
                    <input
                      type="radio"
                      name="existing-cv"
                      checked={selectedId === doc.id}
                      onChange={() => {
                        setSelectedId(doc.id)
                      }}
                      className="mt-1"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium text-gray-900">{doc.title}</span>
                      <span className="block text-xs text-gray-500">{documentSubtitle(doc, templates)}</span>
                      {doc.jobId && doc.jobId !== job?.id && <span className="block text-xs text-gray-400">Made for another job</span>}
                    </span>
                    <a
                      href={mediaDownloadUrl(doc.mediaFileId)}
                      target="_blank"
                      rel="noreferrer"
                      onClick={(e) => {
                        e.stopPropagation()
                      }}
                      className="shrink-0 text-xs text-gray-600 underline hover:text-gray-900"
                    >
                      Open PDF
                    </a>
                  </label>
                </li>
              ))}
            </ul>
          </>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleUse}
            disabled={!selectedId || saving}
            className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
          >
            {saving ? 'Attaching…' : 'Use this CV'}
          </button>
        </div>
      </Modal>
      <ConfirmationModal
        open={confirmingReplace && replace !== null}
        title={replace?.title ?? ''}
        message={replace?.message ?? ''}
        confirmLabel={replace?.confirmLabel}
        onConfirm={() => {
          setConfirmingReplace(false)
          attach()
        }}
        onCancel={() => {
          setConfirmingReplace(false)
        }}
      />
    </>
  )
}
