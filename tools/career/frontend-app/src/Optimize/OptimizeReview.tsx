import { useEffect, useRef, useState } from 'react'
import type { OptimizeDecided, OptimizeDecision, OptimizeRun, OptimizeSuggestion } from '../api'
import { Modal } from '../Shared/Modal/Modal'
import { announcement, donePauseMs, leaveTotalMs } from './motion'
import { decisionsForAll, diffWords, groupSuggestions, plainWhiteSpace, refusedOf, type DiffPart } from './optimizeRules'

// The review of the suggestions of one run (plan 258): each improved text next to the text it replaces, with the changed words marked, and Approve or Decline for each. Nothing is
// written until Approve; a declined suggestion is deleted. A suggestion whose text changed after the AI read it is marked and can only be declined.
// Step 7: the cards come in one after the other; a decided card flashes green (approved) or slides out (declined) and then folds away so the cards below glide up. It stays in the page until
// that has ended (the server already has the decision), a card the server refused shakes and stays, and after the last card "All suggestions decided" is shown for a moment before the
// window closes. With reduced motion every one of those waits is zero.
function Parts({ parts, tone }: { parts: DiffPart[]; tone: 'removed' | 'added' }) {
  const mark = tone === 'removed' ? 'bg-red-100 text-red-900' : 'bg-green-100 text-green-900'
  return (
    <p className="opt-marks whitespace-pre-wrap break-words text-sm text-gray-800">
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

interface Leaving {
  kind: 'approved' | 'declined'
  // Its place among the cards that leave together (60 ms apart).
  k: number
}

const FOCUS_RING = 'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-purple-400 focus-visible:ring-offset-1'

function SuggestionCard({
  s,
  index,
  leaving,
  busy,
  error,
  shakes,
  onDecide,
}: {
  s: OptimizeSuggestion
  index: number
  leaving: Leaving | undefined
  busy: boolean
  error: string
  shakes: number
  onDecide: (approve: boolean) => void
}) {
  const diff = diffWords(s.before, s.after)
  const off = busy || leaving !== undefined
  return (
    <li
      data-testid="optimize-suggestion"
      data-leaving={leaving?.kind}
      className="opt-row"
      style={{ '--i': index, '--k': leaving?.k ?? 0 } as React.CSSProperties}
    >
      <div className="opt-row-inner">
        <div className="pb-3">
          <div
            className={`opt-card opt-card-in rounded-xl border border-gray-200 bg-white p-4 shadow-sm ${shakes > 0 ? (shakes % 2 === 1 ? 'opt-shake-a' : 'opt-shake-b') : ''}`}
          >
            <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
              <p className="text-sm font-semibold text-gray-900">
                {s.label || 'A text that is no longer there'} <span className="font-normal text-gray-400">· {s.field}</span>
                {leaving?.kind === 'approved' && (
                  <span aria-hidden="true" className="opt-pop ml-2 font-bold text-green-600">
                    ✓
                  </span>
                )}
              </p>
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={off}
                  onClick={() => {
                    onDecide(false)
                  }}
                  className={`opt-lift rounded-md border border-gray-200 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 ${FOCUS_RING}`}
                >
                  Decline
                </button>
                <button
                  type="button"
                  data-approve
                  disabled={off || s.stale}
                  onClick={() => {
                    onDecide(true)
                  }}
                  className={`opt-lift rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800 disabled:opacity-40 ${FOCUS_RING}`}
                >
                  Approve
                </button>
              </div>
            </div>
            {s.stale && (
              <p data-testid="optimize-stale" className="opt-amber-once mb-2 rounded-md bg-amber-50 px-2 py-1 text-xs text-amber-800">
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
              <p role="alert" className="opt-fade-in mt-2 text-xs text-red-700">
                {error}
              </p>
            )}
          </div>
        </div>
      </div>
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
  const [shakes, setShakes] = useState<Record<string, number>>({})
  const [leaving, setLeaving] = useState<Record<string, Leaving>>({})
  const [failure, setFailure] = useState('')
  const [working, setWorking] = useState(false)
  // "All suggestions decided", shown for a moment before the window closes.
  const [finished, setFinished] = useState(false)
  // What a screen reader is told about the cards just decided.
  const [said, setSaid] = useState('')
  const body = useRef<HTMLDivElement>(null)
  const timers = useRef<number[]>([])
  useEffect(
    () => () => {
      timers.current.forEach((t) => {
        window.clearTimeout(t)
      })
    },
    [],
  )
  const later = (fn: () => void, ms: number) => {
    timers.current.push(window.setTimeout(fn, ms))
  }

  // A decided card is no longer in the run once the page has read it again, but it stays in the window until it has left: it is remembered here, in the order it was first seen.
  const known = useRef(new Map<string, OptimizeSuggestion>())
  const order = useRef<string[]>([])
  for (const s of run.suggestions) {
    known.current.set(s.id, s)
    if (!order.current.includes(s.id)) order.current.push(s.id)
  }
  const current = new Map(run.suggestions.map((s) => [s.id, s]))
  const shown = order.current.flatMap((id) => {
    const s = current.get(id) ?? (id in leaving ? known.current.get(id) : undefined)
    return s ? [s] : []
  })
  const active = shown.filter((s) => !(s.id in leaving))

  // The focus goes to the next card's Approve (or its first button when that one is out of date) once a card has left.
  function focusNext() {
    const card = body.current?.querySelector<HTMLElement>('[data-testid="optimize-suggestion"]:not([data-leaving])')
    const target =
      card?.querySelector<HTMLButtonElement>('button[data-approve]:not(:disabled)') ??
      card?.querySelector<HTMLButtonElement>('button:not(:disabled)')
    target?.focus()
  }

  async function send(decisions: OptimizeDecision[]) {
    if (decisions.length === 0) return
    setFailure('')
    setWorking(true)
    setBusy(Object.fromEntries(decisions.map((d) => [d.suggestionId, true])))
    try {
      const decided = await onDecide(decisions)
      const refused = refusedOf(decided)
      // A refused decision keeps its message (and its card shakes); one carried out loses any earlier message.
      const sent = new Set(decisions.map((d) => d.suggestionId))
      setErrors((prev) => {
        const kept: Record<string, string> = {}
        for (const [id, message] of Object.entries(prev)) if (!sent.has(id)) kept[id] = message
        return { ...kept, ...refused }
      })
      setShakes((prev) => {
        const next = { ...prev }
        for (const id of Object.keys(refused)) next[id] = (next[id] ?? 0) + 1
        return next
      })
      const okIds = new Set(decided.outcomes.filter((o) => o.ok).map((o) => o.suggestionId))
      const done = decisions.filter((d) => okIds.has(d.suggestionId))
      for (const kind of ['approved', 'declined'] as const) {
        const labels = done.filter((d) => d.approve === (kind === 'approved')).map((d) => known.current.get(d.suggestionId)?.label ?? '')
        if (labels.length > 0) setSaid(announcement(kind, labels))
      }
      const total = leaveTotalMs(done.length)
      if (total > 0) {
        setLeaving((prev) => {
          const next = { ...prev }
          done.forEach((d, k) => {
            next[d.suggestionId] = { kind: d.approve ? 'approved' : 'declined', k }
          })
          return next
        })
        later(() => {
          setLeaving((prev) => {
            const next: Record<string, Leaving> = {}
            for (const [id, l] of Object.entries(prev)) if (!okIds.has(id)) next[id] = l
            return next
          })
          if (decided.closed) {
            setFinished(true)
            later(onClose, donePauseMs())
          } else {
            focusNext()
          }
        }, total)
      } else if (decided.closed) {
        onClose()
      } else {
        later(focusNext, 0)
      }
    } catch (err) {
      setFailure(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy({})
      setWorking(false)
    }
  }

  const groups = groupSuggestions(shown)
  const approvable = decisionsForAll(active, true).length

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
              disabled={working || finished || active.length === 0}
              onClick={() => {
                void send(decisionsForAll(active, false))
              }}
              className={`opt-lift rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 ${FOCUS_RING}`}
            >
              Decline all
            </button>
            <button
              type="button"
              disabled={working || finished || approvable === 0}
              onClick={() => {
                void send(decisionsForAll(active, true))
              }}
              className={`opt-lift rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50 ${FOCUS_RING}`}
            >
              {`Approve all (${String(approvable)})`}
            </button>
          </div>
        </div>
      }
    >
      <div ref={body} className="relative">
        <p role="status" aria-live="polite" className="sr-only">
          {said}
        </p>
        {failure && (
          <p role="alert" className="opt-fade-in mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
            {failure}
          </p>
        )}
        {finished ? (
          <div data-testid="optimize-done" className="opt-pop flex flex-col items-center gap-3 py-16 text-green-700">
            <svg
              width="44"
              height="44"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <circle cx="12" cy="12" r="10" />
              <path className="opt-check-path" d="M7 12.5l3.2 3.2L17 9" />
            </svg>
            <p className="text-sm font-medium">All suggestions decided</p>
          </div>
        ) : (
          <>
            {shown.length === 0 && <p className="text-sm text-gray-500">There is nothing left to review.</p>}
            <div className="space-y-6">
              {groups.map((g) => (
                <section key={g.heading} aria-label={g.heading}>
                  <h3 className="mb-2 text-sm font-semibold text-gray-700">{g.heading}</h3>
                  <ul>
                    {g.items.map((s) => (
                      <SuggestionCard
                        key={s.id}
                        s={s}
                        index={shown.findIndex((x) => x.id === s.id)}
                        leaving={leaving[s.id]}
                        busy={working || busy[s.id]}
                        error={errors[s.id] ?? ''}
                        shakes={shakes[s.id] ?? 0}
                        onDecide={(approve) => {
                          void send([{ suggestionId: s.id, approve }])
                        }}
                      />
                    ))}
                  </ul>
                </section>
              ))}
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}
