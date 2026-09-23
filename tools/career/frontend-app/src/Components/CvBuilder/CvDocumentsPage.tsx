import { useEffect, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvPersonaDefaults,
  fetchCvDocuments,
  createCvDocument,
  updateCvDocumentTitle,
  deleteCvDocument,
  type CvData,
  type CvTemplate,
  type CvDocument,
} from '../../api'
import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'
import { Field } from '../Field/Field'
import { CvComponentsSidebar } from './CvComponentsSidebar'
import { CvPreviewModal } from './CvPreviewModal'
import { ActionMenu, type ActionMenuItem } from '../../Shared/ActionMenu/ActionMenu'
import { EyeIcon, TrashIcon, PlusIcon } from '../../Shared/Icons/icons'

type Mode = { kind: 'list' } | { kind: 'builder'; editingDocumentId: string | null }

// CV Builder — a deterministic (no AI), manual "pick a persona, pick a
// design, get a PDF" library, distinct from Jobs' own AI-tailored
// per-job CV flow. One page, two modes (list / builder) rather than
// two separate routes — this codebase's own existing pages
// (PersonasPage, etc.) already keep list+create+edit on one page; the
// builder here just needs meaningfully more screen space (a two-column
// layout with an editable sidebar) than a plain inline form would fit
// alongside a list. See
// plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
//
// No toast/confirm-dialog primitive exists anywhere in this frontend
// bundle (confirmed by reading it directly) — errors surface as a
// plain inline red string, matching every other page here exactly;
// delete uses a native window.confirm(), a small, zero-dependency
// safety net this bundle's OTHER delete buttons skip entirely, kept
// here since destroying a generated PDF is easy to regret.
export function CvDocumentsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [documents, setDocuments] = useState<CvDocument[]>([])
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  const [mode, setMode] = useState<Mode>({ kind: 'list' })
  const [error, setError] = useState('')
  const [previewDoc, setPreviewDoc] = useState<CvDocument | null>(null)

  const [templateId, setTemplateId] = useState('')
  const [title, setTitle] = useState('')
  const [cvData, setCvData] = useState<CvData | null>(null)
  const [generating, setGenerating] = useState(false)

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
    setError('')
    setTitle('')
    setTemplateId(templates[0]?.id ?? '')
    setCvData(null)
    setMode({ kind: 'builder', editingDocumentId: null })
    fetchCvPersonaDefaults(personaId)
      .then(setCvData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function startEditCv(doc: CvDocument) {
    setError('')
    setTitle(doc.title)
    setTemplateId(doc.templateId)
    setCvData(doc.data)
    setMode({ kind: 'builder', editingDocumentId: doc.id })
  }

  function cancelBuilder() {
    setMode({ kind: 'list' })
    setCvData(null)
  }

  function handleGenerate() {
    if (!personaId || !cvData) return
    if (!templateId) {
      setError('Pick a template')
      return
    }
    if (!title.trim()) {
      setError('Title is required')
      return
    }
    setError('')
    setGenerating(true)
    createCvDocument(personaId, templateId, title.trim(), cvData)
      .then(() => {
        loadDocuments(personaId)
        setMode({ kind: 'list' })
        setCvData(null)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setGenerating(false)
      })
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
    if (!window.confirm(`Delete "${doc.title}"? This cannot be undone.`)) return
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
          setPreviewDoc(doc)
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

      {mode.kind === 'list' && personaId && (
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

      {mode.kind === 'builder' && (
        <div className="flex items-start gap-6">
          <div className="min-w-0 flex-1 space-y-4">
            <Field label="Title" value={title} onChange={setTitle} />

            <div>
              <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Template</h3>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                {templates.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => {
                      setTemplateId(t.id)
                    }}
                    className={`rounded-md border p-3 text-left text-sm shadow-sm ${
                      templateId === t.id ? 'border-gray-900 ring-1 ring-gray-900' : 'border-gray-200 hover:border-gray-300'
                    }`}
                  >
                    <p className="font-medium text-gray-900">{t.name}</p>
                    <p className="mt-1 text-xs text-gray-500">{t.description}</p>
                  </button>
                ))}
              </div>
            </div>

            <div className="flex gap-2">
              <button
                type="button"
                onClick={handleGenerate}
                disabled={generating || !cvData || !templateId || !title.trim()}
                className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
              >
                {generating ? 'Generating…' : 'Generate'}
              </button>
              <button
                type="button"
                onClick={cancelBuilder}
                disabled={generating}
                className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
              >
                Cancel
              </button>
            </div>
          </div>

          {cvData && <CvComponentsSidebar data={cvData} onChange={setCvData} />}
        </div>
      )}

      {previewDoc && (
        <CvPreviewModal
          mediaFileId={previewDoc.mediaFileId}
          title={previewDoc.title}
          onClose={() => {
            setPreviewDoc(null)
          }}
        />
      )}
    </div>
  )
}
