import { useEffect, useRef } from 'react'
import type { Persona } from '../../api'
import type { UseAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { CreateCvWithAi } from '../../CvAi/CreateCvWithAi'
import { failedRunToShow, hasRunningCvRun, openRunChat } from '../../CvAi/cvAiRules'
import { useCvAiRuns } from '../../CvAi/useCvAiRuns'

// "Create CV with AI" on one row of the CV list (plan 260): the same button and dialog as on the target role's own page, with that
// persona's runs (started, resumed after a reload, finished when the AI's turn ends) read by the existing useCvAiRuns, so each row
// watches its own persona. While a run works the button says so and waits; when one ends `onRunEnded` is called once (the list then
// reads the CVs again so the counts and titles follow); a failure of the newest run is told under the button, with its chat and a Dismiss.
export function PersonaCvAiButton({
  persona,
  ai,
  dismissed,
  onDismiss,
  onRunEnded,
}: {
  persona: Pick<Persona, 'id' | 'name'>
  // The AI in use (the Career AI settings), read once by the page and shared by every row.
  ai: UseAiChoice
  // The failed runs the person dismissed, shared by every row (one list, so a dismissal never overwrites another row's).
  dismissed: ReadonlySet<string>
  onDismiss: (runId: string) => void
  onRunEnded: () => void
}) {
  const aiRuns = useCvAiRuns(persona.id)
  const running = hasRunningCvRun(aiRuns.runs)
  // The runs this row saw working: when one is no longer generating it has ended (once).
  const watched = useRef(new Set<string>())
  const handled = useRef(new Set<string>())
  useEffect(() => {
    let ended = false
    for (const r of aiRuns.runs) {
      if (r.status === 'generating') watched.current.add(r.id)
      else if (watched.current.has(r.id) && !handled.current.has(r.id)) {
        handled.current.add(r.id)
        ended = true
      }
    }
    if (ended) onRunEnded()
  }, [aiRuns.runs, onRunEnded])

  const failed = failedRunToShow(aiRuns.runs, dismissed)
  const disabledReason = running
    ? 'The AI is already creating a CV for this target role.'
    : !ai.platform
      ? 'No AI platform is available. Choose one in Settings.'
      : ''
  return (
    <div className="flex max-w-xs flex-col items-end gap-1.5">
      <CreateCvWithAi
        compact
        busy={running}
        ai={ai}
        platforms={ai.platforms}
        personaId={persona.id}
        personaName={persona.name}
        disabledReason={disabledReason}
        onStart={(focus, choice) => aiRuns.start({ id: persona.id, name: persona.name }, focus, choice)}
      />
      {failed && (
        <div role="alert" className="w-full rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">
          <p className="font-medium">The AI could not create a CV.</p>
          <p className="mt-0.5 line-clamp-2 break-words" title={failed.error ?? undefined}>
            {failed.error ?? 'No reason was recorded.'}
          </p>
          <div className="mt-1.5 flex gap-1.5">
            <button
              type="button"
              onClick={() => {
                openRunChat(failed)
              }}
              className="rounded-full border border-red-200 bg-white px-2.5 py-0.5 font-medium hover:bg-red-100"
            >
              Open chat
            </button>
            <button
              type="button"
              onClick={() => {
                onDismiss(failed.id)
              }}
              className="rounded-full border border-red-200 bg-white px-2.5 py-0.5 font-medium hover:bg-red-100"
            >
              Dismiss
            </button>
          </div>
        </div>
      )}
      {aiRuns.error !== '' && <p className="text-xs text-red-700">{aiRuns.error}</p>}
    </div>
  )
}
