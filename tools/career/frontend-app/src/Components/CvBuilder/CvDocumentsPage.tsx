import { useEffect, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvDocuments,
  updateCvDocumentTitle,
  deleteCvDocument,
  mediaDownloadUrl,
  CvDocumentInUseError,
  type CvDocumentJob,
  type CvTemplate,
  type CvDocument,
} from '../../api'
import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { Modal } from '../../Shared/Modal/Modal'
import { inUseFooter, inUseHeading, inUseLines } from './cvDeleteDisplay'
import { PDFIcon, PlusIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { CvCard } from './CvCard'
import { CARD_MIN_REM, templateLabel } from './cvCardRules'

const CV_BUILDER_PATH = '/tools/career/cv-builder'

// CV Builder's own list page — creating/editing a CV now lives on its
// own dedicated route (CvBuilderPage, a 5-step wizard), per your own
// instruction. This page is list-only: pick a persona, see that
// persona's own CV documents as cards with a preview (the way the Media page
// shows its files), and navigate away to the builder route for "+ New CV"
// (?personaId=) or a card's own "Edit" (?id=). See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
//
// No toast primitive exists anywhere in this frontend bundle (confirmed
// by reading it directly) — errors surface as a plain inline red
// string, matching every other page here exactly. Delete goes through
// the shared ConfirmationModal (not a native window.confirm() any
// more, per your own instruction) — pendingDelete tracks which
// document is awaiting confirmation, since the modal is async/
// callback-based rather than a blocking call this function can just
// read a return value from.
export function CvDocumentsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [documents, setDocuments] = useState<CvDocument[]>([])
  // True while a persona's documents are being read (the grid shows placeholder tiles).
  const [loading, setLoading] = useState(false)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  const [error, setError] = useState('')
  const [pendingDelete, setPendingDelete] = useState<CvDocument | null>(null)
  // Set when the server says jobs use the CV being deleted (step 98): the second
  // confirmation lists every one of them.
  const [inUse, setInUse] = useState<{ doc: CvDocument; jobs: CvDocumentJob[] } | null>(null)

  useEffect(() => {
    fetchCvTemplates()
      .then(setTemplates)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  function loadDocuments(forPersonaId: string) {
    setError('')
    setLoading(true)
    fetchCvDocuments(forPersonaId)
      .then(setDocuments)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }

  useEffect(() => {
    if (personaId) loadDocuments(personaId)
  }, [personaId])

  function startNewCv() {
    if (!personaId) return
    window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?personaId=${encodeURIComponent(personaId)}`)
  }

  function startEditCv(doc: CvDocument) {
    window.__cinqoToolBridge.navigate(`${CV_BUILDER_PATH}?id=${encodeURIComponent(doc.id)}`)
  }

  function handleRename(doc: CvDocument) {
    const next = window.prompt('New title', doc.title)
    if (!next?.trim() || next.trim() === doc.title) return
    setError('')
    updateCvDocumentTitle(doc.id, next.trim())
      .then((updated) => {
        setDocuments((prev) => prev.map((d) => (d.id === updated.id ? updated : d)))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(doc: CvDocument) {
    setPendingDelete(doc)
  }

  function confirmDelete() {
    if (!pendingDelete) return
    const doc = pendingDelete
    setPendingDelete(null)
    setError('')
    deleteCvDocument(doc.id)
      .then(() => {
        setDocuments((prev) => prev.filter((d) => d.id !== doc.id))
      })
      .catch((err: unknown) => {
        if (err instanceof CvDocumentInUseError) {
          setInUse({ doc, jobs: err.jobs })
          return
        }
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  // The second confirmation: the jobs lose their link, then the CV is deleted.
  function confirmDeleteAndUnlink() {
    if (!inUse) return
    const { doc } = inUse
    setInUse(null)
    setError('')
    deleteCvDocument(doc.id, { unlinkJobs: true })
      .then(() => {
        setDocuments((prev) => prev.filter((d) => d.id !== doc.id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function previewPdf(doc: CvDocument) {
    window.open(mediaDownloadUrl(doc.mediaFileId), '_blank', 'noopener,noreferrer')
  }

  return (
    <div className="max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<PDFIcon className="h-6 w-6" />}
        title="CV Documents"
        description="Generate a CV PDF from a persona's own data, using one of a few fixed designs."
        pill={personaId && !loading ? { text: plural(documents.length, 'CV', 'CVs') } : undefined}
      />

      <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

      {error && <p className="mb-4 text-sm text-red-700">{error}</p>}

      {personaId && (
        <>
          <div className="mb-4 flex items-center justify-between gap-3">
            <p className="text-xs text-gray-500">Click a CV to edit it; hover for preview, rename and delete.</p>
            <button
              type="button"
              onClick={startNewCv}
              className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
            >
              <PlusIcon /> New CV
            </button>
          </div>

          {/* A uniquely-named grid class, as the Media page does: an installed tool's globally-injected
              Tailwind stylesheet can redeclare a bare `.grid-cols-*` and collapse a plain Tailwind grid. */}
          <style>{`
            .cv-card-grid { display: grid; gap: 1.5rem 1rem; grid-template-columns: repeat(auto-fill, minmax(${CARD_MIN_REM}rem, 1fr)); }
          `}</style>

          {loading && documents.length === 0 && (
            <div className="cv-card-grid" aria-label="Loading CVs">
              {[0, 1, 2, 3].map((i) => (
                <div key={i} className="animate-pulse">
                  <div className="aspect-[3/4] rounded-xl bg-gray-100" />
                  <div className="mt-2 h-3 w-3/4 rounded bg-gray-100" />
                  <div className="mt-2 h-3 w-1/2 rounded bg-gray-100" />
                </div>
              ))}
            </div>
          )}

          {!loading && documents.length === 0 && (
            <div
              className="rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No CV documents yet. Create the first one with “New CV”.
            </div>
          )}

          {documents.length > 0 && (
            <div className="cv-card-grid">
              {documents.map((doc) => (
                <CvCard
                  key={doc.id}
                  doc={doc}
                  templateName={templateLabel(templates, doc.templateId)}
                  onPreview={() => {
                    previewPdf(doc)
                  }}
                  onEdit={() => {
                    startEditCv(doc)
                  }}
                  onRename={() => {
                    handleRename(doc)
                  }}
                  onDelete={() => {
                    handleDelete(doc)
                  }}
                />
              ))}
            </div>
          )}
        </>
      )}

      <ConfirmationModal
        open={pendingDelete !== null}
        title="Delete CV document"
        message={pendingDelete ? `Delete "${pendingDelete.title}"? This cannot be undone.` : ''}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={confirmDelete}
        onCancel={() => {
          setPendingDelete(null)
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
    </div>
  )
}
