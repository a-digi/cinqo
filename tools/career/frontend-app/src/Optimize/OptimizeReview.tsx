import { useState } from 'react'
import type { OptimizeDecided, OptimizeDecision, OptimizeRun, OptimizeSuggestion } from '../api'
import { Modal } from '../Shared/Modal/Modal'
import { decisionsForAll, diffWords, groupSuggestions, plainWhiteSpace, refusedOf, type DiffPart } from './optimizeRules'

// The review of the suggestions of one run (plan 258): each improved text next to the text it replaces, with the changed words marked, and Approve or Decline for each. Nothing is
// written until Approve; a declined suggestion is deleted. A suggestion whose text changed after the AI read it is marked and can only be declined.
function Parts({ parts, tone }: { parts: DiffPart[]; tone: 'removed' | 'added' }) {
  const mark = tone === 'removed' ? 'bg-red-100 text-red-900' : 'bg-green-100 text-green-900'
  return (
    <p className="whitespace-pre-wrap break-words text-sm text-gray-800">
      {parts.map((p, i) =>
        p.changed ? (
          <mark key={i} className={`rounded-sm px-0.5 ${mark}`}>
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </p>
  )
}

function SuggestionCard({
  s,
  busy,
  error,
  onDecide,
}: {
  s: OptimizeSuggestion
  busy: boolean
  error: string
  onDecide: (approve: boolean) => void
}) {
  const diff = diffWords(s.before, s.after)
  return (
    <li data-testid="optimize-suggestion" className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm font-semibold text-gray-900">
          {s.label || 'A text that is no longer there'} <span className="font-normal text-gray-400">· {s.field}</span>
        </p>
        <div className="flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              onDecide(false)
            }}
            className="rounded-md border border-gray-200 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            Decline
          </button>
          <button
            type="button"
            disabled={busy || s.stale}
            onClick={() => {
              onDecide(true)
            }}
            className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800 disabled:opacity-40"
          >
            Approve
          </button>
        </div>
      </div>
      {s.stale && (
        <p data-testid="optimize-stale" className="mb-2 rounded-md bg-amber-50 px-2 py-1 text-xs text-amber-800">
          This text changed after the AI read it, so this suggestion can only be declined. Ask the AI again afterwards.
        </p>
      )}
      <div className="grid gap-3 md:grid-cols-2">
        <div>
          <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">Now</p>
          <Parts parts={plainWhiteSpace(diff.before)} tone="removed" />
        </div>
        <div>
          <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-400">Suggested</p>
          <Parts parts={plainWhiteSpace(diff.after)} tone="added" />
        </div>
      </div>
      {s.reason && <p className="mt-2 text-xs italic text-gray-500">{s.reason}</p>}
      {error && (
        <p role="alert" className="mt-2 text-xs text-red-700">
          {error}
        </p>
      )}
    </li>
  )
}

export function OptimizeReview({
  open,
  run,
  subjectName,
  onClose,
  onDecide,
}: {
  open: boolean
  run: OptimizeRun
  subjectName: string
  onClose: () => void
  // Takes the decisions; resolves with what happened to each. Rejects when the server could not be reached.
  onDecide: (decisions: OptimizeDecision[]) => Promise<OptimizeDecided>
}) {
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [failure, setFailure] = useState('')
  const [working, setWorking] = useState(false)

  async function send(decisions: OptimizeDecision[]) {
    if (decisions.length === 0) return
    setFailure('')
    setWorking(true)
    setBusy(Object.fromEntries(decisions.map((d) => [d.suggestionId, true])))
    try {
      const decided = await onDecide(decisions)
      const refused = refusedOf(decided)
      // A refused decision keeps its message; one carried out loses any earlier message.
      const sent = new Set(decisions.map((d) => d.suggestionId))
      setErrors((prev) => {
        const kept: Record<string, string> = {}
        for (const [id, message] of Object.entries(prev)) if (!sent.has(id)) kept[id] = message
        return { ...kept, ...refused }
      })
      if (decided.closed) onClose()
    } catch (err) {
      setFailure(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy({})
      setWorking(false)
    }
  }

  const groups = groupSuggestions(run.suggestions)
  const approvable = decisionsForAll(run.suggestions, true).length

  return (
    <Modal
      open={open}
      title="Review the AI's suggestions"
      subtitle={subjectName}
      size="full"
      onClose={onClose}
      footer={
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-gray-500">Nothing changes until you approve. A declined suggestion is deleted.</p>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={working || run.suggestions.length === 0}
              onClick={() => {
                void send(decisionsForAll(run.suggestions, false))
              }}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Decline all
            </button>
            <button
              type="button"
              disabled={working || approvable === 0}
              onClick={() => {
                void send(decisionsForAll(run.suggestions, true))
              }}
              className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {`Approve all (${String(approvable)})`}
            </button>
          </div>
        </div>
      }
    >
      {failure && (
        <p role="alert" className="mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
          {failure}
        </p>
      )}
      {run.suggestions.length === 0 && <p className="text-sm text-gray-500">There is nothing left to review.</p>}
      <div className="space-y-6">
        {groups.map((g) => (
          <section key={g.heading} aria-label={g.heading}>
            <h3 className="mb-2 text-sm font-semibold text-gray-700">{g.heading}</h3>
            <ul className="space-y-3">
              {g.items.map((s) => (
                <SuggestionCard
                  key={s.id}
                  s={s}
                  busy={working || busy[s.id]}
                  error={errors[s.id] ?? ''}
                  onDecide={(approve) => {
                    void send([{ suggestionId: s.id, approve }])
                  }}
                />
              ))}
            </ul>
          </section>
        ))}
      </div>
    </Modal>
  )
}
