import { useState } from 'react'
import type { PersonaAiRun } from '../api'
import { PAGE_ACCENT } from '../Components/JobsPage/jobsTheme'
import { ConfirmationModal } from '../Shared/ConfirmationModal/ConfirmationModal'
import { TrashIcon } from '../Shared/Icons/icons'
import { canRemove, canReview, formatRunTime, summarizeRun, visibleRuns } from './runDisplay'

const TONE_STYLE = {
  working: { background: '#f3e8ff', color: '#6b21a8' },
  ready: { background: '#dcfce7', color: '#166534' },
  failed: { background: '#fee2e2', color: '#991b1b' },
} as const

// The AI history of one profile (step 112): every "create personas with AI" run, newest first: how
// it went, what the user asked for, what was proposed and created, the chat behind it, and
// removing it from the history (the personas created from it stay). This is the one place these
// chats are listed: they are hidden conversations, not in the Conversations list.
export function PersonaAiHistory({
  runs,
  onReview,
  onRemove,
}: {
  runs: PersonaAiRun[]
  onReview: (run: PersonaAiRun) => void
  onRemove: (run: PersonaAiRun) => Promise<void>
}) {
  const [showAll, setShowAll] = useState(false)
  const [removing, setRemoving] = useState<PersonaAiRun | null>(null)
  const [error, setError] = useState('')
  if (runs.length === 0) return null
  const { shown, hidden } = visibleRuns(runs, showAll)

  function confirmRemove() {
    const run = removing
    if (!run) return
    setRemoving(null)
    setError('')
    onRemove(run).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : String(err))
    })
  }

  return (
    <section aria-label="AI history" className="mb-6 rounded-2xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-gray-400">AI history</p>
      <ul className="space-y-4">
        {shown.map((run) => {
          const summary = summarizeRun(run)
          return (
            <li key={run.id} className="border-b border-gray-100 pb-4 last:border-b-0 last:pb-0">
              <div className="flex items-center justify-between gap-2">
                <span
                  className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold"
                  style={TONE_STYLE[summary.tone]}
                >
                  {summary.tone === 'working' && (
                    <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-purple-600" aria-hidden="true" />
                  )}
                  {summary.label}
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="text-[11px] text-gray-400">{formatRunTime(run.createdAt)}</span>
                  {canRemove(run) && (
                    <button
                      type="button"
                      aria-label="Remove this run from the history"
                      title="Remove this run from the history. The personas created from it stay."
                      onClick={() => {
                        setRemoving(run)
                      }}
                      className="flex h-6 w-6 items-center justify-center rounded-full text-gray-400 hover:bg-red-50 hover:text-red-700"
                    >
                      <TrashIcon />
                    </button>
                  )}
                </span>
              </div>
              <p className="mt-2 text-sm text-gray-600">{summary.detail}</p>
              {run.focus && <p className="mt-1.5 text-xs text-gray-500">Focus: {run.focus}</p>}
              <div className="mt-2.5 flex flex-wrap items-center gap-2">
                {canReview(run) && (
                  <button
                    type="button"
                    onClick={() => {
                      onReview(run)
                    }}
                    className="rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                  >
                    Review and create
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => {
                    window.__cinqoToolBridge.openConversation(run.conversationId)
                  }}
                  className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                >
                  Open chat
                </button>
              </div>
            </li>
          )
        })}
      </ul>
      {hidden > 0 && (
        <button
          type="button"
          onClick={() => {
            setShowAll(true)
          }}
          className="mt-3 text-xs text-blue-700 hover:underline"
        >
          Show all {runs.length} runs
        </button>
      )}
      {error && <p className="mt-3 text-xs text-red-700">{error}</p>}

      <ConfirmationModal
        open={removing !== null}
        title="Remove from the history"
        message="Remove this run from the history? The personas created from it stay, and the chat itself is not deleted."
        confirmLabel="Remove"
        variant="danger"
        onConfirm={confirmRemove}
        onCancel={() => {
          setRemoving(null)
        }}
      />
    </section>
  )
}
