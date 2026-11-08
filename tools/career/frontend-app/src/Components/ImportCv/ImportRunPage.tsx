// ImportRunPage.tsx — one import run on its own URL, /tools/career/import-cv/{runId} (plan 166, step 2). It shows the run by its
// status: a failed one with the failure notice, one that is still analysing (looked at again every few seconds), a finished one as
// the read-only result of what was proposed and saved, and a plain message for an address that matches no run (deleted or wrong).
import { useEffect, useState } from 'react'
import { deleteCVImportRun, fetchCVImportRun, finishCVImportRun, type CVImportRun } from '../../api'
import { awaitTurnCompletion, fetchTurnStatus, sendMessage } from '../../Cinqo/Conversation/conversation'
import { resumeAnalysis } from './analysisFlow'
import { IMPORT_CV_PATH } from '../../paths'
import { cardClassName, cardStyle } from '../../Shared/cardStyle'
import { UploadIcon } from '../../Shared/Icons/icons'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { FailureNotice } from './FailureNotice'
import { ImportPipeline } from './ImportPipeline'
import { ImportReview } from './ImportReview'
import { ImportRunDetail } from './ImportRunDetail'
import { pipelineStages } from './pipelineRules'
import { RUN_POLL_INTERVAL_MS, runPageView, shouldPollRun } from './runPageRules'

function goToHistory() {
  window.__cinqoToolBridge.navigate(IMPORT_CV_PATH)
}

function BackLink() {
  return (
    <a
      href={IMPORT_CV_PATH}
      onClick={(e) => {
        e.preventDefault()
        goToHistory()
      }}
      className="text-sm text-blue-700 underline hover:text-blue-800"
    >
      ← Import history
    </a>
  )
}

export function ImportRunPage({ runId }: { runId: string }) {
  // undefined: loading, null: no such run.
  const [run, setRun] = useState<CVImportRun | null | undefined>(undefined)
  const [loadError, setLoadError] = useState('')
  const [actionError, setActionError] = useState('')
  const view = runPageView(run)

  useEffect(() => {
    let cancelled = false
    function load() {
      fetchCVImportRun(runId)
        .then((next) => {
          if (cancelled) return
          setRun(next)
          setLoadError('')
        })
        .catch((err: unknown) => {
          if (cancelled) return
          setLoadError(err instanceof Error ? err.message : String(err))
          setRun((prev) => prev ?? null)
        })
    }
    load()
    return () => {
      cancelled = true
    }
  }, [runId])

  useEffect(() => {
    if (!shouldPollRun(view)) return
    const timer = setInterval(() => {
      fetchCVImportRun(runId)
        .then(setRun)
        .catch(() => undefined)
    }, RUN_POLL_INTERVAL_MS)
    return () => {
      clearInterval(timer)
    }
  }, [view, runId])

  // A run that is still "generating" when this page opens and that this page's own code is not finishing (a reload, a new tab) is
  // picked up: the AI's turn is waited for and the run ended as ready or failed, as a fresh analysis would (plan 166, step 4).
  const resumeRunId = run && view === 'generating' ? run.id : null
  const resumeConversationId = run && view === 'generating' ? run.conversationId : null
  useEffect(() => {
    if (resumeRunId === null || resumeConversationId === null) return
    let cancelled = false
    resumeAnalysis(
      {
        turnState: fetchTurnStatus,
        awaitTurn: async (conversationId) => (await awaitTurnCompletion(conversationId)).content,
        askAi: async (conversationId, _runId, followUp) => (await sendMessage(conversationId, followUp ?? '')).content,
        endRun: finishCVImportRun,
        onReply: () => undefined,
        onRunsChanged: () => undefined,
      },
      resumeConversationId,
      resumeRunId,
    )
      .catch(() => undefined)
      .finally(() => {
        if (cancelled) return
        fetchCVImportRun(resumeRunId)
          .then(setRun)
          .catch(() => undefined)
      })
    return () => {
      cancelled = true
    }
  }, [resumeRunId, resumeConversationId])

  async function handleDelete() {
    setActionError('')
    try {
      await deleteCVImportRun(runId)
      goToHistory()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  function viewTranscript() {
    if (run) window.__cinqoToolBridge.openConversation(run.conversationId)
  }

  return (
    <div className="career-v-swing career-a-glow career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UploadIcon className="h-6 w-6" />}
        title={run ? run.originalFilename : 'Import from CV'}
        description={run ? `Imported ${new Date(run.createdAt).toLocaleString()}` : 'One import of a CV.'}
      />
      <div className="mb-3">
        <BackLink />
      </div>

      {view === 'loading' && <p className="text-sm text-gray-400">Loading…</p>}

      {view === 'missing' && (
        <section className={cardClassName} style={cardStyle}>
          <p className="text-sm text-gray-700">
            {loadError !== '' ? `This import could not be loaded: ${loadError}` : 'This import does not exist (any more).'}
          </p>
        </section>
      )}

      {run && (view === 'generating' || view === 'failed') && (
        <ImportPipeline
          stages={pipelineStages({
            state: view === 'generating' ? 'analyzing' : 'error',
            uploadError: false,
            uploading: false,
            inserting: false,
            insertedCount: 0,
            insertErrorCount: 0,
          })}
          onStartOver={goToHistory}
        />
      )}

      {run && view === 'generating' && (
        <section className={cardClassName} style={cardStyle}>
          <p className="text-sm text-gray-700">
            The AI is still analysing this CV, or the page that started it was closed before it finished. This page updates by itself.
          </p>
          <p className="mt-2 text-xs text-gray-400">
            <button type="button" onClick={viewTranscript} className="underline hover:text-gray-600">
              View transcript
            </button>
          </p>
        </section>
      )}

      {run && view === 'failed' && (
        <section className={cardClassName} style={cardStyle}>
          <FailureNotice error={run.error} onViewTranscript={viewTranscript} />
          <p className="mt-3 text-xs text-gray-500">To try again, use “Process again” in the import history.</p>
        </section>
      )}

      {run && view === 'review' && <ImportReview key={run.id} run={run} onStartOver={goToHistory} />}
      {run && view === 'ready' && <ImportRunDetail run={run} onClose={goToHistory} />}

      {run && (
        <div className="mt-4">
          {actionError !== '' && <p className="mb-2 text-sm text-red-700">{actionError}</p>}
          <button
            type="button"
            onClick={() => {
              void handleDelete()
            }}
            className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
          >
            Delete this import
          </button>
        </div>
      )}
    </div>
  )
}
