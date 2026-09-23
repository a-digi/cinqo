import { useEffect, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvDocuments,
  updateCvDocumentTitle,
  deleteCvDocument,
  mediaDownloadUrl,
  type CvTemplate,
  type CvDocument,
} from '../../api'
import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'
import { ActionMenu, type ActionMenuItem } from '../../Shared/ActionMenu/ActionMenu'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { EyeIcon, TrashIcon, PlusIcon } from '../../Shared/Icons/icons'

const CV_BUILDER_PATH = '/tools/career/cv-builder'

// CV Builder's own list page — creating/editing a CV now lives on its
// own dedicated route (CvBuilderPage, a 5-step wizard), per your own
// instruction. This page is list-only: pick a persona, see that
// persona's own CV documents, and navigate away to the builder route
// for "+ New CV" (?personaId=) or a row's own "Edit" (?id=). See
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
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  const [error, setError] = useState('')
  const [pendingDelete, setPendingDelete] = useState<CvDocument | null>(null)

  useEffect(() => {
    fetchCvTemplates()
      .then(setTemplates)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  function loadDocuments(forPersonaId: string) {
    setError('')
    fetchCvDocuments(forPersonaId)
      .then(setDocuments)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
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
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function buildDocumentActionItems(doc: CvDocument): ActionMenuItem[] {
    return [
      {
        key: 'preview',
        label: 'Preview',
        icon: <EyeIcon />,
        onClick: () => {
          window.open(mediaDownloadUrl(doc.mediaFileId), '_blank', 'noopener,noreferrer')
        },
      },
      {
        key: 'edit',
        label: 'Edit',
        onClick: () => {
          startEditCv(doc)
        },
      },
      {
        key: 'rename',
        label: 'Rename',
        onClick: () => {
          handleRename(doc)
        },
      },
      {
        key: 'delete',
        label: 'Delete',
        icon: <TrashIcon />,
        variant: 'danger',
        onClick: () => {
          handleDelete(doc)
        },
      },
    ]
  }

  const templateName = (id: string) => templates.find((t) => t.id === id)?.name ?? id

  return (
    <div className="p-6">
      <h1 className="mb-1 text-xl font-semibold text-gray-900">CV Documents</h1>
      <p className="mb-4 text-sm text-gray-500">Generate a CV PDF from a persona's own data, using one of a few fixed designs.</p>

      <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

      {error && <p className="mb-4 text-sm text-red-700">{error}</p>}

      {personaId && (
        <>
          <button
            type="button"
            onClick={startNewCv}
            className="mb-4 flex items-center gap-1 rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800"
          >
            <PlusIcon /> New CV
          </button>

          {documents.length === 0 && <p className="text-sm text-gray-400">No CV documents yet.</p>}

          {documents.length > 0 && (
            <div className="overflow-hidden rounded-md border border-gray-200">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="bg-gray-50 text-xs uppercase text-gray-500">
                    <th className="p-3 font-medium">Title</th>
                    <th className="p-3 font-medium">Template</th>
                    <th className="p-3 font-medium">Created</th>
                    <th className="p-3 font-medium" />
                  </tr>
                </thead>
                <tbody>
                  {documents.map((doc) => (
                    <tr key={doc.id}>
                      <td className="border-b border-gray-200 p-3 text-gray-900">{doc.title}</td>
                      <td className="border-b border-gray-200 p-3 text-gray-600">{templateName(doc.templateId)}</td>
                      <td className="border-b border-gray-200 p-3 text-gray-600">{new Date(doc.createdAt).toLocaleString()}</td>
                      <td className="border-b border-gray-200 p-3 text-right">
                        <ActionMenu triggerLabel={`Actions for ${doc.title}`} items={buildDocumentActionItems(doc)} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
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
    </div>
  )
}
