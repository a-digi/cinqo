import { useEffect, useRef, useState } from 'react'
import {
  uploadCV,
  fetchCVImportRuns,
  createCVImportRun,
  finishCVImportRun,
  deleteCVImportRun,
  type CVUploadResult,
  type CVImportRun,
} from '../../api'
import { createConversation, sendMessage } from '../../Cinqo/Conversation/conversation'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { PDFIcon, UploadIcon } from '../../Shared/Icons/icons'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { plural } from '../CompaniesPage/companyStats'
import { IMPORT_RUN_TOOLS, buildImportPrompt } from './buildImportPrompt'
import { RESTRICTION_NOT_APPLIED_MESSAGE, restrictionHolds } from '../../PersonaAi/personaTools'
import { analyseCv } from './analysisFlow'
import { runBadge } from './runStatusRules'
import { importCvRunPathFor } from '../../paths'
import { FailureNotice } from './FailureNotice'
import { ImportPipeline } from './ImportPipeline'
import { pipelineStages } from './pipelineRules'
import { PageTips } from '../../Shared/PageTips/PageTips'

// The 5 areas a CV import populates — Profile/Personas/Personal
// Details/Skills/Experience, matching the real Career entities these
// map onto. See
// plan/ai/tools/career/import-cv/step-04-import-cv-page-and-menu-entry.md.
const AREAS = ['Profile', 'Personas', 'Personal Details', 'Skills', 'Experience'] as const

type PageState = 'upload' | 'analyzing' | 'error'

