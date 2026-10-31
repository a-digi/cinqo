import { useState } from 'react'
import { applyPersonaAiRun, type PersonaAiRun, type ProposedPersona } from '../api'
import { ConfirmationModal } from '../Shared/ConfirmationModal/ConfirmationModal'
import { SparkleIcon } from '../Shared/Icons/icons'
import { newItems, newItemsText, type KnownData } from './knownData'
import { checkAccepted } from './personaProposal'
import { PersonaEditForm } from './PersonaEditForm'
import { PersonaPreview } from './PersonaPreview'
import { acceptDecision, declineDecision, duplicateHint, finalPersona, waitingSuggestions, type Suggestion } from './reviewRules'
import { useKnownData } from './useKnownData'
import { hasRunningRun } from './runDisplay'

// The AI's suggested personas, on the Personas page above the saved personas, on their own purple
// background. Each suggestion is decided on its own: Accept creates that persona right now, Decline
// creates nothing and the card goes away. Edit fixes it before accepting. Suggestions appear here
// the moment the AI is done; nothing is saved until Accept is pressed.
export function PersonaSuggestions({
  runs,
  existing,
  onAccepted,
  onChanged,
}: {
  runs: PersonaAiRun[]
  existing: { id: string; name: string }[]
  onAccepted: (created: { id: string; name: string }[]) => void
  onChanged: () => void
}) {
  const waiting = waitingSuggestions(runs)
  const working = hasRunningRun(runs)
  const [decliningAll, setDecliningAll] = useState(false)
  const [error, setError] = useState('')
  const knownState = useKnownData(existing.map((e) => e.id))
  const known = knownState.status === 'ready' && knownState.comparable ? knownState.known : null
  const notes = [...new Set(waiting.map((s) => s.run.proposal?.note.trim() ?? '').filter((n) => n !== ''))]

  if (waiting.length === 0 && !working) return null

  async function declineAll() {
    setDecliningAll(false)
    setError('')
    try {
      const runIds = [...new Set(waiting.map((s) => s.run.id))]
      for (const id of runIds) {
        await applyPersonaAiRun(
          id,
          waiting.filter((s) => s.run.id === id).map((s) => declineDecision(s.index)),
        )
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    }
    onChanged()
  }

  return (
    <section
      id="persona-suggestions"
      aria-label="AI suggestions"
      className="mb-5 rounded-3xl border-2 p-5 shadow-sm"
      style={{ borderColor: '#c4b5fd', background: 'linear-gradient(160deg, #ede9fe 0%, #f5f3ff 100%)', animation: 'none' }}
    >
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="inline-flex items-center gap-1.5 text-sm font-semibold text-purple-900">
          <SparkleIcon />
          AI suggestions
        </span>
        {waiting.length > 0 && (
          <span className="rounded-full bg-purple-600 px-2 py-0.5 text-[11px] font-semibold text-white">
            {waiting.length} waiting for you
          </span>
        )}
        {waiting.length > 1 && (
          <button
            type="button"
            onClick={() => {
              setDecliningAll(true)
            }}
            className="ml-auto rounded-full border border-rose-200 bg-white px-3 py-1 text-xs font-medium text-rose-700 hover:bg-rose-50"
          >
            Decline all remaining
          </button>
        )}
      </div>

      {working && (
        <p role="status" className="mb-3 flex items-center gap-2 rounded-xl bg-white/70 px-4 py-3 text-sm text-purple-900">
          <span className="h-2 w-2 animate-pulse rounded-full bg-purple-600" aria-hidden="true" />
          The AI is working on suggestions. They appear here as soon as it is done; you can stay on this page.
        </p>
      )}
      {notes.map((n) => (
        <p key={n} className="mb-3 text-sm text-purple-900">
          {n}
        </p>
      ))}
      {waiting.length > 0 && (
        <p className="mb-3 text-xs text-purple-800">
          Nothing is saved until you accept. Amber items are not in your existing skills, employers or job titles: check them first.
        </p>
      )}

      <div className="space-y-4">
        {waiting.map((s) => (
          <SuggestionCard
            key={`${s.run.id}-${s.index}`}
            suggestion={s}
            existing={existing}
            known={known}
            onAccepted={onAccepted}
            onChanged={onChanged}
          />
        ))}
      </div>
      {error !== '' && <p className="mt-3 text-sm text-red-700">{error}</p>}

      <ConfirmationModal
        open={decliningAll}
        title="Decline all remaining"
        message="Decline every suggestion that is still waiting? Nothing will be created."
        confirmLabel="Decline all"
        variant="danger"
        onConfirm={() => {
          void declineAll()
        }}
        onCancel={() => {
          setDecliningAll(false)
        }}
      />
    </section>
  )
}

function SuggestionCard({
  suggestion,
  existing,
  known,
  onAccepted,
  onChanged,
}: {
  suggestion: Suggestion
  existing: { id: string; name: string }[]
  known: KnownData | null
  onAccepted: (created: { id: string; name: string }[]) => void
  onChanged: () => void
}) {
  const [draft, setDraft] = useState<ProposedPersona>(() => structuredClone(suggestion.persona))
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const final = finalPersona(draft)
  const problem = checkAccepted([final])
  const hint = duplicateHint(draft, existing)
  const notInData = known ? newItemsText(newItems(known, final)) : null

  function decide(decision: 'accepted' | 'declined') {
    setBusy(true)
    setError('')
    const send = decision === 'accepted' ? acceptDecision(suggestion.index, draft) : declineDecision(suggestion.index)
    applyPersonaAiRun(suggestion.run.id, [send])
      .then((created) => {
        if (decision === 'accepted') onAccepted(created)
        else onChanged()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
        setBusy(false)
      })
  }

  return (
    <article aria-label={`Suggestion: ${final.name || 'no name'}`} className="rounded-2xl bg-white p-3 shadow-sm ring-1 ring-purple-200">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-purple-700">Suggested persona</span>
        <span className="ml-auto flex gap-2">
          <button
            type="button"
            disabled={busy || problem !== null}
            onClick={() => {
              decide('accepted')
            }}
            className="rounded-full bg-green-600 px-3.5 py-1 text-xs font-semibold text-white hover:bg-green-700 disabled:opacity-50"
          >
            {busy ? 'Working…' : 'Accept'}
          </button>
          <button
            type="button"
            disabled={busy}
            aria-pressed={editing}
            onClick={() => {
              setEditing((e) => !e)
            }}
            className="rounded-full border border-gray-200 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            {editing ? 'Done editing' : 'Edit'}
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => {
              decide('declined')
            }}
            className="rounded-full border border-rose-300 px-3 py-1 text-xs font-medium text-rose-800 hover:bg-rose-50 disabled:opacity-50"
          >
            Decline
          </button>
        </span>
      </div>
      {hint && <p className="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">{hint}</p>}
      {editing && notInData !== null && <p className="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">{notInData}</p>}
      {editing ? (
        <PersonaEditForm
          persona={draft}
          known={known}
          onChange={(change) => {
            setDraft(change)
          }}
        />
      ) : (
        <PersonaPreview persona={final} known={known} />
      )}
      {(error !== '' || problem !== null) && <p className="mt-3 text-sm text-red-700">{error !== '' ? error : problem}</p>}
    </article>
  )
}
