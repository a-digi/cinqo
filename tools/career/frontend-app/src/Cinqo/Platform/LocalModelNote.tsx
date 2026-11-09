import { useState } from 'react'
import { LOCAL_AI_PATH, LOCAL_MODEL_NOTE } from './modelLabels'
import { missingModels, type SuggestedModel } from './suggestedModels'
import { opensLocalAgents, outcomeText, suggestErrorText, suggestModel, type SuggestOutcome } from './suggestLocalModel'

type Ask = { kind: 'idle' } | { kind: 'busy' } | { kind: 'done'; outcome: SuggestOutcome } | { kind: 'error'; text: string }

const linkClass =
  'font-medium text-gray-700 underline underline-offset-2 hover:no-underline focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-400'

// One line of the note: a model Career suggests that the platform does not list, with the button that asks Cinqo for it. Career only says which model it
// wants; Cinqo asks the person on its Local Agents page and does the download. The picker stays as it is (this can sit inside a dialog), so the answer is
// shown here and the page is opened only when the person chooses to.
function AskLine({ model }: { model: SuggestedModel }) {
  const [ask, setAsk] = useState<Ask>({ kind: 'idle' })

  function askCinqo() {
    setAsk({ kind: 'busy' })
    suggestModel(model)
      .then((outcome) => {
        setAsk({ kind: 'done', outcome })
      })
      .catch((err: unknown) => {
        setAsk({ kind: 'error', text: suggestErrorText(err) })
      })
  }

  return (
    <p className="text-xs text-gray-500" data-testid={`ask-cinqo-${model.id}`}>
      {ask.kind === 'done' ? (
        <>
          {outcomeText(ask.outcome, model)}{' '}
          {opensLocalAgents(ask.outcome) && (
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
        </>
      ) : (
        <>
          {model.noteLead} is not downloaded.{' '}
          <button type="button" disabled={ask.kind === 'busy'} onClick={askCinqo} className={`${linkClass} disabled:opacity-50`}>
            {ask.kind === 'busy' ? 'Asking Cinqo…' : 'Ask Cinqo to download it'}
          </button>
        </>
      )}
      {ask.kind === 'error' && (
        <span role="alert" className="ml-1 text-red-700">
          {ask.text}
        </span>
      )}
    </p>
  )
}

// The note under a local platform's model picker (plan 206): one model runs at a time, and where to get more of them. The link opens the Cinqo Local AI
// page (downloads, switch on/off, default model); choosing among the downloaded models needs no visit there. plan 220 and 221: for each model Career
// suggests (Qwen2.5-Coder-3B for the crawl instructions, Phi-4-mini for the CV import) that the platform does not list, one more line offers to ask Cinqo
// for it.
export function LocalModelNote({ platform }: { platform: { models: string[] } }) {
  const missing = missingModels(platform)
  return (
    <div className="space-y-1" data-testid="local-model-note">
      <p className="text-xs text-gray-500">
        {LOCAL_MODEL_NOTE}{' '}
        <button
          type="button"
          onClick={() => {
            window.__cinqoToolBridge.navigate(LOCAL_AI_PATH)
          }}
          className={linkClass}
        >
          Get more models
        </button>
      </p>
      {missing.map((m) => (
        <AskLine key={m.id} model={m} />
      ))}
    </div>
  )
}