// Import CV's own page — upload a CV PDF (or process an earlier one again) and the import history. The analysis itself starts here:
// an AI conversation reads the file and hands a proposal over (step 5). As soon as the run is recorded the page moves to the run's
// own URL (ImportRunPage, plan 166), where the analysis is waited for and the proposal is reviewed (ImportReview) and inserted.
// The conversation is a normal, visible one (not hidden): the user gets a full, inspectable transcript.
export function ImportCvPage() {
  const [state, setState] = useState<PageState>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [fileName, setFileName] = useState('')
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')
  const [uploadResult, setUploadResult] = useState<CVUploadResult | null>(null)
  const [conversationId, setConversationId] = useState<string | null>(null)
  // Import history — every past analysis (AI proposal + eventual save
  // outcome), newest first. Deletable, viewable read-only, and
  // reprocessable (re-runs the AI against the same underlying upload,
  // skipping re-upload) — see plan/ai/media/step-05-career-history.md.
  const [runs, setRuns] = useState<CVImportRun[] | null>(null)
  // Set while the read-only history detail view is open; null closes
  // it. A plain state field, not a route — this page has no router of
  // its own (main.tsx's own top-level menu paths are the only routes
  // this tool declares).
  const [runActionError, setRunActionError] = useState('')
  // The AI's last message (one plain sentence since step 155: the proposal itself arrives through the suggest_cv_import tool) —
  // kept so the 'error' state can still show it if the analysis fails.
  const [rawReply, setRawReply] = useState('')
  // The AI platform and model are the one saved setting for the whole tool
  // (Career ▸ Settings). Import CV is a one-shot, fully automatic flow, so it never
  // shows a picker; handleUpload surfaces "no platform" as a real error if there is none.
  const ai = useAiChoice()
  const platform = ai.platform
  const platformsLoaded = ai.loaded

  // Dropzone state — visual only (the click-to-browse path already
  // worked before; this adds a real drag-and-drop path to match the
  // dropzone's own dashed-border affordance, since offering that look
  // without the behavior would be a little deceptive).
  const [isDragOver, setIsDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  function pickFile(next: File | null) {
    setError('')
    setFile(next)
  }

  function loadRuns() {
    fetchCVImportRuns()
      .then(setRuns)
      .catch(() => {
        // Non-fatal, matches the platforms effect's own reasoning above
        // — the upload flow itself doesn't depend on this list, so a
        // failed load just leaves the section empty rather than
        // blocking anything.
        setRuns([])
      })
  }

  useEffect(() => {
    loadRuns()
  }, [])

  // runAnalysis is the shared tail of both a fresh upload
  // (handleUpload) and reprocessing an existing one
  // (handleProcessAgain) — everything from "we have a fileId" onward:
  // create the AI conversation, record the run, send the prompt, and
  // when the AI's turn ends read the proposal it handed over and enter
  // the review state. A reprocess of an upload whose underlying Media
  // file has since expired surfaces as a normal 'error' state here
  // (pdf_to_markdown's own tool-unavailable text) — no separate "is this
  // still live" check is made beforehand.
  //
  // Step 155: the AI does not print the proposal (the chat would show a JSON object). The run is recorded first ("generating");
  // the AI hands the proposal over by calling suggest_cv_import with the run id, which stores it; its last message is one plain
  // sentence. Only this page can end the run (the AI's turn lives in the core app), so it does, as ready or failed. See
  // plan/ai/tools/career/step-155-cv-import-result-by-tool-not-chat.md.
  async function runAnalysis(fileId: string, originalFilename: string) {
    setFileName(originalFilename)
    setState('analyzing')
    try {
      if (!platform) {
        throw new Error('No AI platform is configured — ask an admin to add one under Platforms.')
      }
      const model = ai.model ?? undefined
      await analyseCv({
        startConversation: async () => {
          // The AI may only use the tools an import needs: the server enforces it, not just the prompt. Unrestricted, a weak model
          // called create_profile with an invented name and wrote it into the user's data (plan 165).
          const conversation = await createConversation({
            title: `Import CV: ${originalFilename}`,
            platformId: platform.id,
            model,
            allowedTools: [...IMPORT_RUN_TOOLS],
          })
          if (!restrictionHolds(conversation.allowedTools, IMPORT_RUN_TOOLS)) throw new Error(RESTRICTION_NOT_APPLIED_MESSAGE)
          return conversation.id
        },
        recordRun: (conversationId) => createCVImportRun({ fileId, originalFilename, conversationId }),
        askAi: async (conversationId, runId, followUp) =>
          (await sendMessage(conversationId, followUp ?? buildImportPrompt(fileId, runId))).content,
        endRun: finishCVImportRun,
        onConversation: setConversationId,
        // The run exists: its own page takes over from here (the analysis goes on and ends the run; the page polls it).
        onRun: (runId) => {
          window.__cinqoToolBridge.navigate(importCvRunPathFor(runId))
        },
        onReply: setRawReply,
        onRunsChanged: loadRuns,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setState('error')
    }
  }

  async function handleUpload() {
    if (!file) return
    setUploading(true)
    setError('')

    let upload: CVUploadResult
    try {
      upload = await uploadCV(file)
      setUploadResult(upload)
      setUploading(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setUploading(false)
      return // stays on the 'upload' state — the inline error above already shows it
    }

    // The upload itself succeeded — any failure from here on is a real
    // 'error' state, not an inline retry-in-place message, since the
    // uploaded file (and its capability URL) are already consumed.
    await runAnalysis(upload.fileId, file.name)
  }

  async function handleProcessAgain(run: CVImportRun) {
    setRunActionError('')
    await runAnalysis(run.mediaFileId, run.originalFilename)
  }

  async function handleDeleteRun(run: CVImportRun) {
    setRunActionError('')
    try {
      await deleteCVImportRun(run.id)
      setRuns((prev) => (prev ? prev.filter((r) => r.id !== run.id) : prev))
    } catch (err) {
      setRunActionError(err instanceof Error ? err.message : String(err))
    }
  }

  function startOver() {
    setState('upload')
    setFile(null)
    setFileName('')
    setUploadResult(null)
    setConversationId(null)
    setRawReply('')
    setError('')
  }

  function viewTranscript() {
    if (conversationId) window.__cinqoToolBridge.openConversation(conversationId)
  }

  // The header's pill is the history count; where the import is lives in the pipeline.
  const headerPill = runs !== null && runs.length > 0 ? { text: plural(runs.length, 'past import', 'past imports') } : undefined
  const stages = pipelineStages({
    state,
    uploadError: state === 'upload' && error !== '',
    uploading,
    inserting: false,
    insertedCount: 0,
    insertErrorCount: 0,
  })

  return (
    <div className="career-v-swing career-a-glow career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UploadIcon className="h-6 w-6" />}
        title="Import from CV"
        description="Upload a CV and let the AI propose a profile, persona, summary, skills and experience from it. You review everything before it is saved."
        pill={headerPill}
      />
      {/* Steps 105 to 108: the tips sit in a column of their own beside the page (under it on a narrow window). */}
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <ImportPipeline stages={stages} onStartOver={startOver} />

          {state === 'upload' && (
            <section className={`career-enter career-delay-1 ${cardClassName}`} style={cardStyle}>
              <div
                role="button"
                tabIndex={0}
                onClick={() => {
                  fileInputRef.current?.click()
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') fileInputRef.current?.click()
                }}
                onDragOver={(e) => {
                  e.preventDefault()
                  setIsDragOver(true)
                }}
                onDragLeave={() => {
                  setIsDragOver(false)
                }}
                onDrop={(e) => {
                  e.preventDefault()
                  setIsDragOver(false)
                  pickFile(e.dataTransfer.files.length > 0 ? e.dataTransfer.files[0] : null)
                }}
                className={`flex cursor-pointer flex-col items-center rounded-2xl border-2 border-dashed px-6 py-10 text-center transition-colors ${
                  isDragOver ? 'border-blue-400 bg-blue-50' : 'border-blue-200 bg-white hover:border-blue-400 hover:bg-blue-50'
                }`}
              >
                <div className="relative mb-3 h-16 w-16 text-gray-300">
                  <PDFIcon className="h-16 w-16" />
                  <span className="absolute -bottom-1 -right-1 flex h-6 w-6 items-center justify-center rounded-full bg-gray-900 text-white">
                    <UploadIcon className="h-3.5 w-3.5" />
                  </span>
                </div>
                <p className="mb-1 text-sm font-medium text-gray-700">{file ? file.name : 'Click to choose a PDF, or drag one here'}</p>
                <p className="text-xs text-gray-400">PDF up to 10MB</p>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".pdf,application/pdf"
                  onChange={(e) => {
                    pickFile(e.target.files?.[0] ?? null)
                  }}
                  className="hidden"
                />
              </div>

              <div className="mt-3 min-h-[1.2em] text-sm text-red-700">{error}</div>
              <button
                type="button"
                onClick={() => {
                  void handleUpload()
                }}
                disabled={!file || uploading || !platformsLoaded}
                className="mt-1 rounded-md bg-gray-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-40"
              >
                {uploading ? 'Uploading…' : 'Upload'}
              </button>
            </section>
          )}

          {state === 'upload' && runs !== null && runs.length > 0 && (
            <section className={`career-enter career-delay-2 mt-4 ${cardClassName}`} style={cardStyle}>
              <h2 className="mb-2 text-sm font-semibold text-gray-900">Import history</h2>
              <div className="mb-2 min-h-[1.2em] text-sm text-red-700">{runActionError}</div>
              <ul className="divide-y divide-gray-100">
                {runs.map((run) => {
                  const badge = runBadge(run)
                  return (
                    <li key={run.id} className="flex flex-wrap items-center justify-between gap-3 py-2 text-sm">
                      <div className="min-w-0">
                        <a
                          href={importCvRunPathFor(run.id)}
                          onClick={(e) => {
                            e.preventDefault()
                            window.__cinqoToolBridge.navigate(importCvRunPathFor(run.id))
                          }}
                          className="block truncate text-gray-700 hover:text-blue-700 hover:underline"
                        >
                          {run.originalFilename}
                        </a>
                        <p className="text-xs text-gray-400">{new Date(run.createdAt).toLocaleString()}</p>
                      </div>
                      <div className="flex shrink-0 items-center gap-3">
                        {badge && (
                          <span
                            title={badge.title}
                            className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.tone === 'red' ? 'bg-red-100 text-red-700' : 'bg-amber-100 text-amber-700'}`}
                          >
                            {badge.label}
                          </span>
                        )}
                        <a
                          href={importCvRunPathFor(run.id)}
                          onClick={(e) => {
                            e.preventDefault()
                            window.__cinqoToolBridge.navigate(importCvRunPathFor(run.id))
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Open
                        </a>
                        <button
                          type="button"
                          onClick={() => {
                            void handleProcessAgain(run)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Process again
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            void handleDeleteRun(run)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </div>
                      {run.status === 'failed' && (
                        <div className="w-full">
                          <FailureNotice error={run.error} />
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            </section>
          )}

          {state === 'analyzing' && (
            <div className="career-stagger space-y-4">
              <p className="text-sm text-gray-500">
                Analyzing “{fileName}”…
                {uploadResult && <span className="ml-1 font-mono text-xs text-gray-400">({uploadResult.fileId})</span>}
              </p>
              {conversationId && (
                <p className="text-xs text-gray-400">
                  AI conversation:{' '}
                  <button type="button" onClick={viewTranscript} className="underline hover:text-gray-600">
                    view transcript
                  </button>
                </p>
              )}
              {AREAS.map((area) => (
                <section key={area} className="rounded-md border border-gray-200 p-4 shadow-sm">
                  <h2 className="text-base font-semibold text-gray-900">{area}</h2>
                </section>
              ))}
            </div>
          )}

          {state === 'error' && (
            <section className={cardClassName} style={cardStyle}>
              <div className="mb-3">
                <FailureNotice error={error} onViewTranscript={conversationId ? viewTranscript : undefined} />
              </div>
              {rawReply && (
                <details className="mb-3">
                  <summary className="cursor-pointer text-xs text-gray-500">Show the AI's last message</summary>
                  <pre className="mt-2 whitespace-pre-wrap break-words text-xs text-gray-600">{rawReply}</pre>
                </details>
              )}
              <button
                type="button"
                onClick={startOver}
                className="rounded-full border border-blue-200 bg-white px-3.5 py-1.5 text-sm text-blue-700 hover:bg-blue-50"
              >
                Start over
              </button>
            </section>
          )}
        </div>
        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-72">
          <PageTips page="importCv" orientation="vertical" />
        </aside>
      </div>
    </div>
  )
}
