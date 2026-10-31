import { useEffect, useRef, useState } from 'react'
import {
  fetchCvTemplates,
  fetchCvDocuments,
  fetchPersonas,
  fetchProfiles,
  attachCvToPersona,
  detachCvFromPersona,
  updateCvDocumentTitle,
  deleteCvDocument,
  CvDocumentInUseError,
  type CvDocumentJob,
  type CvTemplate,
  type CvDocument,
  type Persona,
  type Profile,
} from '../../api'
import { profileLabel } from '../ProfileSwitcher/profileLabel'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { Modal } from '../../Shared/Modal/Modal'
import { inUseFooter, inUseHeading, inUseLines } from './cvDeleteDisplay'
import { PDFIcon, PlusIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { CvCardGrid, CvCardGridSkeleton } from './CvCardGrid'
import { openCvBuilderFor } from './openCvBuilder'
import { PageTips } from '../../Shared/PageTips/PageTips'
import { Comments } from '../../Shared/Comments/Comments'
import { COMMENT_TYPE_CV } from '../../Shared/Comments/commentTypes'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { CreateCvWithAi } from '../../CvAi/CreateCvWithAi'
import { failedRunToShow, hasRunningCvRun, newlyCreatedCv, openRunChat, chatsByCv } from '../../CvAi/cvAiRules'
import { CvAiChatsModal } from '../../CvAi/CvAiChatsModal'
import { useDismissedCvRuns } from '../../CvAi/useDismissedCvRuns'
import { useCvAiRuns } from '../../CvAi/useCvAiRuns'

const CV_DOCUMENTS_PATH = '/tools/career/cv-documents'

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
export function CvDocumentsPage({ personaId }: { personaId: string }) {
  // The persona comes from the address (/tools/career/cv-documents/:personaId), not from a dropdown.
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [personasLoaded, setPersonasLoaded] = useState(false)
  const [documents, setDocuments] = useState<CvDocument[]>([])
  // True while a persona's documents are being read (the grid shows placeholder tiles).
  const [loading, setLoading] = useState(false)
  const [templates, setTemplates] = useState<CvTemplate[]>([])
  // Create a CV with AI (step 115): the AI in use, the persona's runs, the CV just created.
  const ai = useAiChoice()
  const [personaName, setPersonaName] = useState('')
  const [newCvId, setNewCvId] = useState<string | null>(null)
  const watchedRuns = useRef(new Set<string>())
  const handledRuns = useRef(new Set<string>())
  const [scrollToNew, setScrollToNew] = useState(false)
  const aiRuns = useCvAiRuns(personaId)
  const [dismissedRuns, dismissRun] = useDismissedCvRuns()
  const [chatsOpen, setChatsOpen] = useState(false)
  // The CV whose comments are open in the modal.
  const [commentsFor, setCommentsFor] = useState<CvDocument | null>(null)
  const workingRun = aiRuns.runs.find((r) => r.status === 'generating') ?? null
  // Every persona, to offer the ones a CV can be attached to (step 114).
  const [allPersonas, setAllPersonas] = useState<Persona[]>([])
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

  useEffect(() => {
    fetchPersonas()
      .then(setAllPersonas)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setPersonasLoaded(true)
      })
    fetchProfiles()
      .then(setProfiles)
      .catch(() => {
        setProfiles([])
      })
  }, [])

  // Attaches or detaches, then reads the list again: detaching the persona being looked at takes
  // the CV out of this list.
  function changeLink(change: Promise<void>) {
    setError('')
    change
      .then(() => {
        if (personaId) loadDocuments(personaId)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

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

  // The persona's name, for the AI dialog.
  useEffect(() => {
    setPersonaName(allPersonas.find((p) => p.id === personaId)?.name ?? '')
  }, [allPersonas, personaId])

  useEffect(() => {
    setNewCvId(null)
    setScrollToNew(false)
  }, [personaId])

  // A run this page saw working that has ended: read the CVs again, and show the new one.
  useEffect(() => {
    for (const r of aiRuns.runs) if (r.status === 'generating') watchedRuns.current.add(r.id)
    const ended = aiRuns.runs.filter((r) => r.status !== 'generating' && watchedRuns.current.has(r.id) && !handledRuns.current.has(r.id))
    if (ended.length === 0 || !personaId) return
    for (const r of ended) handledRuns.current.add(r.id)
    loadDocuments(personaId)
    const created = newlyCreatedCv(aiRuns.runs, watchedRuns.current)
    if (created !== null) {
      setNewCvId(created)
      setScrollToNew(true)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aiRuns.runs])

  useEffect(() => {
    if (!scrollToNew || newCvId === null) return
    const el = document.getElementById(`cv-${newCvId}`)
    if (!el) return
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setScrollToNew(false)
  }, [scrollToNew, newCvId, documents])

  function startNewCv() {
    openCvBuilderFor(personaId)
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

  const persona = allPersonas.find((p) => p.id === personaId)
  const profile = profiles.find((p) => p.id === persona?.profileId)
  const backLink = (
    <a
      href={CV_DOCUMENTS_PATH}
      onClick={(e) => {
        e.preventDefault()
        window.__cinqoToolBridge.navigate(CV_DOCUMENTS_PATH)
      }}
      className="mb-3 inline-block text-sm font-medium text-blue-700 hover:underline"
    >
      ← All CVs
    </a>
  )

  if (!persona) {
    return (
      <div className="max-w-3xl p-6 font-sans text-gray-900">
        {backLink}
        <div className="text-sm text-gray-500">{personasLoaded ? 'Persona not found' : 'Loading…'}</div>
      </div>
    )
  }

  return (
    <div className="career-v-pop career-a-float career-t-type max-w-7xl p-6 font-sans text-gray-900">
      {backLink}
      <PageHeaderCard
        icon={<PDFIcon className="h-6 w-6" />}
        title={`CVs: ${persona.name}`}
        description={`${profile ? `${profileLabel(profile)} · ` : ''}Generate a CV PDF from this persona's own data, using one of a few fixed designs.`}
        pill={!loading ? { text: plural(documents.length, 'CV', 'CVs') } : undefined}
      />
      {/* Steps 105 to 108: the tips sit in a column of their own on the right of the CVs (under them on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <div className="mb-3 flex flex-wrap items-center justify-start gap-3">
            <button
              type="button"
              onClick={startNewCv}
              className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
            >
              <PlusIcon /> New CV
            </button>
            <CreateCvWithAi
              ai={ai}
              personaName={personaName}
              disabledReason={
                !ai.platform
                  ? 'No AI platform is available. Choose one in Settings.'
                  : hasRunningCvRun(aiRuns.runs)
                    ? 'The AI is already creating a CV for this persona.'
                    : ''
              }
              onStart={(focus) => aiRuns.start({ id: personaId, name: personaName }, focus, ai)}
            />
          </div>

          {error && <p className="mb-4 text-sm text-red-700">{error}</p>}

          {personaId && hasRunningCvRun(aiRuns.runs) && (
            <p
              role="status"
              className="mb-4 flex items-center gap-2 rounded-2xl border-2 px-4 py-3 text-sm text-purple-900"
              style={{ borderColor: '#c4b5fd', background: 'linear-gradient(160deg, #ede9fe 0%, #f5f3ff 100%)' }}
            >
              <span className="h-2 w-2 animate-pulse rounded-full bg-purple-600" aria-hidden="true" />
              The AI is creating a CV. It appears below as soon as it is done; you can stay on this page.
              {workingRun && (
                <button
                  type="button"
                  onClick={() => {
                    openRunChat(workingRun)
                  }}
                  className="ml-1 text-blue-700 underline"
                >
                  Open the chat
                </button>
              )}
            </p>
          )}
          {personaId &&
            (() => {
              const failed = failedRunToShow(aiRuns.runs, dismissedRuns)
              return failed ? (
                <div role="alert" className="mb-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
                  <p className="font-medium">The AI could not create a CV.</p>
                  <p className="mt-0.5 break-words">{failed.error ?? 'No reason was recorded.'}</p>
                  <div className="mt-2 flex gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        openRunChat(failed)
                      }}
                      className="rounded-full border border-red-200 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100"
                    >
                      Open chat
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        dismissRun(failed.id)
                      }}
                      className="rounded-full border border-red-200 bg-white px-3 py-1 text-xs font-medium text-red-800 hover:bg-red-100"
                    >
                      Dismiss
                    </button>
                  </div>
                </div>
              ) : null
            })()}
          {personaId && aiRuns.error !== '' && <p className="mb-4 text-sm text-red-700">{aiRuns.error}</p>}
          {personaId && aiRuns.runs.length > 0 && (
            <p className="mb-4 text-xs">
              <button
                type="button"
                onClick={() => {
                  setChatsOpen(true)
                }}
                className="text-blue-700 hover:underline"
              >
                AI chats ({aiRuns.runs.length})
              </button>
            </p>
          )}

          {personaId && (
            <>
              <p className="mb-4 text-xs text-gray-500">Click a CV to edit it; hover for preview, rename and delete.</p>

              {loading && documents.length === 0 && <CvCardGridSkeleton />}

              {!loading && documents.length === 0 && (
                <div
                  className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
                  style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
                >
                  No CV documents yet. Create the first one with “New CV”.
                </div>
              )}

              {documents.length > 0 && (
                <CvCardGrid
                  className="career-stagger"
                  docs={documents}
                  templates={templates}
                  aiChats={chatsByCv(aiRuns.runs)}
                  highlightedId={newCvId}
                  management={{
                    onRename: handleRename,
                    onComments: setCommentsFor,
                    onDelete: handleDelete,
                    allPersonas,
                    onAttach: (doc, id) => {
                      changeLink(attachCvToPersona(doc.id, id))
                    },
                    onDetach: (doc, id) => {
                      changeLink(detachCvFromPersona(doc.id, id))
                    },
                  }}
                />
              )}
            </>
          )}
        </div>
        <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="cvs" orientation="vertical" />
        </aside>
      </div>

      <Modal
        open={commentsFor !== null}
        title={commentsFor ? `Comments: ${commentsFor.title}` : 'Comments'}
        onClose={() => {
          setCommentsFor(null)
        }}
        size="wide"
      >
        {commentsFor && <Comments entityType={COMMENT_TYPE_CV} entityId={commentsFor.id} inModal />}
      </Modal>

      <CvAiChatsModal
        open={chatsOpen}
        runs={aiRuns.runs}
        docs={documents}
        onClose={() => {
          setChatsOpen(false)
        }}
      />

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
