// ImportRunPage.tsx — one import run on its own URL, /tools/career/import-cv/{runId} (plan 166, step 2). It shows the run by its
// status: a failed one with the failure notice, one that is still analysing (looked at again every few seconds), a finished one as
// the read-only result of what was proposed and saved, and a plain message for an address that matches no run (deleted or wrong).
import { useEffect, useState } from 'react'
import { deleteCVImportRun, fetchCVImportRun, finishCVImportRun, type CVImportRun } from '../../api'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import { awaitTurnCompletion, fetchTurnStatus, sendMessage } from '../../Cinqo/Conversation/conversation'
import { isAnalysisActive, resumeAnalysis } from './analysisFlow'
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
import { RunActivity } from './RunActivity'
import { declineRun } from './declineRun'
import { RestartPanel } from './RestartPanel'
import { DECLINED_MESSAGE, restartOffer } from './runOutcome'
import { ABANDONED_MESSAGE, isAbandonedCodeRun } from './runActivityRules'

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
  // Starting the import again from this page (plan 242, step 3): the panel is open, a decline waits for the person's confirmation, and is under way.
  const [restartOpen, setRestartOpen] = useState(false)
  const [confirmDecline, setConfirmDecline] = useState(false)
  const [declining, setDeclining] = useState(false)
  const offer = restartOffer(view)

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
  // A run the page drives in code (driver "code") is never picked up this way: its state lives in the page that started it, and asking the AI
  // to "continue" in its conversation would fail it (plan 209).
  const resumable = run && view === 'generating' && run.driver !== 'code'
  const resumeRunId = resumable ? run.id : null
  const resumeConversationId = resumable ? run.conversationId : null
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

  // A run driven in code whose page is gone (nothing stored for minutes, and this page is not driving it) is ended as failed, so the history does
  // not keep it "analysing" for ever.
  useEffect(() => {
    if (run?.driver !== 'code') return
    const timer = setInterval(() => {
      if (!isAbandonedCodeRun(run, new Date(), isAnalysisActive(run.id))) return
      finishCVImportRun(run.id, { status: 'failed', error: ABANDONED_MESSAGE })
        .catch(() => undefined)
        .finally(() => {
          fetchCVImportRun(run.id)
            .then(setRun)
            .catch(() => undefined)
        })
    }, 15000)
    return () => {
      clearInterval(timer)
    }
  }, [run])

  async function handleDelete() {
    setActionError('')
    try {
      await deleteCVImportRun(runId)
      goToHistory()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  // Declines everything that was proposed (nothing was saved from it) and opens the panel to start again. The run is read again, so the page shows it as
  // declined at once.
  async function handleDecline() {
    setConfirmDecline(false)
    setActionError('')
    setDeclining(true)
    try {
      await declineRun(runId)
      setRun(await fetchCVImportRun(runId))
      setRestartOpen(true)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setDeclining(false)
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

      {run && restartOpen && offer === 'restart' && (
        <RestartPanel
          run={run}
          onClose={() => {
            setRestartOpen(false)
          }}
        />
      )}

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
          <RunActivity run={run} />
          <p className="mt-3 text-xs text-gray-400">
            This page updates by itself.
            {run.driver === 'code' && ' If the page that started the import is closed before it finishes, the import stops.'}{' '}
            <button type="button" onClick={viewTranscript} className="underline hover:text-gray-600">
              View transcript
            </button>
          </p>
        </section>
      )}

      {run && view === 'failed' && (
        <section className={cardClassName} style={cardStyle}>
          <FailureNotice error={run.error} onViewTranscript={viewTranscript} />
          {offer === 'restart' && !restartOpen && (
            <button
              type="button"
              onClick={() => {
                setRestartOpen(true)
              }}
              className="mt-3 rounded-full border border-blue-200 bg-white px-3.5 py-1.5 text-sm text-blue-700 hover:bg-blue-50"
            >
              Start again
            </button>
          )}
        </section>
      )}

      {run && view === 'review' && <ImportReview key={run.id} run={run} onStartOver={goToHistory} />}
      {run && view === 'ready' && <ImportRunDetail run={run} onClose={goToHistory} />}

      {run && view === 'declined' && (
        <div className="space-y-4">
          <section className={cardClassName} style={cardStyle}>
            <p className="text-sm text-gray-700">{DECLINED_MESSAGE}</p>
            <p className="mt-1 text-xs text-gray-500">What the AI proposed is shown below; none of it was taken.</p>
            {offer === 'restart' && !restartOpen && (
              <button
                type="button"
                onClick={() => {
                  setRestartOpen(true)
                }}
                className="mt-3 rounded-full border border-blue-200 bg-white px-3.5 py-1.5 text-sm text-blue-700 hover:bg-blue-50"
              >
                Start again
              </button>
            )}
          </section>
          <ImportRunDetail run={run} onClose={goToHistory} />
        </div>
      )}

      {run && (
        <div className="mt-4">
          {actionError !== '' && <p className="mb-2 text-sm text-red-700">{actionError}</p>}
          {offer === 'decline-and-restart' && (
            <button
              type="button"
              onClick={() => {
                setConfirmDecline(true)
              }}
              disabled={declining}
              className="mr-2 rounded-full border border-blue-200 bg-white px-3 py-1 text-xs text-blue-700 hover:bg-blue-50 disabled:opacity-40"
            >
              {declining ? 'Declining…' : 'Decline all and start again'}
            </button>
          )}
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
      <ConfirmationModal
        open={confirmDecline}
        title="Decline this import?"
        message="Nothing from this import is saved. It stays in your history as declined, and you can start again."
        confirmLabel="Decline and start again"
        onConfirm={() => {
          void handleDecline()
        }}
        onCancel={() => {
          setConfirmDecline(false)
        }}
      />
    </div>
  )
}
