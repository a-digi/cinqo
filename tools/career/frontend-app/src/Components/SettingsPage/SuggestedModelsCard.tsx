import { useCallback, useEffect, useState } from 'react'
import {
  fetchCinqoModelState,
  canAsk,
  pointsToCinqo,
  rowState,
  rowStateText,
  type CinqoModelState,
} from '../../Cinqo/Platform/cinqoModelState'
import { LOCAL_AI_PATH } from '../../Cinqo/Platform/modelLabels'
import { SUGGESTED_MODELS, type SuggestedModel } from '../../Cinqo/Platform/suggestedModels'
import { outcomeText, suggestErrorText, suggestModel } from '../../Cinqo/Platform/suggestLocalModel'
import { RobotIcon } from '../../Shared/Icons/icons'

type Ask = { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; text: string } | { kind: 'error'; text: string }

const linkClass =
  'font-medium text-gray-700 underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-400'

function Row({
  model,
  platformModels,
  cinqo,
  onAsked,
}: {
  model: SuggestedModel
  platformModels: string[]
  cinqo: CinqoModelState | null
  onAsked: () => void
}) {
  const [ask, setAsk] = useState<Ask>({ kind: 'idle' })
  const state = rowState(model, platformModels, cinqo)

  function askCinqo() {
    setAsk({ kind: 'busy' })
    suggestModel(model)
      .then((outcome) => {
        setAsk({ kind: 'done', text: outcomeText(outcome, model) })
        onAsked()
      })
      .catch((err: unknown) => {
        setAsk({ kind: 'error', text: suggestErrorText(err) })
      })
  }

  return (
    <li className="rounded-2xl bg-white/70 p-4" data-testid={`suggested-model-${model.id}`} data-state={state}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <p className="text-sm font-semibold text-gray-900">{model.name}</p>
        <span
          className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${state === 'downloaded' ? 'bg-green-50 text-green-800' : state === 'waiting' ? 'bg-violet-50 text-violet-800' : 'bg-gray-100 text-gray-700'}`}
        >
          {state === 'downloaded'
            ? 'Downloaded'
            : state === 'waiting'
              ? 'Waiting for you'
              : state === 'dismissed'
                ? 'Dismissed'
                : 'Not downloaded'}
        </span>
      </div>
      <p className="mt-1 text-sm text-gray-600">Career suggests it for: {model.purpose}</p>
      <p className="mt-2 text-xs text-gray-500">
        {ask.kind === 'done' ? ask.text : rowStateText(state)}{' '}
        {canAsk(state) && ask.kind !== 'done' && (
          <button type="button" disabled={ask.kind === 'busy'} onClick={askCinqo} className={`${linkClass} disabled:opacity-50`}>
            {ask.kind === 'busy' ? 'Asking Cinqo…' : 'Ask Cinqo to download it'}
          </button>
        )}
        {(pointsToCinqo(state) || ask.kind === 'done') && state !== 'downloaded' && (
          <button
            type="button"
            onClick={() => {
              window.__cinqoToolBridge.navigate(LOCAL_AI_PATH)
            }}
            className={linkClass}
          >
            Open the Local Agents page
          </button>
        )}
        {ask.kind === 'error' && (
          <span role="alert" className="ml-1 text-red-700">
            {ask.text}
          </span>
        )}
      </p>
    </li>
  )
}

// The card of the Settings page (plan 221): the local models Career suggests to Cinqo, what it suggests each one for, and where each one stands. Career
// only says which model it wants; Cinqo asks the person on its Local Agents page and does the download. The state comes from Cinqo (what it lists, what
// waits, what was dismissed) and from the platform's own list of models; Cinqo being unreachable leaves the platform's list and the button.
export function SuggestedModelsCard({ platformModels, accent }: { platformModels: string[]; accent: { soft: string; to: string } }) {
  const [cinqo, setCinqo] = useState<CinqoModelState | null>(null)
  const load = useCallback(() => {
    void fetchCinqoModelState().then(setCinqo)
  }, [])
  useEffect(() => {
    load()
  }, [load])

  return (
    <section
      className="career-enter career-delay-2 relative mt-5 overflow-hidden rounded-3xl border p-6 shadow-sm"
      style={{ borderColor: accent.soft, background: `linear-gradient(160deg, #ffffff 40%, ${accent.soft} 100%)` }}
      aria-label="Local models Career suggests"
    >
      <div className="relative flex items-center gap-3">
        <span
          className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm"
          style={{ background: accent.to }}
        >
          <RobotIcon className="h-6 w-6" />
        </span>
        <div>
          <h2 className="text-base font-semibold">Local models Career suggests</h2>
          <p className="text-sm text-gray-500">
            Career asks Cinqo for these. You confirm on Cinqo&apos;s Local Agents page, and Cinqo does the download.
          </p>
        </div>
      </div>
      <ul className="relative mt-5 space-y-3">
        {SUGGESTED_MODELS.map((m) => (
          <Row key={m.id} model={m} platformModels={platformModels} cinqo={cinqo} onAsked={load} />
        ))}
      </ul>
    </section>
  )
}
